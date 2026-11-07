package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/proto/waWeb"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

// Updates delivered through OnUpdate.

// Reaction puts (or, with Emoji "", takes away) someone's emoji on a message.
type Reaction struct {
	Chat, TargetID     string
	Sender, SenderName string
	FromMe             bool
	Emoji              string
	At                 time.Time
}

// Revoke deletes a message for everyone.
type Revoke struct {
	Chat, TargetID, Sender string
	FromMe                 bool
}

// Edit replaces a message's text.
type Edit struct {
	Chat, TargetID, Text string
	At                   time.Time
}

// Receipt says how far the owner's messages got ("delivered", "read", "played"), or, with
// FromMe, that the owner read the messages on another device (their phone).
type Receipt struct {
	Chat   string
	IDs    []string
	Status string
	FromMe bool
	At     time.Time
}

// ChatRead is a chat marked read (or unread) on another device.
type ChatRead struct {
	Chat string
	Read bool
	At   time.Time
}

// ChatMeta is a chat muted, pinned or archived on another device.
type ChatMeta struct {
	Chat  string
	Field string // "mute" | "pin" | "archive"
	On    bool
	Until time.Time // "mute" only: zero with On means forever
}

// ChatPresence is someone typing or recording in a chat.
type ChatPresence struct {
	Chat, Sender string
	State        string // "composing" | "recording" | "paused"
}

// Presence is a contact going online or offline.
type Presence struct {
	JID      string
	Online   bool
	LastSeen time.Time
}

// History is one batch of a history sync from the phone.
type History struct {
	Type  string // "initial", "recent", "full", "on-demand", ...
	Chats []HistoryChat
}

// HistoryChat is one chat in a history sync. Its messages are oldest first.
type HistoryChat struct {
	JID        string
	Name       string
	HasMeta    bool // the fields below come from the phone (not in on-demand syncs)
	Unread     int
	Archived   bool
	Pinned     bool
	MutedUntil time.Time
	Messages   []Incoming
	Updates    []any // reactions, edits and deletions inside the batch
}

func textOf(m *waE2E.Message) (text, kind string) {
	switch {
	case m == nil:
		return "", ""
	case m.GetConversation() != "":
		return m.GetConversation(), "text"
	case m.GetExtendedTextMessage() != nil:
		return m.GetExtendedTextMessage().GetText(), "text"
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetCaption(), "image"
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetCaption(), "video"
	case m.GetAudioMessage() != nil:
		if m.GetAudioMessage().GetPTT() {
			return "", "voice"
		}
		return "", "audio"
	case m.GetDocumentMessage() != nil:
		d := m.GetDocumentMessage()
		if d.GetCaption() != "" {
			return d.GetCaption(), "document"
		}
		return d.GetFileName(), "document"
	case m.GetStickerMessage() != nil:
		return "", "sticker"
	}
	return "", "other"
}

func receiptStatus(t types.ReceiptType) string {
	switch t {
	case types.ReceiptTypeDelivered:
		return "delivered"
	case types.ReceiptTypeRead, types.ReceiptTypeReadSelf:
		return "read"
	case types.ReceiptTypePlayed, types.ReceiptTypePlayedSelf:
		return "played"
	}
	return ""
}

