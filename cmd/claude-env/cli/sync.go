package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var syncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync marketplace plugin paths for the current environment",
	Long: `Rewrites installLocation entries in known_marketplaces.json so they point
to the active environment's plugin directory. This fixes path mismatches
caused by plugins being installed or updated from a different environment.

Use --dry-run to preview changes without writing.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		envName, err := resolveEnvFlag(mgr, cmd)
		if err != nil {
			return err
		}

		envDir := mgr.ConfigDir(envName)
		rewrites, err := mgr.SyncMarketplacePaths(envDir)
		if err != nil {
			return fmt.Errorf("sync marketplace paths: %w", err)
		}

		if len(rewrites) == 0 {
			fmt.Fprintf(os.Stderr, "All marketplace paths already point to %q. Nothing to do.\n", envName)
			return nil
		}

		for _, r := range rewrites {
			fmt.Fprintf(os.Stderr, "  %s:\n    - %s\n    + %s\n", r.Plugin, r.OldPath, r.NewPath)
		}

		action := "Synced"
		if dryRun {
			action = "Would sync"
		}
		fmt.Fprintf(os.Stderr, "%s %d plugin path(s) for %q.\n", action, len(rewrites), envName)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(syncCmd)
	syncCmd.Flags().String("env", "", "target environment (default: current)")
}
