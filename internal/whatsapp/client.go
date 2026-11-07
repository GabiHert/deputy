// Package whatsapp links Deputy to the person's WhatsApp as a linked device, through whatsmeow.
package whatsapp

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCompanionReg"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// States of the link.
const (
	StateStarting     = "starting"
	StateQR           = "qr"
	StatePairing      = "pairing"
	StateConnected    = "connected"
	StateDisconnected = "disconnected"
	StateError        = "error"
)

// State is the link's state for the UI.
type State struct {
	State       string
	QR          string // PNG data URL
	QRExpiresAt time.Time
	Phone       string // full number, digits only
	PushName    string
	Since       time.Time
	Error       string
}

// Incoming is a message received in any chat.
type Incoming struct {
	ID         string
	Chat       string // chat JID
	IsGroup    bool
	Sender     string // sender JID
	SenderName string
	FromMe     bool
	FromDeputy bool // sent by Deputy's own linked device, not typed on the phone
	At         time.Time
	Kind       string // domain.MsgKind values
	Text       string
	Mime       string
	Seconds    int
	FileName   string
	MentionsMe bool
	QuotedID   string
	QuotesMe   bool
	// Download fetches the media bytes; nil without media.
	Download func(ctx context.Context) ([]byte, error)

	QuotedSender string
	QuotedText   string
	QuotedKind   string
	Mentions     []string
	FileSize     int64
	MediaRef     []byte // the media message, for DownloadRef
	History      bool   // from a history sync: old, never for agents or notifications
	Status       string // own messages from a history sync: "sent", "delivered", "read", "played"
	Revoked      bool   // deleted for everyone before Deputy saw it
	// ViewOnce: a view-once photo, video or voice note. Without MediaRef it is only a placeholder:
	// WhatsApp keeps the media on the phone.
	ViewOnce bool
}

// Group is a group the person is in.
type Group struct {
	JID     string
	Name    string
	Members int
}

// Client is the WhatsApp link.
type Client struct {
	container *sqlstore.Container

	mu    sync.Mutex
	cli   *whatsmeow.Client
	state State
	stop  context.CancelFunc

	onState   func(State)
	onMessage func(Incoming)
	onGroups  func([]Group)
	onUpdate  func(any)

	groupNames sync.Map // jid -> name
	skipped    sync.Map // shapes of messages Deputy does not show, logged once each

	timersMu    sync.Mutex
	timers      map[string]Timer // chat JID -> disappearing-messages timer
	timerSource func(chat string) (Timer, bool)
}

// Open opens the session store at dbPath.
func Open(ctx context.Context, dbPath string) (*Client, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		return nil, err
	}
	store.SetOSInfo("Deputy (Mac)", [3]uint32{0, 1, 0})
	store.DeviceProps.PlatformType = waCompanionReg.DeviceProps_DESKTOP.Enum()
	c, err := sqlstore.New(ctx, "sqlite3", "file:"+dbPath+"?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000", waLog.Stdout("WAStore", "WARN", false))
	if err != nil {
		return nil, fmt.Errorf("could not open the WhatsApp session store: %w", err)
	}
	return &Client{container: c, state: State{State: StateStarting}}, nil
}

// OnState, OnMessage and OnGroups set the callbacks. Set them before Start.
func (c *Client) OnState(f func(State))      { c.onState = f }
func (c *Client) OnMessage(f func(Incoming)) { c.onMessage = f }
func (c *Client) OnGroups(f func([]Group))   { c.onGroups = f }

// OnUpdate receives everything that is not a new message: Reaction, Revoke, Edit, Receipt,
// ChatRead, ChatMeta, ChatPresence, Presence and History. Set it before Start.
func (c *Client) OnUpdate(f func(any)) { c.onUpdate = f }

func (c *Client) update(u any) {
	if c.onUpdate != nil && u != nil {
		c.onUpdate(u)
	}
}

// State returns the current state.
func (c *Client) State() State {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}

func (c *Client) setState(f func(*State)) {
	c.mu.Lock()
	f(&c.state)
	s := c.state
	c.mu.Unlock()
	if c.onState != nil {
		c.onState(s)
	}
}

