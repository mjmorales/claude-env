// Package usage parses Claude Code session JSONL files and computes
// per-model token consumption with estimated costs.
//
// # Data Source
//
// Claude Code stores session transcripts as JSONL files under
// CLAUDE_CONFIG_DIR/projects/<project-slug>/<session-id>.jsonl.
// Each line is a JSON object with a "type" field. Only "assistant"
// entries carry a "usage" object with token counts.
//
// # Entry Format
//
// Top-level fields used:
//
//	type      string          — "assistant", "user", "system", etc.
//	timestamp number|string   — unix millis or ISO 8601 (varies by version)
//	message   object          — contains "model" and "usage"
//
// The message.usage object contains:
//
//	input_tokens                 uint64
//	output_tokens                uint64
//	cache_creation_input_tokens  uint64
//	cache_read_input_tokens      uint64
//
// # Pricing
//
// Static pricing table matching Claude Code's implementation:
//
//	Opus:   $15.00 / $75.00 / $18.75 / $1.50 per million (input/output/cache-create/cache-read)
//	Sonnet: $15.00 / $75.00 / $18.75 / $1.50 per million
//	Haiku:   $1.00 /  $5.00 /  $1.25 / $0.10 per million
//
// Unknown models fall back to Sonnet pricing. PricingForModel returns
// a second bool indicating whether the model was recognized.
//
// # Usage
//
//	data, err := usage.CollectEnvUsage("/path/to/env", time.Time{})
//	for model, tokens := range data.Models {
//	    cost := usage.EstimateCost(*tokens, model)
//	    fmt.Printf("%s: %s\n", model, usage.FormatUSD(cost.Total()))
//	}
package usage
