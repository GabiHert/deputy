package whatsapp

import (
	"log"
	"sort"
	"strings"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// View-once ("visualização única") photos, videos and voice notes. WhatsApp mostly keeps them
// from linked devices: the phone gets the media, Deputy gets either nothing (an "unavailable"
// stanza of type view_once) or the message without what a download needs. Deputy still shows
// that one arrived, and keeps the media when it did come.

// viewOnceKind decides how a view-once message is kept. kind is its inner media ("image",
// "video", "voice" or "audio"; "image" when unknown) and downloadable is true when the media
// came with the key and path a download needs.
func viewOnceKind(m *waE2E.Message) (kind string, downloadable bool) {
	has := func(key []byte, path string) bool { return len(key) > 0 && path != "" }
	switch {
	case m.GetImageMessage() != nil:
		im := m.GetImageMessage()
		return "image", has(im.GetMediaKey(), im.GetDirectPath())
	case m.GetVideoMessage() != nil:
		v := m.GetVideoMessage()
		return "video", has(v.GetMediaKey(), v.GetDirectPath())
	case m.GetAudioMessage() != nil:
		au := m.GetAudioMessage()
		kind = "audio"
		if au.GetPTT() {
			kind = "voice"
		}
		return kind, has(au.GetMediaKey(), au.GetDirectPath())
	}
	return "image", false
}

// viewOncePlaceholder fills in a view-once message Deputy cannot download: what kind it is, its caption
// and what it replies to, but no media.
func viewOncePlaceholder(in *Incoming, m *waE2E.Message, kind string) *waE2E.ContextInfo {
	in.Kind, in.ViewOnce = kind, true
	switch {
	case m.GetImageMessage() != nil:
		im := m.GetImageMessage()
		in.Text, in.Mime = im.GetCaption(), im.GetMimetype()
		return im.GetContextInfo()
	case m.GetVideoMessage() != nil:
		v := m.GetVideoMessage()
		in.Text, in.Mime, in.Seconds = v.GetCaption(), v.GetMimetype(), int(v.GetSeconds())
		return v.GetContextInfo()
	case m.GetAudioMessage() != nil:
		au := m.GetAudioMessage()
		in.Mime, in.Seconds = au.GetMimetype(), int(au.GetSeconds())
		return au.GetContextInfo()
	}
	return nil
}

// fieldNames lists the populated top-level fields of a message, sorted. Names only: never content.
func fieldNames(m *waE2E.Message) []string {
	if m == nil {
		return nil
	}
	var out []string
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		out = append(out, string(fd.Name()))
		return true
	})
	sort.Strings(out)
	return out
}

// logSkipped logs, once per distinct shape, a message Deputy does not show, so what arrives
// (view-once variants in particular) can be seen in the log.
func (c *Client) logSkipped(e *events.Message) {
	raw, inner := fieldNames(e.RawMessage), fieldNames(e.Message)
	rawKey, innerKey := strings.Join(raw, ","), strings.Join(inner, ",")
	if _, seen := c.skipped.LoadOrStore(rawKey+"|"+innerKey, true); seen {
		return
	}
	fields := raw
	if innerKey != rawKey {
		fields = append(append(append([]string(nil), raw...), "->"), inner...)
	}
	if e.IsViewOnce {
		fields = append(fields, "(view once)")
	}
	log.Printf("whatsapp: skipped message %s in %s: fields %v", e.Info.ID, e.Info.Chat, fields)
}

// unavailableViewOnce turns WhatsApp's "this view-once message is on the phone only" into a
// placeholder message, so the chat still shows that something arrived.
func (c *Client) unavailableViewOnce(e *events.UndecryptableMessage) (Incoming, bool) {
	if !e.IsUnavailable || e.UnavailableType != events.UnavailableTypeViewOnce {
		return Incoming{}, false
	}
	cli, err := c.client()
	if err != nil {
		return Incoming{}, false
	}
	in, ok := c.header(cli, &e.Info)
	if !ok {
		return Incoming{}, false
	}
	in.Kind, in.ViewOnce = "image", true
	return in, true
}
