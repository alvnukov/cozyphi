package voice

import (
	"encoding/binary"
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeWAVWritesACanonicalHeader(t *testing.T) {
	samples := []int16{0, 1, -1, math.MaxInt16}
	out := EncodeWAV(samples, SampleRate)

	require.Len(t, out, wavHeaderSize+len(samples)*2)
	assert.Equal(t, "RIFF", string(out[0:4]))
	assert.Equal(t, "WAVE", string(out[8:12]))
	assert.Equal(t, "fmt ", string(out[12:16]))
	assert.Equal(t, "data", string(out[36:40]))
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(out[20:22]), "PCM format")
	assert.Equal(t, uint16(1), binary.LittleEndian.Uint16(out[22:24]), "mono")
	assert.Equal(t, uint32(SampleRate), binary.LittleEndian.Uint32(out[24:28]))
	assert.Equal(t, uint16(16), binary.LittleEndian.Uint16(out[34:36]), "bits per sample")
	assert.Equal(t, uint32(len(samples)*2), binary.LittleEndian.Uint32(out[40:44]))
}

func TestEncodeWAVFallsBackToTheDefaultRate(t *testing.T) {
	out := EncodeWAV(nil, 0)
	assert.Equal(t, uint32(SampleRate), binary.LittleEndian.Uint32(out[24:28]))
}

func TestDecodePCMRoundTripsAndDropsATrailingOddByte(t *testing.T) {
	samples := []int16{0, 1234, -4321, math.MinInt16}
	raw := EncodeWAV(samples, SampleRate)[wavHeaderSize:]
	assert.Equal(t, samples, DecodePCM(raw))

	assert.Equal(t, []int16{1234}, DecodePCM(append(raw[2:4:4], 0x7f)))
}

func TestIsSilentSeparatesAQuietBufferFromSpeech(t *testing.T) {
	quiet := make([]int16, 1000)
	for i := range quiet {
		quiet[i] = 20
	}
	assert.True(t, IsSilent(quiet))
	assert.True(t, IsSilent(nil))

	loud := make([]int16, 1000)
	for i := range loud {
		loud[i] = 8000
	}
	assert.False(t, IsSilent(loud))
}

func TestLevelFromRMSIsClampedToTheMeterScale(t *testing.T) {
	assert.InDelta(t, 0.0, LevelFromRMS(0), 1e-9)
	assert.InDelta(t, 0.0, LevelFromRMS(1e-9), 1e-9, "below -60 dBFS clamps to an empty bar")
	assert.InDelta(t, 1.0, LevelFromRMS(1), 1e-9, "full scale fills the bar")
	assert.InDelta(t, 0.5, LevelFromRMS(0.03162), 0.01, "-30 dBFS is half a bar")
}
