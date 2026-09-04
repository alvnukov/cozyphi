package composer

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// fakeVoice records what the composer asked the microphone to do. The editor
// owns the real session; the composer only drives this seam.
type fakeVoice struct {
	starts   int
	pauses   int
	resumes  int
	flushes  int
	ends     int
	discards int
}

func (f *fakeVoice) VoiceStart()   { f.starts++ }
func (f *fakeVoice) VoicePause()   { f.pauses++ }
func (f *fakeVoice) VoiceResume()  { f.resumes++ }
func (f *fakeVoice) VoiceFlush()   { f.flushes++ }
func (f *fakeVoice) VoiceEnd()     { f.ends++ }
func (f *fakeVoice) VoiceDiscard() { f.discards++ }

// fakeClock is the composer's time source under test. It moves only when the
// test says so, so the tap and hold rules need no sleeps.
type fakeClock struct{ at time.Time }

func (c *fakeClock) now() time.Time { return c.at }

func (c *fakeClock) advance(d time.Duration) { c.at = c.at.Add(d) }

// newVoicePane builds a wired composer with a fake microphone, plus the clock
// the Space rule reads. Hold mode starts unproven, the way it does at startup:
// only a release that actually arrives turns it on.
func newVoicePane(t *testing.T) (*ComposerPane, *fakeVoice, *fakeClock) {
	t.Helper()
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})
	v := &fakeVoice{}
	c.SetVoice(v)
	clk := &fakeClock{at: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
	c.now = clk.now
	// The hint row is measured against the width DrawChat last saw; a wide
	// composer keeps the key hints in every row unless a test narrows it.
	c.chatWidth = 80
	return c, v, clk
}

// listening puts the composer in the mode without going through the session.
func listening(c *ComposerPane, pending int) {
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateListening, Pending: pending})
}

// send hands one key event over the way dispatch does: the focused widget —
// the chat input while typing — sees it first, and the pane ladder only gets
// what the input left unconsumed, marked with where it has already been.
func send(c *ComposerPane, ev xui.KeyEvent) {
	ctx := &components.EventContext{}
	c.Chat.Handle(ctx, ev)
	if ctx.Consume {
		return
	}
	ctx.DeliveredTo = &c.Chat
	c.Handle(ctx, ev)
}

func spacePress() xui.KeyEvent   { return xui.KeyEvent{Code: xui.KeyRune, Rune: ' ', Press: true} }
func spaceRelease() xui.KeyEvent { return xui.KeyEvent{Code: xui.KeyRune, Rune: ' '} }

// spaceRepeat is the auto-repeat a terminal that reports event types sends
// while Space stays down: still a press, but marked as one.
func spaceRepeat() xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: ' ', Press: true, Repeat: true}
}

// hintText is the hint row as the user reads it, styles dropped.
func hintText(spans []components.Span) string {
	var b strings.Builder
	for _, s := range spans {
		b.WriteString(s.Text)
	}
	return b.String()
}

func TestVoiceChordEntersTheModeAndLeavesItKeepingTheWords(t *testing.T) {
	c, v, _ := newVoicePane(t)

	send(c, xui.KeyEvent{Code: xui.KeyRune, Rune: 'g', Mods: xui.ModCtrl, Press: true})
	require.Equal(t, 1, v.starts, "Ctrl+G reaches the microphone through the keys table")

	listening(c, 0)
	send(c, xui.KeyEvent{Code: xui.KeyRune, Rune: 'g', Mods: xui.ModCtrl, Press: true})

	assert.Equal(t, 1, v.ends, "Ctrl+G again ends the mode, draining the queue")
	assert.Zero(t, v.discards, "ending is not discarding")
}

func TestEscDiscardsTheModeAndItsLateEvents(t *testing.T) {
	c, v, _ := newVoicePane(t)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 4, State: voice.StateListening})

	send(c, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	require.Equal(t, 1, v.discards)
	assert.Equal(t, voice.StateIdle, c.VoiceState(), "the hint row goes away with the mode")

	c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 4, Text: "stale words"})
	assert.Empty(t, c.Chat.Value, "a result from the discarded mode never lands")
}

func TestSpaceTapPausesAndResumes(t *testing.T) {
	c, v, clk := newVoicePane(t)
	listening(c, 0)

	send(c, spacePress())
	clk.advance(50 * time.Millisecond)
	send(c, spaceRelease())

	require.Equal(t, 1, v.pauses, "a tap while listening pauses")
	require.Equal(t, voice.StatePaused, c.VoiceState())
	assert.Zero(t, v.resumes, "a short release leaves the pause standing")

	send(c, spacePress())
	clk.advance(50 * time.Millisecond)
	send(c, spaceRelease())

	assert.Equal(t, 1, v.resumes, "a tap while paused resumes")
	assert.Equal(t, voice.StateListening, c.VoiceState())
}

