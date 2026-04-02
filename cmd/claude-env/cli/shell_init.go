package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var shellInitCmd = &cobra.Command{
	Use:   "shell-init",
	Short: "Output shell function to wrap claude with env resolution",
	Long: `Prints a shell function that wraps the 'claude' command to automatically
set CLAUDE_CONFIG_DIR based on the active claude-env environment.

Add to your shell profile:
  eval "$(claude-env shell-init)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Print(shellFunction)
		return nil
	},
}

const shellFunction = `# claude-env: automatic environment resolution for Claude Code
claude() {
  local env_dir
  env_dir="$(command claude-env config-dir 2>/dev/null)"
  if [ -n "$env_dir" ]; then
    CLAUDE_CONFIG_DIR="$env_dir" command claude "$@"
  else
    command claude "$@"
  fi
}
`

func init() {
	rootCmd.AddCommand(shellInitCmd)
}
