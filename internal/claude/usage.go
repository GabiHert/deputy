package claude

import (
	"encoding/json"

	"github.com/GabiHert/positron/internal/domain"
)

// runOutput is the part of claude -p --output-format json that reports what a run used.
type runOutput struct {
	TotalCostUSD float64 `json:"total_cost_usd"`
	DurationMs   int64   `json:"duration_ms"`
	NumTurns     int     `json:"num_turns"`
	Usage        struct {
		Input      int64 `json:"input_tokens"`
		CacheWrite int64 `json:"cache_creation_input_tokens"`
		CacheRead  int64 `json:"cache_read_input_tokens"`
		Output     int64 `json:"output_tokens"`
	} `json:"usage"`
	// ModelUsage covers every model the run called, including the small model Claude Code uses
	// for its own chores, so its sums are the run's full use.
	ModelUsage map[string]struct {
		Input      int64   `json:"inputTokens"`
		Output     int64   `json:"outputTokens"`
		CacheRead  int64   `json:"cacheReadInputTokens"`
		CacheWrite int64   `json:"cacheCreationInputTokens"`
		CostUSD    float64 `json:"costUSD"`
	} `json:"modelUsage"`
}

// parseUsage reads what a run used from claude's JSON output. It returns nil when the output
// reports nothing, as an older claude might.
func parseUsage(stdout []byte) *domain.Usage {
	var o runOutput
	if json.Unmarshal(stdout, &o) != nil {
		return nil
	}
	u := domain.Usage{CostUSD: o.TotalCostUSD, DurationMs: o.DurationMs, Turns: o.NumTurns}
	if len(o.ModelUsage) > 0 {
		var sum float64
		top := -1.0
		for name, m := range o.ModelUsage {
			u.Input += m.Input
			u.Output += m.Output
			u.CacheRead += m.CacheRead
			u.CacheWrite += m.CacheWrite
			sum += m.CostUSD
			if m.CostUSD > top || (m.CostUSD == top && name < u.Model) {
				top, u.Model = m.CostUSD, name
			}
		}
		if u.CostUSD == 0 {
			u.CostUSD = sum
		}
	} else {
		u.Input, u.Output = o.Usage.Input, o.Usage.Output
		u.CacheRead, u.CacheWrite = o.Usage.CacheRead, o.Usage.CacheWrite
	}
	if u.Tokens() == 0 && u.CostUSD == 0 {
		return nil
	}
	return &u
}
