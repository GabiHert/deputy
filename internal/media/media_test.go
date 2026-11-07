package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/GabiHert/deputy/internal/domain"
)

// TestVoiceRoundTrip makes a voice note like WhatsApp's (Ogg Opus) and decodes it back for the speech engine.
func TestVoiceRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	env := append(os.Environ(), "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin")
	ogg, secs, err := Speaker{EnvFn: func() []string { return env }, TmpDir: t.TempDir()}.Speak(context.Background(), "Oi, tudo bem? Chego meio-dia.", domain.LangPTBR)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(ogg, []byte("OggS")) || secs < 1 {
		t.Fatalf("not an ogg voice note (%d bytes, %d s)", len(ogg), secs)
	}
	p := filepath.Join(t.TempDir(), "v.ogg")
	_ = os.WriteFile(p, ogg, 0o600)
	samples, err := decode16k(context.Background(), t.TempDir(), p)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(samples) / 16000; got < secs-1 || got > secs+1 {
		t.Fatalf("decoded %d s, want about %d s", got, secs)
	}
}

func TestStoreSaveRelAndExt(t *testing.T) {
	s := Store{Dir: t.TempDir()}
	p, err := s.Save("123@g.us", "ABC/../x", "image/jpeg", []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if rel := s.Rel(p); rel != "123_g.us/ABC_.._x.jpg" {
		t.Fatalf("rel %q", rel)
	}
	if Ext("audio/ogg; codecs=opus") != ".ogg" {
		t.Fatal("ext")
	}
}

func oggPage(granule uint64, payload []byte) []byte {
	p := make([]byte, 27)
	copy(p, "OggS")
	binary.LittleEndian.PutUint64(p[6:], granule)
	return append(p, payload...)
}

func TestOggSeconds(t *testing.T) {
	head := append([]byte("OpusHead"), 1, 1, 0x38, 0x01) // version, channels, pre-skip 312
	data := append(oggPage(0, head), oggPage(48000*7+312, []byte("audio"))...)
	if !IsOggOpus(data) {
		t.Fatal("not recognized as Ogg Opus")
	}
	if s := OggSeconds(data); s != 7 {
		t.Fatalf("seconds %d", s)
	}
	if OggSeconds([]byte("not ogg")) != 0 || IsOggOpus([]byte("RIFF....WAVE")) {
		t.Fatal("garbage")
	}
	ogg, secs, err := ToVoiceNote(context.Background(), nil, t.TempDir(), data, "audio/ogg")
	if err != nil || secs != 7 || len(ogg) != len(data) {
		t.Fatalf("Ogg Opus should pass through: %d %v", secs, err)
	}
}
