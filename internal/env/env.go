//nolint:revive // magic numbers and string constants are clear in context
package env

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/credentials"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

const LocalPinFile = ".claude-env"

// Manager handles environment switching and credential management.
type Manager struct {
	Paths config.Paths
	Cfg   config.Config
	Fs    *fsutil.SymlinkFs

	// Claude runs the claude CLI for login/setup-token flows.
	Claude ClaudeRunner
	// Keychain captures and purges the macOS per-config-dir credential entry.
	Keychain KeychainStore
	// Now returns the current time as Unix milliseconds; injectable for tests.
	Now func() int64
}

// New creates an environment manager with production adapters and clock.
func New(paths config.Paths, cfg config.Config, fs *fsutil.SymlinkFs) *Manager {
	return &Manager{
		Paths:    paths,
		Cfg:      cfg,
		Fs:       fs,
		Claude:   ExecClaudeRunner{},
		Keychain: keychainAdapter{},
		Now:      func() int64 { return time.Now().UnixMilli() },
	}
}

// requireEnv returns the config directory for a registered environment, or an
// error if it is unknown.
func (m *Manager) requireEnv(name string) (string, error) {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return "", fmt.Errorf("environment %q not found", name)
	}
	return m.Paths.EnvDir(name), nil
}

// Init sets up ~/.claude-envs/ and creates a "default" environment.
// If Claude Code is currently authenticated, snapshots that into the default env.
func (m *Manager) Init() error {
	if err := m.Fs.MkdirAll(m.Paths.EnvsDir, 0o755); err != nil {
		return fmt.Errorf("create envs directory: %w", err)
	}
	if err := m.Fs.MkdirAll(m.Paths.PoolDir, 0o755); err != nil {
		return fmt.Errorf("create pool directory: %w", err)
	}

	if _, exists := m.Cfg.Environments["default"]; exists {
		return fmt.Errorf("already initialized (environment 'default' exists)")
	}

	envDir := m.Paths.EnvDir("default")
	if err := m.Fs.MkdirAll(envDir, 0o755); err != nil {
		return fmt.Errorf("create default env directory: %w", err)
	}

	// Best-effort: adopt the current default Claude Code login (~/.claude) as
	// the 'default' environment's OAuth token. Clean break — we no longer copy
	// .claude.json (which never held the token, only onboarding/account metadata).
	if m.adoptDefaultCredentials(envDir) {
		fmt.Println("Adopted existing Claude Code login as the 'default' environment.")
	} else {
		fmt.Println("Created 'default' environment. Run 'claude-env login default' to authenticate.")
	}

	m.copyBootstrapFiles(envDir)

	m.Cfg.Global = "default"
	m.Cfg.Environments["default"] = config.Environment{}

	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Add registers a new environment with its own config directory.
func (m *Manager) Add(name string) error {
	if _, exists := m.Cfg.Environments[name]; exists {
		return fmt.Errorf("environment %q already exists", name)
	}

	envDir := m.Paths.EnvDir(name)
	if err := m.Fs.MkdirAll(envDir, 0o755); err != nil {
		return fmt.Errorf("create env directory: %w", err)
	}

	m.copyBootstrapFiles(envDir)

	m.Cfg.Environments[name] = config.Environment{}
	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// Use switches the global environment.
func (m *Manager) Use(name string) error {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	envDir := m.Paths.EnvDir(name)
	if _, err := m.Fs.Stat(envDir); err != nil {
		return fmt.Errorf("env directory missing for %q: %w", name, err)
	}

	m.Cfg.Global = name
	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}

	if _, err := m.SyncMarketplacePaths(envDir); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to sync marketplace paths: %v\n", err)
	}

	return nil
}

// Login runs an interactive 'claude auth login' with CLAUDE_CONFIG_DIR pointed
// at the named environment, then captures the resulting OAuth token into the
// environment's .credentials.json so the token — not an opaque keychain entry —
// is the environment's portable unit of identity.
func (m *Manager) Login(name string) error {
	envDir, err := m.requireEnv(name)
	if err != nil {
		return err
	}
	if err := m.Claude.Available(); err != nil {
		//nolint:wrapcheck // ClaudeRunner.Available already returns a descriptive error
		return err
	}

	fmt.Fprintf(os.Stderr, "Logging in for environment %q...\n", name)
	if err := m.Claude.Login(envDir); err != nil {
		return fmt.Errorf("claude auth login failed: %w", err)
	}

	if err := m.captureCredentials(envDir); err != nil {
		return err
	}

	// Ensure the env's .claude.json has onboarding flags so interactive mode
	// doesn't prompt for setup.
	m.patchClaudeConfig(envDir)

	fmt.Fprintf(os.Stderr, "Environment %q authenticated. Token stored in %s\n", name, credentials.Path(envDir))
	return nil
}

