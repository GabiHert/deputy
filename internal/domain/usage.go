package domain

// Usage is what one Claude Code run used, as claude -p reports it. CostUSD is the price at API
// list rates; on a Claude subscription nothing is billed per run, but it shows how much of the
// plan a run takes compared with others.
type Usage struct {
	Model      string  `json:"model"`      // the model that did most of the work
	Input      int64   `json:"input"`      // input tokens not read from or written to the cache
	CacheRead  int64   `json:"cacheRead"`  // input tokens read from the prompt cache
	CacheWrite int64   `json:"cacheWrite"` // input tokens written to the prompt cache
	Output     int64   `json:"output"`
	CostUSD    float64 `json:"costUsd"`
	Turns      int     `json:"turns"`
	DurationMs int64   `json:"durationMs"`
}

// Tokens is every input and output token of the run.
func (u Usage) Tokens() int64 { return u.Input + u.CacheRead + u.CacheWrite + u.Output }

// UsageTotal adds up the usage of several runs.
type UsageTotal struct {
	Runs     int // all runs counted
	Measured int // runs that carry usage; runs from before it was recorded have none
	Usage
}

// SumUsage adds up the usage of runs.
func SumUsage(runs []Run) UsageTotal {
	var t UsageTotal
	for _, r := range runs {
		t.Runs++
		u := r.Usage
		if u == nil {
			continue
		}
		t.Measured++
		t.Input += u.Input
		t.CacheRead += u.CacheRead
		t.CacheWrite += u.CacheWrite
		t.Output += u.Output
		t.CostUSD += u.CostUSD
		t.Turns += u.Turns
		t.DurationMs += u.DurationMs
	}
	return t
}
