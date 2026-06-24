package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/credentials"
)

var resetCmd = &cobra.Command{
	Use:   "reset",
	Short: "Uninstall claude-env and restore default Claude Code flow",
	Long: `Restores the active environment's OAuth token to the default Claude Code
location (~/.claude/.credentials.json), removes ~/.claude-envs/, and prints
shell cleanup instructions.

After running this, Claude Code authenticates from ~/.claude and
CLAUDE_CONFIG_DIR is no longer needed.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, paths, err := loadManager()
		if err != nil {
			return err
		}

		// Resolve active env and restore its token to the default location.
		name, _, err := mgr.Current(mustCwd())
		if err != nil {
			fmt.Fprintf(os.Stderr, "No active environment found, skipping credential restore.\n")
		} else {
			envDir := mgr.ConfigDir(name)
			data, readErr := credentials.ReadRaw(mgr.Fs, envDir)
			if readErr == nil && len(data) > 0 {
				if writeErr := credentials.WriteRaw(mgr.Fs, paths.ClaudeDir, data); writeErr != nil {
					fmt.Fprintf(os.Stderr, "Warning: could not restore default credentials: %v\n", writeErr)
					fmt.Fprintf(os.Stderr, "Your token is still in %s\n", credentials.Path(envDir))
				} else {
					fmt.Fprintf(os.Stderr, "Restored %q token to %s\n", name, credentials.Path(paths.ClaudeDir))
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
