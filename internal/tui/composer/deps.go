package composer

import (
	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// Input is the composer surface Submitter and BashRunner use.
type Input interface {
	HideCompleters()
	ClearInput()
	PendingSkills() []string
	ClearPendingSkills()
	SyncBashBorder(text string)
	SetBashBorderActive(active bool)
}

// BusyChecker is the submit side of ComposerPane wiring (avoids composer→submit import).
type BusyChecker interface {
	CanSubmit() bool
	SyncBashBorder(text string)
}

// SubmitBus is the bus/frame surface ComposerPane submits and schedules through.
// The Editor implements it so the composer never imports the editor.
type SubmitBus interface {
	Publish(controller.Msg)
	DrainNow()
	RequestRefresh()
}

// Focuser moves focus between the editor root and inner composer widgets.
type Focuser interface {
	FocusEditor()
	Focus(components.Widget)
}

// OverlayComposer is the composer surface permission/continue overlays need.
type OverlayComposer interface {
	HideCompleters()
	HidePalette()
}

// PaletteComposer receives hook and builtin palette updates.
type PaletteComposer interface {
	SetPaletteCommands([]palette.PaletteCommand)
	PushPalette(title string, cmds []palette.PaletteCommand)
}

// VoiceController is the microphone seam. The editor owns the *voice.Session;
// the composer only asks it to start, stop or forget a recording, so the
// composer never imports the editor and tests can drive a fake.
type VoiceController interface {
	// ToggleVoice starts a recording, stops the running one, or reports that
	// a transcription is still in flight.
	ToggleVoice()
	// StopVoice ends the recording and transcribes what was heard.
	StopVoice()
	// CancelVoice discards the recording without transcribing it.
	CancelVoice()
	// VoiceAutoSend reports whether voice.auto_send is on.
	VoiceAutoSend() bool
}
