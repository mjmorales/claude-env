package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/symlink"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show environment status and symlink health",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, paths, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		name, source, err := mgr.Current(mustCwd())
		if err != nil {
			return fmt.Errorf("get current environment: %w", err)
		}
		fmt.Printf("Active: %s (%s)\n", name, source)
		fmt.Printf("Config: %s\n", mgr.ConfigDir(name))

		//nolint:errcheck // auth state is advisory in status output
		info, _ := mgr.AuthStatus(name)
		fmt.Printf("Auth:   %s\n", authTag(info))

		e, ok := mgr.Cfg.Environments[name]
		if !ok {
			return nil
		}

		if len(e.Shared) == 0 {
			fmt.Println("Shared: (none)")
			return nil
		}

		envDir := paths.EnvDir(name)
		r := symlink.New(paths.PoolDir, envDir, paths.LockFile, newFs())
		statuses, err := r.Status()
		if err != nil {
			return fmt.Errorf("check symlink status: %w", err)
		}

		fmt.Println("Shared resources:")
		for _, s := range statuses {
			fmt.Printf("  %s: %s\n", s.Name, s.State)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
