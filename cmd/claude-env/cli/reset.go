package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/keychain"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Uninstall claude-env and restore default Claude Code flow",
	Long: `Restores the active environment's credentials to the macOS Keychain,
removes ~/.claude-envs/, and prints shell cleanup instructions.

After running this, Claude Code will use its default credential storage
and CLAUDE_CONFIG_DIR is no longer needed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, paths, err := loadManager()
		if err != nil {
			return err
		}

		// Resolve active env and restore its creds to keychain.
		name, _, err := mgr.Current(mustCwd())
		if err != nil {
			fmt.Fprintf(os.Stderr, "No active environment found, skipping keychain restore.\n")
		} else {
			envDir := mgr.ConfigDir(name)
			data, readErr := mgr.Fs.ReadFile(envDir + "/.claude.json")
			if readErr == nil && len(data) > 0 {
				if writeErr := keychain.Write(data); writeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not restore keychain: %v\n", writeErr)
					fmt.Fprintf(os.Stderr, "Your credentials are still in %s/.claude.json\n", envDir)
				} else {
					fmt.Fprintf(os.Stderr, "Restored %q credentials to macOS Keychain.\n", name)
				}
			}
		}

		// Remove ~/.claude-envs/
		if err := os.RemoveAll(paths.EnvsDir); err != nil {
			return fmt.Errorf("remove %s: %w", paths.EnvsDir, err)
		}
		fmt.Fprintf(os.Stderr, "Removed %s\n", paths.EnvsDir)

		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Cleanup complete. To finish:")
		fmt.Fprintln(os.Stderr, "  1. Remove 'eval \"$(claude-env shell-init)\"' from your shell profile")
		fmt.Fprintln(os.Stderr, "  2. Restart your shell")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(resetCmd)
}
