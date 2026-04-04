package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/config"
	"github.com/mjmorales/claude-env/internal/env"
	"github.com/mjmorales/claude-env/internal/fsutil"
)

var (
	cfgFile string
	dryRun  bool
)

// rootCmd is the root command.
var rootCmd = &cobra.Command{
	Use:   "claude-env",
	Short: "Manage multiple Claude Code subscriptions",
	Long:  `claude-env manages multiple Claude Code OAuth sessions with easy swapping and declarative shared state (agents, skills, commands, plugins).`,
}

// Execute runs the CLI.
func Execute() error {
	if err := rootCmd.Execute(); err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	return nil
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
		return nil, paths, fmt.Errorf("resolve default paths: %w", err)
	}

	if cfgFile != "" {
		paths.ConfigFile = cfgFile
	}

	cfg, err := config.Load(paths.ConfigFile)
	if err != nil {
		return nil, paths, fmt.Errorf("load config: %w", err)
	}

	return env.New(paths, cfg, newFs()), paths, nil
}

func mustCwd() string {
	dir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot determine working directory: %v\n", err)
		//nolint:revive // os.Exit in helper is acceptable for CLI tools
		os.Exit(1)
	}
	return dir
}

// resolveEnvFlag returns the environment name from --env flag, falling back to
// the current active environment.
func resolveEnvFlag(mgr *env.Manager, cmd *cobra.Command) (string, error) {
	envFlag, err := cmd.Flags().GetString("env")
	if err == nil && envFlag != "" {
		return envFlag, nil
	}
	name, _, err := mgr.Current(mustCwd())
	if err != nil {
		return "", fmt.Errorf("resolve current environment: %w", err)
	}
	return name, nil
}
