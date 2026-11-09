// Package port holds the speech and audio interfaces shared by the speech packages.
package port

import (
	"context"

	"github.com/GabiHert/positron/internal/domain"
)

// SpeechSpan is one stretch of speech found by the detector.
type SpeechSpan struct {
	StartSample int       // offset in the input, in samples
	Samples     []float32 // 16 kHz mono, [-1, 1]
}

// ModelStatus says whether one language's speech models are installed.
type ModelStatus struct {
	Language      domain.Language
	Ready         bool
	Missing       string // what is missing, "" when ready
	DownloadBytes int64  // sum of sizes still to download for this language
	Downloading   bool
}

// ModelProgress reports a model download.
type ModelProgress struct {
	Language domain.Language
	Name     string // artifact name, "" on the final "done"
	Phase    string // "download" | "verify" | "extract" | "done" | "error"
	Done     int64
	Total    int64
	Err      string
}

// SpeechEngine turns audio into text.
type SpeechEngine interface {
	Ready(lang domain.Language) (bool, string)
	Transcribe(ctx context.Context, samples []float32, sampleRate int, lang domain.Language) (string, error)
}

// SpeechDetector finds the parts of audio that hold speech.
type SpeechDetector interface {
	Ready() (bool, string)
	Detect(samples []float32, sampleRate int) ([]SpeechSpan, error)
}

// ModelManager installs speech models.
type ModelManager interface {
	Status(lang domain.Language) ModelStatus
	Download(ctx context.Context, lang domain.Language, onProgress func(ModelProgress)) error
}

// AudioInspector reads a WAV file's level.
type AudioInspector interface {
	Inspect(path string) (WavInfo, error)
}

// WavInfo is what AudioInspector found.
type WavInfo struct {
	Silent         bool // the level is below the silence threshold
	DigitalSilence bool // every sample is exactly 0
}