// Start connects, showing a QR code first when the device is not linked yet.
func (c *Client) Start() error {
	ctx, cancel := context.WithCancel(context.Background())
	dev, err := c.container.GetFirstDevice(ctx)
	if err != nil {
		cancel()
		c.setState(func(s *State) { *s = State{State: StateError, Error: err.Error()} })
		return err
	}
	cli := whatsmeow.NewClient(dev, waLog.Stdout("WhatsApp", "WARN", false))
	cli.AddEventHandler(c.handle)
	c.mu.Lock()
	c.cli = cli
	c.stop = cancel
	c.mu.Unlock()

	if cli.Store.ID == nil {
		qr, err := cli.GetQRChannel(ctx)
		if err != nil {
			cancel()
			return err
		}
		go c.watchQR(ctx, qr)
		c.setState(func(s *State) { *s = State{State: StateStarting} })
	} else {
		c.setState(func(s *State) {
			*s = State{State: StateDisconnected, Phone: cli.Store.ID.User, PushName: cli.Store.PushName}
		})
	}
	if err := cli.Connect(); err != nil {
		c.setState(func(s *State) { s.State = StateError; s.Error = "Could not reach WhatsApp: " + err.Error() })
		return err
	}
	return nil
}

func (c *Client) watchQR(ctx context.Context, ch <-chan whatsmeow.QRChannelItem) {
	for item := range ch {
		switch item.Event {
		case "code":
			png, err := qrcode.Encode(item.Code, qrcode.Medium, 512)
			if err != nil {
				log.Printf("whatsapp: qr: %v", err)
				continue
			}
			url := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			exp := time.Now().Add(item.Timeout)
			c.setState(func(s *State) { *s = State{State: StateQR, QR: url, QRExpiresAt: exp} })
		case "success":
			c.setState(func(s *State) { s.State = StatePairing; s.QR = "" })
		case "timeout":
			// No scan for a while: start a fresh QR session.
			go c.restart()
			return
		case "error":
			msg := "Linking failed."
			if item.Error != nil {
				msg = "Linking failed: " + item.Error.Error()
			}
			c.setState(func(s *State) { s.State = StateError; s.Error = msg })
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// restart drops the current client and starts again, with a fresh QR code if logged out.
func (c *Client) restart() {
	c.mu.Lock()
	cli, stop := c.cli, c.stop
	c.cli = nil
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if cli != nil {
		cli.Disconnect()
	}
	time.Sleep(time.Second)
	if err := c.Start(); err != nil {
		log.Printf("whatsapp: restart: %v", err)
	}
}

// Stop disconnects.
func (c *Client) Stop() {
	c.mu.Lock()
	cli, stop := c.cli, c.stop
	c.mu.Unlock()
	if stop != nil {
		stop()
	}
	if cli != nil {
		cli.Disconnect()
	}
}

func (c *Client) client() (*whatsmeow.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cli == nil {
		return nil, errors.New("WhatsApp is not connected")
	}
	return c.cli, nil
}

// Logout unlinks Deputy from the phone and shows a new QR code.
func (c *Client) Logout(ctx context.Context) error {
	cli, err := c.client()
	if err != nil {
		return err
	}
	if cli.Store.ID != nil {
		if err := cli.Logout(ctx); err != nil {
			// Unlinked on the phone already, or offline: forget the session anyway.
			_ = cli.Store.Delete(ctx)
		}
	}
	go c.restart()
	return nil
}

// LinkWithPhone asks WhatsApp for an 8-character code to type on the phone instead of scanning.
func (c *Client) LinkWithPhone(ctx context.Context, phone string) (string, error) {
	cli, err := c.client()
	if err != nil {
		return "", err
	}
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, phone)
	if len(digits) < 8 {
		return "", errors.New("Type the full number with the country code, like +55 51 99999 1234.")
	}
	return cli.PairPhone(ctx, digits, true, whatsmeow.PairClientChrome, "Chrome (Mac OS)")
}

func (c *Client) handle(evt any) {
	switch e := evt.(type) {
	case *events.PairSuccess:
		c.setState(func(s *State) { s.State = StatePairing; s.Phone = e.ID.User; s.QR = "" })
	case *events.Connected:
		cli, _ := c.client()
		if cli == nil || cli.Store.ID == nil {
			return
		}
		c.setState(func(s *State) {
			*s = State{State: StateConnected, Phone: cli.Store.ID.User, PushName: cli.Store.PushName, Since: time.Now()}
		})
		go c.loadGroups()
	case *events.Disconnected:
		c.setState(func(s *State) {
			if s.State == StateConnected {
				s.State = StateDisconnected
			}
		})
	case *events.StreamReplaced:
		c.setState(func(s *State) {
			s.State = StateError
			s.Error = "WhatsApp was opened with this session somewhere else. Quit the other copy of Deputy."
		})
	case *events.LoggedOut:
		c.setState(func(s *State) { *s = State{State: StateStarting} })
		go c.restart()
	case *events.Message:
		in, upd := c.classify(e)
		c.learnFromMessage(e, in, upd)
		switch {
		case upd != nil:
			if _, ok := upd.(Disappearing); !ok { // learnFromMessage reported it
				c.update(upd)
			}
		case in.ID != "" && c.onMessage != nil:
			c.onMessage(in)
		}
	case *events.UndecryptableMessage:
		if in, ok := c.unavailableViewOnce(e); ok && c.onMessage != nil {
			c.onMessage(in)
		}
	default:
		c.handleInbox(evt)
	}
}

func (c *Client) loadGroups() {
	cli, err := c.client()
	if err != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gs, err := cli.GetJoinedGroups(ctx)
	if err != nil {
		log.Printf("whatsapp: groups: %v", err)
		return
	}
	out := make([]Group, 0, len(gs))
	for _, g := range gs {
		c.groupNames.Store(g.JID.String(), g.Name)
		c.learnGroupTimer(g.JID, g.GroupEphemeral, time.Time{})
		out = append(out, Group{JID: g.JID.String(), Name: g.Name, Members: len(g.Participants)})
	}
	if c.onGroups != nil {
		c.onGroups(out)
	}
}

// ChatName names a chat: the group's subject, or the contact's name or number.
func (c *Client) ChatName(jid string) string {
	j, err := types.ParseJID(jid)
	if err != nil {
		return jid
	}
	if j.Server == types.GroupServer {
		if v, ok := c.groupNames.Load(jid); ok && v.(string) != "" {
			return v.(string)
		}
		if cli, err := c.client(); err == nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if g, err := cli.GetGroupInfo(ctx, j); err == nil {
				c.groupNames.Store(jid, g.Name)
				return g.Name
			}
		}
		return "Group"
	}
	return c.contactName(j)
}

func (c *Client) contactName(j types.JID) string {
	cli, err := c.client()
	if err != nil {
		return "+" + j.User
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	look := []types.JID{j}
	if j.Server == types.HiddenUserServer {
		if pn, err := cli.Store.LIDs.GetPNForLID(ctx, j); err == nil && !pn.IsEmpty() {
			look = append(look, pn)
		}
	}
	for _, k := range look {
		if ci, err := cli.Store.Contacts.GetContact(ctx, k); err == nil && ci.Found {
			for _, n := range []string{ci.FullName, ci.FirstName, ci.PushName, ci.BusinessName} {
				if n != "" {
					return n
				}
			}
		}
	}
	for _, k := range look {
		if k.Server == types.DefaultUserServer {
			return "+" + k.User
		}
	}
	return "Unknown contact"
}

// IsMe reports whether jid is the owner, by phone number or LID.
func (c *Client) IsMe(jid string) bool {
	cli, err := c.client()
	if err != nil {
		return false
	}
	return c.isMe(cli, jid)
}

// isMe reports whether jid is the person, by phone number or LID.
func (c *Client) isMe(cli *whatsmeow.Client, jid string) bool {
	if jid == "" || cli.Store.ID == nil {
		return false
	}
	user := strings.SplitN(strings.SplitN(jid, "@", 2)[0], ":", 2)[0]
	if user == cli.Store.ID.User {
		return true
	}
	return !cli.Store.LID.IsEmpty() && user == cli.Store.LID.User
}

// classify turns a message event into a new message (Incoming) or a change to an earlier one
// (Reaction, Revoke or Edit). Both are empty for what Deputy does not show.
func (c *Client) classify(e *events.Message) (Incoming, any) {
	cli, err := c.client()
	if err != nil {
		return Incoming{}, nil
	}
	if e.Info.Chat.Server == types.BroadcastServer || e.Info.Chat.Server == types.NewsletterServer {
		return Incoming{}, nil
	}
	m := e.Message
	if m == nil {
		return Incoming{}, nil
	}
	chat := e.Info.Chat.String()
	sender := e.Info.Sender.ToNonAD().String()
	if pm := m.GetProtocolMessage(); pm != nil {
		switch pm.GetType() {
		case waE2E.ProtocolMessage_REVOKE:
			return Incoming{}, Revoke{Chat: chat, TargetID: pm.GetKey().GetID(), Sender: sender, FromMe: e.Info.IsFromMe}
		case waE2E.ProtocolMessage_MESSAGE_EDIT:
			text, _ := textOf(pm.GetEditedMessage())
			return Incoming{}, Edit{Chat: chat, TargetID: pm.GetKey().GetID(), Text: text, At: e.Info.Timestamp}
		case waE2E.ProtocolMessage_EPHEMERAL_SETTING:
			return Incoming{}, ephemeralSetting(chat, pm, e.Info.Timestamp)
		}
		return Incoming{}, nil
	}
	if r := m.GetReactionMessage(); r != nil {
		name := e.Info.PushName
		if name == "" && !e.Info.IsFromMe {
			name = c.contactName(e.Info.Sender.ToNonAD())
		}
		at := e.Info.Timestamp
		if ms := r.GetSenderTimestampMS(); ms > 0 {
			at = time.UnixMilli(ms)
		}
		return Incoming{}, Reaction{Chat: chat, TargetID: r.GetKey().GetID(), Sender: sender, SenderName: name,
			FromMe: e.Info.IsFromMe, Emoji: r.GetText(), At: at}
	}
	if e.IsEdit {
		return Incoming{}, nil
	}
	in, _ := c.header(cli, &e.Info)
	var ctxInfo *waE2E.ContextInfo
	var media proto.Message // the downloadable part, kept so it can be fetched later
	if e.IsViewOnce {
		// A view-once photo, video or voice note: kept as media when it came with its key and
		// path (viewOnceKind only says so for those three), else a placeholder.
		kind, downloadable := viewOnceKind(m)
		if !downloadable {
			c.fillContext(cli, &in, viewOncePlaceholder(&in, m, kind))
			return in, nil
		}
		in.ViewOnce = true
	}
	switch {
	case m.GetConversation() != "":
		in.Kind, in.Text = "text", m.GetConversation()
	case m.GetExtendedTextMessage() != nil:
		t := m.GetExtendedTextMessage()
		in.Kind, in.Text, ctxInfo = "text", t.GetText(), t.GetContextInfo()
	case m.GetImageMessage() != nil:
		im := m.GetImageMessage()
		in.Kind, in.Text, in.Mime, ctxInfo = "image", im.GetCaption(), im.GetMimetype(), im.GetContextInfo()
		in.FileSize, media = int64(im.GetFileLength()), &waE2E.Message{ImageMessage: im}
		in.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, im) }
	case m.GetAudioMessage() != nil:
		au := m.GetAudioMessage()
		in.Kind, in.Mime, in.Seconds, ctxInfo = "audio", au.GetMimetype(), int(au.GetSeconds()), au.GetContextInfo()
		if au.GetPTT() {
			in.Kind = "voice"
		}
		in.FileSize, media = int64(au.GetFileLength()), &waE2E.Message{AudioMessage: au}
		in.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, au) }
	case m.GetVideoMessage() != nil:
		v := m.GetVideoMessage()
		in.Kind, in.Text, in.Mime, in.Seconds, ctxInfo = "video", v.GetCaption(), v.GetMimetype(), int(v.GetSeconds()), v.GetContextInfo()
		in.FileSize, media = int64(v.GetFileLength()), &waE2E.Message{VideoMessage: v}
		in.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, v) }
	case m.GetDocumentMessage() != nil:
		d := m.GetDocumentMessage()
		in.Kind, in.Text, in.Mime, in.FileName, ctxInfo = "document", d.GetCaption(), d.GetMimetype(), d.GetFileName(), d.GetContextInfo()
		in.FileSize, media = int64(d.GetFileLength()), &waE2E.Message{DocumentMessage: d}
		in.Download = func(ctx context.Context) ([]byte, error) { return cli.Download(ctx, d) }
	case m.GetStickerMessage() != nil:
		st := m.GetStickerMessage()
		in.Kind, in.Mime, ctxInfo = "sticker", st.GetMimetype(), st.GetContextInfo()
		in.FileSize, media = int64(st.GetFileLength()), &waE2E.Message{StickerMessage: st}
	default:
		c.logSkipped(e)
		return Incoming{}, nil // receipts, polls, calls and other things Deputy does not show
	}
	if media != nil {
		in.MediaRef, _ = proto.Marshal(media)
	}
	c.fillContext(cli, &in, ctxInfo)
	return in, nil
}

