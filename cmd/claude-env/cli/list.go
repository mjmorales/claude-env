package cli

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/env"
)

var listCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List all environments",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return err
		}

		envs := mgr.List(mustCwd())
		sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })

		for _, e := range envs {
			printEnv(e)
		}
		return nil
	},
}

func printEnv(e env.EnvInfo) {
	marker := "  "
	if e.Active {
		marker = "* "
	}
	fmt.Printf("%s%s", marker, e.Name)
	if len(e.Shared) > 0 {
		fmt.Printf(" (shared: %d)", len(e.Shared))
	}
	fmt.Println()
}

func init() {
	rootCmd.AddCommand(listCmd)
}
