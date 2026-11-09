package backend

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/GabiHert/positron/internal/app"
	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/media"
	"github.com/GabiHert/positron/internal/store"
	"github.com/GabiHert/positron/internal/whatsapp"
)

// The inbox: Deputy as the owner's own WhatsApp client.

const (
	historyTimeout = 45 * time.Second // how long LoadOlder waits for the phone
	historyCount   = 50               // messages asked per LoadOlder
	avatarTTL      = 24 * time.Hour
	maxFileBytes   = 100 << 20
	notifyMaxAge   = time.Hour // older messages (a backlog after sleep) don't pop up
)

type inbox struct {
	mu      sync.Mutex
	active  string // the chat open and visible in the window
	online  bool
	pending map[string]*time.Timer // LoadOlder requests waiting for the phone
	typing  map[string]*time.Timer // auto-pause for SetTyping

	avatarSem    chan struct{}
	avatarFailed sync.Map // jid -> time of the last failure, to back off
}

func (b *Backend) ib() *inbox {
	b.inboxOnce.Do(func() {
		b.inboxState = &inbox{pending: map[string]*time.Timer{}, typing: map[string]*time.Timer{}, avatarSem: make(chan struct{}, 4)}
	})
	return b.inboxState
}

// ---- engine and WhatsApp hooks (wired in run.go) ----

// HandleWAUpdate takes everything WhatsApp reports that is not a new message.
func (b *Backend) HandleWAUpdate(u any) {
	switch v := u.(type) {
	case whatsapp.ChatPresence:
		name := ""
		if v.Sender != "" {
			name = b.d.WA.ChatName(v.Sender)
		}
		b.emit(app.EventPresence, app.PresenceDTO{Chat: v.Chat, JID: v.Sender, Name: name, State: v.State})
	case whatsapp.Presence:
		st := "offline"
		if v.Online {
			st = "online"
		}
		b.emit(app.EventPresence, app.PresenceDTO{JID: v.JID, State: st, LastSeen: ms(v.LastSeen)})
	case whatsapp.History:
		go b.d.Engine.HandleUpdate(v) // big batches must not hold up live messages
	default:
		b.d.Engine.HandleUpdate(u)
	}
}

// OnMessage is the engine's report of a stored message.
func (b *Backend) OnMessage(m domain.Message, change string) {
	b.emit(app.EventMessage, app.MessageEventDTO{Chat: m.ChatJID, Change: change, Message: b.chatMessageDTO(m)})
	if change == "new" {
		b.maybeNotify(m)
	}
}

// OnUnread is the engine's report that a chat's unread count changed ("" for many chats).
func (b *Backend) OnUnread(chat string) {
	total := b.d.Store.TotalUnread(time.Now())
	ev := app.UnreadDTO{Chat: chat, Total: total}
	if chat != "" {
		if c, err := b.d.Store.Chat(chat); err == nil {
			ev.Unread = c.Unread
			if c.Unread == 0 && b.d.ClearNotification != nil {
				b.d.ClearNotification(notificationID(chat))
			}
		}
	}
	b.emit(app.EventUnread, ev)
	if b.d.OnUnreadTotal != nil {
		b.d.OnUnreadTotal(total)
	}
}

// OnReceipt is the engine's report that the owner's messages got further.
func (b *Backend) OnReceipt(chat string, ids []string, status string) {
	b.emit(app.EventReceipt, app.ReceiptEventDTO{Chat: chat, IDs: ids, Status: status})
}

// OnHistory is the engine's report of a stored history sync batch.
func (b *Backend) OnHistory(kind string, added map[string]int) {
	ib := b.ib()
	for chat, n := range added {
		ib.mu.Lock()
		t, waiting := ib.pending[chat]
		if waiting {
			t.Stop()
			delete(ib.pending, chat)
		}
		ib.mu.Unlock()
		if n > 0 || waiting {
			b.emit(app.EventHistory, app.HistoryEventDTO{Chat: chat, Added: n, Kind: kind})
		}
	}
}

func notificationID(chat string) string { return "chat:" + chat }