func (c *Client) handleInbox(evt any) {
	switch e := evt.(type) {
	case *events.Receipt:
		st := receiptStatus(e.Type)
		if st == "" || len(e.MessageIDs) == 0 {
			return
		}
		self := e.Type == types.ReceiptTypeReadSelf || e.Type == types.ReceiptTypePlayedSelf
		if !self && e.IsFromMe {
			return // the owner's other devices confirming delivery to themselves
		}
		c.update(Receipt{Chat: e.Chat.String(), IDs: append([]string(nil), e.MessageIDs...), Status: st, FromMe: self, At: e.Timestamp})
	case *events.MarkChatAsRead:
		c.update(ChatRead{Chat: e.JID.String(), Read: e.Action.GetRead(), At: e.Timestamp})
	case *events.Mute:
		u := ChatMeta{Chat: e.JID.String(), Field: "mute", On: e.Action.GetMuted()}
		if end := e.Action.GetMuteEndTimestamp(); u.On && end > 0 {
			u.Until = time.UnixMilli(end)
		}
		c.update(u)
	case *events.Pin:
		c.update(ChatMeta{Chat: e.JID.String(), Field: "pin", On: e.Action.GetPinned()})
	case *events.Archive:
		c.update(ChatMeta{Chat: e.JID.String(), Field: "archive", On: e.Action.GetArchived()})
	case *events.ChatPresence:
		st := "paused"
		if e.State == types.ChatPresenceComposing {
			st = "composing"
			if e.Media == types.ChatPresenceMediaAudio {
				st = "recording"
			}
		}
		c.update(ChatPresence{Chat: e.Chat.String(), Sender: e.Sender.ToNonAD().String(), State: st})
	case *events.Presence:
		c.update(Presence{JID: e.From.ToNonAD().String(), Online: !e.Unavailable, LastSeen: e.LastSeen})
	case *events.HistorySync:
		if h, ok := c.history(e.Data); ok {
			c.update(h)
		}
	case *events.GroupInfo:
		if e.Ephemeral != nil {
			c.learnGroupTimer(e.JID, *e.Ephemeral, e.Timestamp)
		}
	case *events.JoinedGroup:
		c.learnGroupTimer(e.JID, e.GroupEphemeral, time.Time{})
	}
}

var syncTypes = map[waHistorySync.HistorySync_HistorySyncType]string{
	waHistorySync.HistorySync_INITIAL_BOOTSTRAP: "initial",
	waHistorySync.HistorySync_RECENT:            "recent",
	waHistorySync.HistorySync_FULL:              "full",
	waHistorySync.HistorySync_ON_DEMAND:         "on-demand",
}

func webStatus(s waWeb.WebMessageInfo_Status) string {
	switch s {
	case waWeb.WebMessageInfo_SERVER_ACK:
		return "sent"
	case waWeb.WebMessageInfo_DELIVERY_ACK:
		return "delivered"
	case waWeb.WebMessageInfo_READ:
		return "read"
	case waWeb.WebMessageInfo_PLAYED:
		return "played"
	}
	return ""
}

// history parses a history sync batch. Messages in it are marked History so nothing reacts to them.
func (c *Client) history(d *waHistorySync.HistorySync) (History, bool) {
	typ, ok := syncTypes[d.GetSyncType()]
	if !ok {
		return History{}, false // status updates, push names, ...
	}
	cli, err := c.client()
	if err != nil {
		return History{}, false
	}
	h := History{Type: typ}
	for _, conv := range d.GetConversations() {
		jid, err := types.ParseJID(conv.GetID())
		if err != nil || jid.Server == types.BroadcastServer || jid.Server == types.NewsletterServer {
			continue
		}
		hc := HistoryChat{JID: jid.String(), Name: conv.GetName()}
		if hc.Name == "" {
			hc.Name = conv.GetDisplayName()
		}
		if typ != "on-demand" {
			hc.HasMeta = true
			hc.Unread = int(conv.GetUnreadCount())
			if conv.GetMarkedAsUnread() && hc.Unread == 0 {
				hc.Unread = 1
			}
			hc.Archived, hc.Pinned = conv.GetArchived(), conv.GetPinned() > 0
			if end := conv.GetMuteEndTime(); end > 0 {
				hc.MutedUntil = time.Unix(int64(end), 0)
			}
			// The phone's own record of the chat's disappearing messages, when it sends one.
			if conv.EphemeralExpiration != nil || conv.GetEphemeralSettingTimestamp() > 0 {
				c.learnTimer(hc.JID, timerSeen{Seconds: conv.GetEphemeralExpiration(), SetAt: conv.GetEphemeralSettingTimestamp(), Authoritative: true})
			}
		}
		for _, hm := range conv.GetMessages() {
			web := hm.GetMessage()
			if web == nil {
				continue
			}
			if web.GetMessageStubType() == waWeb.WebMessageInfo_REVOKE {
				key := web.GetKey()
				hc.Messages = append(hc.Messages, Incoming{ID: key.GetID(), Chat: hc.JID, IsGroup: jid.Server == types.GroupServer,
					FromMe: key.GetFromMe(), At: time.Unix(int64(web.GetMessageTimestamp()), 0), Kind: "text", History: true, Revoked: true})
				continue
			}
			if web.GetMessage() == nil {
				continue
			}
			evt, err := cli.ParseWebMessage(jid, web)
			if err != nil {
				continue
			}
			in, upd := c.classify(evt)
			if _, ok := upd.(Disappearing); ok {
				continue // an old setting change: the chat's record above is the current one
			}
			switch {
			case upd != nil:
				hc.Updates = append(hc.Updates, upd)
			case in.ID != "":
				in.History = true
				if in.FromMe {
					in.Status = webStatus(web.GetStatus())
				}
				hc.Messages = append(hc.Messages, in)
				for _, r := range web.GetReactions() {
					key := r.GetKey()
					rx := Reaction{Chat: hc.JID, TargetID: in.ID, FromMe: key.GetFromMe(), Emoji: r.GetText(), At: time.UnixMilli(r.GetSenderTimestampMS())}
					if !rx.FromMe {
						rx.Sender = key.GetParticipant()
						if rx.Sender == "" {
							rx.Sender = hc.JID
						}
					}
					hc.Updates = append(hc.Updates, rx)
				}
			}
		}
		sort.SliceStable(hc.Messages, func(i, j int) bool { return hc.Messages[i].At.Before(hc.Messages[j].At) })
		h.Chats = append(h.Chats, hc)
	}
	return h, true
}

