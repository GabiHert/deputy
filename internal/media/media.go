// Package media stores chat media, transcribes voice notes and makes spoken replies.
package media

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/GabiHert/deputy/internal/claude"
	"github.com/GabiHert/deputy/internal/domain"
	"github.com/GabiHert/deputy/internal/port"
	"github.com/GabiHert/deputy/internal/wavinfo"
)

// Store keeps media files under Dir, one folder per chat.
type Store struct {
	Dir string
}

var unsafe = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// Ext picks a file extension for a MIME type.
func Ext(mime string) string {
	m := strings.ToLower(strings.TrimSpace(strings.SplitN(mime, ";", 2)[0]))
	switch m {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "audio/mp4", "audio/aac":
		return ".m4a"
	}
	if i := strings.Index(m, "/"); i >= 0 && i < len(m)-1 {
		return "." + unsafe.ReplaceAllString(m[i+1:], "")
	}
	return ".bin"
}

// Save writes a message's media and returns its absolute path.
func (s Store) Save(chat, id, mime string, data []byte) (string, error) {
	dir := filepath.Join(s.Dir, unsafe.ReplaceAllString(chat, "_"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, unsafe.ReplaceAllString(id, "_")+Ext(mime))
	return p, os.WriteFile(p, data, 0o600)
}

// SaveAs writes a message's file with the given extension (like ".pdf") and returns its path.
func (s Store) SaveAs(chat, id, ext string, data []byte) (string, error) {
	ext = unsafe.ReplaceAllString(strings.ToLower(ext), "")
	if ext == "" || ext == "." || len(ext) > 12 {
		ext = ".bin"
	}
	if !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	dir := filepath.Join(s.Dir, unsafe.ReplaceAllString(chat, "_"))
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	p := filepath.Join(dir, unsafe.ReplaceAllString(id, "_")+ext)
	return p, os.WriteFile(p, data, 0o600)
}

// ThumbPath is where a message's JPEG preview is kept.
func (s Store) ThumbPath(chat, id string) string {
	return filepath.Join(s.Dir, unsafe.ReplaceAllString(chat, "_"), unsafe.ReplaceAllString(id, "_")+".thumb.jpg")
}

// AvatarPath is where a chat's profile picture is kept. "_avatars" can't clash with a chat
// folder: those are named after JIDs, which hold an "@" turned into "_" after a number.
func (s Store) AvatarPath(jid string) string {
	return filepath.Join(s.Dir, "_avatars", unsafe.ReplaceAllString(jid, "_")+".jpg")
}

// Rel returns p relative to the media folder, for URLs; "" when p is outside it.
func (s Store) Rel(p string) string {
	r, err := filepath.Rel(s.Dir, p)
	if err != nil || strings.HasPrefix(r, "..") {
		return ""
	}
	return filepath.ToSlash(r)
}

// Usage counts the files and bytes in the media folder.
func (s Store) Usage() (files int, bytes int64) {
	_ = filepath.WalkDir(s.Dir, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				files++
				bytes += info.Size()
			}
		}
		return nil
	})
	return
}

// Prune deletes media older than days. days <= 0 keeps everything.
func (s Store) Prune(days int) {
	if days <= 0 {
		return
	}
	cutoff := time.Now().AddDate(0, 0, -days)
	_ = filepath.WalkDir(s.Dir, func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil && info.ModTime().Before(cutoff) {
				_ = os.Remove(p)
			}
		}
		return nil
	})
}

// Transcriber turns voice notes into text on this Mac.
type Transcriber struct {
	Engine   port.SpeechEngine
	Detector port.SpeechDetector
	TmpDir   string
}

// decode16k converts any audio file macOS can read (Ogg Opus included) to 16 kHz mono samples.
func decode16k(ctx context.Context, tmpDir, in string) ([]float32, error) {
	f, err := os.CreateTemp(tmpDir, "deputy-*.wav")
	if err != nil {
		return nil, err
	}
	out := f.Name()
	f.Close()
	defer os.Remove(out)
	cmd := exec.CommandContext(ctx, "/usr/bin/afconvert", "-f", "WAVE", "-d", "LEI16@16000", "-c", "1", in, out)
	if b, err := cmd.CombinedOutput(); err != nil {
		return nil, fmt.Errorf("could not decode the audio: %s", strings.TrimSpace(string(b)))
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return nil, err
	}
	samples, rate, err := wavinfo.DecodeWAV(b)
	if err != nil {
		return nil, err
	}
	if rate != 16000 {
		return nil, fmt.Errorf("unexpected sample rate %d", rate)
	}
	return samples, nil
}

