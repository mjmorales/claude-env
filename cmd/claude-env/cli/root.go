package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/env"
	"github.com/mjmorales/claude-env/internal/fsutil"
	"github.com/mjmorales/claude-env/internal/symlink"
)

var (
	cfgFile string
	dryRun  bool
)

var rootCmd = &cobra.Command{
	Use:   "claude-env",
	Short: "Manage multiple Claude Code subscriptions",
	Long:  `claude-env manages multiple Claude Code OAuth sessions with easy swapping and declarative shared state (agents, skills, commands, plugins).`,
}

func Execute() error {
	return rootCmd.Execute()
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgFile, "config", "", "config file (default: ~/.claude-envs/config.toml)")
	rootCmd.PersistentFlags().BoolVar(&dryRun, "dry-run", false, "preview changes without modifying the filesystem")
}

func newFs() *fsutil.SymlinkFs {
	return fsutil.NewOs(dryRun)
}

// loadManager loads config and creates a Manager with the appropriate filesystem.
func loadManager() (*env.Manager, config.Paths, error) {
	paths, err := config.DefaultPaths()
	if err != nil {
		return nil, paths, err
	}

	if cfgFile != "" {
		paths.ConfigFile = cfgFile
	}

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return nil, paths, err
	}

	return env.New(paths, cfg, newFs()), paths, nil
}

// reconcileShared runs symlink reconciliation for the active environment.
// Shared resources from the pool are symlinked into the env's config dir.
func reconcileShared(mgr *env.Manager, paths config.Paths) {
	name, _, err := mgr.Current(mustCwd())
	if err != nil {
		return
	}
	e, ok := mgr.Cfg.Environments[name]
	if !ok || len(e.Shared) == 0 {
		return
	}

	envDir := paths.EnvDir(name)
	r := symlink.New(paths.PoolDir, envDir, paths.LockFile, newFs())
	if err := r.Reconcile(e.Shared); err != nil {
		fmt.Fprintf(os.Stderr, "warning: symlink reconciliation failed: %v\n", err)
	}
}

func mustCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine working directory: %v\n", err)
		os.Exit(1)
	}
	return dir
}
