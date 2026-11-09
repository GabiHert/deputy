package mcpserver

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/GabiHert/positron/internal/app"
)

// resolveChat finds a chat by JID, exact name (any case), phone number or a unique part of its
// name, in that order. A JID or phone number Positron hasn't seen yet still resolves, without a name.
func resolveChat(chats []app.ChatDTO, q string) (app.ChatDTO, error) {
	q = strings.TrimSpace(q)
	if q == "" {
		return app.ChatDTO{}, fmt.Errorf("name the chat: a name, a phone number or a JID from list_chats")
	}
	if strings.Contains(q, "@") {
		for _, c := range chats {
			if c.JID == q {
				return c, nil
			}
		}
		return app.ChatDTO{JID: q, IsGroup: strings.HasSuffix(q, "@g.us")}, nil
	}
	if c, err := pick(chats, q, func(c app.ChatDTO) string { return c.Name }, chatLabel, "chat", true); err == nil || !isNotFound(err) {
		return c, err
	}
	if digits, ok := phoneDigits(q); ok {
		jid := digits + "@s.whatsapp.net"
		for _, c := range chats {
			if c.JID == jid {
				return c, nil
			}
		}
		return app.ChatDTO{JID: jid}, nil
	}
	return pick(chats, q, func(c app.ChatDTO) string { return c.Name }, chatLabel, "chat", false)
}

// resolveAgent finds an agent by ID, exact name (any case) or a unique part of its name.
func resolveAgent(agents []app.AgentDTO, q string) (app.AgentDTO, error) {
	q = strings.TrimSpace(q)
	for _, a := range agents {
		if a.ID == q {
			return a, nil
		}
	}
	label := func(a app.AgentDTO) string { return fmt.Sprintf("%s (id %s)", a.Name, a.ID) }
	name := func(a app.AgentDTO) string { return a.Name }
	if a, err := pick(agents, q, name, label, "agent", true); err == nil || !isNotFound(err) {
		return a, err
	}
	return pick(agents, q, name, label, "agent", false)
}

// resolveClaudeChat finds a Claude chat by ID, exact title or a unique part of it.
func resolveClaudeChat(chats []app.AssistantChatDTO, q string) (app.AssistantChatDTO, error) {
	q = strings.TrimSpace(q)
	for _, c := range chats {
		if c.ID == q {
			return c, nil
		}
	}
	label := func(c app.AssistantChatDTO) string { return fmt.Sprintf("%s (id %s)", c.Title, c.ID) }
	title := func(c app.AssistantChatDTO) string { return c.Title }
	if c, err := pick(chats, q, title, label, "Claude chat", true); err == nil || !isNotFound(err) {
		return c, err
	}
	return pick(chats, q, title, label, "Claude chat", false)
}

type notFound struct{ msg string }

func (e notFound) Error() string { return e.msg }

func isNotFound(err error) bool { _, ok := err.(notFound); return ok }

func chatLabel(c app.ChatDTO) string { return fmt.Sprintf("%s (%s)", c.Name, c.JID) }

// pick matches q against name: the whole name when exact, else a part of it. One match wins;
// several are an error that lists them.
func pick[T any](items []T, q string, name, label func(T) string, noun string, exact bool) (T, error) {
	var zero T
	if q == "" {
		return zero, notFound{"name the " + noun}
	}
	lq := strings.ToLower(q)
	var found []T
	for _, it := range items {
		n := strings.ToLower(strings.TrimSpace(name(it)))
		if n == "" {
			continue
		}
		if (exact && n == lq) || (!exact && strings.Contains(n, lq)) {
			found = append(found, it)
		}
	}
	switch len(found) {
	case 0:
		return zero, notFound{fmt.Sprintf("no %s matches %q", noun, q)}
	case 1:
		return found[0], nil
	}
	var names []string
	for i, it := range found {
		if i == 10 {
			names = append(names, fmt.Sprintf("and %d more", len(found)-10))
			break
		}
		names = append(names, label(it))
	}
	return zero, fmt.Errorf("%q matches %d %ss: %s. Use the exact name or ID", q, len(found), noun, strings.Join(names, "; "))
}

// phoneDigits reads a phone number like "+55 (51) 99999-4412" as its digits.
func phoneDigits(q string) (string, bool) {
	var b strings.Builder
	for i, r := range q {
		switch {
		case unicode.IsDigit(r):
			b.WriteRune(r)
		case r == '+' && i == 0, r == ' ', r == '-', r == '(', r == ')', r == '.':
		default:
			return "", false
		}
	}
	d := b.String()
	return d, len(d) >= 8 && len(d) <= 15
}
