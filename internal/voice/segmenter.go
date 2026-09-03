package voice

import "time"

// SpeechRMS is the per-frame RMS above which a 20 ms frame counts as speech.
// It sits above SilenceRMS, which stays the whole-buffer floor answering "did
// the microphone produce anything at all": the segmenter needs a threshold
// that a quiet room reliably stays under, not one that merely proves the
// device is alive.
const SpeechRMS = 0.01

const (
	// segmentFrame is the classification unit: 20 ms at SampleRate. Chunks
	// arrive in whatever size the reader produced, so they are cut into
	// fixed frames and the remainder is carried to the next Push. Without
	// that the answer would depend on how the audio happened to be split.
	segmentFrame = SampleRate / 50
	// segmentPreRoll is the audio kept ahead of the first speech frame, so a
	// segment does not start on a clipped consonant.
	segmentPreRoll = 300 * time.Millisecond
	// segmentKeepTail is how much trailing silence survives the trim. Some
	// tail helps the transcriber hear the final word out; the rest is the
	// pause that closed the segment and is only dead weight in the queue.
	segmentKeepTail = 200 * time.Millisecond
)

// segmenter cuts a continuous stream into utterances. It is a pure function
// object: it owns no goroutine, no clock and no audio source, and every
// decision follows from the samples handed to Push. Time is counted in
// samples, so tests run at whatever speed they like.
//
// The rules are the ones a speaker expects. A segment opens on the first
// speech frame and carries the pre-roll before it. It closes after trailing
// silence longer than the configured gap, or when it reaches the maximum
// length. Silence alone never opens anything, so a quiet microphone produces
// no segments at all however long it runs.
type segmenter struct {
	preRoll  int // samples of lead-in kept before speech
	silence  int // trailing silence that closes a segment, in samples
	maxLen   int // longest segment, in samples
	keepTail int // trailing silence left in a closed segment

	pending []int16 // partial frame carried to the next Push
	lead    []int16 // pre-roll ring, only while no segment is open
	open    []int16 // the segment being built
	quiet   int     // trailing silence samples inside open
	idle    int     // continuous silence samples, across segments
}

// newSegmenter builds a segmenter from the decoded config. Out-of-range values
// fall back to the defaults, so a zero Config still segments sensibly.
func newSegmenter(cfg Config) *segmenter {
	silenceMS := cfg.SegmentSilenceMS
	if silenceMS <= 0 {
		silenceMS = DefaultSegmentSilenceMS
	}
	maxSeconds := cfg.MaxSeconds
	if maxSeconds <= 0 {
		maxSeconds = DefaultMaxSeconds
	}
	return &segmenter{
		preRoll:  durationSamples(segmentPreRoll),
		silence:  silenceMS * SampleRate / 1000,
		maxLen:   maxSeconds * SampleRate,
		keepTail: durationSamples(segmentKeepTail),
	}
}

// durationSamples converts a duration to a sample count at SampleRate.
func durationSamples(d time.Duration) int {
	return int(d.Milliseconds()) * SampleRate / 1000
}

// Push feeds a chunk and returns every segment the chunk closed, in order. A
// single chunk can close more than one segment when the caller drains a long
// backlog, so the result is a slice rather than one segment: nothing is lost
// because the reader fell behind.
func (s *segmenter) Push(chunk []int16) [][]int16 {
	if len(chunk) == 0 {
		return nil
	}
	s.pending = append(s.pending, chunk...)
	var out [][]int16
	for len(s.pending) >= segmentFrame {
		frame := s.pending[:segmentFrame]
		s.pending = s.pending[segmentFrame:]
		if seg := s.frame(frame); seg != nil {
			out = append(out, seg)
		}
	}
	// Reclaim the head of pending so it does not grow without bound.
	if len(s.pending) == 0 && cap(s.pending) > 0 {
		s.pending = s.pending[:0:0]
	}
	return out
}

// frame classifies one fixed-size frame and returns a segment if it closed one.
func (s *segmenter) frame(frame []int16) []int16 {
	speech := RMS(frame) >= SpeechRMS
	if speech {
		s.idle = 0
	} else {
		s.idle += len(frame)
	}

	if s.open == nil {
		if !speech {
			s.remember(frame)
			return nil
		}
		s.open = make([]int16, 0, len(s.lead)+s.maxLen/8)
		s.open = append(s.open, s.lead...)
		s.lead = s.lead[:0]
		s.open = append(s.open, frame...)
		s.quiet = 0
		return nil
	}

	s.open = append(s.open, frame...)
	if speech {
		s.quiet = 0
	} else {
		s.quiet += len(frame)
	}
	switch {
	case s.quiet >= s.silence:
		return s.close(true)
	case len(s.open) >= s.maxLen:
		// The speaker never paused. Keep every sample: the cut is
		// arbitrary, so trimming a "tail" here would drop real speech.
		return s.close(false)
	}
	return nil
}

// remember keeps the frame in the pre-roll ring, dropping the oldest audio.
func (s *segmenter) remember(frame []int16) {
	s.lead = append(s.lead, frame...)
	if extra := len(s.lead) - s.preRoll; extra > 0 {
		s.lead = append(s.lead[:0], s.lead[extra:]...)
	}
}

// close finishes the open segment and returns it, trimming the trailing
// silence down to keepTail when the segment ended on a pause.
func (s *segmenter) close(trim bool) []int16 {
	seg, quiet := s.open, s.quiet
	s.open = nil
	s.quiet = 0
	s.lead = s.lead[:0]
	if trim {
		if drop := quietTail(len(seg), quiet, s.keepTail); drop > 0 {
			seg = seg[:len(seg)-drop]
		}
	}
	if len(seg) == 0 {
		return nil
	}
	return seg
}

// quietTail reports how many trailing samples to drop from a closed segment of
// length n that ended on quiet samples of trailing silence.
func quietTail(n, quiet, keep int) int {
	drop := quiet - keep
	if drop <= 0 {
		return 0
	}
	return min(drop, n)
}

// Flush closes whatever is open and returns it, or nil when the segmenter is
// between utterances. A partial frame still in pending joins the segment, so
// ending the mode does not clip the last syllable.
func (s *segmenter) Flush() []int16 {
	if len(s.pending) > 0 && s.open != nil {
		s.open = append(s.open, s.pending...)
	}
	s.pending = s.pending[:0:0]
	if s.open == nil {
		return nil
	}
	// The tail here is whatever silence had accumulated before the flush,
	// which is by definition shorter than the closing gap; trim it anyway so
	// a flush after a long pause does not queue a mostly-empty segment.
	return s.close(true)
}

// Reset forgets every buffer. Used when the mode pauses: audio heard before
// the pause must not leak into the segment recorded after it.
func (s *segmenter) Reset() {
	s.pending = s.pending[:0:0]
	s.lead = s.lead[:0]
	s.open = nil
	s.quiet = 0
	s.idle = 0
}

// Silence reports how long the microphone has heard nothing but silence. It
// spans segments, so a speaker who stops talking crosses the auto-pause
// threshold at the same moment whether or not a segment was just closed.
func (s *segmenter) Silence() time.Duration {
	return time.Duration(s.idle) * time.Second / time.Duration(SampleRate)
}

// Open reports whether a segment is being built.
func (s *segmenter) Open() bool { return s.open != nil }