// captureCredentials materializes an environment's OAuth token into its
// .credentials.json file. On macOS, 'claude auth login' writes the token to the
// per-config-dir Keychain entry, so it is read, decoded, written to the file,
// and purged from the Keychain so the file is the single source of truth. On
// platforms that authenticate from files natively, claude writes the file and
// this only validates it.
func (m *Manager) captureCredentials(envDir string) error {
	if m.Keychain.Available() {
		if data, err := m.Keychain.Read(envDir); err == nil {
			if err := credentials.WriteRaw(m.Fs, envDir, data); err != nil {
				return fmt.Errorf("materialize captured credentials: %w", err)
			}
			//nolint:errcheck // best-effort purge; the file is now source of truth
			_ = m.Keychain.Delete(envDir)
			return nil
		}
	}

	if credentials.Exists(m.Fs, envDir) {
		if _, err := credentials.Read(m.Fs, envDir); err != nil {
			return fmt.Errorf("validate captured credentials: %w", err)
		}
		return nil
	}
	return fmt.Errorf("login did not produce credentials in %s", envDir)
}

// adoptDefaultCredentials best-effort copies the current default Claude Code
// login (from ~/.claude — a .credentials.json file, or the macOS Keychain) into
// a new environment's credential file. Returns whether a token was adopted.
func (m *Manager) adoptDefaultCredentials(envDir string) bool {
	source := m.Paths.ClaudeDir

	if data, err := credentials.ReadRaw(m.Fs, source); err == nil {
		if credentials.WriteRaw(m.Fs, envDir, data) == nil {
			return true
		}
	}
	if m.Keychain.Available() {
		if data, err := m.Keychain.Read(source); err == nil {
			if credentials.WriteRaw(m.Fs, envDir, data) == nil {
				return true
			}
		}
	}
	return false
}

// AuthInfo is the native authentication status for an environment, derived from
// its .credentials.json (and .claude.json for the account email) without
// invoking the claude CLI.
type AuthInfo struct {
	Authenticated    bool
	Email            string
	SubscriptionType string
	ExpiresAt        int64
	Expired          bool
	ExpiresIn        time.Duration
}

// AuthStatus reports an environment's authentication state from its credential
// file. A missing file means not authenticated; it is not an error.
func (m *Manager) AuthStatus(name string) (AuthInfo, error) {
	envDir, err := m.requireEnv(name)
	if err != nil {
		return AuthInfo{}, err
	}

	blob, err := credentials.Read(m.Fs, envDir)
	if errors.Is(err, credentials.ErrNotAuthenticated) {
		return AuthInfo{Authenticated: false}, nil
	}
	if err != nil {
		return AuthInfo{}, fmt.Errorf("read credentials: %w", err)
	}

	o := blob.ClaudeAiOauth
	now := m.Now()
	return AuthInfo{
		Authenticated:    true,
		Email:            m.accountEmail(envDir),
		SubscriptionType: o.SubscriptionType,
		ExpiresAt:        o.ExpiresAt,
		Expired:          o.Expired(now),
		ExpiresIn:        o.ExpiresIn(now),
	}, nil
}