func TestSpaceHeldWhileListeningPausesThenResumesOnRelease(t *testing.T) {
	c, v, clk := newVoicePane(t)
	listening(c, 0)

	send(c, spacePress())
	require.Equal(t, 1, v.pauses, "the press pauses at once")

	clk.advance(350 * time.Millisecond)
	send(c, spaceRelease())

	assert.Equal(t, 1, v.resumes, "a held Space resumes when it comes back up")
	assert.Equal(t, voice.StateListening, c.VoiceState())
}

func TestSpaceHeldWhilePausedTalksAndPausesAgain(t *testing.T) {
	c, v, clk := newVoicePane(t)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})

	send(c, spacePress())
	require.Equal(t, 1, v.resumes, "push-to-talk opens the microphone on the press")
	require.Equal(t, voice.StateListening, c.VoiceState())

	clk.advance(350 * time.Millisecond)
	send(c, spaceRelease())

	assert.Equal(t, 1, v.pauses, "letting go pauses again")
	assert.Equal(t, voice.StatePaused, c.VoiceState())
}

func TestSpaceAutoRepeatAndStrayReleasesAreIgnored(t *testing.T) {
	c, v, clk := newVoicePane(t)
	listening(c, 0)

	send(c, spacePress())
	clk.advance(40 * time.Millisecond)
	send(c, spacePress())
	clk.advance(40 * time.Millisecond)
	send(c, spacePress())
	require.Equal(t, 1, v.pauses, "auto-repeat while the key is down changes nothing")

	clk.advance(50 * time.Millisecond)
	send(c, spaceRelease())
	send(c, spaceRelease())
	assert.Zero(t, v.resumes, "a release with no press behind it belongs to an earlier state")
}

func TestSpaceRepeatsAreTimedOutWhereReleasesNeverArrive(t *testing.T) {
	c, v, clk := newVoicePane(t)
	listening(c, 0)

	send(c, spacePress())
	clk.advance(100 * time.Millisecond)
	send(c, spacePress())
	require.Equal(t, 1, v.pauses, "two presses inside the repeat window count once")
	require.Zero(t, v.resumes)

	clk.advance(tapRepeatWindow + time.Millisecond)
	send(c, spacePress())
	assert.Equal(t, 1, v.resumes, "a press after the window has slid past is a new tap")
}

// TestSpaceRepeatsAreDroppedWhereTheTerminalReportsThem: a reported repeat is
// never a second press, however wide the gap. macOS's slower key-repeat delays
// put the first repeat past tapRepeatWindow, where the timing rule alone read
// it as a fresh tap and flipped the microphone back mid-hold; the terminal's
// own answer settles it without consulting either window.
func TestSpaceRepeatsAreDroppedWhereTheTerminalReportsThem(t *testing.T) {
	c, v, clk := newVoicePane(t)
	listening(c, 0)

	send(c, spacePress())
	require.Equal(t, 1, v.pauses, "the press pauses at once")

	for range 3 {
		clk.advance(holdRepeatWindow + time.Second)
		send(c, spaceRepeat())
	}

	assert.Equal(t, 1, v.pauses, "the repeats change nothing")
	assert.Zero(t, v.resumes, "the microphone stays where the press put it")
	assert.Equal(t, voice.StatePaused, c.VoiceState())
}

func TestSpaceReachesTheChatWhenItIsNotAControlKey(t *testing.T) {
	tests := map[string]struct {
		modeOff    bool
		mods       xui.Modifiers
		slashOpen  bool
		wantTyped  string
		wantReason string
	}{
		"shift types a space":      {mods: xui.ModShift, wantTyped: " "},
		"ctrl belongs to the chat": {mods: xui.ModCtrl},
		"alt belongs to the chat":  {mods: xui.ModAlt},
		"a picker owns the key":    {slashOpen: true, wantTyped: " "},
		"the mode is off":          {modeOff: true, wantTyped: " "},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, v, _ := newVoicePane(t)
			if !tc.modeOff {
				listening(c, 0)
			}
			// The picker flag lives in both places in the running app: the
			// pane opens the picker and mirrors it into the chat input.
			c.slash.Open, c.Chat.SlashOpen = tc.slashOpen, tc.slashOpen

			send(c, xui.KeyEvent{Code: xui.KeyRune, Rune: ' ', Mods: tc.mods, Press: true})

			assert.Zero(t, v.pauses, "the microphone is not a modifier's business")
			assert.Zero(t, v.resumes)
			assert.Equal(t, tc.wantTyped, c.Chat.Value)
		})
	}
}

