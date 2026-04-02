package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize claude-env and adopt existing credentials",
	Long:  `Creates ~/.claude-envs/, adopts any existing ~/.claude/.credentials.json as the "default" environment, and sets up the symlink.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return err
		}

		if err := mgr.Init(); err != nil {
			return err
		}

		fmt.Fprintln(os.Stderr, "Initialized. Active environment: default")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
