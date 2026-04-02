package symlink

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/mjmorales/claude-env/internal/fsutil"
)

// Reconciler manages symlinks between the pool and ~/.claude/.
type Reconciler struct {
	PoolDir   string
	TargetDir string
	LockFile  string
	Fs        *fsutil.SymlinkFs
}

// New creates a symlink reconciler.
func New(poolDir, targetDir, lockFile string, fs *fsutil.SymlinkFs) *Reconciler {
	return &Reconciler{
		PoolDir:   poolDir,
		TargetDir: targetDir,
		LockFile:  lockFile,
		Fs:        fs,
	}
}

// Reconcile ensures the target directory has symlinks for exactly the declared
// shared resources. It removes stale managed symlinks and creates missing ones.
func (r *Reconciler) Reconcile(shared []string) error {
	managed, err := r.readLock()
	if err != nil {
		return fmt.Errorf("read lock file: %w", err)
	}

	want := make(map[string]bool, len(shared))
	for _, s := range shared {
		want[s] = true
	}

	// Remove symlinks no longer wanted.
	for _, entry := range managed {
		if !want[entry] {
			target := r.targetPath(entry)
			info, err := r.Fs.Lstat(target)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				r.Fs.Remove(target)
				fmt.Printf("  removed: %s\n", entry)
			}
		}
	}

	// Create missing symlinks.
	var newManaged []string
	for _, s := range shared {
		src := filepath.Join(r.PoolDir, s)
		dst := r.targetPath(s)

		if _, err := r.Fs.Stat(src); os.IsNotExist(err) {
			fmt.Printf("  warning: pool resource %q not found, skipping\n", s)
			continue
		}

		info, err := r.Fs.Lstat(dst)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				existing, _ := r.Fs.Readlink(dst)
				if existing == src {
					newManaged = append(newManaged, s)
					continue
				}
				r.Fs.Remove(dst)
			} else {
				fmt.Printf("  skipped: %s (real file exists, not managed)\n", s)
				continue
			}
		}

		if err := r.Fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return fmt.Errorf("create parent for %s: %w", s, err)
		}
		if err := r.Fs.Symlink(src, dst); err != nil {
			return fmt.Errorf("symlink %s: %w", s, err)
		}
		fmt.Printf("  linked: %s\n", s)
		newManaged = append(newManaged, s)
	}

	return r.writeLock(newManaged)
}

// Status returns the current state of managed symlinks.
func (r *Reconciler) Status() ([]LinkStatus, error) {
	managed, err := r.readLock()
	if err != nil {
		return nil, err
	}

	var statuses []LinkStatus
	for _, entry := range managed {
		dst := r.targetPath(entry)
		info, err := r.Fs.Lstat(dst)
		if err != nil {
			statuses = append(statuses, LinkStatus{Name: entry, State: "missing"})
			continue
		}
		if info.Mode()&os.ModeSymlink == 0 {
			statuses = append(statuses, LinkStatus{Name: entry, State: "conflict"})
			continue
		}
		target, _ := r.Fs.Readlink(dst)
		expected := filepath.Join(r.PoolDir, entry)
		if target != expected {
			statuses = append(statuses, LinkStatus{Name: entry, State: "stale"})
		} else {
			statuses = append(statuses, LinkStatus{Name: entry, State: "ok"})
		}
	}
	return statuses, nil
}

// LinkStatus describes the state of a single managed symlink.
type LinkStatus struct {
	Name  string
	State string // "ok", "missing", "conflict", "stale"
}

func (r *Reconciler) targetPath(resource string) string {
	return filepath.Join(r.TargetDir, resource)
}

func (r *Reconciler) readLock() ([]string, error) {
	f, err := r.Fs.Open(r.LockFile)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entries = append(entries, line)
		}
	}
	return entries, scanner.Err()
}

func (r *Reconciler) writeLock(entries []string) error {
	sort.Strings(entries)
	content := strings.Join(entries, "\n") + "\n"
	return r.Fs.WriteFile(r.LockFile, []byte(content), 0o644)
}
