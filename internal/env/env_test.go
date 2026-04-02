//nolint:revive // magic numbers and string constants OK in tests
package env_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/env"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

func setupTestDirs(t *testing.T) (config.Paths, *fsutil.SymlinkFs) {
	t.Helper()
	tmp := t.TempDir()

	claudeDir := filepath.Join(tmp, ".claude")
	//nolint:gosec // test directory
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}

	envsDir := filepath.Join(tmp, ".claude-envs")
	paths := config.Paths{
		EnvsDir:    envsDir,
		ConfigFile: filepath.Join(envsDir, "config.toml"),
		PoolDir:    filepath.Join(envsDir, "pool"),
		LockFile:   filepath.Join(envsDir, ".managed-symlinks"),
		ClaudeDir:  claudeDir,
	}
	return paths, fsutil.NewOs(false)
}

func TestInitCreatesEnvDir(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Default env dir should exist.
	envDir := paths.EnvDir("default")
	if _, err := os.Stat(envDir); err != nil {
		t.Fatalf("expected env dir %s to exist: %v", envDir, err)
	}

	// Config should be written.
	if _, err := os.Stat(paths.ConfigFile); err != nil {
		t.Fatalf("expected config file to exist: %v", err)
	}

	// Global should be "default".
	if mgr.Cfg.Global != "default" {
		t.Fatalf("global = %q, want 'default'", mgr.Cfg.Global)
	}
}

func TestInitAdoptsExistingCredentials(t *testing.T) {
	paths, fs := setupTestDirs(t)

	// Create fake existing .claude.json.
	original := []byte(`{"oauthAccount": {"token": "abc123"}}`)
	if err := os.WriteFile(filepath.Join(paths.ClaudeDir, ".claude.json"), original, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// Credentials should be copied to the env dir.
	data, err := os.ReadFile(filepath.Join(paths.EnvDir("default"), ".claude.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, original) {
		t.Fatalf("credentials = %q, want %q", data, original)
	}
}

func TestInitAlreadyInitialized(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Init(); err == nil {
		t.Fatal("expected error on double init")
	}
}

func TestAddAndUse(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Add("work"); err != nil {
		t.Fatalf("Add failed: %v", err)
	}

	// Env dir should exist.
	if _, err := os.Stat(paths.EnvDir("work")); err != nil {
		t.Fatalf("expected work env dir to exist: %v", err)
	}

	if err := mgr.Use("work"); err != nil {
		t.Fatalf("Use failed: %v", err)
	}
	if mgr.Cfg.Global != "work" {
		t.Fatalf("global = %q, want 'work'", mgr.Cfg.Global)
	}
}

func TestCurrentResolvesLocal(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Global: "default",
		Environments: map[string]config.Environment{
			"default": {},
			"work":    {},
		},
	}
	mgr := env.New(paths, cfg, fs)

	projectDir := filepath.Join(t.TempDir(), "myproject")
	//nolint:gosec // test directory
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Local("work", projectDir); err != nil {
		t.Fatal(err)
	}

	name, source, err := mgr.Current(projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if name != "work" {
		t.Fatalf("Current = %q, want 'work'", name)
	}
	if source != "local ("+filepath.Join(projectDir, ".claude-env")+")" {
		t.Fatalf("source = %q", source)
	}
}

func TestCurrentFallsBackToGlobal(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Global:       "default",
		Environments: map[string]config.Environment{"default": {}},
	}
	mgr := env.New(paths, cfg, fs)

	name, source, err := mgr.Current(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if name != "default" || source != "global" {
		t.Fatalf("Current = (%q, %q), want ('default', 'global')", name, source)
	}
}

func TestRemove(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add("work"); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Remove("work"); err != nil {
		t.Fatal(err)
	}

	// Dir should be gone.
	if _, err := os.Stat(paths.EnvDir("work")); !os.IsNotExist(err) {
		t.Fatal("expected work env dir to be removed")
	}

	// Can't remove active global.
	if err := mgr.Remove("default"); err == nil {
		t.Fatal("expected error removing active global environment")
	}
}

func TestAddDuplicate(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add("default"); err == nil {
		t.Fatal("expected error adding duplicate environment")
	}
}

func TestUseNonexistent(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Use("nonexistent"); err == nil {
		t.Fatal("expected error using nonexistent environment")
	}
}

func TestConfigDir(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Global:       "myenv",
		Environments: map[string]config.Environment{"myenv": {}},
	}
	mgr := env.New(paths, cfg, fs)

	got := mgr.ConfigDir("myenv")
	want := filepath.Join(paths.EnvsDir, "myenv")
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
}
