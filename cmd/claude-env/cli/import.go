package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import <name>",
	Short: "Import an OAuth token into an environment",
	Long: `Stores an OAuth credential for an environment as its .credentials.json,
without the interactive browser login. Sources:

  # paste a full {"claudeAiOauth":{...}} blob or a bare sk-ant-* token on stdin
  claude-env import work < token.json
  pbpaste | claude-env import work

  # copy the token from another environment
  claude-env import work --from-env default

  # capture a long-lived token via 'claude setup-token' (inference-only)
  claude-env import work --setup-token`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]

		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		fromEnv, err := cmd.Flags().GetString("from-env")
		if err != nil {
			return fmt.Errorf("read --from-env: %w", err)
		}
		setupToken, err := cmd.Flags().GetBool("setup-token")
		if err != nil {
			return fmt.Errorf("read --setup-token: %w", err)
		}

		if setupToken && fromEnv != "" {
			return fmt.Errorf("--setup-token and --from-env are mutually exclusive")
		}

		switch {
		case setupToken:
			if err := mgr.ImportSetupToken(name); err != nil {
				return fmt.Errorf("import setup-token: %w", err)
			}
		case fromEnv != "":
			if err := mgr.ImportFromEnv(name, fromEnv); err != nil {
				return fmt.Errorf("import from env: %w", err)
			}
		default:
			data, readErr := io.ReadAll(os.Stdin)
			if readErr != nil {
				return fmt.Errorf("read stdin: %w", readErr)
			}
			if len(data) == 0 {
				return fmt.Errorf("no input on stdin (pass a credential blob or sk-ant-* token)")
			}
			if err := mgr.Import(name, data); err != nil {
				return fmt.Errorf("import: %w", err)
			}
		}

		fmt.Fprintf(os.Stderr, "Imported credentials into environment %q.\n", name)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(importCmd)
	importCmd.Flags().String("from-env", "", "copy the token from another environment")
	importCmd.Flags().Bool("setup-token", false, "capture a long-lived token via 'claude setup-token'")
}
