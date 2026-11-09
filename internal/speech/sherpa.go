// Copied from github.com/brienze1/quant (internal/integration/voice/engine/sherpaengine/sherpaengine.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

// Package speech runs the offline speech engine: sherpa-onnx Whisper for
// speech-to-text and the Silero detector, plus the model manifest and downloader.
// The engine serves one language per call and rebuilds its recognizer when the
// language changes, since a sherpa instance's language is fixed at build time.
// Models load lazily on first use and unload after an idle period. All sherpa
// calls are serialized through one mutex (the C objects are not safe for
// concurrent use).
package speech

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"github.com/GabiHert/positron/internal/domain"
	"github.com/GabiHert/positron/internal/port"
	"github.com/GabiHert/positron/internal/wavinfo"
)

var _ port.SpeechEngine = (*Engine)(nil)

// Model directory names under Config.ModelsDir. The downloader extracts the
// model archives into exactly these directories.
const (
	// WhisperEnDirName holds the English-only Whisper small.en model (file
	// prefix "small.en"). WhisperMultiDirName holds the multilingual Whisper
	// turbo model (file prefix "turbo") used for Portuguese.
	WhisperEnDirName    = "sherpa-onnx-whisper-small.en"
	WhisperMultiDirName = "sherpa-onnx-whisper-turbo"
)

const (
	defaultNumThreads = 4
	defaultIdleUnload = 10 * time.Minute

	whisperSampleRate = 16000
	// whisperFeatureDim stays 80 for every model, turbo included: sherpa-onnx reads
	// the mel-bin count from the model. Output with 80 and with 128 was byte-identical
	// on sherpa-onnx-go v1.13.3; re-check this when the library changes.
	whisperFeatureDim = 80
)

// isPTBR reports whether lang is Brazilian Portuguese, in any case.
func isPTBR(lang domain.Language) bool {
	return strings.EqualFold(strings.TrimSpace(string(lang)), string(domain.LangPTBR))
}

// sttDirName is the Whisper model directory for a language.
func sttDirName(lang domain.Language) string {
	if isPTBR(lang) {
		return WhisperMultiDirName
	}
	return WhisperEnDirName
}

// sttFilePrefix is the Whisper file prefix for a language ("turbo" vs "small.en").
func sttFilePrefix(lang domain.Language) string {
	if isPTBR(lang) {
		return "turbo"
	}
	return "small.en"
}

// whisperLang is the Whisper decoder Language value for a language.
func whisperLang(lang domain.Language) string {
	if isPTBR(lang) {
		return "pt"
	}
	return "en"
}

// sttFiles is the Whisper file set for a language.
func sttFiles(lang domain.Language) []string {
	p := sttFilePrefix(lang)
	return []string{
		p + "-encoder.int8.onnx",
		p + "-decoder.int8.onnx",
		p + "-tokens.txt",
	}
}

// RequiredFilesFor returns, per model directory, the files that must exist under
// {ModelsDir}/{dir}/ for the engine to serve lang.
func RequiredFilesFor(lang domain.Language) map[string][]string {
	return map[string][]string{sttDirName(lang): sttFiles(lang)}
}

// Config configures the engine.
type Config struct {
	// ModelsDir is the directory containing the Whisper dirs.
	ModelsDir string
	// NumThreads is the ONNX intra-op thread count (default 4).
	NumThreads int
	// IdleUnload is how long the loaded model is kept in memory after the
	// last call before being released (default 10 minutes).
	IdleUnload time.Duration
}

// Engine is the embedded sherpa-onnx speech-to-text engine. Construct with New.
type Engine struct {
	cfg Config

	// mu serializes ALL sherpa calls and guards the fields below.
	mu   sync.Mutex
	lang domain.Language
	rec  *sherpa.OfflineRecognizer
	// recLang is the language e.rec was built for ("" when not loaded).
	recLang  domain.Language
	idle     *time.Timer
	lastUsed time.Time
}

// New creates an engine rooted at cfg.ModelsDir. No model is loaded until the
// first transcription.
func New(cfg Config) *Engine {
	if cfg.NumThreads <= 0 {
		cfg.NumThreads = defaultNumThreads
	}
	if cfg.IdleUnload <= 0 {
		cfg.IdleUnload = defaultIdleUnload
	}
	return &Engine{cfg: cfg, lang: domain.LangEN}
}

// ReadyFor reports whether all model files required to serve lang exist on disk
// (no model is loaded to answer this). When files are missing, the detail names them.
func (e *Engine) ReadyFor(lang domain.Language) (bool, string) {
	required := RequiredFilesFor(lang)
	dirs := make([]string, 0, len(required))
	for dir := range required {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)

	var missing []string
	total := 0
	for _, dir := range dirs {
		for _, f := range required[dir] {
			total++
			if _, err := os.Stat(filepath.Join(e.cfg.ModelsDir, dir, f)); err != nil {
				missing = append(missing, filepath.Join(dir, f))
			}
		}
	}
	if len(missing) == total {
		return false, "speech models are not installed"
	}
	if len(missing) > 0 {
		return false, "missing speech model files: " + strings.Join(missing, ", ")
	}
	return true, ""
}

