package usage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPricingForModel(t *testing.T) {
	tests := []struct {
		model     string
		wantInput float64
		wantKnown bool
	}{
		{"claude-opus-4-6", 15.0, true},
		{"claude-sonnet-4-6", 15.0, true},
		{"claude-haiku-4-5-20251001", 1.0, true},
		{"Claude-Opus-4-6", 15.0, true},   // case insensitive
		{"custom-model-v1", 15.0, false},   // unknown -> sonnet fallback
		{"some-haiku-variant", 1.0, true},  // substring match
	}

	for _, tt := range tests {
		pricing, known := PricingForModel(tt.model)
		if pricing.InputPerMillion != tt.wantInput {
			t.Errorf("PricingForModel(%q).InputPerMillion = %v, want %v", tt.model, pricing.InputPerMillion, tt.wantInput)
		}
		if known != tt.wantKnown {
			t.Errorf("PricingForModel(%q) known = %v, want %v", tt.model, known, tt.wantKnown)
		}
	}
}

func TestPricingHaikuValues(t *testing.T) {
	pricing, _ := PricingForModel("claude-haiku-4-5-20251001")
	if pricing.OutputPerMillion != 5.0 {
		t.Errorf("Haiku output = %v, want 5.0", pricing.OutputPerMillion)
	}
	if pricing.CacheCreatePerMillion != 1.25 {
		t.Errorf("Haiku cache create = %v, want 1.25", pricing.CacheCreatePerMillion)
	}
	if pricing.CacheReadPerMillion != 0.1 {
		t.Errorf("Haiku cache read = %v, want 0.1", pricing.CacheReadPerMillion)
	}
}

func TestEstimateCost(t *testing.T) {
	tokens := ModelTokens{
		Input:       1_000_000,
		Output:      500_000,
		CacheCreate: 100_000,
		CacheRead:   200_000,
	}

	cost := EstimateCost(tokens, "claude-sonnet-4-6")
	if cost.InputUSD != 15.0 {
		t.Errorf("input cost = %v, want 15.0", cost.InputUSD)
	}
	if cost.OutputUSD != 37.5 {
		t.Errorf("output cost = %v, want 37.5", cost.OutputUSD)
	}
	if cost.CacheCreateUSD != 1.875 {
		t.Errorf("cache create cost = %v, want 1.875", cost.CacheCreateUSD)
	}
	if FormatUSD(cost.CacheReadUSD) != "$0.3000" {
		t.Errorf("cache read cost = %v, want ~0.3", cost.CacheReadUSD)
	}

	wantTotal := 54.675
	if cost.Total() != wantTotal {
		t.Errorf("total cost = %v, want %v", cost.Total(), wantTotal)
	}
}

func TestEstimateCostHaiku(t *testing.T) {
	tokens := ModelTokens{
		Input:       1_000_000,
		Output:      500_000,
		CacheCreate: 0,
		CacheRead:   0,
	}
	cost := EstimateCost(tokens, "claude-haiku-4-5-20251001")
	wantTotal := 3.5
	if cost.Total() != wantTotal {
		t.Errorf("haiku total = %v, want %v", cost.Total(), wantTotal)
	}
}

func TestModelTokensTotal(t *testing.T) {
	tokens := ModelTokens{Input: 10, Output: 20, CacheCreate: 30, CacheRead: 40}
	if tokens.Total() != 100 {
		t.Errorf("Total() = %d, want 100", tokens.Total())
	}
}

func TestFormatUSD(t *testing.T) {
	tests := []struct {
		amount float64
		want   string
	}{
		{0.0, "$0.0000"},
		{15.0, "$15.0000"},
		{0.3, "$0.3000"},
		{54.675, "$54.6750"},
	}

	for _, tt := range tests {
		got := FormatUSD(tt.amount)
		if got != tt.want {
			t.Errorf("FormatUSD(%v) = %q, want %q", tt.amount, got, tt.want)
		}
	}
}

// writeJSONL creates a temporary JSONL file with the given lines.
func writeJSONL(t *testing.T, dir string, lines ...string) string {
	t.Helper()
	f, err := os.CreateTemp(dir, "session-*.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range lines {
		if _, err := f.WriteString(line + "\n"); err != nil {
			t.Fatal(err)
		}
	}
	f.Close()
	return f.Name()
}

func TestParseSessionFile(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`{"type":"permission-mode","permissionMode":"default","sessionId":"abc"}`,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
		`{"type":"user","timestamp":"1700000001000","message":"hello"}`,
		`{"type":"assistant","timestamp":"1700000002000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}`,
		`{"type":"assistant","timestamp":"1700000003000","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[],"usage":{"input_tokens":50,"output_tokens":25,"cache_creation_input_tokens":5,"cache_read_input_tokens":2}}}`,
	)

	models, messages, err := ParseSessionFile(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if messages != 3 {
		t.Errorf("messages = %d, want 3", messages)
	}
	if len(models) != 2 {
		t.Errorf("models count = %d, want 2", len(models))
	}

	opus := models["claude-opus-4-6"]
	if opus == nil {
		t.Fatal("no opus entry")
	}
	if opus.Input != 300 {
		t.Errorf("opus input = %d, want 300", opus.Input)
	}
	if opus.Output != 150 {
		t.Errorf("opus output = %d, want 150", opus.Output)
	}
	if opus.CacheCreate != 30 {
		t.Errorf("opus cache create = %d, want 30", opus.CacheCreate)
	}
	if opus.CacheRead != 15 {
		t.Errorf("opus cache read = %d, want 15", opus.CacheRead)
	}
	if opus.Messages != 2 {
		t.Errorf("opus messages = %d, want 2", opus.Messages)
	}

	haiku := models["claude-haiku-4-5-20251001"]
	if haiku == nil {
		t.Fatal("no haiku entry")
	}
	if haiku.Input != 50 {
		t.Errorf("haiku input = %d, want 50", haiku.Input)
	}
}

