//nolint:revive // magic numbers are clear for file permissions
package symlink

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
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
func New(poolDir, targetDir, lockFile string, symLinkFs *fsutil.SymlinkFs) *Reconciler {
	return &Reconciler{
		PoolDir:   poolDir,
		TargetDir: targetDir,
		LockFile:  lockFile,
		Fs:        symLinkFs,
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

	r.removeUnwanted(managed, want)

	newManaged, err := r.createWanted(shared)
	if err != nil {
		return err
	}

	return r.writeLock(newManaged)
}

//nolint:nestif // acceptable nesting for conditional logic
func (r *Reconciler) removeUnwanted(managed []string, want map[string]bool) {
	for _, entry := range managed {
		if !want[entry] {
			target := r.targetPath(entry)
			info, err := r.Fs.Lstat(target)
			if err != nil {
				continue
			}
			if info.Mode()&os.ModeSymlink != 0 {
				if err := r.Fs.Remove(target); err != nil {
					fmt.Fprintf(os.Stderr, "  warning: failed to remove %s: %v\n", entry, err)
				} else {
					fmt.Printf("  removed: %s\n", entry)
				}
			}
		}
	}
}

func (r *Reconciler) createWanted(shared []string) ([]string, error) {
	newManaged := make([]string, 0, len(shared))
	for _, s := range shared {
		src := filepath.Join(r.PoolDir, s)
		dst := r.targetPath(s)

		if _, err := r.Fs.Stat(src); os.IsNotExist(err) {
			fmt.Printf("  warning: pool resource %q not found, skipping\n", s)
			continue
		}

		created, err := r.createSymlink(s, src, dst)
		if err != nil {
			return nil, err
		}
		if created {
			newManaged = append(newManaged, s)
		}
	}
	return newManaged, nil
}

func (r *Reconciler) createSymlink(name, src, dst string) (bool, error) {
	ok, err := r.handleExisting(name, src, dst)
	if err != nil || ok {
		return ok, err
	}

	if err := r.Fs.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return false, fmt.Errorf("create parent for %s: %w", name, err)
	}
	if err := r.Fs.Symlink(src, dst); err != nil {
		return false, fmt.Errorf("symlink %s: %w", name, err)
	}
	fmt.Printf("  linked: %s\n", name)
	return true, nil
}

func (r *Reconciler) handleExisting(s, src, dst string) (bool, error) {
	info, err := r.Fs.Lstat(dst)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		existing, err := r.Fs.Readlink(dst)
		if err == nil && existing == src {
			return true, nil
		}
		if err := r.Fs.Remove(dst); err != nil {
			fmt.Fprintf(os.Stderr, "  warning: failed to remove %s: %v\n", s, err)
			return false, fmt.Errorf("remove: %w", err)
		}
		return false, nil
	}
	fmt.Printf("  skipped: %s (real file exists, not managed)\n", s)
	return true, nil
}

// Status returns the current state of managed symlinks.
func (r *Reconciler) Status() ([]LinkStatus, error) {
	managed, err := r.readLock()
	if err != nil {
		return nil, err
	}

	statuses := make([]LinkStatus, 0, len(managed))
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
		target, err := r.Fs.Readlink(dst)
		if err != nil {
			statuses = append(statuses, LinkStatus{Name: entry, State: "broken"})
			continue
		}
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
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to close lock file: %v\n", err)
		}
	}()

	var entries []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			entries = append(entries, line)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan lock file: %w", err)
	}
	return entries, nil
}

func (r *Reconciler) writeLock(entries []string) error {
	sort.Strings(entries)
	content := strings.Join(entries, "\n") + "\n"
	if err := r.Fs.WriteFile(r.LockFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write lock file: %w", err)
	}
	return nil
}
