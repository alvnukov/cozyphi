package editor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/pathutil"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// voiceOfferAccept is the option label that means yes on the download offer.
// The overlay answers with labels, so this string is the protocol between the
// question and the reply.
const voiceOfferAccept = "Download and set up"

// voiceDownload is the speech-model download in flight: the catalog name, the
// ggml-<name> label every message uses, and the last progress report.
type voiceDownload struct {
	name     string
	label    string
	progress voice.InstallProgress
}

// percent renders the last progress report. Before the first one — and when
// the server sent no length — it is "0%", so no line ever trails into nothing.
func (d *voiceDownload) percent() string {
	if p := d.progress.Percent(); p != "" {
		return p
	}
	return "0%"
}

// modelLabel is how every voice line names a model: its file without the
// extension, e.g. ggml-small.
func modelLabel(m voice.Model) string {
	return strings.TrimSuffix(m.File, filepath.Ext(m.File))
}

// voiceContext bounds a download by the voice lifetime, so quitting cozyphi
// stops it and leaves the .part file for the next run to resume.
func (e *Editor) voiceContext() context.Context {
	if e.voiceLifetime != nil {
		return e.voiceLifetime
	}
	return context.Background()
}

// offerModelDownload answers Ctrl+G when whisper-cli is there but no model is:
// instead of the failure the session would raise, the user is asked whether to
// download one. It reports whether it handled the key press.
func (e *Editor) offerModelDownload() bool {
	res := e.voiceSession.Resolved()
	// The microphone speaks first: with no capture command a model would not
	// help, and that hint is the one worth showing.
	if res.STT.Missing != voice.MissingModel || res.Capture.Hint != "" {
		return false
	}
	dir := e.voiceEnv.ModelsDir
	m, ok := voice.LookupModel(voice.DefaultModel)
	if !ok || dir == "" {
		return false
	}
	if e.overlays.Active() {
		// Another overlay owns the screen; the hint is the honest answer.
		e.toast.Show(voiceToastText(res.STT.Hint, ""), toast.ToastWarning, 6*time.Second)
		return true
	}
	e.voiceOfferActivity = controller.ActivityIdle
	if act := e.footer.Activity(); act != nil {
		e.voiceOfferActivity = act.Current
	}
	reply := make(chan controller.QuestionReply, 1)
	e.Publish(controller.QuestionAskMsg{
		Questions: []questiontool.Question{{
			Header:   "Voice",
			Question: voiceOfferQuestion(m, dir),
			Options: []questiontool.Option{
				{
					Label:       voiceOfferAccept,
					Description: "Whisper small: fast, fine for dialog. Bigger models: /voice install medium",
				},
				{
					Label:       "Not now",
					Description: "/voice install later, or set voice.stt.model",
				},
			},
		}},
		Reply: reply,
	})
	lifetime := e.voiceContext()
	go func() {
		var answer controller.QuestionReply
		select {
		case answer = <-reply:
		case <-lifetime.Done():
			// The TUI is going away; an unanswered offer answers itself.
			return
		}
		e.Publish(controller.VoiceOfferReplyMsg{Name: m.Name, Accept: voiceOfferAccepted(answer)})
	}()
	return true
}

// voiceOfferQuestion is the one line the overlay asks, naming the download and
// where it lands.
func voiceOfferQuestion(m voice.Model, dir string) string {
	return "Speech model not installed. Download " + modelLabel(m) +
		" (~" + voice.FormatBytes(m.ApproxBytes) + ") to " + pathutil.ShortPath(dir) +
		" and set it up?"
}

// voiceOfferAccepted reads the overlay reply. Anything but the accept option —
// "Not now", Esc, an empty reply — leaves the download unstarted.
func voiceOfferAccepted(reply controller.QuestionReply) bool {
	for _, answer := range reply.Answers {
		if slices.Contains(answer, voiceOfferAccept) {
			return true
		}
	}
	return false
}

// applyVoiceOfferReply acts on the offer once the overlay has closed.
func (e *Editor) applyVoiceOfferReply(msg controller.VoiceOfferReplyMsg) {
	// Every closing ask hands the footer "Calling tools…", which belongs to a
	// model question; this one is ours, so the label from before it goes back.
	if act := e.footer.Activity(); act != nil && act.Current == controller.ActivityTools {
		e.footer.Apply(controller.SetActivityMsg{Activity: e.voiceOfferActivity})
	}
	e.voiceOfferActivity = controller.ActivityIdle
	if !msg.Accept {
		e.toast.Show(
			"voice: no speech model — /voice install when ready, or set voice.stt.model",
			toast.ToastWarning,
			6*time.Second,
		)
		return
	}
	if m, ok := voice.LookupModel(msg.Name); ok {
		e.startInstall(m)
	}
}

// startInstall downloads a model in the background. Both ways in — the offer
// and /voice install — mean the user has already agreed to it.
func (e *Editor) startInstall(m voice.Model) {
	dir := e.voiceEnv.ModelsDir
	if dir == "" {
		e.toast.Show(
			"voice: no models directory — set voice.stt.model to a model file",
			toast.ToastWarning,
			6*time.Second,
		)
		return
	}
	label := modelLabel(m)
	e.voiceDownload = &voiceDownload{name: m.Name, label: label}
	e.toast.Show(
		"voice: downloading "+label+" (~"+voice.FormatBytes(m.ApproxBytes)+") to "+pathutil.ShortPath(dir)+"…",
		toast.ToastSuccess,
		4*time.Second,
	)
	e.footer.Apply(controller.SetActivityMsg{Activity: controller.ActivityDownloadingModel})
	ctx := e.voiceContext()
	go func() {
		path, err := voice.Install(ctx, m, voice.InstallOptions{
			Dir: dir,
			Progress: func(p voice.InstallProgress) {
				e.Publish(controller.VoiceInstallProgressMsg{Name: p.Name, Done: p.Done, Total: p.Total})
			},
		})
		done := controller.VoiceInstallDoneMsg{Name: m.Name, Path: path}
		if err != nil {
			done.ErrText = err.Error()
		}
		e.Publish(done)
	}()
}

