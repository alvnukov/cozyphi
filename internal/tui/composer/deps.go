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
// the composer only drives the dialog mode through it, so the composer never
// imports the editor and tests can drive a fake.
type VoiceController interface {
	// VoiceStart enters the dialog mode and opens the microphone.
	VoiceStart()
	// VoicePause stops listening but keeps the mode on.
	VoicePause()
	// VoiceResume listens again after a pause.
	VoiceResume()
	// VoiceFlush closes the open segment so what was just said is transcribed.
	VoiceFlush()
	// VoiceEnd leaves the mode and keeps the speech: the queue drains first.
	VoiceEnd()
	// VoiceDiscard leaves the mode and throws everything away.
	VoiceDiscard()
	// VoiceHoldKeys reports whether key releases reach the app, which is what
	// makes hold-to-pause and push-to-talk possible.
	VoiceHoldKeys() bool
}
