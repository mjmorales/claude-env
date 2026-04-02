package cli

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/spf13/cobra"
)

var execCmd = &cobra.Command{
	Use:   "exec -- <command> [args...]",
	Short: "Run a command with CLAUDE_CONFIG_DIR set",
	Long: `Resolves the active environment, sets CLAUDE_CONFIG_DIR, and exec's
the given command. Useful as: claude-env exec -- claude <args>`,
	DisableFlagParsing: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return fmt.Errorf("usage: claude-env exec -- <command> [args...]")
		}

		// Strip leading "--" if present.
		if args[0] == "--" {
			args = args[1:]
		}
		if len(args) == 0 {
			return fmt.Errorf("usage: claude-env exec -- <command> [args...]")
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

		env := append(os.Environ(), "CLAUDE_CONFIG_DIR="+envDir)
		return syscall.Exec(binary, args, env)
	},
}

func init() {
	rootCmd.AddCommand(execCmd)
}
