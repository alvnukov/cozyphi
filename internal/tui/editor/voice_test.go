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

func TestVoiceStateMovesTheFooterAndTheMeter(t *testing.T) {
	e := newTestEditor(t)

	e.Update(controller.VoiceStateMsg{State: voice.StateRecording, Level: 0.5})
	assert.Equal(t, controller.ActivityListening, e.footer.Activity().Current)
	assert.Equal(t, voice.StateRecording, e.composer.VoiceState())

	e.Update(controller.VoiceStateMsg{State: voice.StateTranscribing})
	assert.Equal(t, controller.ActivityTranscribing, e.footer.Activity().Current)

	e.Update(controller.VoiceStateMsg{State: voice.StateIdle})
	assert.Equal(t, controller.ActivityIdle, e.footer.Activity().Current)
	assert.Equal(t, voice.StateIdle, e.composer.VoiceState())
}

func TestVoiceNeverTakesTheFooterFromARun(t *testing.T) {
	e := newTestEditor(t)
	e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityStreaming})

	e.Update(controller.VoiceStateMsg{State: voice.StateRecording})
	assert.Equal(t, controller.ActivityStreaming, e.footer.Activity().Current,
		"a running stream keeps the footer while the microphone is open")
	assert.Equal(t, voice.StateRecording, e.composer.VoiceState(),
		"the composer meter is still the indicator that recording is on")

	// Idle must not clear a label voice does not own.
	e.Update(controller.VoiceStateMsg{State: voice.StateIdle})
	assert.Equal(t, controller.ActivityStreaming, e.footer.Activity().Current)
}

func TestVoiceResultLandsAtTheCaret(t *testing.T) {
	e := newTestEditor(t)
	e.composer.Chat.Value = "before"
	e.composer.Chat.Cursor = len("before")
	e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityTranscribing})

	e.Update(controller.VoiceResultMsg{Text: "hello world"})

	assert.Equal(t, "before hello world", e.composer.Chat.Value)
	assert.Equal(t, controller.ActivityIdle, e.footer.Activity().Current)
	assert.False(t, e.toast.Visible(), "a good transcript says nothing")
}

func TestVoiceErrorToastsOneSentence(t *testing.T) {
	e := newTestEditor(t)
	e.Update(controller.VoiceStateMsg{State: voice.StateTranscribing})

	e.Update(controller.VoiceErrorMsg{
		Text: "transcription failed (HTTP 401)",
		Hint: "check voice.stt.api_key",
	})

	require.True(t, e.toast.Visible())
	assert.Equal(t, "voice: transcription failed (HTTP 401) — check voice.stt.api_key", e.toast.Message)
	assert.Equal(t, toast.ToastError, e.toast.Kind)
	assert.Equal(t, controller.ActivityIdle, e.footer.Activity().Current)
	assert.Equal(t, voice.StateIdle, e.composer.VoiceState())
}

func TestVoiceNoticeIsAWarning(t *testing.T) {
	e := newTestEditor(t)

	e.Update(controller.VoiceNoticeMsg{Text: "recording stopped at 5:00 (voice.max_seconds)"})

	require.True(t, e.toast.Visible())
	assert.Equal(t, "voice: recording stopped at 5:00 (voice.max_seconds)", e.toast.Message)
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
	e.ConfigureVoice(VoiceOptions{Config: on, Env: voice.ResolveEnv{GOOS: "linux", LookBin: noBinaries}})
	assert.Contains(t, e.VoiceStatus(), "voice: not ready — ")
	assert.Contains(t, e.VoiceStatus(), "install ffmpeg")

	// Closing twice must be as safe as closing once, because cmd defers it
	// on a quit path that may already have run.
	e.CloseVoice()
	e.CloseVoice()
}

func TestVoiceKeysWithoutASessionExplainThemselves(t *testing.T) {
	e := newTestEditor(t)

	e.ToggleVoice()
	require.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "voice: not configured")
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)

	// Stop and Cancel stay silent no-ops; only the key that starts a
	// recording is worth a word.
	e.toast.Clear()
	e.StopVoice()
	e.CancelVoice()
	assert.False(t, e.toast.Visible())
	assert.False(t, e.VoiceAutoSend())
}

func TestVoiceRetryNeedsARecording(t *testing.T) {
	e := newTestEditor(t)
	cfg := voice.Defaults()
	e.ConfigureVoice(VoiceOptions{
		Config:  cfg,
		Env:     voice.ResolveEnv{GOOS: "linux", LookBin: noBinaries},
		WAVPath: t.TempDir() + "/last.wav",
	})
	t.Cleanup(e.CloseVoice)

	e.VoiceRetry()

	require.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "no recording to retry")
	assert.Equal(t, toast.ToastWarning, e.toast.Kind)
}
