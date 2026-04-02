package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configDirCmd = &cobra.Command{
	Use:    "config-dir",
	Short:  "Print the CLAUDE_CONFIG_DIR for the active environment",
	Hidden: true, // Used by shell-init, not for direct user invocation.
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return err
		}

		name, _, err := mgr.Current(mustCwd())
		if err != nil {
			return err
		}

		fmt.Print(mgr.ConfigDir(name))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(configDirCmd)
}
