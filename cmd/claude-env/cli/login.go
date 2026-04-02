package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login [name]",
	Short: "Authenticate a Claude Code environment",
	Long: `Runs 'claude auth login' with CLAUDE_CONFIG_DIR pointed at the named
environment's directory. If no name given, uses the current environment.`,
	Args: cobra.MaximumNArgs(1),
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

		if err := mgr.Login(name); err != nil {
			return fmt.Errorf("login: %w", err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loginCmd)
}