func (b *Backend) maybeNotify(m domain.Message) {
	if m.FromMe || m.History || m.Revoked || b.d.NotifyChat == nil || time.Since(m.At) > notifyMaxAge {
		return
	}
	if !b.d.Store.Settings().Notify {
		return
	}
	c, err := b.d.Store.Chat(m.ChatJID)
	if err != nil || c.Muted(time.Now()) {
		return
	}
	ib := b.ib()
	ib.mu.Lock()
	active := ib.active
	ib.mu.Unlock()
	if active == m.ChatJID && b.d.Focused != nil && b.d.Focused() {
		return
	}
	title, subtitle := c.Name, ""
	if c.IsGroup {
		subtitle = m.SenderName
	}
	if title == "" {
		title = m.SenderName
	}
	body := snippet(m)
	if body == "" {
		body = "New message"
	}
	b.d.NotifyChat(m.ChatJID, title, subtitle, body)
}

// snippet is a one-line preview of a message without its sender.
func snippet(m domain.Message) string {
	if m.Revoked {
		return "This message was deleted"
	}
	text := strings.TrimSpace(m.Text)
	label := ""
	switch m.Kind {
	case domain.KindImage:
		label = "📷 Photo"
	case domain.KindVideo:
		label = "🎥 Video"
	case domain.KindVoice:
		label = fmt.Sprintf("🎤 Voice note (%d:%02d)", m.Seconds/60, m.Seconds%60)
	case domain.KindAudio:
		label = "🎵 Audio"
	case domain.KindDocument:
		label = "📄 " + firstNonEmpty(m.FileName, "Document")
		if text == m.FileName {
			text = ""
		}
	case domain.KindSticker:
		label = "Sticker"
	}
	if len([]rune(text)) > 200 {
		text = string([]rune(text)[:200]) + "…"
	}
	switch {
	case label == "":
		return text
	case text == "":
		return label
	}
	return label + " " + text
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}

// ---- DTOs ----

