package claude

import "testing"

// A trimmed copy of what claude -p --output-format json printed for a real run.
const sampleOutput = `{"type":"result","subtype":"success","is_error":false,"duration_ms":3120,"num_turns":2,
"total_cost_usd":0.0631,
"usage":{"input_tokens":5,"cache_creation_input_tokens":20100,"cache_read_input_tokens":12900,"output_tokens":120},
"modelUsage":{
 "claude-sonnet-5-5":{"inputTokens":5,"outputTokens":120,"cacheReadInputTokens":12900,"cacheCreationInputTokens":20100,"costUSD":0.0601},
 "claude-haiku-5-5":{"inputTokens":300,"outputTokens":20,"cacheReadInputTokens":0,"cacheCreationInputTokens":0,"costUSD":0.003}}}`

func TestParseUsageSumsEveryModel(t *testing.T) {
	u := parseUsage([]byte(sampleOutput))
	if u == nil {
		t.Fatal("no usage")
	}
	if u.Model != "claude-sonnet-5-5" {
		t.Errorf("model = %q, want the one that cost most", u.Model)
	}
	if u.Input != 305 || u.Output != 140 || u.CacheRead != 12900 || u.CacheWrite != 20100 {
		t.Errorf("tokens = %+v", *u)
	}
	if u.Tokens() != 305+140+12900+20100 {
		t.Errorf("Tokens() = %d", u.Tokens())
	}
	if u.CostUSD != 0.0631 || u.Turns != 2 || u.DurationMs != 3120 {
		t.Errorf("cost, turns, duration = %v %d %d", u.CostUSD, u.Turns, u.DurationMs)
	}
}

func TestParseUsageFallsBackToTopLevelUsage(t *testing.T) {
	u := parseUsage([]byte(`{"total_cost_usd":0.01,"usage":{"input_tokens":10,"cache_read_input_tokens":90,"output_tokens":5}}`))
	if u == nil || u.Input != 10 || u.CacheRead != 90 || u.Output != 5 || u.Model != "" {
		t.Fatalf("usage = %+v", u)
	}
}

func TestParseUsageWithoutReport(t *testing.T) {
	for _, s := range []string{`{"result":"ok"}`, `not json`, ``} {
		if u := parseUsage([]byte(s)); u != nil {
			t.Errorf("%q: usage = %+v, want nil", s, *u)
		}
	}
}
