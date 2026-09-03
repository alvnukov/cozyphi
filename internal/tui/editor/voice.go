package editor

import (
	"context"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// VoiceOptions carries everything the editor needs to build a voice session.
// It is a struct rather than three parameters so cmd can hand over the parsed
// config, the binary lookup and the recording path without the editor having
// to import the project package.
type VoiceOptions struct {
	Config  voice.Config
	Env     voice.ResolveEnv
	WAVPath string
}

// ConfigureVoice wires microphone input. It is called once after NewEditor,
// the way SetAttentionNotifier is, because the config lives in cmd. The
// session is built even when voice is off so /voice status can say so.
func (e *Editor) ConfigureVoice(opts VoiceOptions) {
	e.CloseVoice()
	e.voiceEnv = opts.Env
	lifetime, cancel := context.WithCancel(context.Background())
	e.voiceLifetime, e.voiceCancel = lifetime, cancel
	e.voiceSession = voice.NewSession(voice.Options{
		Config:   opts.Config,
		Resolved: voice.Resolve(opts.Config, opts.Env),
		WAVPath:  opts.WAVPath,
	}, e.publishVoiceEvent)
	e.composer.SetVoice(e)
}

// CloseVoice kills a capture in flight. cmd defers it so no ffmpeg outlives
// the TUI, whichever way the app quits.
func (e *Editor) CloseVoice() {
	if e.voiceCancel != nil {
		e.voiceCancel()
		e.voiceCancel = nil
	}
	if e.voiceSession != nil {
		e.voiceSession.Close()
	}
}

// publishVoiceEvent turns a session event into a bus message. It runs on the
// session's own goroutine, so nothing here touches widget state.
func (e *Editor) publishVoiceEvent(ev voice.Event) {
	switch ev.Kind {
	case voice.EventState:
		e.Publish(controller.VoiceStateMsg{
			Gen:     ev.Gen,
			State:   ev.State,
			Elapsed: ev.Elapsed,
			Level:   ev.Level,
		})
	case voice.EventResult:
		e.Publish(controller.VoiceResultMsg{Gen: ev.Gen, Text: ev.Text, Language: ev.Language})
	case voice.EventError:
		e.Publish(controller.VoiceErrorMsg{Gen: ev.Gen, Text: ev.Text, Hint: ev.Hint})
	case voice.EventNotice:
		e.Publish(controller.VoiceNoticeMsg{Gen: ev.Gen, Text: ev.Text})
	}
}

// applyVoiceState moves the composer meter and the footer activity together.
func (e *Editor) applyVoiceState(msg controller.VoiceStateMsg) {
	e.composer.ApplyVoiceState(msg)
	switch msg.State {
	case voice.StateRecording:
		e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityListening})
	case voice.StateTranscribing:
		e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityTranscribing})
	case voice.StateIdle:
		e.clearVoiceActivity()
	}
}

// clearVoiceActivity gives the footer back only if voice is what is holding
// it: a run that started meanwhile keeps its own label.
func (e *Editor) clearVoiceActivity() {
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityListening})
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityTranscribing})
}

// ToggleVoice implements composer.VoiceController: the voice key starts a
// recording when idle and stops it when recording.
func (e *Editor) ToggleVoice() {
	if e.voiceSession == nil {
		e.toast.Show(
			"voice: not configured — set voice.enabled: true in config.yaml",
			toast.ToastWarning,
			5*time.Second,
		)
		return
	}
	e.voiceSession.Toggle(e.voiceLifetime)
}

// StopVoice ends the recording and transcribes it.
func (e *Editor) StopVoice() {
	if e.voiceSession != nil {
		e.voiceSession.Stop()
	}
}

// CancelVoice throws the recording away. Esc says never mind, silently.
func (e *Editor) CancelVoice() {
	if e.voiceSession != nil {
		e.voiceSession.Cancel()
	}
	e.clearVoiceActivity()
}

// VoiceAutoSend reports whether a transcript may submit itself. The composer
// still requires that it was empty when the recording started.
func (e *Editor) VoiceAutoSend() bool {
	return e.voiceSession != nil && e.voiceSession.Config().AutoSend
}

// VoiceStatus answers /voice status in one line.
func (e *Editor) VoiceStatus() string {
	if e.voiceSession == nil {
		return "voice: off (set voice.enabled: true)"
	}
	return e.voiceSession.Status()
}

// VoiceDevices lists the microphones the capture backend can see. It shells
// out, so it is bounded by its own timeout inside internal/voice.
func (e *Editor) VoiceDevices() ([]string, error) {
	ctx := e.voiceLifetime
	if ctx == nil {
		ctx = context.Background()
	}
	return voice.ListDevices(ctx, e.voiceEnv)
}

// VoiceRetry transcribes the last recording again, which is what a failed
// transcription leaves behind.
func (e *Editor) VoiceRetry() {
	if e.voiceSession == nil {
		e.toast.Show(
			"voice: not configured — set voice.enabled: true in config.yaml",
			toast.ToastWarning,
			5*time.Second,
		)
		return
	}
	if !e.voiceSession.HasRecording() {
		e.toast.Show("voice: no recording to retry — record something first", toast.ToastWarning, 5*time.Second)
		return
	}
	e.voiceSession.Retry(e.voiceLifetime)
}

// voiceToastText joins the failure and its next action into the one sentence
// the toast shows, under the "voice:" prefix every voice line carries.
func voiceToastText(text, hint string) string {
	if hint != "" {
		text += " — " + hint
	}
	if strings.HasPrefix(text, "voice:") {
		return text
	}
	return "voice: " + text
}
