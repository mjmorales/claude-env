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
			return fmt.Errorf("load manager: %w", err)
		}

		dir := mustCwd()
		_, source, err := mgr.Current(dir)
		if err != nil {
			return fmt.Errorf("get current environment: %w", err)
		}
		envs := mgr.List(dir)
		sort.Slice(envs, func(i, j int) bool { return envs[i].Name < envs[j].Name })

		for _, e := range envs {
			printEnv(e, source, mgr.Cfg.Global)
		}
		return nil
	},
}

func printEnv(e env.EnvInfo, source, global string) {
	marker := "  "
	if e.Active {
		marker = "* "
	}
	fmt.Printf("%s%s", marker, e.Name)

	var tags []string
	if e.Name == global {
		tags = append(tags, "global")
	}
	if e.Active && source != "global" {
		tags = append(tags, "local")
	}
	if len(e.Shared) > 0 {
		tags = append(tags, fmt.Sprintf("shared: %d", len(e.Shared)))
	}
	if len(tags) > 0 {
		fmt.Printf(" (")
		for i, tag := range tags {
			if i > 0 {
				fmt.Printf(", ")
			}
			fmt.Print(tag)
		}
		fmt.Printf(")")
	}
	fmt.Println()
}

func init() {
	rootCmd.AddCommand(listCmd)
}
