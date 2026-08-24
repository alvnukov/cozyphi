package composer

import (
	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/palette"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// Input is the composer surface Submitter and BashRunner use.
type Input interface {
	HideCompleters()
	ClearInput()
	PendingSkills() []string
	ClearPendingSkills()
	SyncBashBorder(text string)
	CloseMentionSlash()
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