// ---- sending ----

// Outgoing is a message the owner sends from Deputy.
type Outgoing struct {
	Kind     string // "text" | "image" | "video" | "document" | "audio" | "voice"
	Text     string // the body, or the media caption
	Data     []byte
	Mime     string
	FileName string
	Seconds  int
	// Replying: the quoted message, its sender's JID ("" when it is the owner's) and a snippet.
	QuotedID     string
	QuotedSender string
	QuotedText   string
	Mentions     []string // JIDs
}

// Sent is what WhatsApp took.
type Sent struct {
	ID       string
	At       time.Time
	MediaRef []byte
	FileSize int64
}

func (c *Client) ownJID(cli *whatsmeow.Client) types.JID {
	if cli.Store.ID == nil {
		return types.EmptyJID
	}
	return cli.Store.ID.ToNonAD()
}

func (c *Client) contextInfo(cli *whatsmeow.Client, out Outgoing) *waE2E.ContextInfo {
	if out.QuotedID == "" && len(out.Mentions) == 0 {
		return nil
	}
	ci := &waE2E.ContextInfo{MentionedJID: out.Mentions}
	if out.QuotedID != "" {
		ci.StanzaID = proto.String(out.QuotedID)
		p := out.QuotedSender
		if p == "" {
			p = c.ownJID(cli).String()
		}
		ci.Participant = proto.String(p)
		ci.QuotedMessage = &waE2E.Message{Conversation: proto.String(out.QuotedText)}
	}
	return ci
}

