package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var useCmd = &cobra.Command{
	Use:     "use <name>",
	Aliases: []string{"global"},
	Short:   "Switch the global environment",
	Long:    `Swaps the credential symlink at ~/.claude/.credentials.json and reconciles shared resources.`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, paths, err := loadManager()
		if err != nil {
			return err
		}

		if err := mgr.Use(name); err != nil {
			return err
		}

		reconcileShared(mgr, paths)
		fmt.Fprintf(os.Stderr, "Switched to %q\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
