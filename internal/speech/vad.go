package speech

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	sherpa "github.com/k2-fsa/sherpa-onnx-go/sherpa_onnx"

	"github.com/GabiHert/deputy/internal/port"
)

var _ port.SpeechDetector = (*Detector)(nil)

const (
	vadSampleRate = 16000
	vadWindowSize = 512
	// vadBufferSeconds is the detector's internal buffer; longer than one 30 s chunk.
	vadBufferSeconds = 60
)

// Detector finds speech spans with sherpa-onnx's Silero detector.
type Detector struct {
	modelPath string
	mu        sync.Mutex
}

// NewDetector builds a detector that reads modelsDir/silero_vad/silero_vad.onnx.
func NewDetector(modelsDir string) *Detector {
	return &Detector{modelPath: filepath.Join(modelsDir, "silero_vad", "silero_vad.onnx")}
}

// Ready reports whether the detector model is on disk.
func (d *Detector) Ready() (bool, string) {
	if _, err := os.Stat(d.modelPath); err != nil {
		return false, "speech detector model is not installed"
	}
	return true, ""
}

// Detect returns the speech spans in 16 kHz mono samples.
func (d *Detector) Detect(samples []float32, sampleRate int) ([]port.SpeechSpan, error) {
	if sampleRate != vadSampleRate {
		return nil, fmt.Errorf("speech detector needs %d Hz audio, got %d Hz", vadSampleRate, sampleRate)
	}
	if ok, msg := d.Ready(); !ok {
		return nil, fmt.Errorf("%s", msg)
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	cfg := sherpa.VadModelConfig{}
	cfg.SileroVad.Model = d.modelPath
	cfg.SileroVad.Threshold = 0.5
	cfg.SileroVad.MinSilenceDuration = 0.5
	cfg.SileroVad.MinSpeechDuration = 0.25
	cfg.SileroVad.WindowSize = vadWindowSize
	cfg.SileroVad.MaxSpeechDuration = 20
	cfg.SampleRate = vadSampleRate
	cfg.NumThreads = 1
	cfg.Provider = "cpu"
	vad := sherpa.NewVoiceActivityDetector(&cfg, vadBufferSeconds)
	if vad == nil {
		return nil, fmt.Errorf("failed to load the speech detector from %s", d.modelPath)
	}
	defer sherpa.DeleteVoiceActivityDetector(vad)

	var spans []port.SpeechSpan
	drain := func() {
		for !vad.IsEmpty() {
			s := vad.Front()
			vad.Pop()
			spans = append(spans, port.SpeechSpan{StartSample: s.Start, Samples: s.Samples})
		}
	}
	for i := 0; i < len(samples); i += vadWindowSize {
		end := i + vadWindowSize
		window := samples[i:min(end, len(samples))]
		if len(window) < vadWindowSize {
			padded := make([]float32, vadWindowSize)
			copy(padded, window)
			window = padded
		}
		vad.AcceptWaveform(window)
		drain()
	}
	vad.Flush()
	drain()
	return spans, nil
}