// header fills in who sent a message, where and when. It is false for chats Deputy leaves out.
func (c *Client) header(cli *whatsmeow.Client, info *types.MessageInfo) (Incoming, bool) {
	if info.Chat.Server == types.BroadcastServer || info.Chat.Server == types.NewsletterServer {
		return Incoming{}, false
	}
	in := Incoming{
		ID: info.ID, Chat: info.Chat.String(), IsGroup: info.IsGroup, Sender: info.Sender.ToNonAD().String(),
		SenderName: info.PushName, FromMe: info.IsFromMe, At: info.Timestamp,
	}
	if in.FromMe && cli.Store.ID != nil && info.Sender.Device == cli.Store.ID.Device {
		in.FromDeputy = true
	}
	if in.SenderName == "" && !in.FromMe {
		in.SenderName = c.contactName(info.Sender.ToNonAD())
	}
	return in, true
}

// fillContext reads mentions and the quoted message from a message's ContextInfo.
func (c *Client) fillContext(cli *whatsmeow.Client, in *Incoming, ctxInfo *waE2E.ContextInfo) {
	if ctxInfo != nil {
		for _, j := range ctxInfo.GetMentionedJID() {
			in.Mentions = append(in.Mentions, j)
			if c.isMe(cli, j) {
				in.MentionsMe = true
			}
		}
		in.QuotedID = ctxInfo.GetStanzaID()
		if in.QuotedID != "" {
			in.QuotedSender = ctxInfo.GetParticipant()
			in.QuotedText, in.QuotedKind = textOf(ctxInfo.GetQuotedMessage())
			if c.isMe(cli, in.QuotedSender) {
				in.QuotesMe = true
			}
		}
	}
}

