// Copied from github.com/brienze1/quant (internal/integration/voice/engine/wav.go), MIT License,
// Copyright (c) 2026 Luis Brienze. Modified for Notted; see NOTICE.md.

package wavinfo

import (
	"encoding/binary"
	"fmt"
	"math"
)

// WAVE format tags we understand (from the fmt chunk).
const (
	wavFormatPCM       = 1 // integer PCM
	wavFormatIEEEFloat = 3 // IEEE float
)

// DecodeWAV decodes an in-memory RIFF/WAVE file into normalized float32 samples
// in [-1, 1] plus the sample rate. Both 16-bit integer PCM (format 1) and
// 32-bit IEEE float (format 3) are supported — the browser VAD/PTT capture
// emits float32 WAV, while sherpa's own TTS output is 16-bit PCM. Stereo input
// is downmixed to mono by averaging the channels. Chunks other than "fmt " and
// "data" (LIST, fact, ...) are skipped, including their word-alignment pad
// bytes. Other encodings are rejected with a clear error.
func DecodeWAV(data []byte) ([]float32, int, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, 0, fmt.Errorf("not a RIFF/WAVE file")
	}

	var (
		haveFmt    bool
		format     int
		channels   int
		sampleRate int
		bits       int
	)

	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		if size < 0 || body+size > len(data) {
			return nil, 0, fmt.Errorf("corrupt WAV: chunk %q overruns file", id)
		}

		switch id {
		case "fmt ":
			if size < 16 {
				return nil, 0, fmt.Errorf("corrupt WAV: fmt chunk too short (%d bytes)", size)
			}
			format = int(binary.LittleEndian.Uint16(data[body:]))
			channels = int(binary.LittleEndian.Uint16(data[body+2:]))
			sampleRate = int(binary.LittleEndian.Uint32(data[body+4:]))
			bits = int(binary.LittleEndian.Uint16(data[body+14:]))
			// WAVE_FORMAT_EXTENSIBLE (afconvert writes it): the real format is the
			// first two bytes of the SubFormat GUID, at offset 24 of the fmt body.
			if format == 0xFFFE && size >= 40 && body+26 <= len(data) {
				format = int(binary.LittleEndian.Uint16(data[body+24:]))
			}
			haveFmt = true

		case "data":
			if !haveFmt {
				return nil, 0, fmt.Errorf("corrupt WAV: data chunk before fmt chunk")
			}
			if channels != 1 && channels != 2 {
				return nil, 0, fmt.Errorf("unsupported WAV channel count (%d): only mono or stereo is supported", channels)
			}
			if sampleRate <= 0 {
				return nil, 0, fmt.Errorf("corrupt WAV: invalid sample rate %d", sampleRate)
			}
			raw := data[body : body+size]
			switch {
			case format == wavFormatPCM && bits == 16:
				return decodePCM16(raw, channels), sampleRate, nil
			case format == wavFormatIEEEFloat && bits == 32:
				return decodeFloat32(raw, channels), sampleRate, nil
			default:
				return nil, 0, fmt.Errorf("unsupported WAV encoding (format %d, %d-bit): want 16-bit PCM or 32-bit float", format, bits)
			}
		}

		pos = body + size
		if size%2 == 1 {
			pos++ // RIFF chunks are word-aligned; skip the pad byte
		}
	}
	return nil, 0, fmt.Errorf("corrupt WAV: no data chunk")
}

// decodePCM16 converts little-endian 16-bit PCM bytes to normalized float32
// samples, averaging channel pairs into mono when channels == 2. A trailing
// partial frame is dropped.
func decodePCM16(pcm []byte, channels int) []float32 {
	frameBytes := 2 * channels
	frames := len(pcm) / frameBytes
	samples := make([]float32, frames)
	for i := 0; i < frames; i++ {
		off := i * frameBytes
		if channels == 1 {
			samples[i] = float32(int16(binary.LittleEndian.Uint16(pcm[off:]))) / 32768
			continue
		}
		l := float32(int16(binary.LittleEndian.Uint16(pcm[off:])))
		r := float32(int16(binary.LittleEndian.Uint16(pcm[off+2:])))
		samples[i] = (l + r) / 2 / 32768
	}
	return samples
}

// decodeFloat32 reads little-endian 32-bit IEEE float PCM (already normalized
// to [-1, 1]), averaging channel pairs into mono when channels == 2. A trailing
// partial frame is dropped. This is the format the browser VAD/PTT capture
// emits (@ricky0123/vad-web encodeWAV defaults to 32-bit float).
func decodeFloat32(pcm []byte, channels int) []float32 {
	frameBytes := 4 * channels
	frames := len(pcm) / frameBytes
	samples := make([]float32, frames)
	for i := 0; i < frames; i++ {
		off := i * frameBytes
		if channels == 1 {
			samples[i] = math.Float32frombits(binary.LittleEndian.Uint32(pcm[off:]))
			continue
		}
		l := math.Float32frombits(binary.LittleEndian.Uint32(pcm[off:]))
		r := math.Float32frombits(binary.LittleEndian.Uint32(pcm[off+4:]))
		samples[i] = (l + r) / 2
	}
	return samples
}

// Header holds the fields of a WAV file's fmt chunk and the size of its data chunk.
type Header struct {
	Format        int
	Channels      int
	SampleRate    int
	BitsPerSample int
	DataBytes     int64
	// DataOffset is where the data chunk's samples start in the file.
	DataOffset int
}

// ParseHeader walks the RIFF chunks like DecodeWAV and returns the fmt fields
// and the data size. A data chunk that claims more bytes than the file holds
// (a recording cut short) is clamped to the bytes present.
func ParseHeader(data []byte) (Header, error) {
	if len(data) < 12 || string(data[0:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return Header{}, fmt.Errorf("not a RIFF/WAVE file")
	}
	var h Header
	haveFmt := false
	pos := 12
	for pos+8 <= len(data) {
		id := string(data[pos : pos+4])
		size := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		body := pos + 8
		switch id {
		case "fmt ":
			if size < 16 || body+size > len(data) {
				return Header{}, fmt.Errorf("corrupt WAV: fmt chunk too short (%d bytes)", size)
			}
			h.Format = int(binary.LittleEndian.Uint16(data[body:]))
			h.Channels = int(binary.LittleEndian.Uint16(data[body+2:]))
			h.SampleRate = int(binary.LittleEndian.Uint32(data[body+4:]))
			h.BitsPerSample = int(binary.LittleEndian.Uint16(data[body+14:]))
			haveFmt = true
		case "data":
			if !haveFmt {
				return Header{}, fmt.Errorf("corrupt WAV: data chunk before fmt chunk")
			}
			if body+size > len(data) {
				size = len(data) - body
			}
			h.DataBytes = int64(size)
			h.DataOffset = body
			return h, nil
		}
		if body+size > len(data) {
			return Header{}, fmt.Errorf("corrupt WAV: chunk %q overruns file", id)
		}
		pos = body + size
		if size%2 == 1 {
			pos++
		}
	}
	return Header{}, fmt.Errorf("corrupt WAV: no data chunk")
}