// Ready reports whether lang can be transcribed.
func (t Transcriber) Ready(lang domain.Language) (bool, string) {
	if t.Engine == nil {
		return false, "speech engine missing"
	}
	return t.Engine.Ready(lang)
}

// Transcribe returns the text of the audio file at path.
func (t Transcriber) Transcribe(ctx context.Context, path string, lang domain.Language) (string, error) {
	if ok, msg := t.Ready(lang); !ok {
		return "", fmt.Errorf("voice notes in %s can't be transcribed yet: %s. Download the model in Settings", lang.Label(), msg)
	}
	samples, err := decode16k(ctx, t.TmpDir, path)
	if err != nil {
		return "", err
	}
	var chunks [][]float32
	if t.Detector != nil {
		if ok, _ := t.Detector.Ready(); ok {
			if spans, err := t.Detector.Detect(samples, 16000); err == nil {
				for _, s := range spans {
					chunks = append(chunks, s.Samples)
				}
				if len(spans) == 0 {
					return "", nil // no speech
				}
			}
		}
	}
	if chunks == nil {
		const max = 25 * 16000 // Whisper reads 30-second windows
		for i := 0; i < len(samples); i += max {
			chunks = append(chunks, samples[i:min(i+max, len(samples))])
		}
	}
	var parts []string
	for _, c := range chunks {
		text, err := t.Engine.Transcribe(ctx, c, 16000, lang)
		if err != nil {
			return "", err
		}
		if text = strings.TrimSpace(text); text != "" {
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, " "), nil
}

// Speaker makes voice notes with the macOS voice and ffmpeg.
type Speaker struct {
	EnvFn  func() []string
	TmpDir string
}

var voices = map[domain.Language]string{domain.LangPTBR: "Luciana", domain.LangEN: "Samantha"}

// Speak returns an Ogg Opus voice note of text and its length in seconds.
func (s Speaker) Speak(ctx context.Context, text string, lang domain.Language) ([]byte, int, error) {
	env := os.Environ()
	if s.EnvFn != nil {
		env = s.EnvFn()
	}
	ffmpeg, err := claude.LookPath(env, "ffmpeg")
	if err != nil {
		return nil, 0, errors.New("voice replies need ffmpeg. Install it with: brew install ffmpeg")
	}
	dir, err := os.MkdirTemp(s.TmpDir, "deputy-voice-*")
	if err != nil {
		return nil, 0, err
	}
	defer os.RemoveAll(dir)
	aiff, ogg := filepath.Join(dir, "v.aiff"), filepath.Join(dir, "v.ogg")
	args := []string{"-o", aiff}
	if v, ok := voices[lang]; ok {
		args = append(args, "-v", v)
	}
	say := exec.CommandContext(ctx, "/usr/bin/say", append(args, "--", text)...)
	if b, err := say.CombinedOutput(); err != nil {
		// The named voice may not be installed: use the system voice.
		say = exec.CommandContext(ctx, "/usr/bin/say", "-o", aiff, "--", text)
		if b2, err2 := say.CombinedOutput(); err2 != nil {
			return nil, 0, fmt.Errorf("say failed: %s %s", strings.TrimSpace(string(b)), strings.TrimSpace(string(b2)))
		}
	}
	enc := exec.CommandContext(ctx, ffmpeg, "-loglevel", "error", "-y", "-i", aiff, "-c:a", "libopus", "-b:a", "32k", "-ar", "48000", "-ac", "1", "-application", "voip", ogg)
	enc.Env = env
	if b, err := enc.CombinedOutput(); err != nil {
		return nil, 0, fmt.Errorf("ffmpeg failed: %s", strings.TrimSpace(string(b)))
	}
	data, err := os.ReadFile(ogg)
	if err != nil {
		return nil, 0, err
	}
	return data, duration(ctx, aiff), nil
}

var durRe = regexp.MustCompile(`estimated duration: ([0-9.]+)`)

func duration(ctx context.Context, path string) int {
	out, err := exec.CommandContext(ctx, "/usr/bin/afinfo", path).Output()
	if err != nil {
		return 1
	}
	if m := durRe.FindSubmatch(out); m != nil {
		if f, err := strconv.ParseFloat(string(m[1]), 64); err == nil {
			return max(1, int(f+0.5))
		}
	}
	return 1
}
