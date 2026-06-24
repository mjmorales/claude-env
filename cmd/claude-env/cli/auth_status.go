package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/env"
)

var authStatusCmd = &cobra.Command{
	Use:   "auth-status [name]",
	Short: "Show auth status for an environment",
	Long:  `Reports whether the named environment has a stored OAuth credential, its subscription type, and token expiry. Read natively from the environment's .credentials.json — no network call. If no name is given, uses the current environment.`,
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

		info, err := mgr.AuthStatus(name)
		if err != nil {
			return fmt.Errorf("auth status: %w", err)
		}
		fmt.Fprint(os.Stdout, formatAuthStatus(name, info))
		return nil
	},
}

// formatAuthStatus renders an environment's authentication state for humans.
func formatAuthStatus(name string, info env.AuthInfo) string {
	if !info.Authenticated {
		return fmt.Sprintf("Environment: %s\nAuthenticated: no\nRun 'claude-env login %s' to authenticate.\n", name, name)
	}

	s := fmt.Sprintf("Environment: %s\nAuthenticated: yes\n", name)
	if info.Email != "" {
		s += fmt.Sprintf("Account: %s\n", info.Email)
	}
	if info.SubscriptionType != "" {
		s += fmt.Sprintf("Subscription: %s\n", info.SubscriptionType)
	}
	s += "Token: " + formatExpiry(info) + "\n"
	return s
}

// formatExpiry describes token expiry, or notes its absence.
func formatExpiry(info env.AuthInfo) string {
	if info.ExpiresAt == 0 {
		return "no expiry recorded"
	}
	when := time.UnixMilli(info.ExpiresAt).Format(time.RFC3339)
	if info.Expired {
		return fmt.Sprintf("EXPIRED at %s (refreshes on next claude run)", when)
	}
	return fmt.Sprintf("valid, expires %s (in %s)", when, info.ExpiresIn.Round(time.Minute))
}

func init() {
	rootCmd.AddCommand(authStatusCmd)
}
