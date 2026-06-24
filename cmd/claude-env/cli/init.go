package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize claude-env and adopt the current Claude Code login",
	Long:  `Creates ~/.claude-envs/ and a "default" environment, adopting the current Claude Code login (~/.claude's OAuth token) into default/.credentials.json when present. Sets "default" as the global active environment.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		if err := mgr.Init(); err != nil {
			return fmt.Errorf("initialize: %w", err)
		}

		fmt.Fprintln(os.Stderr, "Initialized. Active environment: default")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
