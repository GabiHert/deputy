// Package query is the board engine's read-only data layer. Board blocks, custom HTML
// blocks and Claude chats all read Deputy's data through these named queries, so a
// live board never needs Claude to fetch or refresh anything.
package query

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GabiHert/deputy/internal/domain"
	"github.com/GabiHert/deputy/internal/gitrepo"
	"github.com/GabiHert/deputy/internal/store"
)

// Shapes of query results.
const (
	ShapeGroups   = "groups"
	ShapeRuns     = "runs"
	ShapeMessages = "messages"
	ShapeCells    = "cells"
	ShapeRows     = "rows"
)

// Result is a query's answer.
type Result struct {
	Query string `json:"query"`
	Shape string `json:"shape"`
	Items any    `json:"items"`
}

// Group is one counted bucket.
type Group struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int64  `json:"value"`
}

// Param describes one query parameter.
type Param struct {
	Name     string
	Kind     string // "time" | "string" | "int" | "enum"
	Enum     []string
	Required bool
	Doc      string
}

// Spec describes one query.
type Spec struct {
	Name   string
	Shape  string
	Doc    string
	Params []Param
}

// Engine runs queries over the store and the agents' repos.
type Engine struct {
	Store *store.Store
	// RepoEnv is git's environment; nil uses the app's own.
	RepoEnv func() []string
	// Runs and Messages turn domain values into what the frontend shows (RunDTOs, MessageDTOs).
	Runs     func([]domain.Run) any
	Messages func([]domain.Message) any
	// MediaUsage reports the media folder's size.
	MediaUsage func() (files int, bytes int64)
	// Now is the clock; nil uses time.Now.
	Now func() time.Time
}

var specs = []Spec{
	{Name: "runs.count", Shape: ShapeGroups, Doc: "Counts agent runs, grouped.", Params: []Param{
		{Name: "since", Kind: "time", Doc: `"today" (default), "yesterday", "7d", "24h", "2w" or a date`},
		{Name: "agent", Kind: "string", Doc: "agent ID"},
		{Name: "chat", Kind: "string", Doc: "chat JID"},
		{Name: "status", Kind: "enum", Enum: statuses},
		{Name: "by", Kind: "enum", Enum: []string{"status", "agent", "chat", "day", "hour"}, Doc: `default "status"`},
	}},
	{Name: "runs.list", Shape: ShapeRuns, Doc: "Agent runs, newest first. status=waiting lists replies waiting for approval.", Params: []Param{
		{Name: "since", Kind: "time", Doc: "default 30d"},
		{Name: "agent", Kind: "string"}, {Name: "chat", Kind: "string"},
		{Name: "status", Kind: "enum", Enum: statuses},
		{Name: "limit", Kind: "int", Doc: "default 20, at most 100"},
	}},
	{Name: "messages.list", Shape: ShapeMessages, Doc: "WhatsApp messages of one chat, newest first.", Params: []Param{
		{Name: "chat", Kind: "string", Required: true},
		{Name: "since", Kind: "time", Doc: "default 7d"},
		{Name: "limit", Kind: "int", Doc: "default 30, at most 200"},
	}},
	{Name: "messages.search", Shape: ShapeMessages, Doc: "WhatsApp messages whose text or voice transcript contains text.", Params: []Param{
		{Name: "text", Kind: "string", Required: true},
		{Name: "chat", Kind: "string"},
		{Name: "since", Kind: "time", Doc: "default 90d"},
		{Name: "limit", Kind: "int", Doc: "default 30, at most 200"},
	}},
	{Name: "messages.byHour", Shape: ShapeCells, Doc: "Message counts per weekday (0 = Sunday) and hour, local time.", Params: []Param{
		{Name: "days", Kind: "int", Doc: "default 7, at most 90"},
		{Name: "chat", Kind: "string"},
	}},
	{Name: "chats.list", Shape: ShapeRows, Doc: "WhatsApp chats, most recent first: jid, name, isGroup, members, lastAt, lastText, agentId, agentName.", Params: []Param{
		{Name: "agent", Kind: "string", Doc: `agent ID, or "none" for chats without an agent`},
		{Name: "activeSince", Kind: "time"},
		{Name: "text", Kind: "string", Doc: "part of the chat name"},
		{Name: "limit", Kind: "int", Doc: "default 50, at most 500"},
	}},
	{Name: "agents.list", Shape: ShapeRows, Doc: "Agents: id, name, enabled, mode, trigger, chats, chatNames."},
	{Name: "knowledge.changes", Shape: ShapeRows, Doc: "Files agents changed in their knowledge repos: agentId, agentName, at, commit, message, file, kind.", Params: []Param{
		{Name: "since", Kind: "time", Doc: "default 7d"},
		{Name: "agent", Kind: "string"},
		{Name: "limit", Kind: "int", Doc: "default 50, at most 200"},
	}},
	{Name: "media.usage", Shape: ShapeGroups, Doc: "Files and megabytes of chat media Deputy keeps."},
}

