//nolint:revive // magic numbers and string constants OK in tests
package env_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mjmorales/claude-env/internal/config"
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
	mgr := newTestManager(paths, cfg, fs)

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

	// An existing default Claude Code login is a .credentials.json under ~/.claude.
	original := []byte(`{"claudeAiOauth":{"accessToken":"sk-ant-oat01-x","subscriptionType":"max"}}`)
	if err := os.WriteFile(filepath.Join(paths.ClaudeDir, ".credentials.json"), original, 0o600); err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)

	if err := mgr.Init(); err != nil {
		t.Fatalf("Init failed: %v", err)
	}

	// The token should be adopted into the default env's credential file.
	data, err := os.ReadFile(filepath.Join(paths.EnvDir("default"), ".credentials.json"))
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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

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
	mgr := newTestManager(paths, cfg, fs)

	got := mgr.ConfigDir("myenv")
	want := filepath.Join(paths.EnvsDir, "myenv")
	if got != want {
		t.Fatalf("ConfigDir = %q, want %q", got, want)
	}
}

func TestSharedAdd(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	// Create a resource in the pool so validation passes.
	poolResource := filepath.Join(paths.PoolDir, "agents", "reviewer.md")
	if err := os.MkdirAll(filepath.Dir(poolResource), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(poolResource, []byte("# Reviewer"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SharedAdd("default", "agents/reviewer.md"); err != nil {
		t.Fatalf("SharedAdd failed: %v", err)
	}

	e := mgr.Cfg.Environments["default"]
	if len(e.Shared) != 1 || e.Shared[0] != "agents/reviewer.md" {
		t.Fatalf("shared = %v, want [agents/reviewer.md]", e.Shared)
	}

	// Adding duplicate should error.
	if err := mgr.SharedAdd("default", "agents/reviewer.md"); err == nil {
		t.Fatal("expected error adding duplicate shared resource")
	}
}

func TestSharedAddCopiesAbsolutePathToPool(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	// Create a source file outside the pool.
	srcDir := filepath.Join(t.TempDir(), "skills", "my-skill")
	if err := os.MkdirAll(srcDir, 0o750); err != nil {
		t.Fatal(err)
	}
	srcFile := filepath.Join(srcDir, "SKILL.md")
	if err := os.WriteFile(srcFile, []byte("# My Skill"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Add using absolute path — should copy to pool as skills/my-skill.
	if err := mgr.SharedAdd("default", srcDir); err != nil {
		t.Fatalf("SharedAdd with absolute path failed: %v", err)
	}

	// Verify it was registered with the relative path.
	e := mgr.Cfg.Environments["default"]
	if len(e.Shared) != 1 || e.Shared[0] != "skills/my-skill" {
		t.Fatalf("shared = %v, want [skills/my-skill]", e.Shared)
	}

	// Verify the file was copied into the pool.
	poolFile := filepath.Join(paths.PoolDir, "skills", "my-skill", "SKILL.md")
	data, err := os.ReadFile(filepath.Clean(poolFile))
	if err != nil {
		t.Fatalf("expected pool file to exist: %v", err)
	}
	if string(data) != "# My Skill" {
		t.Fatalf("pool file content = %q, want '# My Skill'", data)
	}
}

func TestSharedAddRejectsAbsolutePathNotFound(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SharedAdd("default", "/nonexistent/path/agent.md"); err == nil {
		t.Fatal("expected error for nonexistent absolute path")
	}
}

func TestSharedAddRejectsMissingPool(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SharedAdd("default", "agents/nonexistent.md"); err == nil {
		t.Fatal("expected error for resource not in pool")
	}
}

func TestSharedRemove(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Environments: map[string]config.Environment{
			"default": {Shared: []string{"agents/reviewer.md", "commands/deploy.md"}},
		},
		Global: "default",
	}
	mgr := newTestManager(paths, cfg, fs)

	// Create dirs so Save works.
	if err := os.MkdirAll(paths.EnvsDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SharedRemove("default", "agents/reviewer.md"); err != nil {
		t.Fatalf("SharedRemove failed: %v", err)
	}

	e := mgr.Cfg.Environments["default"]
	if len(e.Shared) != 1 || e.Shared[0] != "commands/deploy.md" {
		t.Fatalf("shared = %v, want [commands/deploy.md]", e.Shared)
	}

	// Removing nonexistent should error.
	if err := mgr.SharedRemove("default", "nonexistent"); err == nil {
		t.Fatal("expected error removing nonexistent shared resource")
	}
}

func TestSharedAddNonexistentEnv(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)

	if err := mgr.SharedAdd("ghost", "agents/foo.md"); err == nil {
		t.Fatal("expected error for nonexistent environment")
	}
}

func TestSetOverride(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.SetOverride("default", "/tmp/settings.json"); err != nil {
		t.Fatalf("SetOverride failed: %v", err)
	}

	e := mgr.Cfg.Environments["default"]
	if e.SettingsOverride != "/tmp/settings.json" {
		t.Fatalf("settings_override = %q, want /tmp/settings.json", e.SettingsOverride)
	}
}

func TestClearOverride(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{
		Environments: map[string]config.Environment{
			"default": {SettingsOverride: "/tmp/settings.json"},
		},
		Global: "default",
	}
	mgr := newTestManager(paths, cfg, fs)

	if err := os.MkdirAll(paths.EnvsDir, 0o750); err != nil {
		t.Fatal(err)
	}

	if err := mgr.ClearOverride("default"); err != nil {
		t.Fatalf("ClearOverride failed: %v", err)
	}

	e := mgr.Cfg.Environments["default"]
	if e.SettingsOverride != "" {
		t.Fatalf("settings_override = %q, want empty", e.SettingsOverride)
	}
}

func TestClearOverrideNonexistentEnv(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)

	if err := mgr.ClearOverride("ghost"); err == nil {
		t.Fatal("expected error for nonexistent environment")
	}
}

//nolint:gosec // test file uses temp dirs with relaxed permissions
func writeTestMarketplaces(t *testing.T, poolDir, defaultEnvDir string) string {
	t.Helper()
	kmDir := filepath.Join(poolDir, "plugins")
	if err := os.MkdirAll(kmDir, 0o750); err != nil {
		t.Fatal(err)
	}

	defaultPrefix := filepath.Join(defaultEnvDir, "plugins", "marketplaces")
	km := map[string]any{
		"prove": map[string]any{
			"source":          map[string]any{"source": "github", "repo": "mjmorales/claude-prove"},
			"installLocation": filepath.Join(defaultPrefix, "prove"),
			"lastUpdated":     "2026-04-06T16:57:08.995Z",
		},
		"keel": map[string]any{
			"source":          map[string]any{"source": "directory", "path": "/external/.keel"},
			"installLocation": "/external/.keel",
			"lastUpdated":     "2026-04-07T00:38:56.568Z",
		},
	}
	data, err := json.MarshalIndent(km, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	kmPath := filepath.Join(kmDir, "known_marketplaces.json")
	if err := os.WriteFile(kmPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return kmPath
}

func TestUseRewritesMarketplacePaths(t *testing.T) {
	paths, fs := setupTestDirs(t)

	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add("work"); err != nil {
		t.Fatal(err)
	}

	kmPath := writeTestMarketplaces(t, paths.PoolDir, paths.EnvDir("default"))

	if err := mgr.Use("work"); err != nil {
		t.Fatalf("Use failed: %v", err)
	}

	updated, err := os.ReadFile(kmPath) //#nosec G304
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]map[string]any
	if err := json.Unmarshal(updated, &result); err != nil {
		t.Fatal(err)
	}

	wantLoc := filepath.Join(paths.EnvDir("work"), "plugins", "marketplaces", "prove")
	gotLoc, ok := result["prove"]["installLocation"].(string)
	if !ok || gotLoc != wantLoc {
		t.Fatalf("prove installLocation = %q, want %q", gotLoc, wantLoc)
	}

	keelLoc, ok := result["keel"]["installLocation"].(string)
	if !ok || keelLoc != "/external/.keel" {
		t.Fatalf("keel installLocation = %q, want /external/.keel", keelLoc)
	}
}

// --- v2 OAuth credential flow tests ---

const validBlob = `{"claudeAiOauth":{"accessToken":"sk-ant-oat01-tok","refreshToken":"sk-ant-ort01-ref","subscriptionType":"max","expiresAt":1700000300000}}`

func credPath(paths config.Paths, name string) string {
	return filepath.Join(paths.EnvDir(name), ".credentials.json")
}

func TestLoginCapturesKeychainIntoFile(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	// Simulate macOS: `claude auth login` writes the token to the keychain.
	kc := newFakeKeychain(true)
	mgr.Keychain = kc
	mgr.Claude = &fakeClaude{loginFunc: func(dir string) error {
		kc.entries[dir] = []byte(validBlob)
		return nil
	}}

	if err := mgr.Login("default"); err != nil {
		t.Fatalf("Login: %v", err)
	}

	// Token must be materialized into the file...
	data, err := os.ReadFile(credPath(paths, "default"))
	if err != nil {
		t.Fatalf("expected credential file: %v", err)
	}
	if string(data) != validBlob {
		t.Fatalf("file = %q, want captured blob", data)
	}
	// ...and the keychain entry purged so the file is the single source of truth.
	if len(kc.deleted) == 0 {
		t.Fatal("expected keychain entry to be deleted after capture")
	}
	if _, ok := kc.entries[paths.EnvDir("default")]; ok {
		t.Fatal("keychain entry still present after capture")
	}
}

func TestLoginCapturesFileWhenNoKeychain(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	// Simulate Linux: no keychain; `claude auth login` writes the file directly.
	mgr.Keychain = newFakeKeychain(false)
	mgr.Claude = &fakeClaude{loginFunc: func(dir string) error {
		return os.WriteFile(filepath.Join(dir, ".credentials.json"), []byte(validBlob), 0o600)
	}}

	if err := mgr.Login("default"); err != nil {
		t.Fatalf("Login: %v", err)
	}
	if _, err := os.Stat(credPath(paths, "default")); err != nil {
		t.Fatalf("expected credential file: %v", err)
	}
}

func TestLoginFailsWhenNoCredentialsProduced(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	// claude "succeeds" but writes nothing; capture must error.
	mgr.Keychain = newFakeKeychain(false)
	mgr.Claude = &fakeClaude{}
	if err := mgr.Login("default"); err == nil {
		t.Fatal("expected error when login produces no credentials")
	}
}

func TestAuthStatusNative(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	// Not authenticated until a token exists.
	info, err := mgr.AuthStatus("default")
	if err != nil {
		t.Fatal(err)
	}
	if info.Authenticated {
		t.Fatal("expected not authenticated before import")
	}

	if err := mgr.Import("default", []byte(validBlob)); err != nil {
		t.Fatalf("Import: %v", err)
	}
	info, err = mgr.AuthStatus("default")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Authenticated || info.SubscriptionType != "max" {
		t.Fatalf("info = %+v, want authenticated max", info)
	}
	// testNowMs is 1_700_000_000_000; token expires at +300000ms => not expired.
	if info.Expired {
		t.Fatal("token should not be expired at testNowMs")
	}
	if info.ExpiresIn <= 0 {
		t.Fatalf("ExpiresIn = %v, want positive", info.ExpiresIn)
	}
}

func TestAuthStatusIncludesEmail(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Import("default", []byte(validBlob)); err != nil {
		t.Fatal(err)
	}
	// The email lives in .claude.json's oauthAccount, not the token blob.
	claudeJSON := []byte(`{"oauthAccount":{"emailAddress":"user@example.com"}}`)
	if err := os.WriteFile(filepath.Join(paths.EnvDir("default"), ".claude.json"), claudeJSON, 0o600); err != nil {
		t.Fatal(err)
	}

	info, err := mgr.AuthStatus("default")
	if err != nil {
		t.Fatal(err)
	}
	if info.Email != "user@example.com" {
		t.Fatalf("Email = %q, want user@example.com", info.Email)
	}

	// No .claude.json (or no oauthAccount) => empty email, not an error.
	if err := mgr.Add("work"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Import("work", []byte(validBlob)); err != nil {
		t.Fatal(err)
	}
	info, err = mgr.AuthStatus("work")
	if err != nil {
		t.Fatal(err)
	}
	if info.Email != "" {
		t.Fatalf("Email = %q, want empty when no oauthAccount present", info.Email)
	}
}

func TestImportBareToken(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}

	if err := mgr.Import("default", []byte("sk-ant-oat01-bare\n")); err != nil {
		t.Fatalf("Import bare token: %v", err)
	}
	info, err := mgr.AuthStatus("default")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Authenticated {
		t.Fatal("expected authenticated after bare-token import")
	}
}

func TestImportRejectsGarbage(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Import("default", []byte("not a token and not json")); err == nil {
		t.Fatal("expected error importing garbage")
	}
}

func TestImportFromEnvAndExport(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add("work"); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Import("default", []byte(validBlob)); err != nil {
		t.Fatal(err)
	}
	if err := mgr.ImportFromEnv("work", "default"); err != nil {
		t.Fatalf("ImportFromEnv: %v", err)
	}
	out, err := mgr.Export("work")
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if string(out) != validBlob {
		t.Fatalf("exported = %q, want %q", out, validBlob)
	}
}

func TestRemovePurgesKeychain(t *testing.T) {
	paths, fs := setupTestDirs(t)
	cfg := config.Config{Environments: make(map[string]config.Environment)}
	mgr := newTestManager(paths, cfg, fs)
	if err := mgr.Init(); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Add("work"); err != nil {
		t.Fatal(err)
	}
	kc := newFakeKeychain(true)
	mgr.Keychain = kc
	if err := mgr.Remove("work"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	found := false
	for _, d := range kc.deleted {
		if d == paths.EnvDir("work") {
			found = true
		}
	}
	if !found {
		t.Fatalf("Remove did not purge keychain for work env; deleted=%v", kc.deleted)
	}
}
