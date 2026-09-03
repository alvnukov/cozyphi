package voice

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// samplesFor is how many samples a duration holds at SampleRate.
func samplesFor(d time.Duration) int { return durationSamples(d) }

// tone is speech-loud audio, well above SpeechRMS.
func tone(d time.Duration) []int16 {
	out := make([]int16, samplesFor(d))
	for i := range out {
		out[i] = 9000
	}
	return out
}

// hush is room-quiet audio, well below SpeechRMS.
func hush(d time.Duration) []int16 {
	out := make([]int16, samplesFor(d))
	for i := range out {
		out[i] = 10
	}
	return out
}

func join(parts ...[]int16) []int16 {
	var out []int16
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func testSegmenter(mutate func(*Config)) *segmenter {
	cfg := Defaults()
	if mutate != nil {
		mutate(&cfg)
	}
	return newSegmenter(cfg)
}

func TestSegmenterIgnoresSilence(t *testing.T) {
	s := testSegmenter(nil)

	assert.Empty(t, s.Push(hush(2*time.Second)))
	assert.Nil(t, s.Flush())
	assert.False(t, s.Open())
	assert.InDelta(t, 2.0, s.Silence().Seconds(), 0.05)
}

func TestSegmenterClosesOnTrailingSilence(t *testing.T) {
	s := testSegmenter(nil)
	gap := time.Duration(DefaultSegmentSilenceMS) * time.Millisecond

	segs := s.Push(join(tone(500*time.Millisecond), hush(gap+200*time.Millisecond)))

	require.Len(t, segs, 1)
	// Half a second of speech plus the kept tail, and nothing like the whole
	// pause that closed it.
	assert.Greater(t, len(segs[0]), samplesFor(500*time.Millisecond))
	assert.Less(t, len(segs[0]), samplesFor(900*time.Millisecond))
	assert.False(t, s.Open())
}

func TestSegmenterKeepsPreRoll(t *testing.T) {
	s := testSegmenter(nil)
	gap := time.Duration(DefaultSegmentSilenceMS) * time.Millisecond

	segs := s.Push(join(hush(2*time.Second), tone(400*time.Millisecond), hush(gap+200*time.Millisecond)))

	require.Len(t, segs, 1)
	// The lead-in is bounded by segmentPreRoll however long the room was
	// quiet before the first word.
	lead := len(segs[0]) - samplesFor(400*time.Millisecond) - samplesFor(segmentKeepTail)
	assert.InDelta(t, samplesFor(segmentPreRoll), lead, float64(segmentFrame))
}

func TestSegmenterCutsAtMaxSeconds(t *testing.T) {
	s := testSegmenter(func(c *Config) { c.MaxSeconds = 1 })

	segs := s.Push(tone(2500 * time.Millisecond))

	require.Len(t, segs, 2)
	// A speaker who never pauses is cut on length, and nothing is trimmed:
	// the cut is arbitrary, so every sample is real speech.
	assert.Len(t, segs[0], SampleRate)
	assert.Len(t, segs[1], SampleRate)
	assert.True(t, s.Open())
}

func TestSegmenterReturnsEverySegmentInOneChunk(t *testing.T) {
	s := testSegmenter(func(c *Config) { c.SegmentSilenceMS = 300 })
	utterance := join(tone(300*time.Millisecond), hush(600*time.Millisecond))

	segs := s.Push(join(utterance, utterance, utterance))

	assert.Len(t, segs, 3)
}

func TestSegmenterIsIndependentOfChunkSize(t *testing.T) {
	audio := join(
		hush(400*time.Millisecond),
		tone(600*time.Millisecond),
		hush(time.Second),
		tone(300*time.Millisecond),
		hush(time.Second),
	)

	whole := testSegmenter(nil)
	one := whole.Push(audio)

	piecemeal := testSegmenter(nil)
	var many [][]int16
	// 700 samples is not a whole number of frames, so every chunk leaves a
	// remainder behind: exactly the case that must not change the answer.
	for start := 0; start < len(audio); start += 700 {
		end := min(start+700, len(audio))
		many = append(many, piecemeal.Push(audio[start:end])...)
	}

	require.Len(t, one, 2)
	require.Len(t, many, 2)
	for i := range one {
		assert.Len(t, many[i], len(one[i]), "segment %d", i)
	}
}

func TestSegmenterFlushClosesTheOpenSegment(t *testing.T) {
	s := testSegmenter(nil)

	assert.Empty(t, s.Push(tone(400*time.Millisecond)))
	require.True(t, s.Open())

	flushed := s.Flush()

	assert.NotEmpty(t, flushed)
	assert.False(t, s.Open())
	assert.Nil(t, s.Flush())
}

func TestSegmenterFlushKeepsThePartialFrame(t *testing.T) {
	s := testSegmenter(nil)
	// One frame plus a sliver, so the sliver only survives if Flush picks up
	// what Push could not classify yet.
	loud := tone(time.Second)[:segmentFrame+100]
	s.Push(loud)

	flushed := s.Flush()

	assert.Len(t, flushed, segmentFrame+100)
}

func TestSegmenterResetForgetsEverything(t *testing.T) {
	s := testSegmenter(nil)
	s.Push(join(hush(time.Second), tone(400*time.Millisecond)))
	require.True(t, s.Open())

	s.Reset()

	assert.False(t, s.Open())
	assert.Nil(t, s.Flush())
	assert.Zero(t, s.Silence())
}

func TestSegmenterSilenceSpansSegments(t *testing.T) {
	s := testSegmenter(nil)

	s.Push(join(tone(300*time.Millisecond), hush(3*time.Second)))

	// The silence that closed the segment keeps counting afterwards, so the
	// auto-pause fires when the speaker stopped, not when the queue emptied.
	assert.InDelta(t, 3.0, s.Silence().Seconds(), 0.05)
}