func parseChat(jid string) (types.JID, error) {
	j, err := types.ParseJID(jid)
	if err != nil {
		return j, fmt.Errorf("bad chat id %q", jid)
	}
	return j, nil
}

// SendText sends a text message, quoting quotedID from quotedSender when set.
func (c *Client) SendText(ctx context.Context, chat, text string) (string, error) {
	cli, err := c.client()
	if err != nil {
		return "", err
	}
	j, err := parseChat(chat)
	if err != nil {
		return "", err
	}
	resp, err := c.sendMessage(ctx, cli, j, &waE2E.Message{Conversation: proto.String(text)})
	return resp.ID, err
}

// SendImage uploads and sends an image with an optional caption.
func (c *Client) SendImage(ctx context.Context, chat string, data []byte, mime, caption string) (string, error) {
	cli, err := c.client()
	if err != nil {
		return "", err
	}
	j, err := parseChat(chat)
	if err != nil {
		return "", err
	}
	up, err := cli.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("could not upload the image: %w", err)
	}
	msg := &waE2E.Message{ImageMessage: &waE2E.ImageMessage{
		Caption: proto.String(caption), Mimetype: proto.String(mime),
		URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
	}}
	resp, err := c.sendMessage(ctx, cli, j, msg)
	return resp.ID, err
}

