package usage

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ModelPricing holds per-million-token costs for a model family.
type ModelPricing struct {
	InputPerMillion       float64
	OutputPerMillion      float64
	CacheCreatePerMillion float64
	CacheReadPerMillion   float64
}

var (
	pricingOpus = ModelPricing{
		InputPerMillion:       15.0,
		OutputPerMillion:      75.0,
		CacheCreatePerMillion: 18.75,
		CacheReadPerMillion:   1.5,
	}
	pricingSonnet = ModelPricing{
		InputPerMillion:       15.0,
		OutputPerMillion:      75.0,
		CacheCreatePerMillion: 18.75,
		CacheReadPerMillion:   1.5,
	}
	pricingHaiku = ModelPricing{
		InputPerMillion:       1.0,
		OutputPerMillion:      5.0,
		CacheCreatePerMillion: 1.25,
		CacheReadPerMillion:   0.1,
	}
)

// PricingForModel returns pricing for the given model name.
// Returns Sonnet pricing as fallback and false if the model is unknown.
func PricingForModel(model string) (ModelPricing, bool) {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return pricingOpus, true
	case strings.Contains(lower, "haiku"):
		return pricingHaiku, true
	case strings.Contains(lower, "sonnet"):
		return pricingSonnet, true
	default:
		return pricingSonnet, false
	}
}

// ModelTokens holds token counts for a single model.
type ModelTokens struct {
	Input       uint64
	Output      uint64
	CacheCreate uint64
	CacheRead   uint64
	Messages    uint64
}

// Total returns the sum of all token buckets.
func (m ModelTokens) Total() uint64 {
	return m.Input + m.Output + m.CacheCreate + m.CacheRead
}

// CostEstimate holds estimated USD costs broken down by token type.
type CostEstimate struct {
	InputUSD       float64
	OutputUSD      float64
	CacheCreateUSD float64
	CacheReadUSD   float64
}

// Total returns the sum of all cost buckets.
func (c CostEstimate) Total() float64 {
	return c.InputUSD + c.OutputUSD + c.CacheCreateUSD + c.CacheReadUSD
}

// EstimateCost computes the estimated USD cost for the given token counts and model.
func EstimateCost(tokens ModelTokens, model string) CostEstimate {
	pricing, _ := PricingForModel(model)
	return CostEstimate{
		InputUSD:       float64(tokens.Input) / 1_000_000.0 * pricing.InputPerMillion,
		OutputUSD:      float64(tokens.Output) / 1_000_000.0 * pricing.OutputPerMillion,
		CacheCreateUSD: float64(tokens.CacheCreate) / 1_000_000.0 * pricing.CacheCreatePerMillion,
		CacheReadUSD:   float64(tokens.CacheRead) / 1_000_000.0 * pricing.CacheReadPerMillion,
	}
}

// FormatUSD formats a dollar amount to 4 decimal places.
func FormatUSD(amount float64) string {
	return fmt.Sprintf("$%.4f", amount)
}

// EnvUsage holds aggregated usage data for one environment.
type EnvUsage struct {
	Models   map[string]*ModelTokens
	Sessions int
	Messages int
}

// jsonlEntry is the top-level structure of a session JSONL line.
type jsonlEntry struct {
	Type      string          `json:"type"`
	Timestamp json.Number     `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

// assistantMessage extracts only the fields we need from the message object.
type assistantMessage struct {
	Model string       `json:"model"`
	Usage *tokenUsage  `json:"usage"`
}

type tokenUsage struct {
	InputTokens                uint64 `json:"input_tokens"`
	OutputTokens               uint64 `json:"output_tokens"`
	CacheCreationInputTokens   uint64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens       uint64 `json:"cache_read_input_tokens"`
}

// ParseSessionFile reads a JSONL session file and aggregates token usage per model.
// Lines that fail to parse are skipped. Only entries at or after since are included.
// A zero since includes all entries.
func ParseSessionFile(path string, since time.Time) (map[string]*ModelTokens, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close()

	models := make(map[string]*ModelTokens)
	messages := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024) // 10MB max line

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue
		}
		if entry.Type != "assistant" {
			continue
		}

		if !since.IsZero() {
			ts, err := entry.Timestamp.Int64()
			if err != nil {
				continue
			}
			entryTime := time.UnixMilli(ts)
			if entryTime.Before(since) {
				continue
			}
		}

		var msg assistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil {
			continue
		}
		if msg.Usage == nil {
			continue
		}

		model := msg.Model
		if model == "" {
			model = "unknown"
		}

		if models[model] == nil {
			models[model] = &ModelTokens{}
		}
		models[model].Input += msg.Usage.InputTokens
		models[model].Output += msg.Usage.OutputTokens
		models[model].CacheCreate += msg.Usage.CacheCreationInputTokens
		models[model].CacheRead += msg.Usage.CacheReadInputTokens
		models[model].Messages++
		messages++
	}

	return models, messages, nil
}

// CollectEnvUsage walks an environment directory and aggregates usage from all session files.
func CollectEnvUsage(envDir string, since time.Time) (*EnvUsage, error) {
	projectsDir := filepath.Join(envDir, "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return &EnvUsage{Models: make(map[string]*ModelTokens)}, nil
	}

	result := &EnvUsage{
		Models: make(map[string]*ModelTokens),
	}

	sessionFiles := map[string]bool{}
	err := filepath.Walk(projectsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}
		sessionFiles[path] = true
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk projects directory: %w", err)
	}

	result.Sessions = len(sessionFiles)

	for path := range sessionFiles {
		models, messages, err := ParseSessionFile(path, since)
		if err != nil {
			continue // skip unreadable session files
		}
		result.Messages += messages
		for model, tokens := range models {
			if result.Models[model] == nil {
				result.Models[model] = &ModelTokens{}
			}
			result.Models[model].Input += tokens.Input
			result.Models[model].Output += tokens.Output
			result.Models[model].CacheCreate += tokens.CacheCreate
			result.Models[model].CacheRead += tokens.CacheRead
			result.Models[model].Messages += tokens.Messages
		}
	}

	return result, nil
}

// RateLimitTier holds published rate limit thresholds.
type RateLimitTier struct {
	Model           string
	RequestsPerMin  int
	InputTokensPerMin  int
	OutputTokensPerMin int
}

// RateLimits returns published Anthropic API rate limit thresholds.
// These are for the Max plan; actual limits depend on subscription tier.
func RateLimits() []RateLimitTier {
	return []RateLimitTier{
		{Model: "Opus", RequestsPerMin: 1000, InputTokensPerMin: 2_000_000, OutputTokensPerMin: 100_000},
		{Model: "Sonnet", RequestsPerMin: 1000, InputTokensPerMin: 2_000_000, OutputTokensPerMin: 100_000},
		{Model: "Haiku", RequestsPerMin: 1000, InputTokensPerMin: 2_000_000, OutputTokensPerMin: 100_000},
	}
}

// ParseSince parses a --since value into a time.Time.
// Accepts durations (24h, 7d, 30d) and dates (2006-01-02).
// Returns zero time for empty input.
func ParseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	// Try date format first
	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	// Handle "Nd" format (days) by converting to hours
	if numStr, ok := strings.CutSuffix(s, "d"); ok {
		var days int
		if _, err := fmt.Sscanf(numStr, "%d", &days); err == nil && days > 0 {
			return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}

	// Try standard Go duration (e.g., 24h, 2h30m)
	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since value %q: use a duration (24h, 7d) or date (2006-01-02)", s)
	}
	return time.Now().Add(-d), nil
}
