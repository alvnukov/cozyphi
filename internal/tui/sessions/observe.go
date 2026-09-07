package sessions

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/voice"
)

// publishUIStatus hands the surface's own account of itself to the controller,
// detached. It runs on the goroutine that owns the widgets — every caller is
// a lifecycle point on the UI thread — and the controller stores a pointer
// the harness view reads from a tool goroutine. Nothing in what is handed
// over leads back to a widget, a binding table or a live notifier, so the
// reader has nothing to race against.
//
// It observes and publishes. It repaints nothing, sends no notification,
// opens no microphone, changes no palette, no keybinding and no preference,
// and reads no configuration file: every value comes from an owner that
// already holds it.
func (e *View) publishUIStatus() {
	if e == nil || e.ctrl == nil {
		return
	}
	e.ctrl.PublishUIStatus(e.observeSurface())
}

// observeSurface collects the four subjects of a surface into one snapshot,
// so the fields of one answer describe one moment rather than four.
func (e *View) observeSurface() diag.UISurfaceFacts {
	facts := diag.UISurfaceFacts{
		Known: true,
		Shape: diag.UIShapeTerminal,
		Theme: components.ObserveTheme(e.bootTheme, e.theme),
		Keys:  e.observeKeys(),
		Voice: voice.Observe(e.voiceSession, e.voiceGate),
	}
	if e.notifier != nil {
		facts.Notifications = e.notifier.Observe()
	}
	facts.Revision = surfaceRevision(facts)
	return facts
}

// observeKeys adds the composer's own dialect to the process-wide table's
// account of itself. The table is shared by every session on screen and the
// composer's dialect is this one's; they are the same answer in a session
// that is working, and reporting them apart is what lets one that is not say
// so.
func (e *View) observeKeys() diag.KeybindRuntimeFacts {
	facts := keys.Observe()
	if e.composer != nil {
		facts.Editing = e.composer.Chat.EditingMode().String()
	}
	return facts
}

// surfaceRevision fingerprints what this snapshot describes: the palette in
// force, the dialect being edited in, what a notification would do and what
// voice is doing. Nothing about a surface increments a counter, so this is
// only a way to see that two snapshots taken across a theme switch, a
// keymap switch, a failed sender or a recording are of two different states.
func surfaceRevision(facts diag.UISurfaceFacts) string {
	return fmt.Sprintf("t%s.k%s.n%s%t.v%s%d",
		facts.Theme.Live,
		facts.Keys.Editing,
		facts.Notifications.Mode,
		facts.Notifications.Broken,
		facts.Voice.State,
		facts.Voice.Pending,
	)
}
