package composer

import (
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
	toggles  int
	stops    int
	cancels  int
	autoSend bool
}

func (f *fakeVoice) ToggleVoice()        { f.toggles++ }
func (f *fakeVoice) StopVoice()          { f.stops++ }
func (f *fakeVoice) CancelVoice()        { f.cancels++ }
func (f *fakeVoice) VoiceAutoSend() bool { return f.autoSend }

func newVoicePane(t *testing.T) (*ComposerPane, *fakeVoice) {
	t.Helper()
	c := newTestPane()
	c.Wire(nil, nil, nil, "", &fakeBus{}, &fakeFocus{})
	v := &fakeVoice{}
	c.SetVoice(v)
	return c, v
}

func TestVoiceChordTogglesTheMicrophone(t *testing.T) {
	c, v := newVoicePane(t)

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: 'g', Mods: xui.ModCtrl, Press: true})

	assert.Equal(t, 1, v.toggles, "Ctrl+G reaches the microphone through the keys table")
}

func TestEnterWhileRecordingStopsAndDoesNotSend(t *testing.T) {
	c, v := newVoicePane(t)
	var submitted string
	c.Chat.OnSubmit = func(text string) { submitted = text }
	c.Chat.Value = "half a prompt"
	c.Chat.Cursor = len(c.Chat.Value)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateRecording})

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEnter, Press: true})

	assert.Equal(t, 1, v.stops, "Enter stops the recording")
	assert.Empty(t, submitted, "finishing a recording never sends the prompt")
}

func TestEscCancelsTheRecordingSilently(t *testing.T) {
	c, v := newVoicePane(t)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateRecording})

	c.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyEscape, Press: true})

	assert.Equal(t, 1, v.cancels)
	assert.Equal(t, voice.StateIdle, c.VoiceState(), "the meter goes away with the recording")
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
			c, _ := newVoicePane(t)
			c.Chat.Value = tc.value
			c.Chat.Cursor = tc.cursor

			c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 1, Text: "hello there"})

			assert.Equal(t, tc.want, c.Chat.Value)
		})
	}
}

func TestVoiceResultFromACancelledRecordingIsIgnored(t *testing.T) {
	c, _ := newVoicePane(t)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 3, State: voice.StateRecording})
	c.CancelVoice()

	c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 3, Text: "stale words"})

	assert.Empty(t, c.Chat.Value, "a result from the cancelled generation never lands")
}

func TestVoiceAutoSendOnlyWhenTheComposerWasEmpty(t *testing.T) {
	tests := []struct {
		name     string
		autoSend bool
		before   string
		want     string
	}{
		{name: "empty composer and auto_send on", autoSend: true, before: "", want: "spoken words"},
		{name: "auto_send off", autoSend: false, before: "", want: ""},
		{name: "composer had a draft", autoSend: true, before: "draft", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, v := newVoicePane(t)
			v.autoSend = tc.autoSend
			var submitted string
			c.Chat.OnSubmit = func(text string) { submitted = text }
			c.Chat.Value = tc.before
			c.Chat.Cursor = len(tc.before)

			c.ToggleVoice()
			c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateRecording})
			c.ApplyVoiceResult(controller.VoiceResultMsg{Gen: 1, Text: "spoken words"})

			assert.Equal(t, tc.want, submitted)
		})
	}
}

func TestVoiceMeterCoversTheUsageHintsAndGivesThemBack(t *testing.T) {
	c, _ := newVoicePane(t)
	usage := []components.Span{{Text: "12k tokens"}}
	c.SetUsageHints(usage)
	require.Equal(t, usage, c.Chat.HintsRight)

	c.ApplyVoiceState(
		controller.VoiceStateMsg{Gen: 1, State: voice.StateRecording, Elapsed: 3 * time.Second, Level: 0.5},
	)
	require.NotEqual(t, usage, c.Chat.HintsRight, "the meter owns the hint row while recording")

	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateIdle})
	assert.Equal(t, usage, c.Chat.HintsRight, "the usage hints come back when the microphone stops")
}

func TestVoiceErrorClearsTheMeter(t *testing.T) {
	c, _ := newVoicePane(t)
	c.ApplyVoiceState(controller.VoiceStateMsg{Gen: 1, State: voice.StateTranscribing})

	c.ApplyVoiceError(controller.VoiceErrorMsg{Gen: 1, Text: "voice: transcription failed"})

	assert.Equal(t, voice.StateIdle, c.VoiceState())
	assert.Empty(t, c.Chat.HintsRight)
}
