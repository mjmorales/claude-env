package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/symlink"
)

var sharedCmd = &cobra.Command{
	Use:   "shared",
	Short: "Manage shared files between environments",
	Long:  `Add, remove, and list shared pool resources declared for an environment.`,
}

var sharedAddCmd = &cobra.Command{
	Use:   "add <path>",
	Short: "Add a shared resource to an environment",
	Long:  `Declares a pool resource as shared for the environment and reconciles symlinks. The path is relative to the pool directory (e.g. "agents/reviewer.md").`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resource := args[0]

		mgr, paths, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		if err := mgr.SharedAdd(envName, resource); err != nil {
			return fmt.Errorf("add shared resource: %w", err)
		}

		// Reconcile symlinks for the updated environment.
		e := mgr.Cfg.Environments[envName]
		envDir := paths.EnvDir(envName)
		r := symlink.New(paths.PoolDir, envDir, paths.LockFile, newFs())
		if err := r.Reconcile(e.Shared); err != nil {
			return fmt.Errorf("reconcile symlinks: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Added %q to %q shared resources\n", resource, envName)
		return nil
	},
}

var sharedRemoveCmd = &cobra.Command{
	Use:     "remove <path>",
	Aliases: []string{"rm"},
	Short:   "Remove a shared resource from an environment",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		resource := args[0]

		mgr, paths, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		if err := mgr.SharedRemove(envName, resource); err != nil {
			return fmt.Errorf("remove shared resource: %w", err)
		}

		// Reconcile symlinks for the updated environment.
		e := mgr.Cfg.Environments[envName]
		envDir := paths.EnvDir(envName)
		r := symlink.New(paths.PoolDir, envDir, paths.LockFile, newFs())
		if err := r.Reconcile(e.Shared); err != nil {
			return fmt.Errorf("reconcile symlinks: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Removed %q from %q shared resources\n", resource, envName)
		return nil
	},
}

var sharedListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List shared resources for an environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		e, ok := mgr.Cfg.Environments[envName]
		if !ok {
			return fmt.Errorf("environment %q not found", envName)
		}

		if len(e.Shared) == 0 {
			fmt.Fprintf(os.Stderr, "No shared resources for %q\n", envName)
			return nil
		}

		for _, s := range e.Shared {
			fmt.Println(s)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(sharedCmd)

	sharedCmd.AddCommand(sharedAddCmd)
	sharedCmd.AddCommand(sharedRemoveCmd)
	sharedCmd.AddCommand(sharedListCmd)

	sharedAddCmd.Flags().String("env", "", "target environment (default: current)")
	sharedRemoveCmd.Flags().String("env", "", "target environment (default: current)")
	sharedListCmd.Flags().String("env", "", "target environment (default: current)")
}
