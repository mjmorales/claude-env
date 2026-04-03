//nolint:revive // magic numbers are clear in pricing/token context
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

const tokensPerMillion = 1_000_000.0

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
		InputUSD:       float64(tokens.Input) / tokensPerMillion * pricing.InputPerMillion,
		OutputUSD:      float64(tokens.Output) / tokensPerMillion * pricing.OutputPerMillion,
		CacheCreateUSD: float64(tokens.CacheCreate) / tokensPerMillion * pricing.CacheCreatePerMillion,
		CacheReadUSD:   float64(tokens.CacheRead) / tokensPerMillion * pricing.CacheReadPerMillion,
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
// Timestamp can be a number (unix millis) or an ISO 8601 string depending on
// the Claude Code version that wrote the file.
type jsonlEntry struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
	Message   json.RawMessage `json:"message"`
}

type assistantMessage struct {
	Model string      `json:"model"`
	Usage *tokenUsage `json:"usage"`
}

type tokenUsage struct {
	InputTokens              uint64 `json:"input_tokens"`
	OutputTokens             uint64 `json:"output_tokens"`
	CacheCreationInputTokens uint64 `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     uint64 `json:"cache_read_input_tokens"`
}

// ParseSessionFile reads a JSONL session file and aggregates token usage per model.
// Lines that fail to parse are skipped. Only entries at or after since are included.
// A zero since includes all entries.
//
//nolint:gocritic // unnamedResult: three returns are clear from usage
func ParseSessionFile(path string, since time.Time) (map[string]*ModelTokens, int, error) {
	f, err := os.Open(path) //#nosec G304 -- path is constructed from trusted env directories
	if err != nil {
		return nil, 0, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	models := make(map[string]*ModelTokens)
	messages := 0

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		model, u, ok := parseAssistantEntry(scanner.Bytes(), since)
		if !ok {
			continue
		}
		if models[model] == nil {
			models[model] = &ModelTokens{}
		}
		models[model].Input += u.InputTokens
		models[model].Output += u.OutputTokens
		models[model].CacheCreate += u.CacheCreationInputTokens
		models[model].CacheRead += u.CacheReadInputTokens
		models[model].Messages++
		messages++
	}

	return models, messages, nil
}

// parseAssistantEntry extracts model name and usage from a JSONL line.
// Returns false if the line is not a valid assistant entry within the time window.
func parseAssistantEntry(line []byte, since time.Time) (string, *tokenUsage, bool) {
	var entry jsonlEntry
	if err := json.Unmarshal(line, &entry); err != nil {
		return "", nil, false
	}
	if entry.Type != "assistant" {
		return "", nil, false
	}

	if !since.IsZero() {
		entryTime, ok := parseTimestamp(entry.Timestamp)
		if !ok || entryTime.Before(since) {
			return "", nil, false
		}
	}

	var msg assistantMessage
	if err := json.Unmarshal(entry.Message, &msg); err != nil {
		return "", nil, false
	}
	if msg.Usage == nil {
		return "", nil, false
	}

	model := msg.Model
	if model == "" {
		model = "unknown"
	}
	return model, msg.Usage, true
}

// CollectEnvUsage walks an environment directory and aggregates usage from all session files.
func CollectEnvUsage(envDir string, since time.Time) (*EnvUsage, error) { //nolint:unparam // error return kept for API consistency
	projectsDir := filepath.Join(envDir, "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return &EnvUsage{Models: make(map[string]*ModelTokens)}, nil
	}

	result := &EnvUsage{
		Models: make(map[string]*ModelTokens),
	}

	sessionFiles := collectJSONLFiles(projectsDir, time.Time{})
	result.Sessions = len(sessionFiles)

	for _, path := range sessionFiles {
		models, messages, err := ParseSessionFile(path, since)
		if err != nil {
			continue
		}
		result.Messages += messages
		mergeModels(result.Models, models)
	}

	return result, nil
}

func mergeModels(dst map[string]*ModelTokens, src map[string]*ModelTokens) {
	for model, tokens := range src {
		if dst[model] == nil {
			dst[model] = &ModelTokens{}
		}
		dst[model].Input += tokens.Input
		dst[model].Output += tokens.Output
		dst[model].CacheCreate += tokens.CacheCreate
		dst[model].CacheRead += tokens.CacheRead
		dst[model].Messages += tokens.Messages
	}
}

// collectJSONLFiles returns all .jsonl file paths under dir.
// If modifiedAfter is non-zero, only files modified after that time are returned.
func collectJSONLFiles(dir string, modifiedAfter time.Time) []string {
	var files []string
	//nolint:errcheck // best-effort walk; unreadable dirs are skipped
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil //nolint:nilerr // intentionally skip errors in walk
		}
		if !modifiedAfter.IsZero() && info.ModTime().Before(modifiedAfter) {
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files
}

// RateLimitTier holds published rate limit thresholds.
type RateLimitTier struct {
	Model              string
	RequestsPerMin     int
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

// parseTimestamp handles both numeric (unix millis) and ISO 8601 string timestamps.
func parseTimestamp(raw json.RawMessage) (time.Time, bool) {
	if len(raw) == 0 {
		return time.Time{}, false
	}

	if raw[0] != '"' {
		return parseNumericTimestamp(raw)
	}
	return parseStringTimestamp(raw)
}

func parseNumericTimestamp(raw json.RawMessage) (time.Time, bool) {
	var ms int64
	if err := json.Unmarshal(raw, &ms); err == nil {
		return time.UnixMilli(ms), true
	}
	return time.Time{}, false
}

const minMillisTimestamp = 1_000_000_000_000

func parseStringTimestamp(raw json.RawMessage) (time.Time, bool) {
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return time.Time{}, false
	}

	var ms int64
	if _, err := fmt.Sscanf(s, "%d", &ms); err == nil && ms > minMillisTimestamp {
		return time.UnixMilli(ms), true
	}

	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t, err = time.Parse("2006-01-02T15:04:05.000Z", s)
		if err != nil {
			return time.Time{}, false
		}
	}
	return t, true
}

// TimestampedUsage holds token counts for a single API response with its timestamp.
type TimestampedUsage struct {
	Time   time.Time
	Model  string
	Output uint64
	Input  uint64
}

// CollectRecentMessages parses all session files in an environment and returns
// individual timestamped usage entries from the given window. Used for rate
// limit analysis where per-message granularity matters.
func CollectRecentMessages(envDir string, window time.Duration) ([]TimestampedUsage, error) { //nolint:unparam // error return kept for API consistency
	projectsDir := filepath.Join(envDir, "projects")
	if _, err := os.Stat(projectsDir); os.IsNotExist(err) {
		return nil, nil
	}

	since := time.Now().Add(-window)
	var results []TimestampedUsage

	for _, path := range collectJSONLFiles(projectsDir, since) {
		entries, err := parseTimestampedEntries(path, since)
		if err != nil {
			continue
		}
		results = append(results, entries...)
	}

	return results, nil
}

func parseTimestampedEntries(path string, since time.Time) ([]TimestampedUsage, error) {
	f, err := os.Open(path) //#nosec G304 -- path from trusted env directories
	if err != nil {
		return nil, fmt.Errorf("open session file: %w", err)
	}
	defer f.Close() //nolint:errcheck // read-only file

	var results []TimestampedUsage
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024*1024), 10*1024*1024)

	for scanner.Scan() {
		var entry jsonlEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil || entry.Type != "assistant" {
			continue
		}

		entryTime, ok := parseTimestamp(entry.Timestamp)
		if !ok || entryTime.Before(since) {
			continue
		}

		var msg assistantMessage
		if err := json.Unmarshal(entry.Message, &msg); err != nil || msg.Usage == nil {
			continue
		}

		model := msg.Model
		if model == "" {
			model = "unknown"
		}

		results = append(results, TimestampedUsage{
			Time:   entryTime,
			Model:  model,
			Output: msg.Usage.OutputTokens,
			Input:  msg.Usage.InputTokens,
		})
	}

	return results, nil
}

// Rate limit status constants.
const (
	StatusReady     = "ready"
	StatusCaution   = "caution"
	StatusThrottled = "throttled"

	cautionThreshold = 0.2 // headroom below 20% triggers caution
)

// RateStatus describes the rate limit status for a model tier in an environment.
type RateStatus struct {
	Tier           string
	OutputRate     float64 // tokens per minute (averaged over window)
	OutputLimit    int     // published limit tokens per minute
	HeadroomPct    float64 // percentage of limit remaining
	Status         string  // StatusReady, StatusCaution, or StatusThrottled
	MinutesToLimit float64 // estimated minutes until throttled (-1 if sustainable)
}

// ComputeRateStatus analyzes recent timestamped messages against published rate
// limits and returns a status per model tier found.
func ComputeRateStatus(entries []TimestampedUsage, windowMinutes float64) []RateStatus {
	tierOutput := aggregateTierOutput(entries)

	var results []RateStatus
	for _, l := range RateLimits() {
		output, ok := tierOutput[l.Model]
		if !ok {
			continue
		}

		rate := float64(output) / windowMinutes
		headroom := max(0, 1.0-rate/float64(l.OutputTokensPerMin))

		results = append(results, classifyRateStatus(l, rate, headroom))
	}

	return results
}

func aggregateTierOutput(entries []TimestampedUsage) map[string]uint64 {
	tierOutput := map[string]uint64{}
	for _, e := range entries {
		tier := modelTier(e.Model)
		if tier != "" {
			tierOutput[tier] += e.Output
		}
	}
	return tierOutput
}

func classifyRateStatus(limit RateLimitTier, rate, headroom float64) RateStatus {
	status := StatusReady
	minutesToLimit := -1.0

	switch {
	case headroom <= 0:
		status = StatusThrottled
		minutesToLimit = 0
	case headroom < cautionThreshold:
		status = StatusCaution
		remaining := float64(limit.OutputTokensPerMin) - rate
		if rate > 0 {
			minutesToLimit = remaining / rate
		}
	}

	return RateStatus{
		Tier:           limit.Model,
		OutputRate:     rate,
		OutputLimit:    limit.OutputTokensPerMin,
		HeadroomPct:    headroom * 100,
		Status:         status,
		MinutesToLimit: minutesToLimit,
	}
}

func modelTier(model string) string {
	lower := strings.ToLower(model)
	switch {
	case strings.Contains(lower, "opus"):
		return "Opus"
	case strings.Contains(lower, "sonnet"):
		return "Sonnet"
	case strings.Contains(lower, "haiku"):
		return "Haiku"
	default:
		return ""
	}
}

// ParseSince parses a --since value into a time.Time.
// Accepts durations (24h, 7d, 30d) and dates (2006-01-02).
// Returns zero time for empty input.
func ParseSince(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}

	if t, err := time.Parse("2006-01-02", s); err == nil {
		return t, nil
	}

	if numStr, ok := strings.CutSuffix(s, "d"); ok {
		var days int
		if _, err := fmt.Sscanf(numStr, "%d", &days); err == nil && days > 0 {
			return time.Now().Add(-time.Duration(days) * 24 * time.Hour), nil
		}
	}

	d, err := time.ParseDuration(s)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid --since value %q: use a duration (24h, 7d) or date (2006-01-02)", s)
	}
	return time.Now().Add(-d), nil
}