// Send sends a message from the owner: text (optionally quoting another), a photo, video,
// document, audio file or voice note (Ogg Opus).
func (c *Client) Send(ctx context.Context, chat string, out Outgoing) (Sent, error) {
	cli, err := c.client()
	if err != nil {
		return Sent{}, err
	}
	j, err := parseChat(chat)
	if err != nil {
		return Sent{}, err
	}
	ci := c.contextInfo(cli, out)
	msg := &waE2E.Message{}
	var sent Sent
	upload := func(kind whatsmeow.MediaType) (whatsmeow.UploadResponse, error) {
		if len(out.Data) == 0 {
			return whatsmeow.UploadResponse{}, errors.New("the file is empty")
		}
		up, err := cli.Upload(ctx, out.Data, kind)
		if err != nil {
			return up, fmt.Errorf("could not upload the file: %w", err)
		}
		sent.FileSize = int64(up.FileLength)
		return up, nil
	}
	switch out.Kind {
	case "", "text":
		if ci == nil {
			msg.Conversation = proto.String(out.Text)
		} else {
			msg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{Text: proto.String(out.Text), ContextInfo: ci}
		}
	case "image":
		up, err := upload(whatsmeow.MediaImage)
		if err != nil {
			return Sent{}, err
		}
		msg.ImageMessage = &waE2E.ImageMessage{Caption: proto.String(out.Text), Mimetype: proto.String(out.Mime), ContextInfo: ci,
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
	case "video":
		up, err := upload(whatsmeow.MediaVideo)
		if err != nil {
			return Sent{}, err
		}
		msg.VideoMessage = &waE2E.VideoMessage{Caption: proto.String(out.Text), Mimetype: proto.String(out.Mime), ContextInfo: ci,
			Seconds: proto.Uint32(uint32(max(out.Seconds, 0))),
			URL:     proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
	case "audio", "voice":
		up, err := upload(whatsmeow.MediaAudio)
		if err != nil {
			return Sent{}, err
		}
		mime := out.Mime
		if out.Kind == "voice" {
			mime = "audio/ogg; codecs=opus"
		}
		msg.AudioMessage = &waE2E.AudioMessage{Mimetype: proto.String(mime), PTT: proto.Bool(out.Kind == "voice"), ContextInfo: ci,
			Seconds: proto.Uint32(uint32(max(out.Seconds, 0))),
			URL:     proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
	case "document":
		up, err := upload(whatsmeow.MediaDocument)
		if err != nil {
			return Sent{}, err
		}
		name := out.FileName
		if name == "" {
			name = "file"
		}
		d := &waE2E.DocumentMessage{Mimetype: proto.String(out.Mime), FileName: proto.String(name), Title: proto.String(name), ContextInfo: ci,
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength)}
		if out.Text != "" {
			d.Caption = proto.String(out.Text)
			msg.DocumentWithCaptionMessage = &waE2E.FutureProofMessage{Message: &waE2E.Message{DocumentMessage: d}}
		} else {
			msg.DocumentMessage = d
		}
	default:
		return Sent{}, fmt.Errorf("can't send a %s", out.Kind)
	}
	resp, err := c.sendMessage(ctx, cli, j, msg)
	if err != nil {
		return Sent{}, err
	}
	sent.ID, sent.At = resp.ID, resp.Timestamp
	if sent.At.IsZero() {
		sent.At = time.Now()
	}
	switch {
	case msg.ImageMessage != nil:
		sent.MediaRef, _ = proto.Marshal(&waE2E.Message{ImageMessage: msg.ImageMessage})
	case msg.VideoMessage != nil:
		sent.MediaRef, _ = proto.Marshal(&waE2E.Message{VideoMessage: msg.VideoMessage})
	case msg.AudioMessage != nil:
		sent.MediaRef, _ = proto.Marshal(&waE2E.Message{AudioMessage: msg.AudioMessage})
	case msg.DocumentMessage != nil:
		sent.MediaRef, _ = proto.Marshal(&waE2E.Message{DocumentMessage: msg.DocumentMessage})
	case msg.DocumentWithCaptionMessage != nil:
		sent.MediaRef, _ = proto.Marshal(msg.DocumentWithCaptionMessage.GetMessage())
	}
	return sent, nil
}

// React puts emoji on a message, or takes the owner's reaction away with "". sender is the
// message's sender JID, "" for one of the owner's own messages.
func (c *Client) React(ctx context.Context, chat, sender, id, emoji string) error {
	cli, err := c.client()
	if err != nil {
		return err
	}
	j, err := parseChat(chat)
	if err != nil {
		return err
	}
	s := types.EmptyJID
	if sender != "" {
		if s, err = types.ParseJID(sender); err != nil {
			return fmt.Errorf("bad sender %q", sender)
		}
	}
	_, err = cli.SendMessage(ctx, j, cli.BuildReaction(j, s, id, emoji))
	return err
}

// Revoke deletes one of the owner's messages for everyone.
func (c *Client) Revoke(ctx context.Context, chat, id string) error {
	cli, err := c.client()
	if err != nil {
		return err
	}
	j, err := parseChat(chat)
	if err != nil {
		return err
	}
	_, err = cli.SendMessage(ctx, j, cli.BuildRevoke(j, types.EmptyJID, id))
	return err
}

// EditText replaces the text of one of the owner's messages (WhatsApp allows it for 20 minutes).
func (c *Client) EditText(ctx context.Context, chat, id, text string) error {
	cli, err := c.client()
	if err != nil {
		return err
	}
	j, err := parseChat(chat)
	if err != nil {
		return err
	}
	_, err = cli.SendMessage(ctx, j, cli.BuildEdit(j, id, &waE2E.Message{Conversation: proto.String(text)}))
	return err
}

// RequestHistory asks the phone for up to count messages before the given one. The answer, if
// the phone sends one, arrives later as a History update of type "on-demand".
func (c *Client) RequestHistory(ctx context.Context, chat, oldestID string, oldestFromMe bool, oldestAt time.Time, count int) error {
	cli, err := c.client()
	if err != nil {
		return err
	}
	j, err := parseChat(chat)
	if err != nil {
		return err
	}
	info := &types.MessageInfo{MessageSource: types.MessageSource{Chat: j, IsFromMe: oldestFromMe, IsGroup: j.Server == types.GroupServer},
		ID: oldestID, Timestamp: oldestAt}
	_, err = cli.SendPeerMessage(ctx, cli.BuildHistorySyncRequest(info, count))
	return err
}

// DownloadRef downloads the media of a message from its stored MediaRef.
func (c *Client) DownloadRef(ctx context.Context, ref []byte) ([]byte, error) {
	cli, err := c.client()
	if err != nil {
		return nil, err
	}
	var m waE2E.Message
	if err := proto.Unmarshal(ref, &m); err != nil {
		return nil, fmt.Errorf("the media details are damaged: %w", err)
	}
	var d whatsmeow.DownloadableMessage
	switch {
	case m.GetImageMessage() != nil:
		d = m.GetImageMessage()
	case m.GetVideoMessage() != nil:
		d = m.GetVideoMessage()
	case m.GetAudioMessage() != nil:
		d = m.GetAudioMessage()
	case m.GetDocumentMessage() != nil:
		d = m.GetDocumentMessage()
	case m.GetStickerMessage() != nil:
		d = m.GetStickerMessage()
	default:
		return nil, errors.New("this message has no media")
	}
	data, err := cli.Download(ctx, d)
	if errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith404) || errors.Is(err, whatsmeow.ErrMediaDownloadFailedWith410) {
		return nil, errors.New("WhatsApp no longer has this file; it may be on the phone only")
	}
	return data, err
}

// Thumbnail returns the JPEG thumbnail inside a stored MediaRef, nil without one.
func Thumbnail(ref []byte) []byte {
	if len(ref) == 0 {
		return nil
	}
	var m waE2E.Message
	if proto.Unmarshal(ref, &m) != nil {
		return nil
	}
	switch {
	case m.GetImageMessage() != nil:
		return m.GetImageMessage().GetJPEGThumbnail()
	case m.GetVideoMessage() != nil:
		return m.GetVideoMessage().GetJPEGThumbnail()
	case m.GetDocumentMessage() != nil:
		return m.GetDocumentMessage().GetJPEGThumbnail()
	}
	return nil
}

// ---- contacts, presence, pictures ----

// Contact is someone in the owner's WhatsApp contacts.
type Contact struct {
	JID, Name, PushName, BusinessName, Phone string
}

// Contacts lists the owner's contacts from the session store.
func (c *Client) Contacts(ctx context.Context) ([]Contact, error) {
	cli, err := c.client()
	if err != nil {
		return nil, err
	}
	all, err := cli.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Contact, 0, len(all))
	for j, ci := range all {
		if j.Server != types.DefaultUserServer && j.Server != types.HiddenUserServer {
			continue
		}
		ct := Contact{JID: j.String(), Name: ci.FullName, PushName: ci.PushName, BusinessName: ci.BusinessName}
		if ct.Name == "" {
			ct.Name = ci.FirstName
		}
		if j.Server == types.DefaultUserServer {
			ct.Phone = "+" + j.User
		} else if pn, err := cli.Store.LIDs.GetPNForLID(ctx, j); err == nil && !pn.IsEmpty() {
			ct.Phone = "+" + pn.User
		}
		out = append(out, ct)
	}
	return out, nil
}

