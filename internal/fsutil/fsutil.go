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
	DryRun bool
	realFs afero.Fs // always OsFs, used for reads in dry-run mode
}

// New creates a SymlinkFs wrapping the given afero filesystem.
func New(fs afero.Fs, dryRun bool) *SymlinkFs {
	return &SymlinkFs{Fs: fs, DryRun: dryRun, realFs: afero.NewOsFs()}
}

// NewOs creates a SymlinkFs backed by the real OS filesystem.
// In dry-run mode, writes are logged but not executed; reads still work.
// nolint:revive
func NewOs(dryRun bool) *SymlinkFs {
	osFS := afero.NewOsFs()
	if dryRun {
		return &SymlinkFs{Fs: afero.NewReadOnlyFs(osFS), DryRun: true, realFs: osFS}
	}
	return &SymlinkFs{Fs: osFS, DryRun: false, realFs: osFS}
}

// MkdirAll creates directories. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) MkdirAll(path string, perm os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] mkdir -p %s\n", path)
		return nil
	}
	if err := s.Fs.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}
	return nil
}

// Create creates a file. In dry-run mode, logs and returns a no-op file.
//
//nolint:ireturn // returning afero.File interface is acceptable
func (s *SymlinkFs) Create(name string) (afero.File, error) {
	if s.DryRun {
		fmt.Printf("[dry-run] create %s\n", name)
		f, err := afero.NewMemMapFs().Create(name)
		if err != nil {
			return nil, fmt.Errorf("create (dry-run): %w", err)
		}
		return f, nil
	}
	f, err := s.Fs.Create(name)
	if err != nil {
		return nil, fmt.Errorf("create: %w", err)
	}
	return f, nil
}

// WriteFile writes data to a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) WriteFile(path string, data []byte, perm os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] write %s (%d bytes)\n", path, len(data))
		return nil
	}
	if err := afero.WriteFile(s.Fs, path, data, perm); err != nil {
		return fmt.Errorf("writefile: %w", err)
	}
	return nil
}

// Remove removes a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Remove(name string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] remove %s\n", name)
		return nil
	}
	if err := s.Fs.Remove(name); err != nil {
		return fmt.Errorf("remove: %w", err)
	}
	return nil
}

// Rename moves a file. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Rename(oldname, newname string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] move %s -> %s\n", oldname, newname)
		return nil
	}
	if err := s.Fs.Rename(oldname, newname); err != nil {
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// RemoveAll removes a path and all children. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) RemoveAll(path string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] remove -rf %s\n", path)
		return nil
	}
	if err := os.RemoveAll(path); err != nil {
		return fmt.Errorf("removeall: %w", err)
	}
	return nil
}

// OpenFile opens a file. In dry-run mode for write flags, logs and returns a mem file.
//
//nolint:ireturn // returning afero.File interface is acceptable
func (s *SymlinkFs) OpenFile(name string, flag int, perm os.FileMode) (afero.File, error) {
	if s.DryRun && (flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC)) != 0 {
		fmt.Printf("[dry-run] open(write) %s\n", name)
		f, err := afero.NewMemMapFs().OpenFile(name, flag, perm)
		if err != nil {
			return nil, fmt.Errorf("openfile (dry-run): %w", err)
		}
		return f, nil
	}
	// Reads always go to the real filesystem.
	f, err := s.realFs.OpenFile(name, flag, perm)
	if err != nil {
		return nil, fmt.Errorf("openfile: %w", err)
	}
	return f, nil
}

// Stat returns file info, always from the real filesystem.
func (s *SymlinkFs) Stat(name string) (os.FileInfo, error) {
	info, err := s.realFs.Stat(name)
	if err != nil {
		return nil, fmt.Errorf("stat: %w", err)
	}
	return info, nil
}

// Open opens a file for reading, always from the real filesystem.
//
//nolint:ireturn // returning afero.File interface is acceptable
func (s *SymlinkFs) Open(name string) (afero.File, error) {
	f, err := s.realFs.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	return f, nil
}

// Chmod changes permissions. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Chmod(name string, mode os.FileMode) error {
	if s.DryRun {
		fmt.Printf("[dry-run] chmod %s %o\n", name, mode)
		return nil
	}
	if err := s.Fs.Chmod(name, mode); err != nil {
		return fmt.Errorf("chmod: %w", err)
	}
	return nil
}

// Chtimes changes timestamps. In dry-run mode, logs and succeeds.
func (s *SymlinkFs) Chtimes(name string, atime time.Time, mtime time.Time) error {
	if s.DryRun {
		return nil
	}
	if err := s.Fs.Chtimes(name, atime, mtime); err != nil {
		return fmt.Errorf("chtimes: %w", err)
	}
	return nil
}

// ReadFile reads a file's contents, always from the real filesystem.
func (s *SymlinkFs) ReadFile(name string) ([]byte, error) {
	data, err := afero.ReadFile(s.realFs, name)
	if err != nil {
		return nil, fmt.Errorf("readfile: %w", err)
	}
	return data, nil
}

// Symlink creates a symbolic link.
func (s *SymlinkFs) Symlink(oldname, newname string) error {
	if s.DryRun {
		fmt.Printf("[dry-run] symlink %s -> %s\n", newname, oldname)
		return nil
	}
	if err := os.Symlink(oldname, newname); err != nil {
		return fmt.Errorf("symlink: %w", err)
	}
	return nil
}

// Readlink reads the target of a symbolic link.
func (s *SymlinkFs) Readlink(name string) (string, error) {
	target, err := os.Readlink(name)
	if err != nil {
		return "", fmt.Errorf("readlink: %w", err)
	}
	return target, nil
}

// Lstat returns file info without following symlinks.
func (s *SymlinkFs) Lstat(name string) (os.FileInfo, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, fmt.Errorf("lstat: %w", err)
	}
	return info, nil
}

// ForceSymlink removes any existing file/symlink at dst and creates a symlink to src.
//
//nolint:nestif,revive // acceptable nesting for fsops and magic numbers
func (s *SymlinkFs) ForceSymlink(src, dst string) error {
	if err := s.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if s.DryRun {
		info, err := os.Lstat(dst)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				//nolint:errcheck
				existing, _ := os.Readlink(dst)
				fmt.Printf("[dry-run] remove symlink %s (was -> %s)\n", dst, existing)
			} else {
				fmt.Printf("[dry-run] remove %s\n", dst)
			}
		}
		fmt.Printf("[dry-run] symlink %s -> %s\n", dst, src)
		return nil
	}
	if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove existing: %w", err)
	}
	if err := os.Symlink(src, dst); err != nil {
		return fmt.Errorf("force symlink: %w", err)
	}
	return nil
}
