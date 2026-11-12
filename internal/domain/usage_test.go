package domain

import "testing"

func TestSumUsageSkipsRunsWithoutUsage(t *testing.T) {
	runs := []Run{
		{Usage: &Usage{Input: 1, CacheRead: 100, CacheWrite: 50, Output: 10, CostUSD: 0.05, Turns: 1, DurationMs: 2000}},
		{}, // from before usage was recorded
		{Usage: &Usage{Input: 2, CacheRead: 200, Output: 20, CostUSD: 0.01, Turns: 3, DurationMs: 1000}},
	}
	got := SumUsage(runs)
	if got.Runs != 3 || got.Measured != 2 {
		t.Fatalf("runs %d measured %d", got.Runs, got.Measured)
	}
	if got.Tokens() != 383 || got.CacheRead != 300 || got.CacheWrite != 50 || got.Turns != 4 || got.DurationMs != 3000 {
		t.Fatalf("total %+v", got)
	}
	if got.CostUSD < 0.0599 || got.CostUSD > 0.0601 {
		t.Fatalf("cost %v", got.CostUSD)
	}
	if empty := SumUsage(nil); empty.Runs != 0 || empty.Tokens() != 0 {
		t.Fatalf("empty %+v", empty)
	}
}
