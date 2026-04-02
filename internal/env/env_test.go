package env_test

import (
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
		CredsFile:  filepath.Join(claudeDir, ".credentials.json"),
	}
	return paths, fsutil.NewOs(false)
}

func TestInitAdoptsExistingCredentials(t *testing.T) {
	paths, fs := setupTestDirs(t)

	original := []byte(`{"token": "abc123"}`)
	if err := os.WriteFile(paths.CredsFile, original, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	info, err := os.Lstat(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("expected credentials file to be a symlink after init")
	}

	target, err := os.Readlink(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	expected := filepath.Join(paths.EnvsDir, "default.credentials.json")
	if target != expected {
		t.Fatalf("symlink target = %q, want %q", target, expected)
	}

	data, err := os.ReadFile(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(original) {
		t.Fatalf("credentials content = %q, want %q", data, original)
	}
}

func TestInitNoExistingCredentials(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := env.New(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	data, err := os.ReadFile(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "{}" {
		t.Fatalf("expected empty JSON, got %q", data)
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

	workCreds := filepath.Join(paths.EnvsDir, "work.credentials.json")
	if err := os.WriteFile(workCreds, []byte(`{"token": "work-token"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Use("work"); err != nil {
		t.Fatalf("Use failed: %v", err)
	}

	target, err := os.Readlink(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	if target != workCreds {
		t.Fatalf("symlink = %q, want %q", target, workCreds)
	}

	data, err := os.ReadFile(paths.CredsFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"token": "work-token"}` {
		t.Fatalf("credentials = %q", data)
	}
}

func TestCurrentResolvesLocal(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Global: "default",
		Environments: map[string]config.Environment{
			"default": {Credentials: "default.credentials.json"},
			"work":    {Credentials: "work.credentials.json"},
		},
	}
	mgr := env.New(paths, cfg, fs)

	projectDir := filepath.Join(t.TempDir(), "myproject")
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
		Environments: map[string]config.Environment{"default": {Credentials: "default.credentials.json"}},
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