// LookupPhone checks a phone number (digits with the country code) is on WhatsApp and returns its chat JID.
func (c *Client) LookupPhone(ctx context.Context, digits string) (string, error) {
	cli, err := c.client()
	if err != nil {
		return "", err
	}
	res, err := cli.IsOnWhatsApp(ctx, []string{"+" + digits})
	if err != nil {
		return "", fmt.Errorf("could not check the number: %w", err)
	}
	for _, r := range res {
		if r.IsIn {
			// Prefer the phone-number JID: it is how Deputy (and the MCP server) name direct chats.
			if !r.PhoneNumber.IsEmpty() && r.PhoneNumber.Server == types.DefaultUserServer {
				return r.PhoneNumber.ToNonAD().String(), nil
			}
			return r.JID.ToNonAD().String(), nil
		}
	}
	return "", errors.New("this number is not on WhatsApp")
}

// ErrNoPicture is returned by ProfilePicture when there is none to show.
var ErrNoPicture = errors.New("no profile picture")

// ProfilePicture returns the URL and ID of a chat's picture (preview size). With the current
// ID passed as existingID, an unchanged picture returns "" and the same ID.
func (c *Client) ProfilePicture(ctx context.Context, jid, existingID string) (url, id string, err error) {
	cli, err := c.client()
	if err != nil {
		return "", "", err
	}
	j, err := parseChat(jid)
	if err != nil {
		return "", "", err
	}
	info, err := cli.GetProfilePictureInfo(ctx, j, &whatsmeow.GetProfilePictureParams{Preview: true, ExistingID: existingID})
	if errors.Is(err, whatsmeow.ErrProfilePictureNotSet) || errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized) {
		return "", "", ErrNoPicture
	}
	if err != nil {
		return "", "", err
	}
	if info == nil {
		return "", existingID, nil
	}
	return info.URL, info.ID, nil
}

