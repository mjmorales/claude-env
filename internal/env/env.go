package env

import (
	"encoding/json"
	"fmt"
	"os"
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

// Init sets up ~/.claude-envs/ and adopts existing credentials as "default".
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

	credsPath := m.credentialPath("default")
	existingCreds := m.Paths.CredsFile

	// Check existing credentials — use Lstat to detect symlinks.
	info, err := m.Fs.Lstat(existingCreds)
	if err == nil && info.Mode().IsRegular() {
		if err := m.Fs.Rename(existingCreds, credsPath); err != nil {
			return fmt.Errorf("move existing credentials: %w", err)
		}
		fmt.Println("Adopted existing credentials as 'default' environment.")
	} else if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("credentials file is already a symlink — already managed?")
	} else {
		if err := m.Fs.WriteFile( credsPath, []byte("{}"), 0o600); err != nil {
			return fmt.Errorf("create credential placeholder: %w", err)
		}
		fmt.Println("No existing credentials found. Created empty 'default' environment.")
	}

	if err := m.Fs.ForceSymlink(credsPath, existingCreds); err != nil {
		return fmt.Errorf("symlink credentials: %w", err)
	}

	m.Cfg.Global = "default"
	m.Cfg.Environments["default"] = config.Environment{
		Credentials: filepath.Base(credsPath),
	}

	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
}

// Add registers a new environment with an empty credential file.
func (m *Manager) Add(name string) error {
	if _, exists := m.Cfg.Environments[name]; exists {
		return fmt.Errorf("environment %q already exists", name)
	}

	credsFile := name + ".credentials.json"
	credsPath := filepath.Join(m.Paths.EnvsDir, credsFile)

	if err := m.Fs.WriteFile( credsPath, []byte("{}"), 0o600); err != nil {
		return fmt.Errorf("create credential file: %w", err)
	}

	m.Cfg.Environments[name] = config.Environment{
		Credentials: credsFile,
	}

	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
}

// Use switches the global environment.
func (m *Manager) Use(name string) error {
	env, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	credsPath := filepath.Join(m.Paths.EnvsDir, env.Credentials)
	if _, err := m.Fs.Stat(credsPath); err != nil {
		return fmt.Errorf("credential file missing for %q: %w", name, err)
	}

	if err := m.Fs.ForceSymlink(credsPath, m.Paths.CredsFile); err != nil {
		return fmt.Errorf("swap credentials symlink: %w", err)
	}

	m.Cfg.Global = name
	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
}

// Local pins an environment to the current directory.
func (m *Manager) Local(name, dir string) error {
	if _, exists := m.Cfg.Environments[name]; !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	pinPath := filepath.Join(dir, LocalPinFile)
	if err := m.Fs.WriteFile( pinPath, []byte(name+"\n"), 0o644); err != nil {
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

// List returns all environment names with their active status.
func (m *Manager) List(dir string) []EnvInfo {
	activeName, _, _ := m.Current(dir)
	var envs []EnvInfo
	for name, e := range m.Cfg.Environments {
		envs = append(envs, EnvInfo{
			Name:   name,
			Active: name == activeName,
			Creds:  e.Credentials,
			Shared: e.Shared,
		})
	}
	return envs
}

// Remove deletes an environment and its credential file.
func (m *Manager) Remove(name string) error {
	env, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}
	if m.Cfg.Global == name {
		return fmt.Errorf("cannot remove the active global environment — switch first")
	}

	credsPath := filepath.Join(m.Paths.EnvsDir, env.Credentials)
	_ = m.Fs.Remove(credsPath)

	delete(m.Cfg.Environments, name)
	return config.Save(m.Paths.ConfigFile, m.Cfg, m.Fs)
}

// ImportCredentials copies a credentials JSON blob into the named environment.
func (m *Manager) ImportCredentials(name string, data []byte) error {
	env, exists := m.Cfg.Environments[name]
	if !exists {
		return fmt.Errorf("environment %q not found", name)
	}

	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON credentials")
	}

	credsPath := filepath.Join(m.Paths.EnvsDir, env.Credentials)
	return m.Fs.WriteFile( credsPath, data, 0o600)
}

// EnvInfo holds display information for an environment.
type EnvInfo struct {
	Name   string
	Active bool
	Creds  string
	Shared []string
}

func (m *Manager) credentialPath(name string) string {
	return filepath.Join(m.Paths.EnvsDir, name+".credentials.json")
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}
