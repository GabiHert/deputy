package query

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/GabiHert/deputy/internal/domain"
	"github.com/GabiHert/deputy/internal/store"
)

func TestParseSince(t *testing.T) {
	now := time.Date(2026, 11, 4, 15, 30, 0, 0, time.Local)
	cases := map[string]time.Time{
		"today":      time.Date(2026, 11, 4, 0, 0, 0, 0, time.Local),
		"yesterday":  time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local),
		"7d":         now.AddDate(0, 0, -7),
		"24h":        now.Add(-24 * time.Hour),
		"2w":         now.AddDate(0, 0, -14),
		"2026-11-01": time.Date(2026, 11, 1, 0, 0, 0, 0, time.Local),
	}
	for in, want := range cases {
		got, err := ParseSince(in, now)
		if err != nil || !got.Equal(want) {
			t.Errorf("%s: got %v %v, want %v", in, got, err, want)
		}
	}
	if _, err := ParseSince("last tuesday", now); err == nil {
		t.Error("expected an error")
	}
}

func TestValidate(t *testing.T) {
	ok := []struct {
		q string
		p map[string]any
	}{
		{"runs.count", map[string]any{"since": "7d", "by": "agent"}},
		{"messages.list", map[string]any{"chat": "x@g.us", "limit": float64(10)}},
		{"agents.list", nil},
	}
	for _, c := range ok {
		if err := Validate(c.q, c.p); err != nil {
			t.Errorf("%s: %v", c.q, err)
		}
	}
	bad := []struct {
		q string
		p map[string]any
	}{
		{"runs.delete", nil},
		{"runs.count", map[string]any{"by": "weather"}},
		{"runs.count", map[string]any{"since": "soon"}},
		{"runs.count", map[string]any{"color": "red"}},
		{"messages.list", map[string]any{}},
		{"messages.search", map[string]any{"text": "  "}},
	}
	for _, c := range bad {
		if err := Validate(c.q, c.p); err == nil {
			t.Errorf("%s %v should fail", c.q, c.p)
		}
	}
}

func TestRunsCountAndMessages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	now := time.Now()
	_ = st.SaveAgent(domain.Agent{ID: "a1", Name: "Família", Chats: []string{"g1@g.us"}})
	_ = st.SaveChat(domain.Chat{JID: "g1@g.us", Name: "Família Herter", IsGroup: true, LastAt: now})
	_ = st.SaveChat(domain.Chat{JID: "g2@g.us", Name: "Trabalho", IsGroup: true, LastAt: now.Add(-time.Hour)})
	for i, s := range []domain.RunStatus{domain.RunSent, domain.RunSent, domain.RunQuiet, domain.RunWaiting} {
		_ = st.SaveRun(domain.Run{ID: string(rune('a' + i)), At: now, AgentID: "a1", ChatJID: "g1@g.us", Status: s})
	}
	_ = st.SaveMessage(domain.Message{ID: "m1", ChatJID: "g1@g.us", At: now, Text: "vem domingo?"})
	_ = st.SaveMessage(domain.Message{ID: "m2", ChatJID: "g1@g.us", At: now, Kind: domain.KindVoice, Transcript: "levo o pudim"})
	e := &Engine{Store: st}
	ctx := context.Background()

	r, err := e.Run(ctx, "runs.count", map[string]any{"since": "today"})
	if err != nil {
		t.Fatal(err)
	}
	groups := r.Items.([]Group)
	got := map[string]int64{}
	for _, g := range groups {
		got[g.Key] = g.Value
	}
	if r.Shape != ShapeGroups || got["sent"] != 2 || got["quiet"] != 1 || got["waiting"] != 1 || got["failed"] != 0 {
		t.Fatalf("groups %+v", groups)
	}
	r, _ = e.Run(ctx, "runs.count", map[string]any{"by": "agent"})
	if g := r.Items.([]Group); len(g) != 1 || g[0].Label != "Família" || g[0].Value != 4 {
		t.Fatalf("by agent %+v", g)
	}
	r, _ = e.Run(ctx, "messages.search", map[string]any{"text": "pudim"})
	if ms := r.Items.([]domain.Message); len(ms) != 1 || ms[0].ID != "m2" {
		t.Fatalf("search %+v", ms)
	}
	r, _ = e.Run(ctx, "messages.search", map[string]any{"text": "100%_"})
	if ms := r.Items.([]domain.Message); len(ms) != 0 {
		t.Fatalf("wildcards must be literal: %+v", ms)
	}
	r, _ = e.Run(ctx, "chats.list", map[string]any{"agent": "none"})
	if rows := r.Items.([]map[string]any); len(rows) != 1 || rows[0]["name"] != "Trabalho" {
		t.Fatalf("chats %+v", rows)
	}
	r, _ = e.Run(ctx, "messages.byHour", nil)
	if cells := r.Items.([]store.HourCount); len(cells) != 1 || cells[0].Count != 2 {
		t.Fatalf("cells %+v", cells)
	}
}
