package composer

import (
	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/palette"
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

// BusyChecker is the submit side of ComposerWire (avoids composer→submit import).
type BusyChecker interface {
	RunningBash() bool
	IsBusy() bool
	SyncBashBorder(text string)
}

// OverlayComposer is the composer surface permission/continue overlays need.
type OverlayComposer interface {
	HideCompleters()
	HidePalette()
}

// LabelComposer receives footer token/context labels.
type LabelComposer interface {
	SetBottomLeftLabel(layout.BorderLabel)
	ClearBottomLeftLabel()
}

// PaletteComposer receives hook and builtin palette updates.
type PaletteComposer interface {
	SetPaletteCommands([]palette.PaletteCommand)
	PushPalette(title string, cmds []palette.PaletteCommand)
}

// WireKeyHandler handles overlay keyboard input.
type WireKeyHandler func(ctx *components.EventContext, e xui.KeyEvent) bool
