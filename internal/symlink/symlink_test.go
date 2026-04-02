//nolint:gocyclo,cyclop,revive // tests have multiple branches for different scenarios
package symlink_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mjmorales/claude-env/internal/fsutil"
	"github.com/mjmorales/claude-env/internal/symlink"
)

func newTestFs() *fsutil.SymlinkFs {
	return fsutil.NewOs(false)
}

func TestReconcileCreatesSymlinks(t *testing.T) {
	tmp := t.TempDir()
	pool := filepath.Join(tmp, "pool")
	target := filepath.Join(tmp, "claude")
	lockFile := filepath.Join(tmp, "lock")

	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Join(pool, "skills", "brainstorm"), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Join(pool, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test file
	if err := os.WriteFile(filepath.Join(pool, "agents", "reviewer"), []byte("agent"), 0o644); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test directory
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	r := symlink.New(pool, target, lockFile, newTestFs())

	shared := []string{"skills/brainstorm", "agents/reviewer"}
	if err := r.Reconcile(shared); err != nil {
		t.Fatalf("Reconcile failed: %v", err)
	}

	for _, s := range shared {
		dst := filepath.Join(target, s)
		info, err := os.Lstat(dst)
		if err != nil {
			t.Fatalf("expected %s to exist: %v", s, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("expected %s to be a symlink", s)
		}
	}

	statuses, err := r.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != 2 {
		t.Fatalf("expected 2 statuses, got %d", len(statuses))
	}
	for _, s := range statuses {
		if s.State != "ok" {
			t.Fatalf("status %q = %q, want 'ok'", s.Name, s.State)
		}
	}
}

func TestReconcileRemovesStale(t *testing.T) {
	tmp := t.TempDir()
	pool := filepath.Join(tmp, "pool")
	target := filepath.Join(tmp, "claude")
	lockFile := filepath.Join(tmp, "lock")

	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Join(pool, "skills", "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Join(pool, "skills", "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test directory
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	r := symlink.New(pool, target, lockFile, newTestFs())

	if err := r.Reconcile([]string{"skills/old"}); err != nil {
		t.Fatal(err)
	}

	if err := r.Reconcile([]string{"skills/new"}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Lstat(filepath.Join(target, "skills", "old")); !os.IsNotExist(err) {
		t.Fatal("expected old symlink to be removed")
	}
	if _, err := os.Lstat(filepath.Join(target, "skills", "new")); err != nil {
		t.Fatal("expected new symlink to exist")
	}
}

func TestReconcileSkipsRealFiles(t *testing.T) {
	tmp := t.TempDir()
	pool := filepath.Join(tmp, "pool")
	target := filepath.Join(tmp, "claude")
	lockFile := filepath.Join(tmp, "lock")

	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Join(pool, "skills", "mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test directory
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatal(err)
	}

	realPath := filepath.Join(target, "skills", "mine")
	//nolint:gosec // test directory
	if err := os.MkdirAll(filepath.Dir(realPath), 0o755); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // test file
	if err := os.WriteFile(realPath, []byte("real"), 0o644); err != nil {
		t.Fatal(err)
	}

	r := symlink.New(pool, target, lockFile, newTestFs())
	if err := r.Reconcile([]string{"skills/mine"}); err != nil {
		t.Fatal(err)
	}

	info, err := os.Lstat(realPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		t.Fatal("expected real file to be preserved, not replaced with symlink")
	}
}
