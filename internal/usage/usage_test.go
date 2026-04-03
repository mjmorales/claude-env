//nolint:revive // magic numbers are clear in test assertions
package usage_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mjmorales/claude-env/internal/usage"
)

const (
	opusModel  = "claude-opus-4-6"
	haikuModel = "claude-haiku-4-5-20251001"
)

func TestPricingForModel(t *testing.T) {
	tests := []struct {
		model     string
		wantInput float64
		wantKnown bool
	}{
		{opusModel, 15.0, true},
		{"claude-sonnet-4-6", 15.0, true},
		{haikuModel, 1.0, true},
		{"Claude-Opus-4-6", 15.0, true},   // case insensitive
		{"custom-model-v1", 15.0, false},  // unknown -> sonnet fallback
		{"some-haiku-variant", 1.0, true}, // substring match
	}

	for _, tt := range tests {
		pricing, known := usage.PricingForModel(tt.model)
		if pricing.InputPerMillion != tt.wantInput {
			t.Errorf("PricingForModel(%q).InputPerMillion = %v, want %v", tt.model, pricing.InputPerMillion, tt.wantInput)
		}
		if known != tt.wantKnown {
			t.Errorf("PricingForModel(%q) known = %v, want %v", tt.model, known, tt.wantKnown)
		}
	}
}

func TestPricingHaikuValues(t *testing.T) {
	pricing, _ := usage.PricingForModel(haikuModel)
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
	tokens := usage.ModelTokens{
		Input:       1_000_000,
		Output:      500_000,
		CacheCreate: 100_000,
		CacheRead:   200_000,
	}

	cost := usage.EstimateCost(tokens, "claude-sonnet-4-6")
	if cost.InputUSD != 15.0 {
		t.Errorf("input cost = %v, want 15.0", cost.InputUSD)
	}
	if cost.OutputUSD != 37.5 {
		t.Errorf("output cost = %v, want 37.5", cost.OutputUSD)
	}
	if cost.CacheCreateUSD != 1.875 {
		t.Errorf("cache create cost = %v, want 1.875", cost.CacheCreateUSD)
	}
	if usage.FormatUSD(cost.CacheReadUSD) != "$0.3000" {
		t.Errorf("cache read cost = %v, want ~0.3", cost.CacheReadUSD)
	}

	wantTotal := 54.675
	if cost.Total() != wantTotal {
		t.Errorf("total cost = %v, want %v", cost.Total(), wantTotal)
	}
}

func TestEstimateCostHaiku(t *testing.T) {
	tokens := usage.ModelTokens{Input: 1_000_000, Output: 500_000}
	cost := usage.EstimateCost(tokens, haikuModel)
	if cost.Total() != 3.5 {
		t.Errorf("haiku total = %v, want 3.5", cost.Total())
	}
}

func TestModelTokensTotal(t *testing.T) {
	tokens := usage.ModelTokens{Input: 10, Output: 20, CacheCreate: 30, CacheRead: 40}
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
		got := usage.FormatUSD(tt.amount)
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
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return f.Name()
}

func TestParseSessionFile(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`{"type":"permission-mode","permissionMode":"default","sessionId":"abc"}`,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
		`{"type":"user","timestamp":"1700000001000","message":"hello"}`,
		`{"type":"assistant","timestamp":"1700000002000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}`,
		`{"type":"assistant","timestamp":"1700000003000","message":{"model":"`+haikuModel+`","role":"assistant","content":[],"usage":{"input_tokens":50,"output_tokens":25,"cache_creation_input_tokens":5,"cache_read_input_tokens":2}}}`,
	)

	models, messages, err := usage.ParseSessionFile(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}

	if messages != 3 {
		t.Errorf("messages = %d, want 3", messages)
	}
	if len(models) != 2 {
		t.Errorf("models count = %d, want 2", len(models))
	}

	opus := models[opusModel]
	if opus == nil {
		t.Fatal("no opus entry")
	}
	assertTokens(t, "opus", opus, 300, 150, 30, 15, 2)

	haiku := models[haikuModel]
	if haiku == nil {
		t.Fatal("no haiku entry")
	}
	if haiku.Input != 50 {
		t.Errorf("haiku input = %d, want 50", haiku.Input)
	}
}

func assertTokens(t *testing.T, label string, tok *usage.ModelTokens, input, output, cacheCreate, cacheRead, msgs uint64) {
	t.Helper()
	if tok.Input != input {
		t.Errorf("%s input = %d, want %d", label, tok.Input, input)
	}
	if tok.Output != output {
		t.Errorf("%s output = %d, want %d", label, tok.Output, output)
	}
	if tok.CacheCreate != cacheCreate {
		t.Errorf("%s cache create = %d, want %d", label, tok.CacheCreate, cacheCreate)
	}
	if tok.CacheRead != cacheRead {
		t.Errorf("%s cache read = %d, want %d", label, tok.CacheRead, cacheRead)
	}
	if tok.Messages != msgs {
		t.Errorf("%s messages = %d, want %d", label, tok.Messages, msgs)
	}
}

