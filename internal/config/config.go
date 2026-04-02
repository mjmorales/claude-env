package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const (
	EnvsDirName  = ".claude-envs"
	ConfigFile   = "config.toml"
	PoolDirName  = "pool"
	LockFileName = ".managed-symlinks"
)

// Environment represents a single Claude Code subscription environment.
type Environment struct {
	Credentials      string   `toml:"credentials"`
	Shared           []string `toml:"shared,omitempty"`
	SettingsOverride string   `toml:"settings_override,omitempty"`
}

// Config is the top-level claude-env configuration.
type Config struct {
	Global       string                 `toml:"global"`
	Environments map[string]Environment `toml:"environments"`
}

// Paths holds resolved filesystem paths.
type Paths struct {
	EnvsDir    string
	ConfigFile string
	PoolDir    string
	LockFile   string
	ClaudeDir  string
	CredsFile  string
}

// Writer abstracts file writing for dry-run support.
type Writer interface {
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(path string, data []byte, perm os.FileMode) error
}

// DefaultPaths returns the standard paths based on the user's home directory.
func DefaultPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home directory: %w", err)
	}

	envsDir := filepath.Join(home, EnvsDirName)
	return Paths{
		EnvsDir:    envsDir,
		ConfigFile: filepath.Join(envsDir, ConfigFile),
		PoolDir:    filepath.Join(envsDir, PoolDirName),
		LockFile:   filepath.Join(envsDir, LockFileName),
		ClaudeDir:  filepath.Join(home, ".claude"),
		CredsFile:  filepath.Join(home, ".claude", ".credentials.json"),
	}, nil
}

// Load reads the config from disk. Returns a zero Config if the file doesn't exist.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{Environments: make(map[string]Environment)}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]Environment)
	}
	return cfg, nil
}

// Save writes the config to disk using the provided writer.
func Save(path string, cfg Config, w Writer) error {
	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := w.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	if err := w.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
