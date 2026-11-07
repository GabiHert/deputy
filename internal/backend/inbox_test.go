package backend

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/GabiHert/deputy/internal/store"

	"github.com/GabiHert/deputy/internal/domain"
)

func TestInitials(t *testing.T) {
	for in, want := range map[string]string{"Ana Souza": "AS", "família herter": "FH", "+55 51 9999": "#", "": "?", "Zé": "Z", "🎉 Party People": "PP"} {
		if got := Initials(in); got != want {
			t.Errorf("Initials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFileKind(t *testing.T) {
	for in, want := range map[string]string{"image/jpeg": "image", "image/gif": "document", "video/mp4": "video", "video/quicktime": "document",
		"audio/mpeg": "audio", "application/pdf": "document", "": "document", "IMAGE/PNG; x=1": "image"} {
		if got := FileKind(in); got != want {
			t.Errorf("FileKind(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSnippetAndBase64(t *testing.T) {
	if s := snippet(domain.Message{Kind: domain.KindDocument, FileName: "a.pdf", Text: "a.pdf"}); s != "📄 a.pdf" {
		t.Errorf("document: %q", s)
	}
	if s := snippet(domain.Message{Kind: domain.KindVoice, Seconds: 75}); s != "🎤 Voice note (1:15)" {
		t.Errorf("voice: %q", s)
	}
	if s := snippet(domain.Message{Kind: domain.KindText, Text: "x", Revoked: true}); s != "This message was deleted" {
		t.Errorf("revoked: %q", s)
	}
	for _, in := range []string{"aGk=", "data:text/plain;base64,aGk="} {
		if b, err := decode64(in); err != nil || string(b) != "hi" {
			t.Errorf("decode64(%q) = %q, %v", in, b, err)
		}
	}
	if _, err := decode64("%%%"); err == nil {
		t.Error("bad base64 accepted")
	}
}

func TestViewOnceAndTimerDTOs(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	b := &Backend{d: Deps{Store: st}}
	d := b.chatMessageDTO(domain.Message{ID: "V1", Kind: domain.KindImage, ViewOnce: true})
	if !d.ViewOnce || d.Media == nil || d.Media.Downloadable || d.Media.ThumbURL != "" {
		t.Fatalf("placeholder %+v %+v", d, d.Media)
	}
	d = b.chatMessageDTO(domain.Message{ID: "V2", Kind: domain.KindVideo, ViewOnce: true, MediaRef: []byte{1}})
	if !d.ViewOnce || !d.Media.Downloadable {
		t.Fatalf("downloadable %+v %+v", d, d.Media)
	}
	if d := b.chatMessageDTO(domain.Message{ID: "T", Kind: domain.KindText, Text: "x"}); d.ViewOnce {
		t.Fatal("plain message marked view once")
	}
	c := b.inboxDTO(domain.Chat{JID: "g@g.us", Name: "G", Disappearing: 86400, DisappearingKnown: true}, domain.NewRouter(nil), time.Now())
	if c.Disappearing != 86400 {
		t.Fatalf("chat %+v", c)
	}
}
