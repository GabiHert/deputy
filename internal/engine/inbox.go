package engine

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/whatsapp"
)

// This file is the owner's own WhatsApp client: what changes earlier messages (reactions,
// receipts, edits, deletions), history syncs, and what the owner does from Deputy's inbox.

// HandleUpdate applies a change that is not a new message. Presence updates are not the
// engine's business and are ignored here.
func (e *Engine) HandleUpdate(u any) {
	switch v := u.(type) {
	case whatsapp.History:
		e.HandleHistory(v)
	default:
		e.apply(u, true)
	}
}

// apply changes stored state for one update; with notify it reports what changed.
func (e *Engine) apply(u any, notify bool) {
	switch v := u.(type) {
	case whatsapp.Reaction:
		r := domain.Reaction{Sender: v.Sender, SenderName: v.SenderName, FromMe: v.FromMe, Emoji: v.Emoji, At: v.At}
		if r.FromMe {
			r.Sender, r.SenderName = "", ""
		}
		e.updateMessage(v.Chat, v.TargetID, notify, func(m *domain.Message) bool { return m.SetReaction(r) })
	case whatsapp.Edit:
		e.updateMessage(v.Chat, v.TargetID, notify, func(m *domain.Message) bool {
			if m.Revoked || (m.Text == v.Text && m.Edited) {
				return false
			}
			m.Text, m.Edited = v.Text, true
			return true
		})
	case whatsapp.Revoke:
		m, ok := e.updateMessage(v.Chat, v.TargetID, notify, revoke)
		if ok && !m.FromMe {
			c, err := e.d.Store.Chat(m.ChatJID)
			if err == nil && c.Unread > 0 && m.At.After(c.ReadAt) {
				_, _ = e.d.Store.UpdateChat(m.ChatJID, func(c *domain.Chat) { c.Unread = max(0, c.Unread-1) })
				if notify {
					e.unread(m.ChatJID)
				}
			}
		}
		if ok {
			e.refreshPreview(m.ChatJID, m.ID)
		}
	case whatsapp.Receipt:
		if v.FromMe {
			e.readOnPhone(v)
			return
		}
		st := domain.MsgStatus(v.Status)
		var changed []string
		chat := v.Chat
		for _, id := range v.IDs {
			m, ok := e.updateMessage(v.Chat, id, false, func(m *domain.Message) bool {
				if !m.FromMe || m.Status.Upgrade(st) == m.Status {
					return false
				}
				m.Status = m.Status.Upgrade(st)
				return true
			})
			if ok {
				changed = append(changed, id)
				chat = m.ChatJID
			}
		}
		if notify && len(changed) > 0 && e.d.OnReceipt != nil {
			e.d.OnReceipt(chat, changed, string(st))
		}
	case whatsapp.ChatRead:
		before, _ := e.d.Store.Chat(v.Chat)
		c, _ := e.d.Store.UpdateChat(v.Chat, func(c *domain.Chat) {
			if v.Read {
				c.Unread = 0
				if v.At.After(c.ReadAt) {
					c.ReadAt = v.At
				}
			} else if c.Unread == 0 {
				c.Unread = 1 // "mark as unread" on the phone
			}
		})
		if notify && c.Unread != before.Unread {
			e.unread(v.Chat)
			e.chatsChanged()
		}
	case whatsapp.Disappearing:
		before, _ := e.d.Store.Chat(v.Chat)
		c, _ := e.d.Store.UpdateChat(v.Chat, func(c *domain.Chat) {
			c.JID = v.Chat
			c.IsGroup = c.IsGroup || strings.HasSuffix(v.Chat, "@g.us")
			c.Disappearing, c.DisappearingAt, c.DisappearingKnown = int(v.Seconds), v.SetAt, true
		})
		if notify && (c.Disappearing != before.Disappearing || !before.DisappearingKnown) {
			e.chatsChanged()
		}
	case whatsapp.ChatMeta:
		_, _ = e.d.Store.UpdateChat(v.Chat, func(c *domain.Chat) {
			switch v.Field {
			case "mute":
				c.MutedUntil = time.Time{}
				if v.On {
					c.MutedUntil = v.Until
					if v.Until.IsZero() {
						c.MutedUntil = domain.Forever
					}
				}
			case "pin":
				c.Pinned = v.On
			case "archive":
				c.Archived = v.On
			}
		})
		if notify {
			e.chatsChanged()
			if v.Field == "mute" {
				e.unread(v.Chat) // the badge leaves muted chats out
			}
		}
	}
}