// applyVoiceInstallProgress moves the footer percentage. A report from an
// older download — one the user replaced — is ignored.
func (e *Editor) applyVoiceInstallProgress(msg controller.VoiceInstallProgressMsg) {
	d := e.voiceDownload
	if d == nil || d.name != msg.Name {
		return
	}
	d.progress = voice.InstallProgress{Name: msg.Name, Done: msg.Done, Total: msg.Total}
	e.footer.Apply(controller.SetActivityMsg{
		Activity: controller.ActivityDownloadingModel,
		Detail:   d.progress.Percent(),
	})
}

// applyVoiceInstallDone ends the download: the failure is a toast, the success
// selects the model for this session and pins it for the next one.
func (e *Editor) applyVoiceInstallDone(msg controller.VoiceInstallDoneMsg) {
	if d := e.voiceDownload; d != nil && d.name != msg.Name {
		return
	}
	e.voiceDownload = nil
	e.footer.Apply(controller.ClearIfActivityMsg{If: controller.ActivityDownloadingModel})
	if msg.ErrText != "" {
		e.toast.Show(voiceToastText(msg.ErrText, ""), toast.ToastWarning, 8*time.Second)
		return
	}
	m, ok := voice.LookupModel(msg.Name)
	if !ok {
		return
	}
	e.selectVoiceModel(m, true)
}

// selectVoiceModel points the running session at a model that is now on disk
// and pins it in config.yaml, so Ctrl+G works without a restart and after one.
func (e *Editor) selectVoiceModel(m voice.Model, installed bool) {
	label := modelLabel(m)
	verb := " selected"
	if installed {
		verb = " installed"
	}
	cfg := e.voiceConfig
	cfg.STT.Model = m.Name
	resolved := voice.Resolve(cfg, e.voiceEnv)
	if err := e.voiceSession.Reconfigure(cfg, resolved); err != nil {
		// The pin still goes in: it is what makes the restart find the model.
		e.persistVoiceModel(m.Name)
		e.toast.Show("voice: "+label+verb+" — restart cozyphi to use it", toast.ToastWarning, 6*time.Second)
		return
	}
	e.voiceConfig = cfg
	if errText := e.persistVoiceModel(m.Name); errText != "" {
		// The session already uses it; only the next start would not.
		e.toast.Show(
			"voice: "+label+verb+" but config.yaml not updated — "+errText,
			toast.ToastWarning,
			8*time.Second,
		)
		return
	}
	e.toast.Show("voice: "+label+verb+" — press Ctrl+G to talk", toast.ToastSuccess, 6*time.Second)
}

// persistVoiceModel writes voice.stt.model and returns what went wrong, or ""
// when the write succeeded or no settings manager is wired.
func (e *Editor) persistVoiceModel(name string) string {
	if e.voicePersist == nil {
		return ""
	}
	if err := e.voicePersist(name); err != nil {
		return err.Error()
	}
	return ""
}

// VoiceModels reports the catalog for /voice models, marking what is on disk
// and which file the resolver would use right now.
func (e *Editor) VoiceModels() []commands.VoiceModelInfo {
	found := voice.InstalledModels(voice.ModelDirs(e.voiceEnv))
	active := ""
	if e.voiceSession != nil {
		if path := e.voiceSession.Resolved().STT.ModelPath; path != "" {
			for _, in := range found {
				if in.Path == path {
					active = in.Name
				}
			}
		}
	}
	catalog := voice.Catalog()
	out := make([]commands.VoiceModelInfo, 0, len(catalog))
	for _, m := range catalog {
		info := commands.VoiceModelInfo{
			Name:   m.Name,
			Size:   voice.FormatBytes(m.ApproxBytes),
			Active: m.Name == active,
		}
		for _, in := range found {
			if in.Name == m.Name {
				info.Installed = true
			}
		}
		out = append(out, info)
	}
	return out
}

// VoiceInstall answers /voice install [name]. The name is validated here, so a
// typo is one returned error and never a download; the download itself reports
// through the footer and toasts.
func (e *Editor) VoiceInstall(name string) error {
	if e.voiceSession == nil {
		e.voiceUnconfigured()
		return nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = voice.DefaultModel
	}
	m, ok := voice.LookupModel(name)
	if !ok {
		return fmt.Errorf("voice: unknown model %q — /voice models lists them", name)
	}
	if d := e.voiceDownload; d != nil {
		return fmt.Errorf("voice: still downloading %s — %s", d.label, d.percent())
	}
	if installedModelPath(m, voice.ModelDirs(e.voiceEnv)) != "" {
		// Nothing to fetch: the command then means "use this one".
		e.selectVoiceModel(m, false)
		return nil
	}
	e.startInstall(m)
	return nil
}

// installedModelPath returns the model's own file if it is already on disk. A
// quantized variant does not count here: /voice install names an exact file.
func installedModelPath(m voice.Model, dirs []string) string {
	for _, dir := range dirs {
		path := filepath.Join(dir, m.File)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			return path
		}
	}
	return ""
}
