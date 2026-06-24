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
			//nolint:errcheck // auth state is advisory in the listing
			info, _ := mgr.AuthStatus(e.Name)
			printEnv(e, source, mgr.Cfg.Global, info)
		}
		return nil
	},
}

func printEnv(e env.EnvInfo, source, global string, auth env.AuthInfo) {
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
	tags = append(tags, authTag(auth))
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

// authTag summarizes an environment's auth state for the listing.
func authTag(auth env.AuthInfo) string {
	if !auth.Authenticated {
		return "no auth"
	}
	if auth.Expired {
		return "auth: expired"
	}
	if auth.SubscriptionType != "" {
		return "auth: " + auth.SubscriptionType
	}
	return "auth"
}

func init() {
	rootCmd.AddCommand(listCmd)
}