func revoke(m *domain.Message) bool {
	if m.Revoked {
		return false
	}
	m.Revoked, m.Text, m.Transcript, m.Reactions, m.QuotedText = true, "", "", nil, ""
	return true
}

func (e *Engine) updateMessage(chat, id string, notify bool, f func(*domain.Message) bool) (domain.Message, bool) {
	if id == "" {
		return domain.Message{}, false
	}
	m, ok, err := e.d.Store.UpdateMessage(chat, id, f)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		log.Printf("engine: update message %s: %v", id, err)
	}
	if ok && notify {
		e.message(m, "update")
	}
	return m, ok
}

// refreshPreview rewrites the chat's preview when its last message changed.
func (e *Engine) refreshPreview(chat, id string) {
	c, err := e.d.Store.Chat(chat)
	if err != nil || c.LastID != id {
		return
	}
	_, _ = e.d.Store.UpdateChat(chat, func(c *domain.Chat) {
		c.LastBody, c.LastText = "This message was deleted", "This message was deleted"
	})
	e.chatsChanged()
}

// readOnPhone handles the owner reading messages on another device: everything up to the newest
// of them is read.
func (e *Engine) readOnPhone(r whatsapp.Receipt) {
	upTo := r.At
	chat := r.Chat
	for _, id := range r.IDs {
		if m, err := e.d.Store.FindMessage(r.Chat, id); err == nil {
			chat = m.ChatJID
			if m.At.After(upTo) || upTo.IsZero() {
				upTo = m.At
			}
		}
	}
	if upTo.IsZero() {
		upTo = time.Now()
	}
	before, _ := e.d.Store.Chat(chat)
	c, _ := e.d.Store.UpdateChat(chat, func(c *domain.Chat) {
		if upTo.After(c.ReadAt) {
			c.ReadAt = upTo
		}
		c.Unread = e.d.Store.CountUnreadAfter(chat, c.ReadAt)
	})
	if c.Unread != before.Unread {
		e.unread(chat)
		e.chatsChanged()
	}
}

// HandleHistory stores a history sync batch. Nothing in it starts a run or a notification.
func (e *Engine) HandleHistory(h whatsapp.History) {
	e.histMu.Lock()
	defer e.histMu.Unlock()
	added := map[string]int{}
	for _, hc := range h.Chats {
		n := e.storeHistory(hc)
		added[hc.JID] += n
	}
	if e.d.OnHistory != nil {
		e.d.OnHistory(h.Type, added)
	}
	e.chatsChanged()
	e.unread("")
}

// storeHistory stores one chat's old messages, leaving the ones Deputy has alone, and returns
// how many were new.
func (e *Engine) storeHistory(hc whatsapp.HistoryChat) int {
	ms := make([]domain.Message, 0, len(hc.Messages))
	for _, in := range hc.Messages {
		in.History = true
		ms = append(ms, toMessage(in))
	}
	added, err := e.d.Store.SaveMessages(ms)
	if err != nil {
		log.Printf("engine: history for %s: %v", hc.JID, err)
	}
	for _, u := range hc.Updates {
		e.apply(u, false)
	}
	name := hc.Name
	old, err := e.d.Store.Chat(hc.JID)
	if name == "" && (err != nil || old.Name == "" || old.Name == "Group") {
		name = e.d.WA.ChatName(hc.JID)
	}
	var newest *whatsapp.Incoming
	for i := range hc.Messages {
		if newest == nil || !hc.Messages[i].At.Before(newest.At) {
			newest = &hc.Messages[i]
		}
	}
	if len(added) == 0 && !hc.HasMeta && name == "" && err == nil {
		return 0
	}
	_, _ = e.d.Store.UpdateChat(hc.JID, func(c *domain.Chat) {
		c.JID = hc.JID
		c.IsGroup = c.IsGroup || strings.HasSuffix(hc.JID, "@g.us")
		if name != "" && (c.Name == "" || c.Name == "Group" || strings.HasPrefix(c.Name, "+")) {
			c.Name = name
		}
		if newest != nil && newest.At.After(c.LastAt) {
			in := *newest
			if !in.IsGroup && strings.HasSuffix(hc.JID, "@g.us") {
				in.IsGroup = true
			}
			c.LastAt, c.LastText, c.LastID = in.At, preview(in), in.ID
			c.LastBody, c.LastKind, c.LastFromMe = previewBody(in), domain.MsgKind(in.Kind), in.FromMe
			c.LastSender = in.SenderName
			if in.FromMe {
				c.LastSender = ""
			}
		}
		if hc.HasMeta {
			c.Unread, c.Archived, c.Pinned, c.MutedUntil = hc.Unread, hc.Archived, hc.Pinned, hc.MutedUntil
			if hc.Unread == 0 && newest != nil && newest.At.After(c.ReadAt) {
				c.ReadAt = newest.At
			}
		}
	})
	return len(added)
}

