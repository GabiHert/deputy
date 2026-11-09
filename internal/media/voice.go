package media

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/GabiHert/positron/internal/claude"
)

// IsOggOpus reports whether data is an Ogg Opus stream, what WhatsApp wants for voice notes.
func IsOggOpus(data []byte) bool {
	return bytes.HasPrefix(data, []byte("OggS")) && bytes.Contains(data[:min(len(data), 200)], []byte("OpusHead"))
}

// OggSeconds reads the length of an Ogg Opus stream from its last page; 0 when it can't tell.
func OggSeconds(data []byte) int {
	head := bytes.Index(data, []byte("OpusHead"))
	if head < 0 || len(data) < head+12 {
		return 0
	}
	preSkip := int64(binary.LittleEndian.Uint16(data[head+10:]))
	last := bytes.LastIndex(data, []byte("OggS"))
	if last < 0 || len(data) < last+14 {
		return 0
	}
	granule := int64(binary.LittleEndian.Uint64(data[last+6:]))
	if granule <= preSkip {
		return 0
	}
	return int((granule - preSkip + 24000) / 48000) // Opus granules count 48 kHz samples
}

// ToVoiceNote turns a recording (WebM, MP4/AAC, WAV, Ogg...) into an Ogg Opus voice note and
// returns it with its length in seconds. Ogg Opus passes through untouched.
func ToVoiceNote(ctx context.Context, env []string, tmpDir string, data []byte, mime string) ([]byte, int, error) {
	if len(data) == 0 {
		return nil, 0, errors.New("the recording is empty")
	}
	if IsOggOpus(data) {
		return data, max(1, OggSeconds(data)), nil
	}
	if env == nil {
		env = os.Environ()
	}
	ffmpeg, err := claude.LookPath(env, "ffmpeg")
	if err != nil {
		return nil, 0, errors.New("sending voice notes needs ffmpeg. Install it with: brew install ffmpeg")
	}
	dir, err := os.MkdirTemp(tmpDir, "deputy-rec-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	in, out := filepath.Join(dir, "in"+Ext(mime)), filepath.Join(dir, "out.ogg")
	if err := os.WriteFile(in, data, 0o600); err != nil {
		return nil, 0, err
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-loglevel", "error", "-y", "-i", in, "-vn", "-c:a", "libopus", "-b:a", "32k", "-ar", "48000", "-ac", "1", "-application", "voip", out)
	cmd.Env = env
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("could not convert the recording: %s", strings.TrimSpace(string(b)))
	}
	ogg, err := os.ReadFile(out)
	if err != nil {
		return nil, 0, err
	}
	return ogg, max(1, OggSeconds(ogg)), nil
}
