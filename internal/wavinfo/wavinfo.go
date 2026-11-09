// Package wavinfo reads the format, length and level of a WAV file.
package wavinfo

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"

	"github.com/GabiHert/positron/internal/port"
)

// silentBelowDBFS is the level under which a file counts as silent.
const silentBelowDBFS = -60.0

// Info describes one WAV file.
type Info struct {
	SampleRate      int
	Channels        int
	BitsPerSample   int
	DurationSeconds float64 // DataBytes / (SampleRate * Channels * BitsPerSample/8)
	RMSdBFS         float64 // 20*log10(rms) over all samples in [-1,1]; -120 when rms == 0
	Silent          bool    // RMSdBFS < -60
	DigitalSilence  bool    // there are samples and every one is exactly 0, as a switched-off input records
}

// Read opens path and describes it.
func Read(path string) (Info, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}
	return FromBytes(data)
}

// FromBytes describes an in-memory WAV file. The level is measured over every
// sample of every channel; only 16-bit PCM and 32-bit float data have a level.
func FromBytes(data []byte) (Info, error) {
	h, err := ParseHeader(data)
	if err != nil {
		return Info{}, err
	}
	if h.SampleRate <= 0 || h.Channels <= 0 || h.BitsPerSample <= 0 {
		return Info{}, fmt.Errorf("corrupt WAV: invalid format fields")
	}
	info := Info{
		SampleRate:    h.SampleRate,
		Channels:      h.Channels,
		BitsPerSample: h.BitsPerSample,
	}
	info.DurationSeconds = float64(h.DataBytes) / float64(h.SampleRate*h.Channels*h.BitsPerSample/8)

	raw := data[h.DataOffset : h.DataOffset+int(h.DataBytes)]
	var sum float64
	var n int
	nonZero := false
	switch {
	case h.Format == wavFormatPCM && h.BitsPerSample == 16:
		n = len(raw) / 2
		for i := 0; i < n; i++ {
			raw16 := int16(binary.LittleEndian.Uint16(raw[2*i:]))
			nonZero = nonZero || raw16 != 0
			v := float64(raw16) / 32768
			sum += v * v
		}
	case h.Format == wavFormatIEEEFloat && h.BitsPerSample == 32:
		n = len(raw) / 4
		for i := 0; i < n; i++ {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(raw[4*i:])))
			nonZero = nonZero || v != 0
			sum += v * v
		}
	default:
		return Info{}, fmt.Errorf("unsupported WAV encoding (format %d, %d-bit)", h.Format, h.BitsPerSample)
	}
	info.RMSdBFS = -120
	if n > 0 && sum > 0 {
		info.RMSdBFS = 20 * math.Log10(math.Sqrt(sum/float64(n)))
	}
	info.Silent = info.RMSdBFS < silentBelowDBFS
	info.DigitalSilence = n > 0 && !nonZero
	return info, nil
}

// Inspector adapts Read to port.AudioInspector.
type Inspector struct{}

var _ port.AudioInspector = Inspector{}

// Inspect reads path and reports whether it is silent, and whether it is digital silence.
func (Inspector) Inspect(path string) (port.WavInfo, error) {
	info, err := Read(path)
	if err != nil {
		return port.WavInfo{}, err
	}
	return port.WavInfo{Silent: info.Silent, DigitalSilence: info.DigitalSilence}, nil
}
