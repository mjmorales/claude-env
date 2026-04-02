package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Show the active environment",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		name, source, err := mgr.Current(mustCwd())
		if err != nil {
			return fmt.Errorf("get current environment: %w", err)
		}

		fmt.Printf("%s (%s)\n", name, source)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(currentCmd)
}