// ---- the owner's actions ----

// SendOwner sends what the owner wrote in Deputy's inbox and stores it at once. It counts as the
// owner's own message, typed by them: an agent with "answer my own messages" on runs for it, just
// as for one typed on the phone. Agent replies and SendNow never do.
func (e *Engine) SendOwner(ctx context.Context, chat string, out whatsapp.Outgoing) (domain.Message, error) {
	return e.sendOwner(ctx, chat, out, true)
}

// SendQuiet sends like SendOwner, as the owner, but never starts a run: for messages Claude
// Code sends on the owner's behalf (the MCP server), like SendNow.
func (e *Engine) SendQuiet(ctx context.Context, chat string, out whatsapp.Outgoing) (domain.Message, error) {
	return e.sendOwner(ctx, chat, out, false)
}

func (e *Engine) sendOwner(ctx context.Context, chat string, out whatsapp.Outgoing, typed bool) (domain.Message, error) {
	if strings.TrimSpace(chat) == "" {
		return domain.Message{}, errors.New("choose a chat")
	}
	if out.Kind == "" {
		out.Kind = "text"
	}
	out.Text = strings.TrimSpace(out.Text)
	if out.Kind == "text" && out.Text == "" {
		return domain.Message{}, errors.New("the message is empty")
	}
	var quoted domain.Message
	if out.QuotedID != "" {
		q, err := e.d.Store.FindMessage(chat, out.QuotedID)
		if err != nil {
			return domain.Message{}, errors.New("the message you are replying to is gone")
		}
		quoted = q
		out.QuotedSender, out.QuotedText = q.SenderJID, q.Text
		if q.FromMe {
			out.QuotedSender = ""
		}
	}
	sent, err := e.d.WA.Send(ctx, chat, out)
	if err != nil {
		return domain.Message{}, fmt.Errorf("the message could not be sent: %w", err)
	}
	m := domain.Message{
		ID: sent.ID, ChatJID: chat, FromMe: true, FromUI: typed, At: sent.At, Kind: domain.MsgKind(out.Kind), Text: out.Text,
		Mime: out.Mime, Seconds: out.Seconds, FileName: out.FileName, FileSize: sent.FileSize, MediaRef: sent.MediaRef,
		Mentions: out.Mentions, Status: domain.StatusSent,
		QuotedID: out.QuotedID, QuotedSender: quoted.SenderJID, QuotedText: quoted.Text, QuotedKind: quoted.Kind,
	}
	if m.At.IsZero() {
		m.At = time.Now()
	}
	if m.FileSize == 0 {
		m.FileSize = int64(len(out.Data))
	}
	if m.Kind == domain.KindDocument && m.Text == "" {
		m.Text = m.FileName
	}
	if len(out.Data) > 0 && e.d.Media.Dir != "" {
		if p, err := e.d.Media.Save(chat, m.ID, out.Mime, out.Data); err == nil {
			m.MediaPath = p
		}
	}
	_ = e.d.Store.SaveMessage(m)
	in := whatsapp.Incoming{ID: m.ID, Chat: chat, IsGroup: strings.HasSuffix(chat, "@g.us"), FromMe: true, At: m.At,
		Kind: out.Kind, Text: m.Text, Mime: out.Mime, Seconds: out.Seconds, FileName: out.FileName, QuotedID: out.QuotedID}
	if data := out.Data; len(data) > 0 {
		in.Download = func(context.Context) ([]byte, error) { return data, nil }
	}
	e.touchChat(in, true)
	e.message(m, "new")
	if typed {
		e.route(m, in)
	}
	return m, nil
}

