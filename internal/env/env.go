package env

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

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

	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
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
	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
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
	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
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

	c := exec.Command(claudeBin, "auth", "login")
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

	c := exec.Command(claudeBin, "auth", "status")
	c.Env = append(os.Environ(), "CLAUDE_CONFIG_DIR="+m.Paths.EnvDir(name))
	out, err := c.Output()
	if err != nil {
		return "not authenticated", nil
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
func (m *Manager) Current(dir string) (string, string, error) {
	current := dir
	for {
		pinPath := filepath.Join(current, LocalPinFile)
		data, err := m.Fs.ReadFile(pinPath)
		if err == nil {
			name := trimNewline(string(data))
			if _, exists := m.Cfg.Environments[name]; exists {
				return name, "local (" + pinPath + ")", nil
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
	activeName, _, _ := m.Current(dir)
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
	_ = m.Fs.RemoveAll(envDir)

	delete(m.Cfg.Environments, name)
	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
}

// EnvInfo holds display information for an environment.
type EnvInfo struct {
	Name   string
	Active bool
	Shared []string
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
		_ = m.Fs.WriteFile(dst, data, f.perm)
	}
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