func TestParseSessionFileCorruptLines(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`not valid json at all`,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"bad-timestamp","message":{"model":"opus"}}`,
		`{truncated`,
	)

	models, messages, err := usage.ParseSessionFile(path, time.Time{})
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
	path := writeJSONL(t, dir)

	models, messages, err := usage.ParseSessionFile(path, time.Time{})
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

	path := writeJSONL(t, dir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"1700100000000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
	)

	since := time.UnixMilli(1700050000000)
	models, messages, err := usage.ParseSessionFile(path, since)
	if err != nil {
		t.Fatal(err)
	}
	if messages != 1 {
		t.Errorf("messages = %d, want 1", messages)
	}
	opus := models[opusModel]
	if opus == nil {
		t.Fatal("no opus")
	}
	if opus.Input != 200 {
		t.Errorf("input = %d, want 200 (only second message)", opus.Input)
	}
}

func TestCollectEnvUsage(t *testing.T) {
	envDir := t.TempDir()
	projectDir := filepath.Join(envDir, "projects", "-test-project")
	if err := os.MkdirAll(projectDir, 0o750); err != nil {
		t.Fatal(err)
	}

	writeJSONL(t, projectDir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
	)
	writeJSONL(t, projectDir,
		`{"type":"assistant","timestamp":"1700000000000","message":{"model":"`+haikuModel+`","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":20,"cache_read_input_tokens":10}}}`,
	)

	result, err := usage.CollectEnvUsage(envDir, time.Time{})
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

	opus := result.Models[opusModel]
	if opus == nil || opus.Input != 100 {
		t.Errorf("opus input = %v, want 100", opus)
	}
	haiku := result.Models[haikuModel]
	if haiku == nil || haiku.Input != 200 {
		t.Errorf("haiku input = %v, want 200", haiku)
	}
}

func TestCollectEnvUsageNoProjectsDir(t *testing.T) {
	envDir := t.TempDir()

	result, err := usage.CollectEnvUsage(envDir, time.Time{})
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
		{"0d", true},
	}

	for _, tt := range tests {
		_, err := usage.ParseSince(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSince(%q): err = %v, wantErr = %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestParseSinceDate(t *testing.T) {
	result, err := usage.ParseSince("2026-04-01")
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
	result, err := usage.ParseSince("24h")
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
	result, err := usage.ParseSince("7d")
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

func TestParseSessionFileISOTimestamp(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`{"type":"assistant","timestamp":"2026-04-02T15:08:31.768Z","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":10,"cache_read_input_tokens":5}}}`,
	)

	models, messages, err := usage.ParseSessionFile(path, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if messages != 1 {
		t.Errorf("messages = %d, want 1", messages)
	}
	opus := models[opusModel]
	if opus == nil {
		t.Fatal("no opus entry with ISO timestamp")
	}
	if opus.Input != 100 {
		t.Errorf("input = %d, want 100", opus.Input)
	}
}

func TestParseSessionFileISOTimestampSinceFilter(t *testing.T) {
	dir := t.TempDir()

	path := writeJSONL(t, dir,
		`{"type":"assistant","timestamp":"2026-04-01T10:00:00.000Z","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":50,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
		`{"type":"assistant","timestamp":"2026-04-03T10:00:00.000Z","message":{"model":"`+opusModel+`","role":"assistant","content":[],"usage":{"input_tokens":200,"output_tokens":100,"cache_creation_input_tokens":0,"cache_read_input_tokens":0}}}`,
	)

	since := time.Date(2026, 4, 2, 0, 0, 0, 0, time.UTC)
	models, messages, err := usage.ParseSessionFile(path, since)
	if err != nil {
		t.Fatal(err)
	}
	if messages != 1 {
		t.Errorf("messages = %d, want 1 (only after cutoff)", messages)
	}
	opus := models[opusModel]
	if opus == nil {
		t.Fatal("no opus")
	}
	if opus.Input != 200 {
		t.Errorf("input = %d, want 200 (only second message)", opus.Input)
	}
}

func TestRateLimits(t *testing.T) {
	limits := usage.RateLimits()
	if len(limits) != 3 {
		t.Errorf("RateLimits() returned %d entries, want 3", len(limits))
	}
	for _, l := range limits {
		if l.RequestsPerMin <= 0 {
			t.Errorf("model %s has invalid RequestsPerMin: %d", l.Model, l.RequestsPerMin)
		}
	}
}