// React puts the owner's emoji on a message, or takes it away with "".
func (e *Engine) React(ctx context.Context, chat, id, emoji string) (domain.Message, error) {
	m, err := e.d.Store.FindMessage(chat, id)
	if err != nil {
		return m, errors.New("that message is gone")
	}
	sender := m.SenderJID
	if m.FromMe {
		sender = ""
	}
	if err := e.d.WA.React(ctx, m.ChatJID, sender, m.ID, emoji); err != nil {
		return m, fmt.Errorf("the reaction could not be sent: %w", err)
	}
	r := domain.Reaction{FromMe: true, Emoji: emoji, At: time.Now()}
	if saved, ok := e.updateMessage(m.ChatJID, m.ID, true, func(m *domain.Message) bool { return m.SetReaction(r) }); ok {
		return saved, nil
	}
	return m, nil
}

// Revoke deletes one of the owner's messages for everyone.
func (e *Engine) Revoke(ctx context.Context, chat, id string) (domain.Message, error) {
	m, err := e.d.Store.FindMessage(chat, id)
	if err != nil {
		return m, errors.New("that message is gone")
	}
	if !m.FromMe {
		return m, errors.New("only your own messages can be deleted for everyone")
	}
	if m.Revoked {
		return m, nil
	}
	if err := e.d.WA.Revoke(ctx, m.ChatJID, m.ID); err != nil {
		return m, fmt.Errorf("the message could not be deleted: %w", err)
	}
	saved, _ := e.updateMessage(m.ChatJID, m.ID, true, revoke)
	e.refreshPreview(m.ChatJID, m.ID)
	return saved, nil
}

// EditWindow is how long after sending WhatsApp lets the owner change a message.
const EditWindow = 15 * time.Minute

// EditOwn changes the text of one of the owner's text messages.
func (e *Engine) EditOwn(ctx context.Context, chat, id, text string) (domain.Message, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return domain.Message{}, errors.New("the message is empty")
	}
	m, err := e.d.Store.FindMessage(chat, id)
	if err != nil {
		return m, errors.New("that message is gone")
	}
	switch {
	case !m.FromMe:
		return m, errors.New("only your own messages can be edited")
	case m.Kind != domain.KindText || m.Revoked:
		return m, errors.New("only text messages can be edited")
	case time.Since(m.At) > EditWindow:
		return m, errors.New("WhatsApp only lets you edit a message in the first 15 minutes")
	}
	if err := e.d.WA.EditText(ctx, m.ChatJID, m.ID, text); err != nil {
		return m, fmt.Errorf("the message could not be edited: %w", err)
	}
	saved, _ := e.updateMessage(m.ChatJID, m.ID, true, func(m *domain.Message) bool {
		m.Text, m.Edited = text, true
		return true
	})
	return saved, nil
}

// MarkChatRead sends read receipts for the chat's unread messages and clears its count.
func (e *Engine) MarkChatRead(chat string) error {
	c, err := e.d.Store.Chat(chat)
	if err != nil {
		return errors.New("that chat is not known")
	}
	unread := e.d.Store.UnreadAfter(chat, c.ReadAt, 300)
	upTo := c.ReadAt
	bySender := map[string][]string{}
	for _, m := range unread {
		if m.At.After(upTo) {
			upTo = m.At
		}
		if m.History && c.Unread == 0 {
			continue // old messages the phone already read
		}
		sender := m.SenderJID
		if sender == "" {
			sender = chat
		}
		bySender[sender] = append(bySender[sender], m.ID)
	}
	for sender, ids := range bySender {
		e.d.WA.MarkRead(chat, sender, ids)
	}
	if upTo.IsZero() {
		upTo = time.Now()
	}
	_, err = e.d.Store.UpdateChat(chat, func(c *domain.Chat) {
		c.Unread = 0
		if upTo.After(c.ReadAt) {
			c.ReadAt = upTo
		}
	})
	if c.Unread != 0 {
		e.unread(chat)
		e.chatsChanged()
	}
	return err
}