// SendVoice uploads and sends an Ogg Opus voice note.
func (c *Client) SendVoice(ctx context.Context, chat string, ogg []byte, seconds int) (string, error) {
	cli, err := c.client()
	if err != nil {
		return "", err
	}
	j, err := parseChat(chat)
	if err != nil {
		return "", err
	}
	up, err := cli.Upload(ctx, ogg, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("could not upload the voice note: %w", err)
	}
	msg := &waE2E.Message{AudioMessage: &waE2E.AudioMessage{
		Mimetype: proto.String("audio/ogg; codecs=opus"), PTT: proto.Bool(true), Seconds: proto.Uint32(uint32(seconds)),
		URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
		FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
	}}
	resp, err := c.sendMessage(ctx, cli, j, msg)
	return resp.ID, err
}

// Typing shows or clears "typing…" (or "recording audio…") in a chat.
func (c *Client) Typing(chat string, on, audio bool) {
	cli, err := c.client()
	if err != nil {
		return
	}
	j, err := parseChat(chat)
	if err != nil {
		return
	}
	state := types.ChatPresencePaused
	if on {
		state = types.ChatPresenceComposing
	}
	media := types.ChatPresenceMediaText
	if audio {
		media = types.ChatPresenceMediaAudio
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = cli.SendChatPresence(ctx, j, state, media)
}

// MarkRead marks messages as read, so the person's phone does not show them as unread forever.
func (c *Client) MarkRead(chat, sender string, ids []string) {
	cli, err := c.client()
	if err != nil || len(ids) == 0 {
		return
	}
	cj, err1 := parseChat(chat)
	sj, err2 := types.ParseJID(sender)
	if err1 != nil || err2 != nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = cli.MarkRead(ctx, ids, time.Now(), cj, sj)
}
