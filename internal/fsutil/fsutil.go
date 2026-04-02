package fsutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// SymlinkFs extends afero.Fs with symlink operations and dry-run support.
// In dry-run mode, all mutating operations log to stdout instead of acting.
// Read operations always pass through to the real underlying filesystem.
type SymlinkFs struct {
	afero.Fs
	DryRun   bool
	realFs   afero.Fs // always OsFs, used for reads in dry-run mode
}

// New creates a SymlinkFs wrapping the given afero filesystem.
func New(fs afero.Fs, dryRun bool) *SymlinkFs {
	return &SymlinkFs{Fs: fs, DryRun: dryRun, realFs: afero.NewOsFs()}
}

// NewOs creates a SymlinkFs backed by the real OS filesystem.
// In dry-run mode, writes are logged but not executed; reads still work.
func NewOs(dryRun bool) *SymlinkFs {
	real := afero.NewOsFs()
	if dryRun {
		return &SymlinkFs{Fs: afero.NewReadOnlyFs(real), DryRun: true, realFs: real}
	}
	return &SymlinkFs{Fs: real, DryRun: false, realFs: real}
}

// MkdirAll creates directories. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) MkdirAll(path string, perm os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] mkdir -p %s\n", path)
		return nil
	}
	return s.Fs.MkdirAll(path, perm)
}

// Create creates a file. In dry-run mode, logs and returns a no-op file.
func (s *SymlinkFs) Create(name string) (afero.File, error) {
	if s.DryRun {
		fmt.Printf("[dry-run] create %s\n", name)
		return afero.NewMemMapFs().Create(name)
	}
	return s.Fs.Create(name)
}

// WriteFile writes data to a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) WriteFile(path string, data []byte, perm os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] write %s (%d bytes)\n", path, len(data))
		return nil
	}
	return afero.WriteFile(s.Fs, path, data, perm)
}

// Remove removes a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Remove(name string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] remove %s\n", name)
		return nil
	}
	return s.Fs.Remove(name)
}

// Rename moves a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Rename(oldname, newname string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] move %s -> %s\n", oldname, newname)
		return nil
	}
	return s.Fs.Rename(oldname, newname)
}

// RemoveAll removes a path and all children. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) RemoveAll(path string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] remove -rf %s\n", path)
		return nil
	}
	return os.RemoveAll(path)
}

// OpenFile opens a file. In dry-run mode for write flags, logs and returns a mem file.
func (s *SymlinkFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if s.DryRun && (flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC)) != 0 {
		fmt.Printf("[dry-run] open(write) %s\n", name)
		return afero.NewMemMapFs().OpenFile(name, flag, perm)
	}
	// Reads always go to the real filesystem.
	return s.realFs.OpenFile(name, flag, perm)
}

// Stat returns file info, always from the real filesystem.
func (s *SymlinkFs) Stat(name string) (os.FileInfo, error) {
	return s.realFs.Stat(name)
}

// Open opens a file for reading, always from the real filesystem.
func (s *SymlinkFs) Open(name string) (afero.File, error) {
	return s.realFs.Open(name)
}

// Chmod changes permissions. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Chmod(name string, mode os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] chmod %s %o\n", name, mode)
		return nil
	}
	return s.Fs.Chmod(name, mode)
}

// Chtimes changes timestamps. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if s.DryRun {
		return nil
	}
	return s.Fs.Chtimes(name, atime, mtime)
}

// ReadFile reads a file's contents, always from the real filesystem.
func (s *SymlinkFs) ReadFile(name string) ([]byte, error) {
	return afero.ReadFile(s.realFs, name)
}

// Symlink creates a symbolic link.
func (s *SymlinkFs) Symlink(oldname, newname string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] symlink %s -> %s\n", newname, oldname)
		return nil
	}
	return os.Symlink(oldname, newname)
}

// Readlink reads the target of a symbolic link.
func (s *SymlinkFs) Readlink(name string) (string, error) {
	return os.Readlink(name)
}

// Lstat returns file info without following symlinks.
func (s *SymlinkFs) Lstat(name string) (os.FileInfo, error) {
	return os.Lstat(name)
}

// ForceSymlink removes any existing file/symlink at dst and creates a symlink to src.
func (s *SymlinkFs) ForceSymlink(src, dst string) error {
	if err := s.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if s.DryRun {
		info, err := os.Lstat(dst)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				existing, _ := os.Readlink(dst)
				fmt.Printf("[dry-run] remove symlink %s (was -> %s)\n", dst, existing)
			} else {
				fmt.Printf("[dry-run] remove %s\n", dst)
			}
		}
		fmt.Printf("[dry-run] symlink %s -> %s\n", dst, src)
		return nil
	}
	_ = os.Remove(dst)
	return os.Symlink(src, dst)
}
