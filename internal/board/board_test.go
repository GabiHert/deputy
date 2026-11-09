package board

import (
	"strings"
	"testing"
	"time"

	"github.com/GabiHert/positron/internal/app"
)

func TestDefaultBoardIsValid(t *testing.T) {
	l := Default()
	if errs := Validate(l.Blocks, nil); len(errs) != 0 {
		t.Fatal(errs)
	}
	for _, b := range l.Blocks {
		if b.Type == "claude" {
			t.Fatal("the default board must not spend tokens")
		}
	}
}

func TestValidateReportsEveryProblem(t *testing.T) {
	blocks := []app.BoardBlockDTO{
		{ID: "a", Type: "bars", Title: "Bars", Source: &app.BoardSourceDTO{Query: "runs.list"}},
		{ID: "a", Type: "stats", Title: ""},
		{ID: "c", Type: "chart", Title: "C"},
		{ID: "d", Type: "claude", Title: "D", Prompt: "sum up", Schedule: "weekly"},
		{ID: "e", Type: "custom", Title: "E", HTML: "<p>x</p>", Height: 5000},
		{ID: "f", Type: "table", Title: "F", Source: &app.BoardSourceDTO{Query: "agents.list"}},
		{ID: "g", Type: "agent", Title: "G", AgentID: "nope"},
	}
	errs := Validate(blocks, func(id string) bool { return id == "a1" })
	joined := strings.Join(errs, "\n")
	for _, want := range []string{
		"block a: runs.list returns runs",
		"id is used twice", "needs a title", "stats blocks need a source query",
		`type "chart"`, `schedule must be`, "height must be 80 to 1200", "table blocks need columns", "agentId must be",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in:\n%s", want, joined)
		}
	}
}

func TestValidateStripsResolvedFields(t *testing.T) {
	blocks := []app.BoardBlockDTO{{ID: "p", Type: "prose", Title: "P", Text: "hi", Error: "x", TextAt: 5,
		Data: &app.QueryResultDTO{Shape: "rows"}}}
	if errs := Validate(blocks, nil); len(errs) != 0 {
		t.Fatal(errs)
	}
	if blocks[0].Data != nil || blocks[0].Error != "" || blocks[0].TextAt != 0 || blocks[0].Span != 1 {
		t.Fatalf("%+v", blocks[0])
	}
}

func TestScheduleDue(t *testing.T) {
	loc := time.Local
	now := time.Date(2026, 11, 4, 9, 0, 0, 0, loc)
	daily, _ := ParseSchedule("daily 08:00")
	if !daily.Due(time.Date(2026, 11, 4, 7, 0, 0, 0, loc), now) {
		t.Error("written before today's slot: due")
	}
	if daily.Due(time.Date(2026, 11, 4, 8, 5, 0, 0, loc), now) {
		t.Error("written after today's slot: not due")
	}
	early := time.Date(2026, 11, 4, 7, 0, 0, 0, loc)
	if daily.Due(time.Date(2026, 11, 3, 8, 30, 0, 0, loc), early) {
		t.Error("before today's slot, yesterday's text is current")
	}
	hourly, _ := ParseSchedule("hourly")
	if !hourly.Due(now.Add(-61*time.Minute), now) || hourly.Due(now.Add(-10*time.Minute), now) {
		t.Error("hourly")
	}
	manual, _ := ParseSchedule("manual")
	if manual.Due(time.Time{}, now) {
		t.Error("manual is never due")
	}
}
