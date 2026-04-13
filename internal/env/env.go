//nolint:revive // magic numbers and string constants are clear in context
package env

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

const LocalPinFile = ".claude-env"

// Manager handles environment switching and credential management.
type Manager struct {
	Paths config.Paths
	Cfg   config.Config
	Fs    *fsutil.SymlinkFs
}

// New creates an environment manager.
func New(paths config.Paths, cfg config.Config, fs *fsutil.SymlinkFs) *Manager {
	return &Manager{Paths: paths, Cfg: cfg, Fs: fs}
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

	// Copy existing ~/.claude/.claude.json into the new env dir if it exists.
	existingCreds := filepath.Join(m.Paths.ClaudeDir, ".claude.json")
	if data, err := m.Fs.ReadFile(existingCreds); err == nil && len(data) > 0 {
		dst := filepath.Join(envDir, ".claude.json")
		if err := m.Fs.WriteFile(dst, data, 0o600); err != nil {
			return fmt.Errorf("copy existing credentials: %w", err)
		}
		fmt.Println("Adopted existing credentials as 'default' environment.")
	} else {
		fmt.Println("Created 'default' environment. Run 'claude-env login' to authenticate.")
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

// Login runs 'claude auth login' with CLAUDE_CONFIG_DIR pointed at the
// named environment's directory.
func (m *Manager) Login(name string) error {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	envDir := m.Paths.EnvDir(name)

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Logging in for environment %q...\n", name)

	//nolint:gosec // claudeBin is validated by exec.LookPath
	c := exec.CommandContext(context.Background(), claudeBin, "auth", "login")
	c.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+envDir)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr

	if err := c.Run(); err != nil {
		return fmt.Errorf("claude auth login failed: %w", err)
	}

	// Ensure the global config has onboarding flags so interactive mode
	// doesn't prompt for setup. Claude Code checks hasCompletedOnboarding
	// and theme in .claude.json to decide whether to show onboarding.
	m.patchClaudeConfig(envDir)

	fmt.Fprintf(os.Stderr, "Environment %q authenticated.\n", name)
	return nil
}

// AuthStatus returns the auth status for the named environment.
func (m *Manager) AuthStatus(name string) (string, error) {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return "", fmt.Errorf("environment %q not found", name)
	}

	claudeBin, err := exec.LookPath("claude")
	if err != nil {
		return "", fmt.Errorf("claude CLI not found in PATH: %w", err)
	}

	//nolint:gosec // claudeBin is validated by exec.LookPath
	c := exec.CommandContext(context.Background(), claudeBin, "auth", "status")
	c.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+m.Paths.EnvDir(name))
	out, err := c.Output()
	if err != nil {
		return "", fmt.Errorf("run auth status: %w", err)
	}
	return string(out), nil
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