var statuses = []string{"running", "sent", "waiting", "discarded", "quiet", "learned", "failed"}

// Specs lists every query.
func Specs() []Spec { return specs }

// SpecFor returns one query's spec.
func SpecFor(name string) (Spec, bool) {
	for _, s := range specs {
		if s.Name == name {
			return s, true
		}
	}
	return Spec{}, false
}

// Describe is the query catalog as text, for Claude's tool descriptions.
func Describe() string {
	var b strings.Builder
	for _, s := range specs {
		fmt.Fprintf(&b, "- %s → %s. %s", s.Name, s.Shape, s.Doc)
		if len(s.Params) > 0 {
			var ps []string
			for _, p := range s.Params {
				d := p.Name
				if p.Required {
					d += " (required)"
				}
				if len(p.Enum) > 0 {
					d += ": " + strings.Join(p.Enum, "|")
				}
				if p.Doc != "" {
					d += ", " + p.Doc
				}
				ps = append(ps, d)
			}
			fmt.Fprintf(&b, " Params: %s.", strings.Join(ps, "; "))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// Validate checks a query name and its params without running it.
func Validate(name string, params map[string]any) error {
	s, ok := SpecFor(name)
	if !ok {
		return fmt.Errorf("unknown query %q", name)
	}
	known := map[string]Param{}
	for _, p := range s.Params {
		known[p.Name] = p
	}
	for k, v := range params {
		p, ok := known[k]
		if !ok {
			return fmt.Errorf("%s has no param %q", name, k)
		}
		str := fmt.Sprint(v)
		switch p.Kind {
		case "time":
			if _, err := ParseSince(str, time.Now()); err != nil {
				return fmt.Errorf("%s.%s: %v", name, k, err)
			}
		case "int":
			if _, err := toInt(v); err != nil {
				return fmt.Errorf("%s.%s must be a number", name, k)
			}
		case "enum":
			if !contains(p.Enum, str) {
				return fmt.Errorf("%s.%s must be one of %s", name, k, strings.Join(p.Enum, ", "))
			}
		}
	}
	for _, p := range s.Params {
		if !p.Required {
			continue
		}
		if v, ok := params[p.Name]; !ok || v == nil || strings.TrimSpace(fmt.Sprint(v)) == "" {
			return fmt.Errorf("%s needs %s", name, p.Name)
		}
	}
	return nil
}

var relRe = regexp.MustCompile(`^(\d+)\s*([hdw])$`)

// ParseSince reads "today", "yesterday", "7d", "24h", "2w", "2026-11-01" or RFC 3339, relative to now.
func ParseSince(s string, now time.Time) (time.Time, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	switch s {
	case "", "today":
		return midnight, nil
	case "yesterday":
		return midnight.AddDate(0, 0, -1), nil
	case "all":
		return time.Time{}, nil
	}
	if m := relRe.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		switch m[2] {
		case "h":
			return now.Add(-time.Duration(n) * time.Hour), nil
		case "d":
			return now.AddDate(0, 0, -n), nil
		case "w":
			return now.AddDate(0, 0, -7*n), nil
		}
	}
	if t, err := time.ParseInLocation("2006-01-02", s, now.Location()); err == nil {
		return t, nil
	}
	if t, err := time.Parse(time.RFC3339, strings.ToUpper(s)); err == nil {
		return t, nil
	}
	return time.Time{}, fmt.Errorf(`%q is not a time like "today", "7d", "24h" or "2026-11-01"`, s)
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case string:
		return strconv.Atoi(strings.TrimSpace(n))
	}
	return 0, fmt.Errorf("not a number")
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

type args struct {
	m   map[string]any
	now time.Time
}

func (a args) str(k string) string {
	if v, ok := a.m[k]; ok && v != nil {
		return strings.TrimSpace(fmt.Sprint(v))
	}
	return ""
}

func (a args) since(k, def string) time.Time {
	v := a.str(k)
	if v == "" {
		v = def
	}
	t, err := ParseSince(v, a.now)
	if err != nil {
		t, _ = ParseSince(def, a.now)
	}
	return t
}

func (a args) num(k string, def, max int) int {
	n := def
	if v, ok := a.m[k]; ok {
		if x, err := toInt(v); err == nil && x > 0 {
			n = x
		}
	}
	if n > max {
		n = max
	}
	return n
}

// Run runs a query.
func (e *Engine) Run(ctx context.Context, name string, params map[string]any) (Result, error) {
	if params == nil {
		params = map[string]any{}
	}
	if err := Validate(name, params); err != nil {
		return Result{}, err
	}
	now := time.Now()
	if e.Now != nil {
		now = e.Now()
	}
	a := args{m: params, now: now}
	spec, _ := SpecFor(name)
	res := Result{Query: name, Shape: spec.Shape}
	switch name {
	case "runs.count":
		res.Items = e.runsCount(a)
	case "runs.list":
		runs := e.filterRuns(a, a.since("since", "30d"))
		if n := a.num("limit", 20, 100); len(runs) > n {
			runs = runs[:n]
		}
		res.Items = e.outRuns(runs)
	case "messages.list":
		res.Items = e.outMessages(e.Store.Messages(store.MessageQuery{Chat: a.str("chat"), Since: a.since("since", "7d"), Limit: a.num("limit", 30, 200)}))
	case "messages.search":
		res.Items = e.outMessages(e.Store.Messages(store.MessageQuery{Chat: a.str("chat"), Text: a.str("text"), Since: a.since("since", "90d"), Limit: a.num("limit", 30, 200)}))
	case "messages.byHour":
		days := a.num("days", 7, 90)
		hours := e.Store.MessageHours(now.AddDate(0, 0, -days), a.str("chat"))
		if hours == nil {
			hours = []store.HourCount{}
		}
		res.Items = hours
	case "chats.list":
		res.Items = e.chatsList(a)
	case "agents.list":
		res.Items = e.agentsList()
	case "knowledge.changes":
		res.Items = e.knowledgeChanges(ctx, a)
	case "media.usage":
		var files int
		var bytes int64
		if e.MediaUsage != nil {
			files, bytes = e.MediaUsage()
		}
		res.Items = []Group{{Key: "files", Label: "files", Value: int64(files)}, {Key: "mb", Label: "MB", Value: bytes / (1 << 20)}}
	}
	return res, nil
}

func (e *Engine) filterRuns(a args, since time.Time) []domain.Run {
	runs := e.Store.Runs(store.RunQuery{AgentID: a.str("agent"), Status: domain.RunStatus(a.str("status")), Since: since})
	if chat := a.str("chat"); chat != "" {
		var out []domain.Run
		for _, r := range runs {
			if r.ChatJID == chat {
				out = append(out, r)
			}
		}
		runs = out
	}
	return runs
}

var statusLabel = map[string]string{
	"sent": "replies sent", "waiting": "waiting for you", "quiet": "stayed quiet", "learned": "learned",
	"failed": "failed", "discarded": "discarded", "running": "running",
}

func (e *Engine) runsCount(a args) []Group {
	runs := e.filterRuns(a, a.since("since", "today"))
	by := a.str("by")
	if by == "" {
		by = "status"
	}
	counts := map[string]int64{}
	var order []string
	add := func(k string) {
		if _, ok := counts[k]; !ok {
			order = append(order, k)
		}
		counts[k]++
	}
	if by == "status" && a.str("status") == "" {
		for _, s := range []string{"sent", "waiting", "quiet", "failed"} {
			counts[s] = 0
			order = append(order, s)
		}
	}
	for _, r := range runs {
		switch by {
		case "status":
			add(string(r.Status))
		case "agent":
			add(r.AgentID)
		case "chat":
			add(r.ChatJID)
		case "day":
			add(r.At.Local().Format("2006-01-02"))
		case "hour":
			add(fmt.Sprintf("%02d", r.At.Local().Hour()))
		}
	}
	agentNames, chatNames := map[string]string{}, map[string]string{}
	if by == "agent" {
		for _, ag := range e.Store.Agents() {
			agentNames[ag.ID] = ag.Name
		}
	}
	if by == "chat" {
		for _, c := range e.Store.Chats() {
			chatNames[c.JID] = c.Name
		}
	}
	out := make([]Group, 0, len(order))
	for _, k := range order {
		label := k
		switch by {
		case "status":
			if l, ok := statusLabel[k]; ok {
				label = l
			}
		case "agent":
			if n := agentNames[k]; n != "" {
				label = n
			}
		case "chat":
			if n := chatNames[k]; n != "" {
				label = n
			}
		case "day":
			if t, err := time.ParseInLocation("2006-01-02", k, time.Local); err == nil {
				label = t.Format("Mon 2")
			}
		case "hour":
			label = k + "h"
		}
		out = append(out, Group{Key: k, Label: label, Value: counts[k]})
	}
	switch by {
	case "day", "hour":
		sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	case "agent", "chat":
		sort.SliceStable(out, func(i, j int) bool { return out[i].Value > out[j].Value })
	}
	return out
}

func (e *Engine) outRuns(runs []domain.Run) any {
	if runs == nil {
		runs = []domain.Run{}
	}
	if e.Runs != nil {
		return e.Runs(runs)
	}
	return runs
}

func (e *Engine) outMessages(ms []domain.Message) any {
	if ms == nil {
		ms = []domain.Message{}
	}
	if e.Messages != nil {
		return e.Messages(ms)
	}
	return ms
}

func (e *Engine) chatsList(a args) []map[string]any {
	router := domain.NewRouter(e.Store.Agents())
	agent := a.str("agent")
	text := strings.ToLower(a.str("text"))
	var activeSince time.Time
	if a.str("activeSince") != "" {
		activeSince = a.since("activeSince", "30d")
	}
	limit := a.num("limit", 50, 500)
	out := []map[string]any{}
	for _, c := range e.Store.Chats() {
		owner, viaAll, has := router.Owner(c.JID)
		switch {
		case agent == "none" && has, agent != "" && agent != "none" && owner.ID != agent:
			continue
		case !activeSince.IsZero() && c.LastAt.Before(activeSince):
			continue
		case text != "" && !strings.Contains(strings.ToLower(c.Name), text):
			continue
		}
		out = append(out, map[string]any{
			"jid": c.JID, "name": c.Name, "isGroup": c.IsGroup, "members": c.Members,
			"lastAt": c.LastAt.UnixMilli(), "lastText": c.LastText, "agentId": owner.ID, "agentName": owner.Name,
			"viaAllDirect": viaAll,
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func (e *Engine) agentsList() []map[string]any {
	names := map[string]string{}
	for _, c := range e.Store.Chats() {
		names[c.JID] = c.Name
	}
	out := []map[string]any{}
	for _, ag := range e.Store.Agents() {
		var cn []string
		for _, c := range ag.Chats {
			if n := names[c]; n != "" {
				cn = append(cn, n)
			} else {
				cn = append(cn, c)
			}
		}
		out = append(out, map[string]any{
			"id": ag.ID, "name": ag.Name, "enabled": ag.Enabled, "mode": string(ag.Mode), "trigger": string(ag.Trigger),
			"chats": len(ag.Chats), "chatNames": strings.Join(cn, ", "), "allDirect": ag.AllDirect, "answerMe": ag.AnswerMe,
		})
	}
	return out
}

func (e *Engine) knowledgeChanges(ctx context.Context, a args) []map[string]any {
	since := a.since("since", "7d")
	limit := a.num("limit", 50, 200)
	agent := a.str("agent")
	type row struct {
		ag domain.Agent
		c  gitrepo.Change
	}
	var rows []row
	for _, ag := range e.Store.Agents() {
		if agent != "" && ag.ID != agent || ag.RepoPath == "" {
			continue
		}
		for _, c := range (gitrepo.Repo{Dir: ag.RepoPath, EnvFn: e.RepoEnv}).Log(ctx, since, limit) {
			rows = append(rows, row{ag, c})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].c.At.After(rows[j].c.At) })
	if len(rows) > limit {
		rows = rows[:limit]
	}
	out := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		out = append(out, map[string]any{
			"agentId": r.ag.ID, "agentName": r.ag.Name, "at": r.c.At.UnixMilli(), "commit": r.c.Commit,
			"message": r.c.Message, "file": r.c.File, "kind": r.c.Kind,
		})
	}
	return out
}