// Ready satisfies port.SpeechEngine.
func (e *Engine) Ready(lang domain.Language) (bool, string) { return e.ReadyFor(lang) }

// Transcribe satisfies port.SpeechEngine; it is TranscribeSamples.
func (e *Engine) Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error) {
	return e.TranscribeSamples(ctx, samples, sampleRate, lang)
}

// TranscribeWAV decodes the WAV payload and runs it through Whisper.
func (e *Engine) TranscribeWAV(ctx context.Context, wav []byte, lang domain.Language) (string, error) {
	samples, sampleRate, err := wavinfo.DecodeWAV(wav)
	if err != nil {
		return "", fmt.Errorf("speech engine could not decode the audio: %w", err)
	}
	return e.TranscribeSamples(ctx, samples, sampleRate, lang)
}

// TranscribeSamples runs mono float samples in [-1, 1] through Whisper for lang.
// AcceptWaveform resamples internally, so any sample rate is accepted.
func (e *Engine) TranscribeSamples(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(samples) == 0 {
		return "", fmt.Errorf("empty audio")
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.lang = lang
	if err := e.ensureSTT(); err != nil {
		return "", err
	}
	defer e.touch()

	stream := sherpa.NewOfflineStream(e.rec)
	defer sherpa.DeleteOfflineStream(stream)
	stream.AcceptWaveform(sampleRate, samples)
	e.rec.Decode(stream)
	return strings.TrimSpace(stream.GetResult().Text), nil
}

// Unload releases the loaded model. Idempotent; safe to call while other
// calls are in flight (it serializes on the engine mutex).
func (e *Engine) Unload() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.unloadLocked()
}

// ensureSTT lazily loads (or rebuilds) the Whisper model for the current
// language. Caller must hold e.mu.
func (e *Engine) ensureSTT() error {
	lang := e.lang
	if e.rec != nil && e.recLang == lang {
		return nil
	}
	if e.rec != nil {
		sherpa.DeleteOfflineRecognizer(e.rec)
		e.rec = nil
		e.recLang = ""
	}
	if ok, detail := e.ReadyFor(lang); !ok {
		return fmt.Errorf("speech engine not ready: %s", detail)
	}
	dir := filepath.Join(e.cfg.ModelsDir, sttDirName(lang))
	prefix := sttFilePrefix(lang)
	cfg := sherpa.OfflineRecognizerConfig{
		FeatConfig: sherpa.FeatureConfig{SampleRate: whisperSampleRate, FeatureDim: whisperFeatureDim},
		ModelConfig: sherpa.OfflineModelConfig{
			Whisper: sherpa.OfflineWhisperModelConfig{
				Encoder:  filepath.Join(dir, prefix+"-encoder.int8.onnx"),
				Decoder:  filepath.Join(dir, prefix+"-decoder.int8.onnx"),
				Language: whisperLang(lang),
				Task:     "transcribe",
			},
			Tokens:     filepath.Join(dir, prefix+"-tokens.txt"),
			NumThreads: e.cfg.NumThreads,
			Debug:      0,
			Provider:   "cpu",
			ModelType:  "whisper",
		},
		DecodingMethod: "greedy_search",
	}
	rec := sherpa.NewOfflineRecognizer(&cfg)
	if rec == nil {
		return fmt.Errorf("failed to load Whisper STT model from %s", dir)
	}
	e.rec = rec
	e.recLang = lang
	return nil
}

// touch marks the engine as just used and (re)arms the idle-unload timer.
// Caller must hold e.mu.
func (e *Engine) touch() {
	e.lastUsed = time.Now()
	if e.idle == nil {
		e.idle = time.AfterFunc(e.cfg.IdleUnload, e.idleUnload)
		return
	}
	e.idle.Reset(e.cfg.IdleUnload)
}

// idleUnload fires when the idle timer elapses. A call may have slipped in
// between the timer firing and the lock being acquired; in that case the timer
// was already re-armed by touch, so just skip the unload.
func (e *Engine) idleUnload() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if time.Since(e.lastUsed) < e.cfg.IdleUnload {
		return
	}
	e.unloadLocked()
}

// unloadLocked deletes the sherpa objects and stops the idle timer. Caller
// must hold e.mu.
func (e *Engine) unloadLocked() {
	if e.rec != nil {
		sherpa.DeleteOfflineRecognizer(e.rec)
		e.rec = nil
	}
	e.recLang = ""
	if e.idle != nil {
		e.idle.Stop()
		e.idle = nil
	}
}