// TestSpaceIsNotTypedWhileTheModeIsOn pins the focus order: dispatch hands the
// key to the chat input before the pane ladder, so only the input's deferral
// keeps a control Space out of the buffer.
func TestSpaceIsNotTypedWhileTheModeIsOn(t *testing.T) {
	c, v, clk := newVoicePane(t)
	c.Chat.Value = "half said"
	c.Chat.Cursor = len(c.Chat.Value)
	listening(c, 0)
	require.True(t, c.Chat.VoiceMode, "the composer mirrors the mode into the input")

	send(c, spacePress())
	clk.advance(50 * time.Millisecond)
	send(c, spaceRelease())

	require.Equal(t, 1, v.pauses, "the tap reaches the microphone")
	require.Equal(t, voice.StatePaused, c.VoiceState())
	assert.Equal(t, "half said", c.Chat.Value, "the control key never lands in the buffer")
}

// TestEnterWhileListeningNeverSubmitsThroughTheChatInput: bare Enter belongs to
// the mode, so the focused input must not fire its own submit on the way past.
func TestEnterWhileListeningNeverSubmitsThroughTheChatInput(t *testing.T) {
	c, v, _ := newVoicePane(t)
	var submits int
	c.Chat.OnSubmit = func(string) { submits++ }
	listening(c, 0)
	c.Chat.Value = "spoken words"
	c.Chat.Cursor = len(c.Chat.Value)

	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	require.Equal(t, 1, v.flushes, "Enter closes the open segment")
	assert.Zero(t, submits, "the send waits for the microphone, not for the input")
	assert.Contains(t, hintText(c.Chat.HintsRight), "finishing… then send")
}

// TestLeavingTheModeGivesTheKeysBack: with the mode off the input owns Space
// and Enter again.
func TestLeavingTheModeGivesTheKeysBack(t *testing.T) {
	c, v, _ := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	listening(c, 0)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateIdle})
	require.False(t, c.Chat.VoiceMode)

	send(c, spacePress())
	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	assert.Equal(t, " ", c.Chat.Value, "Space types again")
	assert.Equal(t, " ", submitted, "Enter submits again")
	assert.Zero(t, v.flushes, "the microphone is not asked anything once the mode is off")
}

func TestEnterFlushesAndSendsOnceTheQueueIsEmpty(t *testing.T) {
	c, v, _ := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	listening(c, 1)
	c.Chat.Value = "the whole prompt"
	c.Chat.Cursor = len(c.Chat.Value)

	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	require.Equal(t, 1, v.flushes, "Enter closes the open segment")
	require.Empty(t, submitted, "nothing is sent while a segment is still in flight")
	assert.Contains(t, hintText(c.Chat.HintsRight), "finishing… then send")

	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateFinishing, Pending: 0})
	assert.Equal(t, "the whole prompt", submitted, "the send waits for the queue, not the clock")
}

func TestEnterSendsAtOnceWhenNothingIsInFlight(t *testing.T) {
	c, _, _ := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})
	c.Chat.Value = "ready"
	c.Chat.Cursor = len(c.Chat.Value)

	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	assert.Equal(t, "ready", submitted)
}

func TestEscTakesBackAPendingSendAndKeepsTheMode(t *testing.T) {
	c, v, _ := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	listening(c, 1)
	c.Chat.Value = "half said"

	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	send(c, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	require.Zero(t, v.discards, "the first Esc only takes back the send")
	require.Equal(t, voice.StateListening, c.VoiceState())

	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateListening, Pending: 0})
	assert.Empty(t, submitted, "the cancelled send does not fire when the queue drains")
}

func TestAFailedSegmentTakesBackAPendingSend(t *testing.T) {
	c, _, _ := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	listening(c, 1)
	c.Chat.Value = "half said"

	send(c, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	c.ApplyVoiceError(controller.VoiceErrorMsg{Gen: 1, Seq: 2, Text: "voice: transcription failed"})
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateListening, Pending: 0})

	assert.Empty(t, submitted, "the user reads the toast before anything is sent")
	assert.Equal(t, voice.StateListening, c.VoiceState(), "one bad segment does not end the mode")
}

