package editor

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// noBinaries is a binary lookup that finds nothing, so a test resolves voice
// without ffmpeg or whisper-cli ever being consulted on PATH.
func noBinaries(string) (string, error) { return "", errors.New("not found") }

func TestVoiceStateMovesTheFooterAndTheComposer(t *testing.T) {
	e := newTestEditor(t)

	e.Update(controller.VoiceStateMsg{State: voice.StateListening, Level: 0.5})
	assert.Equal(t, controller.ActivityListening, e.footer.Activity().Current)
	assert.Equal(t, voice.StateListening, e.composer.VoiceState())

	e.Update(controller.VoiceStateMsg{State: voice.StatePaused})
	assert.Equal(t, controller.ActivityVoicePaused, e.footer.Activity().Current,
		"a pause is as visible in the footer as it is in the hint row")

	e.Update(controller.VoiceStateMsg{State: voice.StateFinishing})
	assert.Equal(t, controller.ActivityTranscribing, e.footer.Activity().Current)

	e.Update(controller.VoiceStateMsg{State: voice.StateIdle})
	assert.Equal(t, controller.ActivityIdle, e.footer.Activity().Current)
	assert.Equal(t, voice.StateIdle, e.composer.VoiceState())
}

func TestVoiceNeverTakesTheFooterFromARun(t *testing.T) {
	e := newTestEditor(t)
	e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityStreaming})

	e.Update(controller.VoiceStateMsg{State: voice.StateListening})
	assert.Equal(t, controller.ActivityStreaming, e.footer.Activity().Current,
		"a running stream keeps the footer while the microphone is open")
	assert.Equal(t, voice.StateListening, e.composer.VoiceState(),
		"the composer hint row is still the indicator that the mode is on")

	// Idle must not clear a label voice does not own.
	e.Update(controller.VoiceStateMsg{State: voice.StateIdle})
	assert.Equal(t, controller.ActivityStreaming, e.footer.Activity().Current)
}

func TestVoiceResultLandsAtTheCaretAndLeavesTheModeAlone(t *testing.T) {
	e := newTestEditor(t)
	e.composer.Chat.Value = "before"
	e.composer.Chat.Cursor = len("before")
	e.Update(controller.VoiceStateMsg{State: voice.StateListening})

	e.Update(controller.VoiceResultMsg{Seq: 1, Text: "hello world"})

	assert.Equal(t, "before hello world", e.composer.Chat.Value)
	assert.Equal(t, controller.ActivityListening, e.footer.Activity().Current,
		"one segment landing does not end the dialog")
	assert.False(t, e.toast.Visible(), "a good transcript says nothing")
}

func TestVoiceErrorToastsOneSentenceAndKeepsTheMode(t *testing.T) {
	e := newTestEditor(t)
	e.Update(controller.VoiceStateMsg{State: voice.StateListening})

	e.Update(controller.VoiceErrorMsg{
		Seq:  2,
		Text: "transcription failed (HTTP 401)",
		Hint: "check voice.stt.api_key",
	})

	require.True(t, e.toast.Visible())
	assert.Equal(t, "voice: transcription failed (HTTP 401) — check voice.stt.api_key", e.toast.Message)
	assert.Equal(t, toast.ToastError, e.toast.Kind)
	assert.Equal(t, voice.StateListening, e.composer.VoiceState(),
		"a segment that failed does not throw away the rest of the dialog")
}

func TestVoiceNoticeIsAWarning(t *testing.T) {
	e := newTestEditor(t)

	e.Update(controller.VoiceNoticeMsg{Text: "paused after 5:00 of silence — Space resumes"})

	require.True(t, e.toast.Visible())
	assert.Equal(t, "voice: paused after 5:00 of silence — Space resumes", e.toast.Message)
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)
}

func TestVoiceStatusBeforeAndAfterConfigure(t *testing.T) {
	e := newTestEditor(t)
	assert.Equal(t, "voice: off (set voice.enabled: true)", e.VoiceStatus())

	off := voice.Defaults()
	off.Enabled = false
	e.ConfigureVoice(VoiceOptions{Config: off, Env: voice.ResolveEnv{GOOS: "linux", LookBin: noBinaries}})
	assert.Equal(t, "voice: off (set voice.enabled: true)", e.VoiceStatus())

	on := voice.Defaults()
	e.ConfigureVoice(VoiceOptions{
		Config:   on,
		Env:      voice.ResolveEnv{GOOS: "linux", LookBin: noBinaries},
		HoldKeys: true,
	})
	assert.Contains(t, e.VoiceStatus(), "voice: not ready — ")
	assert.Contains(t, e.VoiceStatus(), "install ffmpeg")
	assert.True(t, e.VoiceHoldKeys(), "the composer only promises hold-to-talk where releases arrive")

	// Closing twice must be as safe as closing once, because cmd defers it
	// on a quit path that may already have run.
	e.CloseVoice()
	e.CloseVoice()
}

func TestVoiceKeysWithoutASessionExplainThemselves(t *testing.T) {
	e := newTestEditor(t)

	e.VoiceStart()
	require.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "voice: not configured")
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)

	// Every other control stays a silent no-op; only the key that opens the
	// microphone is worth a word.
	e.toast.Clear()
	e.VoicePause()
	e.VoiceResume()
	e.VoiceFlush()
	e.VoiceEnd()
	e.VoiceDiscard()
	assert.False(t, e.toast.Visible())
	assert.False(t, e.VoiceHoldKeys())
}

func TestVoiceRetryNeedsAFailedSegment(t *testing.T) {
	e := newTestEditor(t)
	e.ConfigureVoice(VoiceOptions{
		Config:  voice.Defaults(),
		Env:     voice.ResolveEnv{GOOS: "linux", LookBin: noBinaries},
		WAVPath: t.TempDir() + "/last.wav",
	})
	t.Cleanup(e.CloseVoice)

	e.VoiceRetry()

	require.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "nothing to retry")
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)
}
