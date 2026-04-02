package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var addCmd = &cobra.Command{
	Use:   "add <name>",
	Short: "Add a new environment",
	Long:  `Registers a new environment with an empty credential file. Log in to Claude Code while this environment is active to populate it.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		if err := mgr.Add(name); err != nil {
			return fmt.Errorf("add environment: %w", err)
		}

		fmt.Fprintf(os.Stderr, "Added environment %q. Switch to it with: claude-env use %s\n", name, name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(addCmd)
}
