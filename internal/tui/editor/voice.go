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
	// HoldKeys says whether the terminal delivers key releases, which is what
	// makes hold-to-pause and push-to-talk possible.
	HoldKeys bool
	// PersistModel pins a model installed while cozyphi runs in config.yaml,
	// so the next start finds it. nil keeps the selection in memory only.
	PersistModel func(name string) error
}

// ConfigureVoice wires microphone input. It is called once after NewEditor,
// the way SetAttentionNotifier is, because the config lives in cmd. The
// session is built even when voice is off so /voice status can say so.
func (e *Editor) ConfigureVoice(opts VoiceOptions) {
	e.CloseVoice()
	e.voiceEnv = opts.Env
	e.voiceHold = opts.HoldKeys
	e.voiceConfig = opts.Config
	e.voicePersist = opts.PersistModel
	e.voiceDownload = nil
	lifetime, cancel := context.WithCancel(context.Background())
	e.voiceLifetime, e.voiceCancel = lifetime, cancel
	e.voiceSession = voice.NewSession(voice.Options{
		Config:   opts.Config,
		Resolved: voice.Resolve(opts.Config, opts.Env),
		WAVPath:  opts.WAVPath,
		HoldKeys: opts.HoldKeys,
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
			Gen:      ev.Gen,
			State:    ev.State,
			Level:    ev.Level,
			Pending:  ev.Pending,
			Starting: ev.Starting,
		})
	case voice.EventResult:
		e.Publish(controller.VoiceResultMsg{Gen: ev.Gen, Seq: ev.Seq, Text: ev.Text, Language: ev.Language})
	case voice.EventError:
		e.Publish(controller.VoiceErrorMsg{Gen: ev.Gen, Seq: ev.Seq, Text: ev.Text, Hint: ev.Hint})
	case voice.EventNotice:
		e.Publish(controller.VoiceNoticeMsg{Gen: ev.Gen, Text: ev.Text})
	}
}

// applyVoiceState moves the composer meter and the footer activity together.
// Each mode state has its own footer label, so a pause is as visible in the
// footer as it is in the hint row.
func (e *Editor) applyVoiceState(msg controller.VoiceStateMsg) {
	e.composer.ApplyVoiceState(msg)
	switch msg.State {
	case voice.StateListening:
		e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityListening})
	case voice.StatePaused:
		e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityVoicePaused})
	case voice.StateFinishing:
		e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityTranscribing})
	case voice.StateIdle:
		e.clearVoiceActivity()
	}
}

// clearVoiceActivity gives the footer back only if voice is what is holding
// it: a run that started meanwhile keeps its own label.
func (e *Editor) clearVoiceActivity() {
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityListening})
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityVoicePaused})
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityTranscribing})
}

// VoiceStart enters the dialog mode and opens the microphone. With a download
// in flight it reports it, and with whisper-cli installed but no model it
// offers the download instead of failing.
func (e *Editor) VoiceStart() {
	if e.voiceSession == nil {
		e.voiceUnconfigured()
		return
	}
	if d := e.voiceDownload; d != nil {
		e.toast.Show("voice: downloading "+d.label+" — "+d.percent(), toast.ToastWarning, 4*time.Second)
		return
	}
	if e.offerModelDownload() {
		return
	}
	e.voiceSession.Start(e.voiceLifetime)
}

// VoicePause stops listening but keeps the mode on.
func (e *Editor) VoicePause() {
	if e.voiceSession != nil {
		e.voiceSession.Pause()
	}
}

// VoiceResume listens again, restarting the capture if the grace period
// already closed it.
func (e *Editor) VoiceResume() {
	if e.voiceSession != nil {
		e.voiceSession.Resume(e.voiceLifetime)
	}
}

// VoiceFlush closes the open segment now, so what was just said is queued.
func (e *Editor) VoiceFlush() {
	if e.voiceSession != nil {
		e.voiceSession.Flush()
	}
}

// VoiceEnd leaves the mode keeping what was said: the queue drains first.
func (e *Editor) VoiceEnd() {
	if e.voiceSession != nil {
		e.voiceSession.End()
	}
}

// VoiceDiscard leaves the mode and throws away everything not yet inserted.
func (e *Editor) VoiceDiscard() {
	if e.voiceSession != nil {
		e.voiceSession.Discard()
	}
	e.clearVoiceActivity()
}

// VoiceHoldKeys reports whether the terminal sends key releases, which is what
// the composer needs before it may promise hold-to-talk.
func (e *Editor) VoiceHoldKeys() bool { return e.voiceHold }

// voiceUnconfigured says why nothing happened when there is no session.
func (e *Editor) voiceUnconfigured() {
	e.toast.Show(
		"voice: not configured — set voice.enabled: true in config.yaml",
		toast.ToastWarning,
		5*time.Second,
	)
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

// VoiceRetry transcribes the last failed segment again. Its audio is kept
// until a retry succeeds or the mode ends.
func (e *Editor) VoiceRetry() {
	if e.voiceSession == nil {
		e.voiceUnconfigured()
		return
	}
	if !e.voiceSession.HasFailed() {
		e.toast.Show(
			"voice: nothing to retry — the last segments were transcribed",
			toast.ToastWarning,
			5*time.Second,
		)
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
