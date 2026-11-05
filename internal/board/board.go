// Package board holds the Activity board's layout: which blocks, in what order, fed by
// which queries. Claude writes the layout; the board engine (package query) fills it.
package board

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GabiHert/deputy/internal/app"
	"github.com/GabiHert/deputy/internal/query"
)

// Layout is the saved board.
type Layout struct {
	Version  int                 `json:"version"`
	Blocks   []app.BoardBlockDTO `json:"blocks"`
	LayoutAt time.Time           `json:"layoutAt"` // zero for the default board
}

// Limits.
const (
	MaxBlocks     = 24
	MaxCustomHTML = 256 << 10
	MaxText       = 20000
)

// Which query shapes each block type can show.
var accepts = map[string][]string{
	"stats":    {query.ShapeGroups},
	"bars":     {query.ShapeGroups},
	"replies":  {query.ShapeRuns},
	"runs":     {query.ShapeRuns},
	"messages": {query.ShapeMessages},
	"heatmap":  {query.ShapeCells},
	"table":    {query.ShapeGroups, query.ShapeRuns, query.ShapeMessages, query.ShapeCells, query.ShapeRows},
	"custom":   {query.ShapeGroups, query.ShapeRuns, query.ShapeMessages, query.ShapeCells, query.ShapeRows},
}

