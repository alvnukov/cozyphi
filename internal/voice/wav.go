package voice

import (
	"encoding/binary"
	"math"
)

// SilenceRMS is the whole-buffer RMS below which a recording is treated as
// silence: about -46 dBFS, well under any speech but above a quiet room's
// noise floor. There is no VAD here on purpose — the check answers one
// question, "did the microphone produce anything at all".
const SilenceRMS = 0.005

// wavHeaderSize is the size of the canonical 16-bit PCM RIFF header.
const wavHeaderSize = 44

// EncodeWAV wraps signed 16-bit little-endian mono samples in a RIFF/WAVE
// container, which is what both whisper-cpp and the HTTP endpoints expect.
func EncodeWAV(samples []int16, rate int) []byte {
	if rate <= 0 {
		rate = SampleRate
	}
	// A RIFF header cannot describe more than 4 GiB of samples. Recordings
	// are capped far below that, so the excess is dropped rather than
	// declared with a wrapped length.
	if limit := (math.MaxUint32 - wavHeaderSize) / 2; len(samples) > limit {
		samples = samples[:limit]
	}
	dataLen := len(samples) * 2
	out := make([]byte, wavHeaderSize+dataLen)

	copy(out[0:4], "RIFF")
	//nolint:gosec // G115: dataLen is clamped above to fit a uint32
	binary.LittleEndian.PutUint32(out[4:8], uint32(36+dataLen))
	copy(out[8:12], "WAVE")

	copy(out[12:16], "fmt ")
	binary.LittleEndian.PutUint32(out[16:20], 16) // PCM chunk size
	binary.LittleEndian.PutUint16(out[20:22], 1)  // PCM format
	binary.LittleEndian.PutUint16(out[22:24], 1)  // mono
	binary.LittleEndian.PutUint32(out[24:28], uint32(rate))
	binary.LittleEndian.PutUint32(out[28:32], uint32(rate*2)) // byte rate
	binary.LittleEndian.PutUint16(out[32:34], 2)              // block align
	binary.LittleEndian.PutUint16(out[34:36], 16)             // bits per sample

	copy(out[36:40], "data")
	//nolint:gosec // G115: dataLen is clamped above to fit a uint32
	binary.LittleEndian.PutUint32(out[40:44], uint32(dataLen))
	for i, s := range samples {
		//nolint:gosec // G115: int16 to uint16 is the little-endian bit pattern, not a range change
		binary.LittleEndian.PutUint16(out[wavHeaderSize+i*2:], uint16(s))
	}
	return out
}

// DecodePCM turns a raw s16le byte run into samples, dropping a trailing odd
// byte (a chunk boundary can split one sample).
func DecodePCM(b []byte) []int16 {
	out := make([]int16, len(b)/2)
	for i := range out {
		//nolint:gosec // G115: uint16 to int16 is the little-endian bit pattern, not a range change
		out[i] = int16(binary.LittleEndian.Uint16(b[i*2:]))
	}
	return out
}

// RMS is the root mean square of the samples, normalized to 0..1 of full
// scale.
func RMS(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		v := float64(s) / math.MaxInt16
		sum += v * v
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// IsSilent reports whether the whole recording is silence.
func IsSilent(samples []int16) bool {
	return len(samples) == 0 || RMS(samples) < SilenceRMS
}

// LevelFromRMS maps an RMS value onto the 0..1 meter scale, logarithmically:
// -60 dBFS is an empty bar and 0 dBFS a full one, which is how a level meter
// has to behave to be readable while someone speaks.
func LevelFromRMS(rms float64) float64 {
	if rms <= 0 {
		return 0
	}
	db := 20 * math.Log10(rms)
	level := (db + 60) / 60
	return math.Min(1, math.Max(0, level))
}
