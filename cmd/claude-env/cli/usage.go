//nolint:revive // magic numbers are clear in formatting context
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
	Long:  `Display token consumption, estimated costs, and rate limit status for Claude Code sessions. Parses session data from environment directories.`,
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, _, err := loadManager()
		if err != nil {
			return fmt.Errorf("load manager: %w", err)
		}

		since, err := usage.ParseSince(usageSince)
		if err != nil {
			return fmt.Errorf("parse --since: %w", err)
		}

		if usageAll {
			return showAllEnvUsage(mgr, since)
		}

		envName, err := resolveEnvName(mgr, args)
		if err != nil {
			return err
		}

		return showEnvUsage(envName, mgr.Paths.EnvDir(envName), since)
	},
}

func resolveEnvName(mgr *env.Manager, args []string) (string, error) {
	if len(args) > 0 {
		name := args[0]
		if _, exists := mgr.Cfg.Environments[name]; !exists {
			return "", fmt.Errorf("environment %q not found", name)
		}
		return name, nil
	}

	name, _, err := mgr.Current(mustCwd())
	if err != nil {
		return "", fmt.Errorf("resolve current environment: %w", err)
	}
	return name, nil
}

func init() {
	usageCmd.Flags().BoolVar(&usageAll, "all", false, "show usage for all environments")
	usageCmd.Flags().StringVar(&usageSince, "since", "", "filter by time window (e.g., 24h, 7d, 2026-04-01)")
	rootCmd.AddCommand(usageCmd)
}

const rateWindowMinutes = 5

func showAllEnvUsage(mgr *env.Manager, since time.Time) error {
	names := sortedEnvNames(mgr)

	for i, name := range names {
		if i > 0 {
			fmt.Println()
		}
		if err := showEnvUsage(name, mgr.Paths.EnvDir(name), since); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %s: %v\n", name, err)
		}
	}

	printRateComparison(mgr, names)
	return nil
}

func sortedEnvNames(mgr *env.Manager) []string {
	names := make([]string, 0, len(mgr.Cfg.Environments))
	for name := range mgr.Cfg.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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
	printRateStatus(envDir)
	return nil
}

func printUsageTable(data *usage.EnvUsage) {
	models := activeModels(data)
	if len(models) == 0 {
		return
	}

	const (
		modelW = 30
		tokenW = 14
		costW  = 12
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

func activeModels(data *usage.EnvUsage) []string {
	models := make([]string, 0, len(data.Models))
	for m, tokens := range data.Models {
		if tokens.Total() > 0 {
			models = append(models, m)
		}
	}
	sort.Strings(models)
	return models
}

func hasUnknownModels(data *usage.EnvUsage) bool {
	for model, tokens := range data.Models {
		if tokens.Total() > 0 {
			if _, known := usage.PricingForModel(model); !known {
				return true
			}
		}
	}
	return false
}

func printRateStatus(envDir string) {
	entries, err := usage.CollectRecentMessages(envDir, time.Duration(rateWindowMinutes)*time.Minute)
	if err != nil || len(entries) == 0 {
		return
	}

	statuses := usage.ComputeRateStatus(entries, rateWindowMinutes)
	if len(statuses) == 0 {
		return
	}

	fmt.Printf("\nRate Limit Status (last %dm):\n", rateWindowMinutes)
	fmt.Printf("  %-10s %12s %12s %10s   %s\n", "", "Output Rate", "Limit", "Headroom", "Status")

	for _, s := range statuses {
		bar := renderBar(s.HeadroomPct)
		statusStr := s.Status
		if s.Status == usage.StatusCaution && s.MinutesToLimit > 0 {
			statusStr = fmt.Sprintf("~%.0fm until throttled", s.MinutesToLimit)
		}

		fmt.Printf("  %-10s %10sK/m %10sK/m %9.0f%%   %s %s\n",
			s.Tier,
			formatNum(uint64(s.OutputRate/1000)),  //nolint:gosec // rate values are always small
			formatNum(uint64(s.OutputLimit/1000)), //nolint:gosec // limit values are always small
			s.HeadroomPct,
			bar,
			statusStr)
	}
}

func printRateComparison(mgr *env.Manager, names []string) {
	rates := collectEnvRates(mgr, names)
	bestIdx := findBestProfile(rates)

	fmt.Printf("\n\nProfile Comparison (last %dm):\n", rateWindowMinutes)
	fmt.Printf("  %-15s %10s   %s\n", "Profile", "Headroom", "Status")
	fmt.Printf("  %s\n", strings.Repeat("─", 50))

	for i, r := range rates {
		bar := renderBar(r.headroom)
		suffix := ""
		if i == bestIdx && len(rates) > 1 {
			suffix = " ← best"
		}
		fmt.Printf("  %-15s %9.0f%%   %s %s%s\n",
			r.name, r.headroom, bar, r.status, suffix)
	}
}

type envRate struct {
	name     string
	headroom float64
	status   string
}

func collectEnvRates(mgr *env.Manager, names []string) []envRate {
	rates := make([]envRate, 0, len(names))
	for _, name := range names {
		r := envRate{name: name, headroom: 100, status: usage.StatusReady}
		entries, err := usage.CollectRecentMessages(
			mgr.Paths.EnvDir(name),
			time.Duration(rateWindowMinutes)*time.Minute,
		)
		if err == nil && len(entries) > 0 {
			for _, s := range usage.ComputeRateStatus(entries, rateWindowMinutes) {
				if s.HeadroomPct < r.headroom {
					r.headroom = s.HeadroomPct
					r.status = s.Status
				}
			}
		}
		rates = append(rates, r)
	}
	return rates
}

func findBestProfile(rates []envRate) int {
	bestIdx := 0
	bestHeadroom := -1.0
	for i, r := range rates {
		if r.headroom > bestHeadroom {
			bestHeadroom = r.headroom
			bestIdx = i
		}
	}
	return bestIdx
}

const barLen = 10

func renderBar(headroomPct float64) string {
	filled := int((100 - headroomPct) / 100 * barLen)
	filled = max(0, min(filled, barLen))
	return strings.Repeat("█", filled) + strings.Repeat("░", barLen-filled)
}

const commaGroupSize = 3

// formatNum formats a number with comma separators.
func formatNum(n uint64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) <= commaGroupSize {
		return s
	}

	var result strings.Builder
	remainder := len(s) % commaGroupSize
	if remainder > 0 {
		result.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += commaGroupSize {
		if result.Len() > 0 {
			result.WriteByte(',')
		}
		result.WriteString(s[i : i+commaGroupSize])
	}
	return result.String()
}