const rateWindow = 5

func TestComputeRateStatusReady(t *testing.T) {
	entries := []usage.TimestampedUsage{
		{Time: time.Now().Add(-4 * time.Minute), Model: opusModel, Output: 10_000},
		{Time: time.Now().Add(-3 * time.Minute), Model: opusModel, Output: 10_000},
		{Time: time.Now().Add(-2 * time.Minute), Model: opusModel, Output: 10_000},
	}

	statuses := usage.ComputeRateStatus(entries, rateWindow)
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	s := statuses[0]
	if s.Tier != "Opus" {
		t.Errorf("tier = %q, want Opus", s.Tier)
	}
	if s.Status != usage.StatusReady {
		t.Errorf("status = %q, want %s", s.Status, usage.StatusReady)
	}
	if s.HeadroomPct < 90 {
		t.Errorf("headroom = %.1f%%, expected >90%%", s.HeadroomPct)
	}
	if s.MinutesToLimit != -1 {
		t.Errorf("minutesToLimit = %v, want -1 (sustainable)", s.MinutesToLimit)
	}
}

func TestComputeRateStatusCaution(t *testing.T) {
	// 450K output over 5 min = 90K/min = 90% of 100K limit.
	entries := makeEntries(opusModel, 90_000, 5)

	statuses := usage.ComputeRateStatus(entries, rateWindow)
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	s := statuses[0]
	if s.Status != usage.StatusCaution {
		t.Errorf("status = %q, want %s", s.Status, usage.StatusCaution)
	}
	if s.HeadroomPct > 20 {
		t.Errorf("headroom = %.1f%%, expected <20%%", s.HeadroomPct)
	}
	if s.MinutesToLimit <= 0 {
		t.Errorf("minutesToLimit = %v, want >0", s.MinutesToLimit)
	}
}

func TestComputeRateStatusThrottled(t *testing.T) {
	// 600K output over 5 min = 120K/min > 100K limit.
	entries := makeEntries(opusModel, 120_000, 5)

	statuses := usage.ComputeRateStatus(entries, rateWindow)
	if len(statuses) != 1 {
		t.Fatalf("statuses = %d, want 1", len(statuses))
	}
	s := statuses[0]
	if s.Status != usage.StatusThrottled {
		t.Errorf("status = %q, want %s", s.Status, usage.StatusThrottled)
	}
	if s.HeadroomPct != 0 {
		t.Errorf("headroom = %.1f%%, want 0", s.HeadroomPct)
	}
}

func TestComputeRateStatusMultipleTiers(t *testing.T) {
	entries := []usage.TimestampedUsage{
		{Time: time.Now().Add(-2 * time.Minute), Model: opusModel, Output: 10_000},
		{Time: time.Now().Add(-1 * time.Minute), Model: haikuModel, Output: 5_000},
	}

	statuses := usage.ComputeRateStatus(entries, rateWindow)
	if len(statuses) != 2 {
		t.Fatalf("statuses = %d, want 2", len(statuses))
	}
	for _, s := range statuses {
		if s.Status != usage.StatusReady {
			t.Errorf("tier %s status = %q, want %s", s.Tier, s.Status, usage.StatusReady)
		}
	}
}

func TestComputeRateStatusNoEntries(t *testing.T) {
	statuses := usage.ComputeRateStatus(nil, rateWindow)
	if len(statuses) != 0 {
		t.Errorf("statuses = %d, want 0", len(statuses))
	}
}

func TestComputeRateStatusUnknownModel(t *testing.T) {
	entries := []usage.TimestampedUsage{
		{Time: time.Now(), Model: "custom-model", Output: 50_000},
	}
	statuses := usage.ComputeRateStatus(entries, rateWindow)
	if len(statuses) != 0 {
		t.Errorf("unknown models should be skipped, got %d statuses", len(statuses))
	}
}

func TestCollectRecentMessagesNoProjectsDir(t *testing.T) {
	envDir := t.TempDir()
	entries, err := usage.CollectRecentMessages(envDir, 5*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %d, want 0", len(entries))
	}
}

// makeEntries creates n entries spread over the last n minutes with the given output per entry.
func makeEntries(model string, outputPerEntry uint64, n int) []usage.TimestampedUsage {
	entries := make([]usage.TimestampedUsage, n)
	for i := range n {
		entries[i] = usage.TimestampedUsage{
			Time:   time.Now().Add(-time.Duration(n-1-i) * time.Minute),
			Model:  model,
			Output: outputPerEntry,
		}
	}
	return entries
}
