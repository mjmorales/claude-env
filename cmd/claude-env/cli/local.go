package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var localCmd = &cobra.Command{
	Use:   "local <name>",
	Short: "Pin an environment to the current directory",
	Long:  `Writes a .claude-env file in the current directory to pin this environment. Claude-env resolves local pins by walking up the directory tree.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		dir := mustCwd()
		if err := mgr.Local(name, dir); err != nil {
			return fmt.Errorf("pin environment: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Pinned %q to %s\n", name, dir)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(localCmd)
}