func TestVoiceResultIsInsertedAtTheCaretWithSpacing(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		cursor int
		want   string
	}{
		{name: "into an empty composer", value: "", cursor: 0, want: "hello there"},
		{name: "after a word", value: "note:", cursor: 5, want: "note: hello there"},
		{name: "after a space", value: "note: ", cursor: 6, want: "note: hello there"},
		{name: "before a word", value: "end", cursor: 0, want: "hello there end"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _, _ := newVoicePane(t)
			c.Chat.Value = tc.value
			c.Chat.Cursor = tc.cursor

			c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 1, Seq: 1, Text: "hello there"})

			assert.Equal(t, tc.want, c.Chat.Value)
		})
	}
}

func TestVoiceResultsLandInTheOrderTheyWereSpoken(t *testing.T) {
	c, _, _ := newVoicePane(t)
	listening(c, 2)

	c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 1, Seq: 1, Text: "first sentence"})
	c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 1, Seq: 2, Text: "second sentence"})

	assert.Equal(t, "first sentence second sentence", c.Chat.Value)
}

func TestVoiceHintRowSaysWhatTheMicrophoneIsDoing(t *testing.T) {
	tests := map[string]struct {
		setup    func(c *ComposerPane, v *fakeVoice)
		contains []string
		absent   []string
	}{
		"listening": {
			setup:    func(c *ComposerPane, _ *fakeVoice) { listening(c, 0) },
			contains: []string{"●", "Space pause · ^G done"},
		},
		"listening with a queue": {
			setup:    func(c *ComposerPane, _ *fakeVoice) { listening(c, 2) },
			contains: []string{"⋯2", "Space pause · ^G done"},
		},
		"starting": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateListening, Starting: true})
			},
			contains: []string{"● ", "starting…"},
		},
		"paused with releases": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				send(c, spaceRelease())
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused, Pending: 1})
			},
			contains: []string{"‖ paused", "⋯1", "Space resume · hold to talk · ^G done"},
		},
		"paused without releases": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})
			},
			contains: []string{"‖ paused", "Space resume · ^G done"},
			absent:   []string{"hold to talk"},
		},
		"talking": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				send(c, spaceRelease())
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})
				send(c, spacePress())
			},
			contains: []string{"talking", "release to pause"},
		},
		"pausing while the key is down": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				send(c, spaceRelease())
				listening(c, 0)
				send(c, spacePress())
			},
			contains: []string{"‖ paused", "release to resume"},
		},
		"finishing": {
			setup: func(c *ComposerPane, _ *fakeVoice) {
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateFinishing, Pending: 1})
			},
			contains: []string{"⋯ finishing…"},
			absent:   []string{"then send"},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, v, _ := newVoicePane(t)
			tc.setup(c, v)

			row := hintText(c.Chat.HintsRight)
			for _, want := range tc.contains {
				assert.Contains(t, row, want)
			}
			for _, unwanted := range tc.absent {
				assert.NotContains(t, row, unwanted)
			}
		})
	}
}

func TestVoiceHintRowDropsTheKeyHintWhenTheComposerIsNarrow(t *testing.T) {
	c, _, _ := newVoicePane(t)
	c.chatWidth = 14
	listening(c, 0)

	row := hintText(c.Chat.HintsRight)
	assert.Contains(t, row, "●", "the state itself always shows")
	assert.NotContains(t, row, "Space pause", "the key hint is what gives way")
}

func TestVoiceHintRowCoversTheUsageHintsAndGivesThemBack(t *testing.T) {
	c, _, _ := newVoicePane(t)
	usage := []components.Span{{Text: "12k tokens"}}
	c.SetUsageHints(usage)
	require.Equal(t, usage, c.Chat.HintsRight)

	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateListening, Level: 0.5})
	require.NotEqual(t, usage, c.Chat.HintsRight, "the meter owns the hint row while the mode is on")

	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateIdle})
	assert.Equal(t, usage, c.Chat.HintsRight, "the usage hints come back when the mode ends")
}

func TestVoicePlaceholderNamesTheState(t *testing.T) {
	tests := map[string]struct {
		setup func(c *ComposerPane)
		want  string
	}{
		"idle keeps the posture placeholder": {setup: func(*ComposerPane) {}, want: askPlaceholder},
		"listening": {
			setup: func(c *ComposerPane) { listening(c, 0) },
			want:  "Listening… speak, or type",
		},
		"paused": {
			setup: func(c *ComposerPane) {
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})
			},
			want: "Paused. Space to resume",
		},
		"talking": {
			setup: func(c *ComposerPane) {
				send(c, spaceRelease())
				c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StatePaused})
				send(c, spacePress())
			},
			want: "Talking…",
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			c, _, _ := newVoicePane(t)
			tc.setup(c)

			assert.Equal(t, tc.want, c.Chat.Placeholder)
		})
	}
}
