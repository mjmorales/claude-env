package cli

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/mjmorales/claude-env/internal/env"
	"github.com/mjmorales/claude-env/internal/usage"
)

var (
	usageAll   bool
	usageSince string
)

var usageCmd = &cobra.Command{
	Use:   "usage [name]",
	Short: "Show token usage and estimated costs",
	Long:  `Display token consumption, estimated costs, and rate limit reference for Claude Code sessions. Parses session data from environment directories.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		since, err := usage.ParseSince(usageSince)
		if err != nil {
			return err
		}

		if usageAll {
			return showAllEnvUsage(mgr, since)
		}

		var envName string
		if len(args) > 0 {
			envName = args[0]
			if _, exists := mgr.Cfg.Environments[envName]; !exists {
				return fmt.Errorf("environment %q not found", envName)
			}
		} else {
			name, _, err := mgr.Current(mustCwd())
			if err != nil {
				return err
			}
			envName = name
		}

		return showEnvUsage(envName, mgr.Paths.EnvDir(envName), since)
	},
}

func init() {
	usageCmd.Flags().BoolVar(&usageAll, "all", false, "show usage for all environments")
	usageCmd.Flags().StringVar(&usageSince, "since", "", "filter by time window (e.g., 24h, 7d, 2026-04-01)")
	rootCmd.AddCommand(usageCmd)
}

func showAllEnvUsage(mgr *env.Manager, since time.Time) error {
	names := make([]string, 0, len(mgr.Cfg.Environments))
	for name := range mgr.Cfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)

	for i, name := range names {
		if i > 0 {
			fmt.Println()
		}
		if err := showEnvUsage(name, mgr.Paths.EnvDir(name), since); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", name, err)
		}
	}
	return nil
}

func showEnvUsage(name, envDir string, since time.Time) error {
	data, err := usage.CollectEnvUsage(envDir, since)
	if err != nil {
		return fmt.Errorf("collect usage for %s: %w", name, err)
	}

	sinceLabel := "all time"
	if usageSince != "" {
		sinceLabel = "since " + usageSince
	}

	fmt.Printf("Environment: %s\n", name)
	fmt.Printf("Period: %s\n\n", sinceLabel)

	if len(data.Models) == 0 {
		fmt.Println("No usage data")
		return nil
	}

	printUsageTable(data)
	printRateLimits(data)
	return nil
}

func printUsageTable(data *usage.EnvUsage) {
	// Sort models for consistent output
	models := make([]string, 0, len(data.Models))
	for m := range data.Models {
		models = append(models, m)
	}
	sort.Strings(models)

	// Column widths
	const (
		modelW  = 30
		tokenW  = 14
		costW   = 12
	)

	header := fmt.Sprintf("%-*s %*s %*s %*s %*s %*s",
		modelW, "Model",
		tokenW, "Input",
		tokenW, "Output",
		tokenW, "Cache Write",
		tokenW, "Cache Read",
		costW, "Cost")
	fmt.Println(header)
	fmt.Println(strings.Repeat("─", len(header)))

	var totalInput, totalOutput, totalCacheCreate, totalCacheRead uint64
	var totalCost float64

	for _, model := range models {
		tokens := data.Models[model]
		cost := usage.EstimateCost(*tokens, model)
		_, known := usage.PricingForModel(model)

		costStr := usage.FormatUSD(cost.Total())
		if !known {
			costStr += " *"
		}

		fmt.Printf("%-*s %*s %*s %*s %*s %*s\n",
			modelW, model,
			tokenW, formatNum(tokens.Input),
			tokenW, formatNum(tokens.Output),
			tokenW, formatNum(tokens.CacheCreate),
			tokenW, formatNum(tokens.CacheRead),
			costW, costStr)

		totalInput += tokens.Input
		totalOutput += tokens.Output
		totalCacheCreate += tokens.CacheCreate
		totalCacheRead += tokens.CacheRead
		totalCost += cost.Total()
	}

	fmt.Println(strings.Repeat("─", len(header)))
	fmt.Printf("%-*s %*s %*s %*s %*s %*s\n",
		modelW, "Total",
		tokenW, formatNum(totalInput),
		tokenW, formatNum(totalOutput),
		tokenW, formatNum(totalCacheCreate),
		tokenW, formatNum(totalCacheRead),
		costW, usage.FormatUSD(totalCost))

	fmt.Printf("\nSessions: %d │ Messages: %d\n", data.Sessions, data.Messages)

	if hasUnknownModels(data) {
		fmt.Println("\n* Cost estimated using Sonnet pricing (unknown model)")
	}
}

func printRateLimits(data *usage.EnvUsage) {
	limits := usage.RateLimits()
	usedTiers := map[string]bool{}
	for model := range data.Models {
		lower := strings.ToLower(model)
		switch {
		case strings.Contains(lower, "opus"):
			usedTiers["Opus"] = true
		case strings.Contains(lower, "sonnet"):
			usedTiers["Sonnet"] = true
		case strings.Contains(lower, "haiku"):
			usedTiers["Haiku"] = true
		}
	}

	if len(usedTiers) == 0 {
		return
	}

	fmt.Println("\nRate Limits (published, per minute):")
	for _, l := range limits {
		if !usedTiers[l.Model] {
			continue
		}
		fmt.Printf("  %-8s Requests: %s │ Input: %s tokens │ Output: %s tokens\n",
			l.Model,
			formatNum(uint64(l.RequestsPerMin)),
			formatNum(uint64(l.InputTokensPerMin)),
			formatNum(uint64(l.OutputTokensPerMin)))
	}
}

func hasUnknownModels(data *usage.EnvUsage) bool {
	for model := range data.Models {
		if _, known := usage.PricingForModel(model); !known {
			return true
		}
	}
	return false
}

// formatNum formats a number with comma separators.
func formatNum(n uint64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}

	var result strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+3])
	}
	return result.String()
}
