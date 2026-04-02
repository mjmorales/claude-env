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
	Long:    `Sets the global active environment. The claude shim will use this to set CLAUDE_CONFIG_DIR.`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		if err := mgr.Use(name); err != nil {
			return fmt.Errorf("switch environment: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Switched to %q (CLAUDE_CONFIG_DIR=%s)\n", name, mgr.ConfigDir(name))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(useCmd)
}