// Initials makes one or two letters for an avatar placeholder: "Ana Souza" -> "AS", "+55 51..." -> "#".
func Initials(name string) string {
	var out []rune
	for _, w := range strings.Fields(name) {
		for _, r := range w {
			if unicode.IsLetter(r) {
				out = append(out, unicode.ToUpper(r))
				break
			}
			if unicode.IsDigit(r) || r == '+' {
				break
			}
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) == 0 {
		if strings.TrimSpace(name) == "" {
			return "?"
		}
		return "#"
	}
	return string(out)
}

func (b *Backend) inboxDTO(c domain.Chat, router domain.Router, now time.Time) app.InboxChatDTO {
	d := app.InboxChatDTO{
		JID: c.JID, Name: c.Name, Initials: Initials(c.Name), IsGroup: c.IsGroup, Members: c.Members,
		AvatarURL: b.cachedAvatar(c.JID), LastAt: ms(c.LastAt), LastID: c.LastID, LastText: c.LastBody,
		LastKind: string(c.LastKind), LastSender: c.LastSender, LastFromMe: c.LastFromMe,
		Unread: c.Unread, Muted: c.Muted(now), Pinned: c.Pinned, Archived: c.Archived, Disappearing: c.Disappearing,
	}
	if d.Name == "" {
		d.Name = c.JID
	}
	if d.LastText == "" {
		d.LastText = c.LastText // chats from before the inbox
	}
	if d.Muted {
		d.MutedUntil = ms(c.MutedUntil)
	}
	if c.LastFromMe && c.LastID != "" {
		if m, err := b.d.Store.Message(c.JID, c.LastID); err == nil {
			d.LastStatus = string(m.Status)
		}
	}
	if a, _, ok := router.Owner(c.JID); ok {
		d.AgentID, d.AgentName = a.ID, a.Name
	}
	return d
}

func (b *Backend) mediaURL(p string) string {
	if p == "" {
		return ""
	}
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	if rel := b.d.Media.Rel(p); rel != "" {
		return "/media/" + pathEscape(rel)
	}
	return ""
}

func isMedia(k domain.MsgKind) bool {
	switch k {
	case domain.KindImage, domain.KindVideo, domain.KindVoice, domain.KindAudio, domain.KindDocument, domain.KindSticker:
		return true
	}
	return false
}

// thumbURL serves the JPEG preview inside the stored media details, writing it out on first use.
func (b *Backend) thumbURL(m domain.Message) string {
	if b.d.Media.Dir == "" || len(m.MediaRef) == 0 || m.ViewOnce || (m.Kind != domain.KindImage && m.Kind != domain.KindVideo && m.Kind != domain.KindDocument) {
		return ""
	}
	p := b.d.Media.ThumbPath(m.ChatJID, m.ID)
	if _, err := os.Stat(p); err != nil {
		jpg := whatsapp.Thumbnail(m.MediaRef)
		if len(jpg) == 0 {
			return ""
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil || os.WriteFile(p, jpg, 0o600) != nil {
			return ""
		}
	}
	return b.mediaURL(p)
}

func (b *Backend) senderName(jid string) string {
	if jid == "" {
		return ""
	}
	if b.d.WA.IsMe(jid) {
		return "You"
	}
	return b.d.WA.ChatName(jid)
}

func (b *Backend) chatMessageDTO(m domain.Message) app.ChatMessageDTO {
	d := app.ChatMessageDTO{
		ID: m.ID, ChatJID: m.ChatJID, SenderJID: m.SenderJID, SenderName: m.SenderName, FromMe: m.FromMe,
		FromAgent: m.FromAgent, FromUI: m.FromUI, At: ms(m.At), Kind: string(m.Kind), Text: m.Text, Transcript: m.Transcript,
		Mentions: append([]string{}, m.Mentions...), MentionsMe: m.MentionsMe, Reactions: []app.ReactionDTO{},
		Status: string(m.Status), Edited: m.Edited, Revoked: m.Revoked, History: m.History, ViewOnce: m.ViewOnce,
	}
	if m.FromAgent {
		d.AgentName = m.SenderName
	}
	if m.FromMe && !m.FromAgent {
		d.SenderName = ""
	}
	if !m.FromMe {
		d.Status = ""
	}
	if m.Kind == domain.KindDocument && d.Text == m.FileName {
		d.Text = ""
	}
	if m.QuotedID != "" {
		q := &app.QuotedDTO{ID: m.QuotedID, SenderJID: m.QuotedSender, Kind: string(m.QuotedKind), Snippet: m.QuotedText}
		if qm, err := b.d.Store.Message(m.ChatJID, m.QuotedID); err == nil {
			q.Found, q.FromMe, q.SenderJID, q.Kind, q.Snippet = true, qm.FromMe, qm.SenderJID, string(qm.Kind), snippet(qm)
			q.SenderName = qm.SenderName
			if qm.FromMe && !qm.FromAgent {
				q.SenderName = "You"
			}
		} else {
			q.FromMe = m.QuotedSender == "" || b.d.WA.IsMe(m.QuotedSender)
			if q.FromMe {
				q.SenderName = "You"
			} else {
				q.SenderName = b.senderName(m.QuotedSender)
			}
		}
		d.Quoted = q
	}
	if len(m.Reactions) > 0 {
		byEmoji := map[string]*app.ReactionDTO{}
		var order []string
		for _, r := range m.Reactions {
			rd := byEmoji[r.Emoji]
			if rd == nil {
				rd = &app.ReactionDTO{Emoji: r.Emoji, Senders: []string{}}
				byEmoji[r.Emoji] = rd
				order = append(order, r.Emoji)
			}
			rd.Count++
			name := r.SenderName
			if r.FromMe {
				name, rd.FromMe = "You", true
			} else if name == "" {
				name = b.senderName(r.Sender)
			}
			rd.Senders = append(rd.Senders, name)
		}
		for _, e := range order {
			d.Reactions = append(d.Reactions, *byEmoji[e])
		}
	}
	if isMedia(m.Kind) && !m.Revoked {
		d.Media = &app.MediaDTO{Mime: m.Mime, Seconds: m.Seconds, FileName: m.FileName, Size: m.FileSize,
			URL: b.mediaURL(m.MediaPath), ThumbURL: b.thumbURL(m), Downloadable: len(m.MediaRef) > 0}
	}
	return d
}

// ---- reading ----

// ListInbox lists chats: "" (not archived, with messages), "archived", "unread", "groups", "direct" or "all".
func (b *Backend) ListInbox(filter string) []app.InboxChatDTO {
	router := domain.NewRouter(b.d.Store.Agents())
	now := time.Now()
	out := []app.InboxChatDTO{}
	for _, c := range b.d.Store.Chats() {
		keep := false
		switch filter {
		case "all":
			keep = true
		case "archived":
			keep = c.Archived
		case "unread":
			keep = c.Unread > 0
		case "groups":
			keep = c.IsGroup && !c.Archived
		case "direct":
			keep = !c.IsGroup && !c.Archived
		default:
			keep = !c.Archived && (!c.LastAt.IsZero() || c.Pinned)
		}
		if keep {
			out = append(out, b.inboxDTO(c, router, now))
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].LastAt > out[j].LastAt
	})
	return out
}

// GetInboxChat returns one chat's inbox row.
func (b *Backend) GetInboxChat(jid string) (app.InboxChatDTO, error) {
	c, err := b.d.Store.Chat(jid)
	if err != nil {
		return app.InboxChatDTO{}, errors.New("that chat is not known")
	}
	return b.inboxDTO(c, domain.NewRouter(b.d.Store.Agents()), time.Now()), nil
}

// ListMessages pages back through a chat, newest first.
func (b *Backend) ListMessages(chat, before string, limit int) (app.MessagePageDTO, error) {
	if strings.TrimSpace(chat) == "" {
		return app.MessagePageDTO{Messages: []app.ChatMessageDTO{}}, errors.New("choose a chat")
	}
	ms, next := b.d.Store.MessagesBefore(chat, store.ParseCursor(before), limit)
	page := app.MessagePageDTO{Messages: make([]app.ChatMessageDTO, 0, len(ms)), Cursor: next.String()}
	page.HasMore = page.Cursor != ""
	for _, m := range ms {
		page.Messages = append(page.Messages, b.chatMessageDTO(m))
	}
	return page, nil
}

// GetMessage returns one stored message.
func (b *Backend) GetMessage(chat, id string) (app.ChatMessageDTO, error) {
	m, err := b.d.Store.FindMessage(chat, id)
	if err != nil {
		return app.ChatMessageDTO{}, errors.New("that message is gone")
	}
	return b.chatMessageDTO(m), nil
}

// MarkChatRead sends read receipts and clears the chat's unread count.
func (b *Backend) MarkChatRead(chat string) error { return b.d.Engine.MarkChatRead(chat) }

// GetUnreadTotal is the unread count of chats that are not muted.
func (b *Backend) GetUnreadTotal() int { return b.d.Store.TotalUnread(time.Now()) }

// SetActiveChat records the open chat: it gets no notifications, its contact's presence is
// followed, and the owner shows online while a chat is open (as WhatsApp Web does).
func (b *Backend) SetActiveChat(jid string) {
	ib := b.ib()
	ib.mu.Lock()
	ib.active = jid
	goOnline := jid != "" && !ib.online
	goOffline := jid == "" && ib.online
	ib.online = jid != ""
	ib.mu.Unlock()
	go func() {
		if goOnline {
			b.d.WA.SetOnline(true)
		}
		if goOffline {
			b.d.WA.SetOnline(false)
		}
		if jid != "" {
			b.d.WA.SubscribePresence(jid)
			if b.d.ClearNotification != nil {
				b.d.ClearNotification(notificationID(jid))
			}
		}
	}()
}

// SetTyping shows or clears "typing…" for the owner; it clears itself after 10 quiet seconds.
func (b *Backend) SetTyping(chat string, on bool) {
	if chat == "" {
		return
	}
	ib := b.ib()
	ib.mu.Lock()
	if t := ib.typing[chat]; t != nil {
		t.Stop()
		delete(ib.typing, chat)
	}
	if on {
		ib.typing[chat] = time.AfterFunc(10*time.Second, func() { b.SetTyping(chat, false) })
	}
	ib.mu.Unlock()
	go b.d.WA.Typing(chat, on, false)
}

// SearchMessages finds messages containing query, newest first.
func (b *Backend) SearchMessages(query, chat string, limit int) []app.SearchHitDTO {
	out := []app.SearchHitDTO{}
	chats := map[string]domain.Chat{}
	for _, m := range b.d.Store.SearchMessages(query, chat, limit) {
		c, ok := chats[m.ChatJID]
		if !ok {
			c, _ = b.d.Store.Chat(m.ChatJID)
			chats[m.ChatJID] = c
		}
		out = append(out, app.SearchHitDTO{ChatJID: m.ChatJID, ChatName: firstNonEmpty(c.Name, m.ChatJID), IsGroup: c.IsGroup, Message: b.chatMessageDTO(m)})
	}
	return out
}

// LoadOlder asks the phone for messages before the oldest stored one; EventHistory reports the answer.
func (b *Backend) LoadOlder(chat string) error {
	oldest, err := b.d.Store.OldestMessage(chat)
	if err != nil {
		return errors.New("there are no messages in this chat to start from")
	}
	ib := b.ib()
	ib.mu.Lock()
	if _, waiting := ib.pending[chat]; waiting {
		ib.mu.Unlock()
		return nil
	}
	ib.pending[chat] = time.AfterFunc(historyTimeout, func() {
		ib.mu.Lock()
		_, still := ib.pending[chat]
		delete(ib.pending, chat)
		ib.mu.Unlock()
		if still {
			b.emit(app.EventHistory, app.HistoryEventDTO{Chat: chat, Kind: "on-demand",
				Error: "Your phone didn't send older messages. Make sure it is online with WhatsApp open, then try again."})
		}
	})
	ib.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := b.d.WA.RequestHistory(ctx, oldest.ChatJID, oldest.ID, oldest.FromMe, oldest.At, historyCount); err != nil {
		ib.mu.Lock()
		if t := ib.pending[chat]; t != nil {
			t.Stop()
			delete(ib.pending, chat)
		}
		ib.mu.Unlock()
		return fmt.Errorf("could not ask the phone for older messages: %w", err)
	}
	return nil
}

// ---- media ----

func (b *Backend) mediaFile(ctx context.Context, chat, id string) (domain.Message, string, error) {
	m, err := b.d.Store.FindMessage(chat, id)
	if err != nil {
		return m, "", errors.New("that message is gone")
	}
	if m.MediaPath != "" {
		if _, err := os.Stat(m.MediaPath); err == nil {
			return m, m.MediaPath, nil
		}
	}
	if m.Revoked {
		return m, "", errors.New("this message was deleted")
	}
	if len(m.MediaRef) == 0 {
		return m, "", errors.New("This file arrived before Deputy kept files, so it can only be opened on your phone.")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	data, err := b.d.WA.DownloadRef(ctx, m.MediaRef)
	if err != nil {
		return m, "", fmt.Errorf("could not download the file: %w", err)
	}
	ext := strings.ToLower(filepath.Ext(m.FileName))
	if ext == "" {
		ext = media.Ext(m.Mime)
	}
	p, err := b.d.Media.SaveAs(m.ChatJID, m.ID, ext, data)
	if err != nil {
		return m, "", err
	}
	saved, ok, _ := b.d.Store.UpdateMessage(m.ChatJID, m.ID, func(s *domain.Message) bool {
		s.MediaPath = p
		return true
	})
	if ok {
		b.OnMessage(saved, "update")
	}
	return saved, p, nil
}

// GetMedia downloads a message's file when needed and returns its URL.
func (b *Backend) GetMedia(ctx context.Context, chat, id string) (string, error) {
	_, p, err := b.mediaFile(ctx, chat, id)
	if err != nil {
		return "", err
	}
	if u := b.mediaURL(p); u != "" {
		return u, nil
	}
	return "", errors.New("the file is outside Deputy's media folder")
}

// OpenMedia downloads a message's file when needed and opens it in its macOS app.
func (b *Backend) OpenMedia(ctx context.Context, chat, id string) error {
	_, p, err := b.mediaFile(ctx, chat, id)
	if err != nil {
		return err
	}
	return exec.Command("/usr/bin/open", p).Run()
}

// ---- sending ----

func (b *Backend) sent(m domain.Message, err error) (app.ChatMessageDTO, error) {
	if err != nil {
		return app.ChatMessageDTO{}, err
	}
	return b.chatMessageDTO(m), nil
}

// send sends as the owner: typed in Deputy's window (an "answer my own messages" agent runs for
// it), or on their behalf from Claude Code (never starts a run).
func (b *Backend) send(ctx context.Context, chat string, out whatsapp.Outgoing, typed bool) (app.ChatMessageDTO, error) {
	if typed {
		return b.sent(b.d.Engine.SendOwner(ctx, chat, out))
	}
	return b.sent(b.d.Engine.SendQuiet(ctx, chat, out))
}

// SendText sends a text message the owner typed in Deputy.
func (b *Backend) SendText(ctx context.Context, in app.SendTextDTO) (app.ChatMessageDTO, error) {
	return b.send(ctx, in.Chat, whatsapp.Outgoing{Kind: "text", Text: in.Text, QuotedID: in.QuotedID, Mentions: in.Mentions}, true)
}

// ReplyFromClaude sends text as the owner, quoting a message when quotedID is set, for Claude
// Code. It never starts an agent's run.
func (b *Backend) ReplyFromClaude(ctx context.Context, chat, quotedID, text string) (app.ChatMessageDTO, error) {
	return b.send(ctx, chat, whatsapp.Outgoing{Kind: "text", Text: text, QuotedID: quotedID}, false)
}

// SendFileFromClaude sends a file as the owner for Claude Code. It never starts an agent's run.
func (b *Backend) SendFileFromClaude(ctx context.Context, in app.SendFileDTO) (app.ChatMessageDTO, error) {
	return b.sendFile(ctx, in, false)
}

func decode64(s string) ([]byte, error) {
	if i := strings.Index(s, ";base64,"); i >= 0 && strings.HasPrefix(s, "data:") {
		s = s[i+len(";base64,"):]
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
	if err != nil {
		return nil, errors.New("the file data is not valid base64")
	}
	return data, nil
}

// FileKind picks how a file goes out: a photo, video or audio message when WhatsApp can play it, else a document.
func FileKind(mimeType string) string {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	switch m {
	case "image/jpeg", "image/png", "image/webp":
		return "image"
	case "video/mp4", "video/3gpp":
		return "video"
	case "audio/ogg", "audio/mpeg", "audio/mp4", "audio/aac", "audio/amr", "audio/x-m4a":
		return "audio"
	}
	return "document"
}

// SendFile sends a photo, video, audio file or document the owner picked in Deputy.
func (b *Backend) SendFile(ctx context.Context, in app.SendFileDTO) (app.ChatMessageDTO, error) {
	return b.sendFile(ctx, in, true)
}

func (b *Backend) sendFile(ctx context.Context, in app.SendFileDTO, typed bool) (app.ChatMessageDTO, error) {
	var data []byte
	name := strings.TrimSpace(in.FileName)
	switch {
	case in.Path != "":
		p := expand(in.Path)
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			return app.ChatMessageDTO{}, errors.New("that file is not there")
		}
		if st.Size() > maxFileBytes {
			return app.ChatMessageDTO{}, errors.New("the file is too big: Deputy sends files up to 100 MB")
		}
		if data, err = os.ReadFile(p); err != nil {
			return app.ChatMessageDTO{}, err
		}
		if name == "" {
			name = filepath.Base(p)
		}
	case in.DataBase64 != "":
		var err error
		if data, err = decode64(in.DataBase64); err != nil {
			return app.ChatMessageDTO{}, err
		}
		if len(data) > maxFileBytes {
			return app.ChatMessageDTO{}, errors.New("the file is too big: Deputy sends files up to 100 MB")
		}
	default:
		return app.ChatMessageDTO{}, errors.New("choose a file to send")
	}
	if len(data) == 0 {
		return app.ChatMessageDTO{}, errors.New("the file is empty")
	}
	mt := strings.TrimSpace(in.Mime)
	if mt == "" {
		mt = mime.TypeByExtension(strings.ToLower(filepath.Ext(name)))
	}
	if mt == "" {
		mt = http.DetectContentType(data)
	}
	if name == "" {
		name = "file" + media.Ext(mt)
	}
	kind := in.Kind
	switch kind {
	case "image", "video", "audio", "document":
	default:
		kind = FileKind(mt)
	}
	return b.send(ctx, in.Chat, whatsapp.Outgoing{Kind: kind, Text: in.Caption, Data: data, Mime: mt, FileName: name, QuotedID: in.QuotedID}, typed)
}

// SendVoice converts a recording to an Ogg Opus voice note when needed and sends it as the owner.
func (b *Backend) SendVoice(ctx context.Context, in app.SendVoiceDTO) (app.ChatMessageDTO, error) {
	data, err := decode64(in.DataBase64)
	if err != nil {
		return app.ChatMessageDTO{}, err
	}
	var env []string
	if b.d.Env != nil {
		env = b.d.Env()
	}
	cctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	ogg, secs, err := media.ToVoiceNote(cctx, env, os.TempDir(), data, in.Mime)
	if err != nil {
		return app.ChatMessageDTO{}, err
	}
	if secs <= 1 && in.Seconds > 1 {
		secs = in.Seconds
	}
	return b.send(ctx, in.Chat, whatsapp.Outgoing{Kind: "voice", Data: ogg, Mime: "audio/ogg; codecs=opus", Seconds: secs, QuotedID: in.QuotedID}, true)
}

// React puts the owner's emoji on a message ("" removes it).
func (b *Backend) React(ctx context.Context, chat, id, emoji string) (app.ChatMessageDTO, error) {
	return b.sent(b.d.Engine.React(ctx, chat, id, emoji))
}

// DeleteForEveryone revokes one of the owner's messages.
func (b *Backend) DeleteForEveryone(ctx context.Context, chat, id string) (app.ChatMessageDTO, error) {
	return b.sent(b.d.Engine.Revoke(ctx, chat, id))
}

// EditMessage edits one of the owner's text messages.
func (b *Backend) EditMessage(ctx context.Context, chat, id, text string) (app.ChatMessageDTO, error) {
	return b.sent(b.d.Engine.EditOwn(ctx, chat, id, text))
}

// ---- contacts and new chats ----

func digitsOf(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, s)
}

// ListContacts lists contacts and groups matching query (name, push name or number), by name.
func (b *Backend) ListContacts(query string) []app.ContactDTO {
	q := strings.ToLower(strings.TrimSpace(query))
	qd := digitsOf(q)
	chats := map[string]domain.Chat{}
	for _, c := range b.d.Store.Chats() {
		chats[c.JID] = c
	}
	seen := map[string]bool{}
	out := []app.ContactDTO{}
	add := func(d app.ContactDTO) {
		if seen[d.JID] {
			return
		}
		if q != "" {
			hay := strings.ToLower(d.Name + " " + d.PushName)
			if !strings.Contains(hay, q) && (qd == "" || len(qd) < 3 || !strings.Contains(digitsOf(d.Phone), qd)) {
				return
			}
		}
		seen[d.JID] = true
		c, ok := chats[d.JID]
		d.HasChat = ok && !c.LastAt.IsZero()
		d.Initials = Initials(d.Name)
		out = append(out, d)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if cs, err := b.d.WA.Contacts(ctx); err == nil {
		for _, ct := range cs {
			name := firstNonEmpty(ct.Name, ct.BusinessName, ct.PushName, ct.Phone)
			if name == "" {
				continue
			}
			add(app.ContactDTO{JID: ct.JID, Name: name, PushName: ct.PushName, Phone: ct.Phone})
		}
	}
	for _, c := range chats {
		d := app.ContactDTO{JID: c.JID, Name: firstNonEmpty(c.Name, c.JID), IsGroup: c.IsGroup}
		if strings.HasSuffix(c.JID, "@s.whatsapp.net") {
			d.Phone = "+" + strings.SplitN(c.JID, "@", 2)[0]
		}
		add(d)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return out
}

// StartChat checks a number is on WhatsApp and returns its chat, created empty when new.
func (b *Backend) StartChat(ctx context.Context, phone string) (app.InboxChatDTO, error) {
	digits := digitsOf(phone)
	if len(digits) < 8 {
		return app.InboxChatDTO{}, errors.New("type the full number with the country code, like +55 51 99999 1234")
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	jid, err := b.d.WA.LookupPhone(cctx, digits)
	if err != nil {
		return app.InboxChatDTO{}, err
	}
	name := b.d.WA.ChatName(jid)
	c, err := b.d.Store.UpdateChat(jid, func(c *domain.Chat) {
		c.JID = jid
		if c.Name == "" {
			c.Name = name
		}
	})
	if err != nil {
		return app.InboxChatDTO{}, err
	}
	b.emit(app.EventChats, nil)
	return b.inboxDTO(c, domain.NewRouter(b.d.Store.Agents()), time.Now()), nil
}

// ---- profile pictures ----

type avatarEntry struct {
	ID      string    `json:"id"`
	File    string    `json:"file"` // relative to the media folder, "" when there is no picture
	Checked time.Time `json:"checked"`
}

func avatarKey(jid string) string { return "avatar:" + jid }

func (b *Backend) avatarEntry(jid string) (avatarEntry, bool) {
	v, ok := b.d.Store.KV(avatarKey(jid))
	if !ok {
		return avatarEntry{}, false
	}
	var e avatarEntry
	if json.Unmarshal([]byte(v), &e) != nil {
		return avatarEntry{}, false
	}
	return e, true
}

func (b *Backend) avatarURL(e avatarEntry) string {
	if e.File == "" {
		return ""
	}
	p := filepath.Join(b.d.Media.Dir, filepath.FromSlash(e.File))
	if _, err := os.Stat(p); err != nil {
		return ""
	}
	return "/media/" + pathEscape(e.File) + "?v=" + pathEscape(e.ID)
}

func (b *Backend) cachedAvatar(jid string) string {
	if e, ok := b.avatarEntry(jid); ok {
		return b.avatarURL(e)
	}
	return ""
}

// GetAvatar returns the URL of a chat's picture, fetching it at most once a day; "" when there is none.
func (b *Backend) GetAvatar(ctx context.Context, jid string) (string, error) {
	if jid == "" || b.d.Media.Dir == "" {
		return "", nil
	}
	e, have := b.avatarEntry(jid)
	url := b.avatarURL(e)
	fresh := have && time.Since(e.Checked) < avatarTTL && (e.File == "" || url != "")
	if fresh {
		return url, nil
	}
	ib := b.ib()
	if t, ok := ib.avatarFailed.Load(jid); ok && time.Since(t.(time.Time)) < 10*time.Minute {
		return url, nil
	}
	select {
	case ib.avatarSem <- struct{}{}:
		defer func() { <-ib.avatarSem }()
	case <-ctx.Done():
		return url, nil
	}
	cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	existing := e.ID
	if url == "" {
		existing = "" // the file is gone: fetch it again
	}
	picURL, id, err := b.d.WA.ProfilePicture(cctx, jid, existing)
	switch {
	case errors.Is(err, whatsapp.ErrNoPicture):
		if e.File != "" {
			_ = os.Remove(filepath.Join(b.d.Media.Dir, filepath.FromSlash(e.File)))
		}
		b.saveAvatar(jid, avatarEntry{Checked: time.Now()})
		return "", nil
	case err != nil:
		ib.avatarFailed.Store(jid, time.Now())
		return url, nil
	case picURL == "" && id == e.ID && url != "":
		e.Checked = time.Now()
		b.saveAvatar(jid, e)
		return url, nil
	}
	data, err := fetchURL(cctx, picURL)
	if err != nil {
		ib.avatarFailed.Store(jid, time.Now())
		log.Printf("inbox: avatar %s: %v", jid, err)
		return url, nil
	}
	p := b.d.Media.AvatarPath(jid)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return url, nil
	}
	if err := os.WriteFile(p, data, 0o600); err != nil {
		return url, nil
	}
	e = avatarEntry{ID: id, File: b.d.Media.Rel(p), Checked: time.Now()}
	b.saveAvatar(jid, e)
	return b.avatarURL(e), nil
}

func (b *Backend) saveAvatar(jid string, e avatarEntry) {
	v, _ := json.Marshal(e)
	_ = b.d.Store.SetKV(avatarKey(jid), string(v))
}

func fetchURL(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 5<<20))
}