// Types lists the block types.
var Types = []string{"stats", "bars", "replies", "runs", "messages", "table", "heatmap", "prose", "claude", "agent", "form", "custom"}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,39}$`)

// Default is the board before Claude changes it. It uses no Claude blocks, so it costs nothing.
func Default() Layout {
	src := func(q string, p map[string]any) *app.BoardSourceDTO { return &app.BoardSourceDTO{Query: q, Params: p} }
	return Layout{Version: 1, Blocks: []app.BoardBlockDTO{
		{ID: "today", Type: "stats", Title: "Today so far", Span: 2, Source: src("runs.count", map[string]any{"since": "today", "by": "status"})},
		{ID: "waiting", Type: "replies", Title: "Waiting for you", Span: 1, Source: src("runs.list", map[string]any{"status": "waiting", "limit": 10})},
		{ID: "by-agent", Type: "bars", Title: "Runs this week, by agent", Span: 1, Source: src("runs.count", map[string]any{"since": "7d", "by": "agent"})},
		{ID: "recent", Type: "runs", Title: "Latest runs", Span: 1, Source: src("runs.list", map[string]any{"since": "2d", "limit": 8})},
		{ID: "busy", Type: "heatmap", Title: "When the chats are busiest", Span: 1, Source: src("messages.byHour", map[string]any{"days": 7})},
		{ID: "learned", Type: "table", Title: "What the agents learned this week", Span: 2,
			Source:  src("knowledge.changes", map[string]any{"since": "7d", "limit": 20}),
			Columns: []app.BoardColumnDTO{{Key: "agentName", Label: "Agent"}, {Key: "message", Label: "Change"}, {Key: "file", Label: "File"}}},
	}}
}

// Parse reads a saved layout, falling back to the default.
func Parse(raw string) Layout {
	var l Layout
	if raw == "" || json.Unmarshal([]byte(raw), &l) != nil || len(l.Blocks) == 0 {
		return Default()
	}
	return l
}

// Validate checks blocks Claude submitted and normalizes them in place. It returns every
// problem, so Claude can fix them all at once.
func Validate(blocks []app.BoardBlockDTO, agentExists func(id string) bool) []string {
	var errs []string
	bad := func(i int, b app.BoardBlockDTO, f string, a ...any) {
		name := b.ID
		if name == "" {
			name = fmt.Sprintf("#%d", i+1)
		}
		errs = append(errs, "block "+name+": "+fmt.Sprintf(f, a...))
	}
	if len(blocks) == 0 {
		return []string{"the board needs at least one block"}
	}
	if len(blocks) > MaxBlocks {
		errs = append(errs, fmt.Sprintf("at most %d blocks, got %d", MaxBlocks, len(blocks)))
	}
	seen := map[string]bool{}
	for i := range blocks {
		b := &blocks[i]
		b.ID = strings.TrimSpace(b.ID)
		if b.ID == "" {
			b.ID = fmt.Sprintf("block-%d", i+1)
		}
		if !idRe.MatchString(b.ID) {
			bad(i, *b, "id must be lower case letters, digits, - or _")
		}
		if seen[b.ID] {
			bad(i, *b, "id is used twice")
		}
		seen[b.ID] = true
		if strings.TrimSpace(b.Title) == "" {
			bad(i, *b, "needs a title")
		}
		if b.Span != 2 {
			b.Span = 1
		}
		// Resolved fields are never accepted from Claude.
		b.Data, b.Agent, b.Error, b.TextAt, b.Refreshing = nil, nil, "", 0, false
		if !contains(Types, b.Type) {
			bad(i, *b, "type %q is not one of %s", b.Type, strings.Join(Types, ", "))
			continue
		}
		if shapes, needs := accepts[b.Type]; needs {
			switch {
			case b.Source == nil && b.Type != "custom":
				bad(i, *b, "%s blocks need a source query", b.Type)
			case b.Source != nil:
				if err := query.Validate(b.Source.Query, b.Source.Params); err != nil {
					bad(i, *b, "%v", err)
				} else if s, _ := query.SpecFor(b.Source.Query); !contains(shapes, s.Shape) {
					bad(i, *b, "%s returns %s, which a %s block can't show (use %s)", b.Source.Query, s.Shape, b.Type, strings.Join(shapes, " or "))
				}
			}
		} else if b.Source != nil {
			bad(i, *b, "%s blocks don't take a source", b.Type)
		}
		switch b.Type {
		case "table":
			if len(b.Columns) == 0 {
				bad(i, *b, "table blocks need columns [{key, label}]")
			}
			if len(b.Columns) > 8 {
				bad(i, *b, "at most 8 columns")
			}
		case "prose":
			if strings.TrimSpace(b.Text) == "" {
				bad(i, *b, "prose blocks need text")
			}
		case "claude":
			if strings.TrimSpace(b.Prompt) == "" {
				bad(i, *b, "claude blocks need a prompt saying what to write")
			}
			if b.Schedule == "" {
				b.Schedule = "manual"
			}
			if _, err := ParseSchedule(b.Schedule); err != nil {
				bad(i, *b, "%v", err)
			}
		case "agent":
			if b.AgentID == "" || agentExists != nil && !agentExists(b.AgentID) {
				bad(i, *b, "agentId must be an existing agent's ID")
			}
		case "form":
			if b.Form == nil || len(b.Form.Questions) == 0 {
				bad(i, *b, "form blocks need form.questions")
			} else {
				for _, e := range ValidateForm(b.Form) {
					bad(i, *b, "%s", e)
				}
			}
		case "custom":
			if strings.TrimSpace(b.HTML) == "" {
				bad(i, *b, "custom blocks need html")
			}
			if len(b.HTML)+len(b.JS) > MaxCustomHTML {
				bad(i, *b, "custom html and js together must be under 256 KB")
			}
			if b.Height == 0 {
				b.Height = 240
			}
			if b.Height < 80 || b.Height > 1200 {
				bad(i, *b, "height must be 80 to 1200")
			}
		}
		if len(b.Text) > MaxText {
			bad(i, *b, "text is too long")
		}
	}
	return errs
}

// ValidateForm checks a form's questions.
func ValidateForm(f *app.FormDTO) []string {
	var errs []string
	ids := map[string]bool{}
	for i := range f.Questions {
		q := &f.Questions[i]
		if q.ID == "" {
			q.ID = fmt.Sprintf("q%d", i+1)
		}
		if ids[q.ID] {
			errs = append(errs, fmt.Sprintf("question id %q is used twice", q.ID))
		}
		ids[q.ID] = true
		if strings.TrimSpace(q.Label) == "" {
			errs = append(errs, fmt.Sprintf("question %s needs a label", q.ID))
		}
		switch q.Kind {
		case "choice", "multi":
			if len(q.Options) < 2 {
				errs = append(errs, fmt.Sprintf("question %s needs at least two options", q.ID))
			}
		case "input", "confirm":
		case "":
			q.Kind = "input"
		default:
			errs = append(errs, fmt.Sprintf("question %s kind must be choice, multi, input or confirm", q.ID))
		}
	}
	if len(f.Questions) > 8 {
		errs = append(errs, "at most 8 questions")
	}
	return errs
}

// Schedule says when a claude block is rewritten.
type Schedule struct {
	Kind   string // "manual" | "hourly" | "daily"
	Hour   int
	Minute int
}

// ParseSchedule reads "manual", "hourly" or "daily HH:MM".
func ParseSchedule(s string) (Schedule, error) {
	f := strings.Fields(strings.ToLower(s))
	switch {
	case len(f) == 1 && (f[0] == "manual" || f[0] == "hourly"):
		return Schedule{Kind: f[0]}, nil
	case len(f) == 1 && f[0] == "daily":
		return Schedule{Kind: "daily", Hour: 8}, nil
	case len(f) == 2 && f[0] == "daily":
		hm := strings.SplitN(f[1], ":", 2)
		if len(hm) == 2 {
			h, e1 := strconv.Atoi(hm[0])
			m, e2 := strconv.Atoi(hm[1])
			if e1 == nil && e2 == nil && h >= 0 && h < 24 && m >= 0 && m < 60 {
				return Schedule{Kind: "daily", Hour: h, Minute: m}, nil
			}
		}
	}
	return Schedule{}, fmt.Errorf(`schedule must be "manual", "hourly" or "daily HH:MM", not %q`, s)
}

// Due reports whether a block last written at last should be rewritten at now.
func (s Schedule) Due(last, now time.Time) bool {
	switch s.Kind {
	case "hourly":
		return now.Sub(last) >= time.Hour
	case "daily":
		slot := time.Date(now.Year(), now.Month(), now.Day(), s.Hour, s.Minute, 0, 0, now.Location())
		if now.Before(slot) {
			slot = slot.AddDate(0, 0, -1)
		}
		return last.Before(slot)
	}
	return false
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