// accountEmail returns the account email from an environment's .claude.json
// oauthAccount block. Best-effort: the token blob carries no email, and the
// field is absent until Claude Code has fetched the account profile.
func (m *Manager) accountEmail(envDir string) string {
	data, err := m.Fs.ReadFile(filepath.Join(envDir, ".claude.json"))
	if err != nil {
		return ""
	}
	var cfg struct {
		OauthAccount struct {
			EmailAddress string `json:"emailAddress"`
		} `json:"oauthAccount"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return ""
	}
	return cfg.OauthAccount.EmailAddress
}

// Import installs an OAuth credential into an environment. The data may be a
// full {"claudeAiOauth":{...}} blob or a bare sk-ant-* token, which is wrapped.
func (m *Manager) Import(name string, data []byte) error {
	envDir, err := m.requireEnv(name)
	if err != nil {
		return err
	}

	if _, vErr := credentials.ValidateRaw(data); vErr == nil {
		return wrapWrite(credentials.WriteRaw(m.Fs, envDir, data))
	}

	token := strings.TrimSpace(string(data))
	if !strings.HasPrefix(token, "sk-ant-") {
		return fmt.Errorf("input is neither a valid credential blob nor an sk-ant-* token")
	}
	return wrapWrite(credentials.Write(m.Fs, envDir, wrapToken(token)))
}

// ImportFromEnv copies the credential from src into dst.
func (m *Manager) ImportFromEnv(dst, src string) error {
	dstDir, err := m.requireEnv(dst)
	if err != nil {
		return err
	}
	srcDir, err := m.requireEnv(src)
	if err != nil {
		return err
	}
	data, err := credentials.ReadRaw(m.Fs, srcDir)
	if err != nil {
		return fmt.Errorf("read source %q credentials: %w", src, err)
	}
	return wrapWrite(credentials.WriteRaw(m.Fs, dstDir, data))
}

// ImportSetupToken runs 'claude setup-token' for an environment and stores the
// resulting long-lived token. Such tokens are inference-only and carry no
// refresh token, so they expire on their own ~1-year horizon.
func (m *Manager) ImportSetupToken(name string) error {
	envDir, err := m.requireEnv(name)
	if err != nil {
		return err
	}
	if err := m.Claude.Available(); err != nil {
		//nolint:wrapcheck // ClaudeRunner.Available already returns a descriptive error
		return err
	}
	token, err := m.Claude.SetupToken(envDir)
	if err != nil {
		return fmt.Errorf("claude setup-token failed: %w", err)
	}
	return wrapWrite(credentials.Write(m.Fs, envDir, wrapToken(token)))
}

// Export returns an environment's raw credential bytes for backup or transfer.
func (m *Manager) Export(name string) ([]byte, error) {
	envDir, err := m.requireEnv(name)
	if err != nil {
		return nil, err
	}
	data, err := credentials.ReadRaw(m.Fs, envDir)
	if err != nil {
		return nil, fmt.Errorf("read credentials: %w", err)
	}
	return data, nil
}

// wrapToken builds a credential blob from a bare OAuth token (e.g. from
// `claude setup-token`), tagging it inference-only.
func wrapToken(token string) credentials.Blob {
	return credentials.Blob{ClaudeAiOauth: credentials.OAuth{
		AccessToken: token,
		Scopes:      []string{"user:inference"},
	}}
}

// wrapWrite annotates a credentials write error so its origin survives the
// package boundary.
func wrapWrite(err error) error {
	if err != nil {
		return fmt.Errorf("write credentials: %w", err)
	}
	return nil
}

// Local pins an environment to the current directory.
func (m *Manager) Local(name, dir string) error {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	pinPath := filepath.Join(dir, LocalPinFile)
	if err := m.Fs.WriteFile(pinPath, []byte(name+"\n"), 0o644); err != nil {
		return fmt.Errorf("write local pin file: %w", err)
	}
	return nil
}

// Current resolves the active environment by checking local pin, then global.
// Returns (name, source, error).
// nolint:gocritic
func (m *Manager) Current(dir string) (string, string, error) {
	current := dir
	for {
		pinPath := filepath.Join(current, LocalPinFile)
		data, err := m.Fs.ReadFile(pinPath)
		if err == nil {
			envName := trimNewline(string(data))
			if _, exists := m.Cfg.Environments[envName]; exists {
				return envName, "local (" + pinPath + ")", nil
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	if m.Cfg.Global != "" {
		return m.Cfg.Global, "global", nil
	}

	return "", "", fmt.Errorf("no environment set (run 'claude-env init' first)")
}

// ConfigDir returns the CLAUDE_CONFIG_DIR for a given environment name.
func (m *Manager) ConfigDir(name string) string {
	return m.Paths.EnvDir(name)
}

// List returns all environment names with their active status.
func (m *Manager) List(dir string) []EnvInfo {
	//nolint:errcheck // error not critical for list operation
	activeName, _, _ := m.Current(dir)
	//nolint:prealloc // make with capacity is sufficient
	var envs []EnvInfo
	for name, e := range m.Cfg.Environments {
		envs = append(envs, EnvInfo{
			Name:   name,
			Active: name == activeName,
			Shared: e.Shared,
		})
	}
	return envs
}

// Remove deletes an environment and its config directory.
func (m *Manager) Remove(name string) error {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return fmt.Errorf("environment %q not found", name)
	}
	if m.Cfg.Global == name {
		return fmt.Errorf("cannot remove the active global environment — switch first")
	}

	envDir := m.Paths.EnvDir(name)
	//nolint:errcheck
	_ = m.Fs.RemoveAll(envDir)
	// Purge any per-config-dir Keychain entry so a deleted env leaves no token.
	//nolint:errcheck
	_ = m.Keychain.Delete(envDir)

	delete(m.Cfg.Environments, name)
	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// EnvInfo holds display information for an environment.
type EnvInfo struct {
	Name   string
	Active bool
	Shared []string
}

// SharedAdd adds a resource to an environment's shared list. If the resource is
// an absolute path, it is copied into the pool automatically. The pool-relative
// path is derived from the last two path components (e.g. skills/humanify).
func (m *Manager) SharedAdd(name, resource string) error {
	e, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	relPath, err := m.ensureInPool(resource)
	if err != nil {
		return err
	}

	if slices.Contains(e.Shared, relPath) {
		return fmt.Errorf("resource %q already shared in environment %q", relPath, name)
	}

	e.Shared = append(e.Shared, relPath)
	m.Cfg.Environments[name] = e

	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// ensureInPool resolves a resource path to a pool-relative path, copying the
// resource into the pool if it lives outside it. Returns the pool-relative path.
func (m *Manager) ensureInPool(resource string) (string, error) {
	// Already a relative path inside the pool — just validate it exists.
	if !filepath.IsAbs(resource) {
		poolPath := filepath.Join(m.Paths.PoolDir, resource)
		if _, err := m.Fs.Stat(poolPath); err != nil {
			return "", fmt.Errorf("resource %q not found in pool (%s)", resource, m.Paths.PoolDir)
		}
		return resource, nil
	}

	// Absolute path — verify source exists.
	srcInfo, err := m.Fs.Stat(resource)
	if err != nil {
		return "", fmt.Errorf("source path does not exist: %s", resource)
	}

	// Derive pool-relative path from last two components (e.g. skills/humanify).
	relPath := poolRelPath(resource)
	dst := filepath.Join(m.Paths.PoolDir, relPath)

	// If already in the pool at the expected location, skip copy.
	if _, err := m.Fs.Stat(dst); err == nil {
		return relPath, nil
	}

	// Copy source into pool.
	if err := m.Fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", fmt.Errorf("create pool directory: %w", err)
	}

	if err := m.copyResource(resource, dst, srcInfo); err != nil {
		return "", err
	}

	fmt.Fprintf(os.Stderr, "Copied %s → %s\n", resource, dst)
	return relPath, nil
}

// poolRelPath derives a pool-relative path from the last two components of an
// absolute path (e.g. /home/user/.claude/skills/humanify → skills/humanify).
func poolRelPath(absPath string) string {
	dir, base := filepath.Split(filepath.Clean(absPath))
	parent := filepath.Base(filepath.Clean(dir))
	return filepath.Join(parent, base)
}

// copyResource copies a file or directory into the pool.
func (m *Manager) copyResource(src, dst string, info os.FileInfo) error {
	if info.IsDir() {
		return m.copyDir(src, dst)
	}
	data, err := m.Fs.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read source: %w", err)
	}
	if err := m.Fs.WriteFile(dst, data, 0o644); err != nil {
		return fmt.Errorf("write to pool: %w", err)
	}
	return nil
}

// copyDir recursively copies a directory tree.
func (m *Manager) copyDir(src, dst string) error {
	if err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("rel path: %w", err)
		}
		target := filepath.Join(dst, rel)

		if d.IsDir() {
			return m.Fs.MkdirAll(target, 0o755)
		}

		data, err := m.Fs.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %s: %w", rel, err)
		}
		return m.Fs.WriteFile(target, data, 0o644)
	}); err != nil {
		return fmt.Errorf("walk source directory: %w", err)
	}
	return nil
}

// SharedRemove removes a resource path from an environment's shared list, saves
// config, and reconciles symlinks.
func (m *Manager) SharedRemove(name, resource string) error {
	e, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	idx := -1
	for i, s := range e.Shared {
		if s == resource {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("resource %q not found in environment %q", resource, name)
	}

	e.Shared = append(e.Shared[:idx], e.Shared[idx+1:]...)
	m.Cfg.Environments[name] = e

	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// SetOverride sets the settings_override path for an environment.
func (m *Manager) SetOverride(name, path string) error {
	e, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	e.SettingsOverride = path
	m.Cfg.Environments[name] = e

	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// ClearOverride removes the settings_override for an environment.
func (m *Manager) ClearOverride(name string) error {
	e, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	e.SettingsOverride = ""
	m.Cfg.Environments[name] = e

	if err := config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// patchClaudeConfig ensures .claude.json in envDir has the flags Claude Code
// needs to skip the interactive onboarding flow.
func (m *Manager) patchClaudeConfig(envDir string) {
	configPath := filepath.Join(envDir, ".claude.json")
	data, err := m.Fs.ReadFile(configPath)
	if err != nil {
		return
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		return
	}

	changed := false
	if _, ok := cfg["hasCompletedOnboarding"]; !ok {
		cfg["hasCompletedOnboarding"] = true
		changed = true
	}
	if _, ok := cfg["theme"]; !ok {
		cfg["theme"] = "dark"
		changed = true
	}

	if !changed {
		return
	}

	patched, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	//nolint:errcheck
	_ = m.Fs.WriteFile(configPath, patched, 0o600)
}

// bootstrapFiles are files from ~/.claude/ that Claude Code expects to find
// in CLAUDE_CONFIG_DIR for a working session.
var bootstrapFiles = []struct {
	name string
	perm os.FileMode
}{
	{"settings.json", 0o644},
	{"CLAUDE.md", 0o644},
}

// copyBootstrapFiles copies essential config files from ~/.claude/ into envDir.
// Missing source files are silently skipped.
func (m *Manager) copyBootstrapFiles(envDir string) {
	for _, f := range bootstrapFiles {
		src := filepath.Join(m.Paths.ClaudeDir, f.name)
		data, err := m.Fs.ReadFile(src)
		if err != nil || len(data) == 0 {
			continue
		}
		dst := filepath.Join(envDir, f.name)
		// Don't overwrite if already exists.
		if _, err := m.Fs.Stat(dst); err == nil {
			continue
		}
		//nolint:errcheck
		_ = m.Fs.WriteFile(dst, data, f.perm)
	}
}

const marketplacesSubpath = "plugins" + string(filepath.Separator) + "marketplaces" + string(filepath.Separator)

// PathRewrite describes a single installLocation change.
type PathRewrite struct {
	Plugin  string
	OldPath string
	NewPath string
}

// SyncMarketplacePaths updates installLocation entries in known_marketplaces.json
// so they point to the given environment's plugin directory. Returns the list of
// rewrites applied (empty if nothing changed). This is needed because the file is
// pooled (shared via symlink) but Claude Code's validator resolves symlinks and
// does a prefix match against the active env's real path.
func (m *Manager) SyncMarketplacePaths(envDir string) ([]PathRewrite, error) {
	kmPath := filepath.Join(m.Paths.PoolDir, "plugins", "known_marketplaces.json")
	data, err := m.Fs.ReadFile(kmPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read known_marketplaces.json: %w", err)
	}

	var marketplaces map[string]json.RawMessage
	if err := json.Unmarshal(data, &marketplaces); err != nil {
		return nil, fmt.Errorf("parse known_marketplaces.json: %w", err)
	}

	targetPrefix := filepath.Join(envDir, "plugins", "marketplaces")
	result := data
	var rewrites []PathRewrite

	for key, raw := range marketplaces {
		if rw, replaced := rewriteEntry(key, raw, targetPrefix, result); replaced {
			result = rw.data
			rewrites = append(rewrites, rw.PathRewrite)
		}
	}

	if len(rewrites) == 0 {
		return nil, nil
	}

	if err := m.Fs.WriteFile(kmPath, result, 0o644); err != nil {
		return nil, fmt.Errorf("write known_marketplaces.json: %w", err)
	}
	return rewrites, nil
}

type rewriteResult struct {
	PathRewrite
	data []byte
}

// rewriteEntry checks a single marketplace entry and returns a rewrite if the
// installLocation needs updating. Uses bytes.Replace on data to preserve JSON ordering.
func rewriteEntry(key string, raw json.RawMessage, targetPrefix string, data []byte) (rewriteResult, bool) {
	var entry map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entry); err != nil {
		return rewriteResult{}, false
	}

	locRaw, ok := entry["installLocation"]
	if !ok {
		return rewriteResult{}, false
	}
	var loc string
	if err := json.Unmarshal(locRaw, &loc); err != nil {
		return rewriteResult{}, false
	}

	idx := strings.Index(loc, marketplacesSubpath)
	if idx < 0 {
		return rewriteResult{}, false
	}

	newLoc := filepath.Join(targetPrefix, loc[idx+len(marketplacesSubpath):])
	if newLoc == loc {
		return rewriteResult{}, false
	}

	replaced := bytes.Replace(data, []byte(`"`+loc+`"`), []byte(`"`+newLoc+`"`), 1)
	return rewriteResult{
		PathRewrite: PathRewrite{Plugin: key, OldPath: loc, NewPath: newLoc},
		data:        replaced,
	}, true
}

func trimNewline(s string) string {
	for s != "" && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
