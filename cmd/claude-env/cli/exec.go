package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:                "exec <command> [args...]",
	Short:              "Run a command with CLAUDE_CONFIG_DIR set to the active environment",
	Long:               `Resolves the active environment, sets CLAUDE_CONFIG_DIR, and replaces the current process with the given command. Like pyenv exec, this is useful for running claude or any other tool under the active environment.`,
	DisableFlagParsing: true,
	SilenceUsage:       true,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Strip leading "--" if present for backwards compat.
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("usage: claude-env exec <command> [args...]")
		}

		mgr, _, err := loadManager()
		if err != nil {
			return err
		}

		name, _, err := mgr.Current(mustCwd())
		if err != nil {
			return err
		}

		envDir := mgr.ConfigDir(name)
		binary, err := exec.LookPath(args[0])
		if err != nil {
			return fmt.Errorf("command not found: %s", args[0])
		}

		environ := append(os.Environ(), "CLAUDE_CONFIG_DIR="+envDir)
		return syscall.Exec(binary, args, environ)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
