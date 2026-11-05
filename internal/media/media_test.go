package media

import (
	"bytes"
	"context"
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