func TestParseSessionFileCorruptLines(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`not valid json at all`,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"bad-timestamp","message":{"model":"opus"}}`,
		`{truncated`,
	)

	models, messages, err := ParseSessionFile(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if messages != 1 {
		t.Errorf("messages = %d, want 1 (only the valid assistant message)", messages)
	}
	if len(models) != 1 {
		t.Errorf("models = %d, want 1", len(models))
	}
}

func TestParseSessionFileEmpty(t *testing.T) {
	dir := t.TempDir()
	path := writeJSONL(t, dir) // empty file

	models, messages, err := ParseSessionFile(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if messages != 0 {
		t.Errorf("messages = %d, want 0", messages)
	}
	if len(models) != 0 {
		t.Errorf("models = %d, want 0", len(models))
	}
}

func TestParseSessionFileSinceFilter(t *testing.T) {
	dir := t.TempDir()

	// Two messages: one at timestamp 1700000000000 (2023-11-14), one at 1700100000000 (~27h later)
	path := writeJSONL(t, dir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"1700100000000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
	)

	// Filter: only messages at or after the second timestamp
	since := time.UnixMilli(1700050000000)
	models, messages, err := ParseSessionFile(path, since)
	if err != nil {
		t.Fatal(err)
	}

	if messages != 1 {
		t.Errorf("messages = %d, want 1", messages)
	}
	opus := models["claude-opus-4-6"]
	if opus == nil {
		t.Fatal("no opus")
	}
	if opus.Input != 200 {
		t.Errorf("input = %d, want 200 (only second message)", opus.Input)
	}
}

func TestCollectEnvUsage(t *testing.T) {
	// Create a fake environment directory structure
	envDir := t.TempDir()
	projectDir := filepath.Join(envDir, "projects", "-test-project")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Two session files
	writeJSONL(t, projectDir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"claude-opus-4-6","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
	)
	writeJSONL(t, projectDir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"claude-haiku-4-5-20251001","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}`,
	)

	result, err := CollectEnvUsage(envDir, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if result.Sessions != 2 {
		t.Errorf("sessions = %d, want 2", result.Sessions)
	}
	if result.Messages != 2 {
		t.Errorf("messages = %d, want 2", result.Messages)
	}
	if len(result.Models) != 2 {
		t.Errorf("models = %d, want 2", len(result.Models))
	}

	opus := result.Models["claude-opus-4-6"]
	if opus == nil || opus.Input != 100 {
		t.Errorf("opus input = %v, want 100", opus)
	}
	haiku := result.Models["claude-haiku-4-5-20251001"]
	if haiku == nil || haiku.Input != 200 {
		t.Errorf("haiku input = %v, want 200", haiku)
	}
}

func TestCollectEnvUsageNoProjectsDir(t *testing.T) {
	envDir := t.TempDir()

	result, err := CollectEnvUsage(envDir, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Sessions != 0 {
		t.Errorf("sessions = %d, want 0", result.Sessions)
	}
	if len(result.Models) != 0 {
		t.Errorf("models = %d, want 0", len(result.Models))
	}
}

func TestParseSince(t *testing.T) {
	tests := []struct {
		input   string
		wantErr bool
	}{
		{"", false},
		{"24h", false},
		{"7d", false},
		{"30d", false},
		{"2026-04-01", false},
		{"2h30m", false},
		{"invalid", true},
		{"0d", true}, // 0 days is not positive
	}

	for _, tt := range tests {
		_, err := ParseSince(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSince(%q): err = %v, wantErr = %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestParseSinceDate(t *testing.T) {
	result, err := ParseSince("2026-04-01")
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	if !result.Equal(want) {
		t.Errorf("ParseSince(2026-04-01) = %v, want %v", result, want)
	}
}

func TestParseSinceDuration(t *testing.T) {
	before := time.Now()
	result, err := ParseSince("24h")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	expectedBefore := before.Add(-24 * time.Hour)
	expectedAfter := after.Add(-24 * time.Hour)

	if result.Before(expectedBefore.Add(-time.Second)) || result.After(expectedAfter.Add(time.Second)) {
		t.Errorf("ParseSince(24h) = %v, expected between %v and %v", result, expectedBefore, expectedAfter)
	}
}

func TestParseSinceDays(t *testing.T) {
	before := time.Now()
	result, err := ParseSince("7d")
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()

	expectedBefore := before.Add(-7 * 24 * time.Hour)
	expectedAfter := after.Add(-7 * 24 * time.Hour)

	if result.Before(expectedBefore.Add(-time.Second)) || result.After(expectedAfter.Add(time.Second)) {
		t.Errorf("ParseSince(7d) = %v, expected between %v and %v", result, expectedBefore, expectedAfter)
	}
}

func TestRateLimits(t *testing.T) {
	limits := RateLimits()
	if len(limits) != 3 {
		t.Errorf("RateLimits() returned %d entries, want 3", len(limits))
	}
	for _, l := range limits {
		if l.RequestsPerMin <= 0 {
			t.Errorf("model %s has invalid RequestsPerMin: %d", l.Model, l.RequestsPerMin)
		}
	}
}
