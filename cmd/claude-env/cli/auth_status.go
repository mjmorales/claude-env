package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var authStatusCmd = &cobra.Command{
	Use:   "auth-status [name]",
	Short: "Show auth status for an environment",
	Long:  `Runs 'claude auth status' with CLAUDE_CONFIG_DIR pointed at the named environment. If no name given, uses the current environment.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		var name string
		if len(args) == 1 {
			name = args[0]
		} else {
			name, _, err = mgr.Current(mustCwd())
			if err != nil {
				return fmt.Errorf("get current environment: %w", err)
			}
		}

		out, err := mgr.AuthStatus(name)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		fmt.Fprint(os.Stdout, out)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(authStatusCmd)
}
