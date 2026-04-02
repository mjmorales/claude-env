package symlink

// Reconciler manages symlinks between the pool and ~/.claude/.
type Reconciler struct {
	PoolDir   string
	TargetDir string
}

// New creates a symlink reconciler.
func New(poolDir, targetDir string) *Reconciler {
	return &Reconciler{
		PoolDir:   poolDir,
		TargetDir: targetDir,
	}
}