// SetOnline shows the owner online (needed to see others typing) or offline. While Deputy shows
// the owner online, WhatsApp may hold back notifications on the phone, like WhatsApp Web does.
func (c *Client) SetOnline(on bool) {
	cli, err := c.client()
	if err != nil {
		return
	}
	st := types.PresenceUnavailable
	if on {
		st = types.PresenceAvailable
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.SendPresence(ctx, st); err != nil && !errors.Is(err, whatsmeow.ErrNoPushName) {
		log.Printf("whatsapp: presence: %v", err)
	}
}

// SubscribePresence asks for a contact's online and typing updates.
func (c *Client) SubscribePresence(jid string) {
	cli, err := c.client()
	if err != nil || !strings.HasSuffix(jid, "@s.whatsapp.net") && !strings.HasSuffix(jid, "@lid") {
		return
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = cli.SubscribePresence(ctx, j)
}

// Member is one participant of a group.
type Member struct {
	JID   string   // the JID to mention them with (LID or phone number, as the group uses)
	Name  string   // contact name, push name or +number
	Phone string   // "+5551…", "" when hidden
	Users []string // every user part they go by (LID and phone), to read incoming @mentions
	Me    bool
	Admin bool
}

// GroupMembers lists a group's participants.
func (c *Client) GroupMembers(ctx context.Context, chat string) ([]Member, error) {
	cli, err := c.client()
	if err != nil {
		return nil, err
	}
	j, err := parseChat(chat)
	if err != nil {
		return nil, err
	}
	info, err := cli.GetGroupInfo(ctx, j)
	if err != nil {
		return nil, err
	}
	out := make([]Member, 0, len(info.Participants))
	for _, p := range info.Participants {
		m := Member{JID: p.JID.String(), Admin: p.IsAdmin || p.IsSuperAdmin, Me: c.isMe(cli, p.JID.String())}
		named := p.JID
		if !p.PhoneNumber.IsEmpty() {
			named = p.PhoneNumber
			m.Phone = "+" + p.PhoneNumber.User
		}
		for _, u := range []types.JID{p.JID, p.PhoneNumber, p.LID} {
			if !u.IsEmpty() {
				m.Users = append(m.Users, u.User)
			}
		}
		m.Name = c.contactName(named)
		if (m.Name == "Unknown contact" || m.Name == "") && p.DisplayName != "" {
			m.Name = p.DisplayName
		}
		out = append(out, m)
	}
	return out, nil
}
