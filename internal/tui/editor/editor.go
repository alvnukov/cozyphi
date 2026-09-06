// Package editor wires the process shell around retained session Views.
package editor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

// Editor owns selection and event routing, not session widgets or execution.
// Every View consumes its own mailbox; only the selected one draws. All mailboxes
// wake the same App scheduler through the process's RedrawRelay.
type Editor struct {
	application *app.App
	registry    *sessions.Registry
	active      *sessions.View
	// childScreen is the sub-agent session shown in place of the selected
	// one. Children are never tabs: the selector keeps marking the parent,
	// and this override alone decides which view the shell draws and routes
	// events to. It is dropped whenever the selection moves.
	childScreen  *sessions.View
	syncSessions func()
	bodyRows     int
	closing      map[string]<-chan error
	// retiring joins the cleanup of sub-agent views the runtime stopped
	// retaining. A child has no tab and no registry slot, so nothing else
	// would ever wait for its shell and history to finish closing.
	retiring     []<-chan error
	closeConfirm *sessions.Entry
}

// NewEditor binds an already assembled registry to the terminal application.
// Registry operations and View presentation run exclusively on the UI goroutine.
func NewEditor(application *app.App, registry *sessions.Registry) *Editor {
	e := &Editor{application: application, registry: registry, closing: make(map[string]<-chan error)}
	e.syncSelection()
	return e
}

// SetSessionSync installs cmd's retained-session reconciliation on the UI
// goroutine. It runs only during a scheduled draw, not in a second event loop.
func (e *Editor) SetSessionSync(syncSessions func()) { e.syncSessions = syncSessions }

func (e *Editor) syncSelection() {
	entry, _ := e.registry.Active()
	if _, closing := e.closing[entry.ID]; closing {
		// Prefer the next live neighbor, falling back to the previous one.
		// Closing entries retain their slots until cleanup but are not candidates.
		var survivor sessions.Entry
		pastCurrent := false
		for _, candidate := range e.registry.Entries() {
			if candidate.ID == entry.ID {
				pastCurrent = true
			}
			if _, closing := e.closing[candidate.ID]; closing {
				continue
			}
			survivor = candidate
			if pastCurrent {
				break
			}
		}
		if survivor.View != nil {
			_ = e.registry.Activate(survivor.ID)
			entry = survivor
		}
	}
	if e.active == entry.View {
		return
	}
	// Selecting another tab leaves whatever child screen was open: the
	// override belongs to the session that was selected, not to the shell.
	e.retireChildScreen()
	if e.active != nil {
		e.active.SetActive(false)
	}
	e.active = entry.View
	if e.active != nil {
		e.active.SetActive(true)
	}
	e.RequestRedraw()
}

// Activate selects retained state without submitting input or canceling work.
func (e *Editor) Activate(id string) error {
	if _, closing := e.closing[id]; closing {
		return errors.New("session is closing: select a surviving tab")
	}
	if err := e.registry.Activate(id); err != nil {
		return err
	}
	e.syncSelection()
	return nil
}

// Jump selects the 1-based opening position, also used by /switch.
func (e *Editor) Jump(n int) error {
	entries := e.registry.Entries()
	if n >= 1 && n <= len(entries) {
		return e.Activate(entries[n-1].ID)
	}
	return e.registry.Jump(n) // Preserve the registry's range/empty diagnostics.
}

// Closing entries retain capacity, but are not navigation destinations.
func (e *Editor) navigate(move func() error) error {
	for range e.registry.Len() + 1 {
		if err := move(); err != nil {
			return err
		}
		entry, _ := e.registry.Active()
		if _, closing := e.closing[entry.ID]; !closing {
			return nil
		}
	}
	return errors.New("no surviving session to select")
}

// Capture claims session navigation and paste rejection before focused widgets
// or modals. Ordinary editing, interrupts and mouse hit testing stay in App.
func (e *Editor) Capture(ctx *components.EventContext, ev xui.Event) {
	e.syncSelection()
	if e.captureCloseConfirmation(ctx, ev) {
		return
	}
	if _, rejected := ev.(xui.PasteRejectedEvent); rejected {
		if screen := e.Screen(); screen != nil {
			screen.Toast(
				"Paste rejected: text exceeds 1 MiB (1,048,576 bytes). Draft unchanged.",
				toast.ToastWarning,
				5*time.Second,
			)
		}
		ctx.ConsumeAndRedraw()
		return
	}
	key, ok := ev.(xui.KeyEvent)
	if !ok {
		return
	}
	command, ok := keys.GlobalCommand(key)
	if !ok {
		return
	}
	var err error
	switch command {
	case keys.CmdSessionNext:
		err = e.navigate(e.registry.Next)
	case keys.CmdSessionPrev:
		err = e.navigate(e.registry.Prev)
	case keys.CmdSessionBack:
		err = e.navigate(e.registry.Back)
	default:
		return
	}
	e.syncSelection()
	if screen := e.Screen(); screen != nil {
		if err != nil {
			screen.Toast(err.Error(), toast.ToastWarning, 3*time.Second)
		} else if entry, ok := e.registry.Active(); ok {
			screen.Toast("Session: "+entry.DisplayName(), toast.ToastSuccess, 2*time.Second)
		}
	}
	ctx.ConsumeAndRedraw()
}

// Handle routes an unclaimed event to the selected View exactly once.
func (e *Editor) Handle(ctx *components.EventContext, ev xui.Event) {
	e.syncSelection()
	if _, ok := ev.(xui.FocusEvent); ok {
		// Terminal focus belongs to the process, while notification and voice
		// callbacks remain attached to their originating View.
		screen := e.Screen()
		for _, view := range e.Views() {
			if view != screen {
				view.Handle(&components.EventContext{}, ev)
			}
		}
	}
	screen := e.Screen()
	if screen != nil && ctx.DeliveredTo != screen {
		if mouse, ok := ev.(xui.MouseEvent); ok {
			mouse.Y -= e.bodyRows
			if mouse.Y < 0 {
				return
			}
			ev = mouse
		}
		screen.Handle(ctx, ev)
	}
}

// Draw drains inactive sessions too, without animating their hidden widgets.
func (e *Editor) Draw(ctx components.DrawContext) components.Surface {
	e.DrainNow()
	// Only pending cleanup needs another frame; idle and failed closes do not poll.
	for _, done := range e.closing {
		if done != nil {
			ctx.WakeIn(100 * time.Millisecond)
			break
		}
	}
	if len(e.retiring) > 0 {
		ctx.WakeIn(100 * time.Millisecond)
	}
	if e.closeConfirm != nil {
		return e.drawCloseConfirmation(ctx)
	}
	if e.active == nil {
		return components.Surface{}
	}
	return e.drawShell(ctx)
}

// DrainNow projects every queued session update without changing selection.
func (e *Editor) DrainNow() {
	if e.syncSessions != nil {
		e.syncSessions()
	}
	e.finishSessionCloses()
	e.syncSelection()
	for i, entry := range e.registry.Entries() {
		entry.View.DrainNow()
		// The input line keeps the stable registry name (e.g. "main"); a mutable
		// session title must not rename what the user is looking at while typing.
		entry.View.SetIdentity(i+1, entry.Name)
		// A child has no tab, so nothing else would ever drain its bus: its
		// rows in the band, and its own screen, are made of these messages.
		family := entry.View.Family()
		for _, child := range family.Views() {
			child.DrainNow()
		}
		family.SyncIdentities(i + 1)
	}
}

// AcceptInterrupt preserves the selected View's layered interrupt behavior.
// The exit it arms is process-wide, so a second Ctrl+C is refused while any
// background session still runs: quitting would kill work the user cannot
// see. The toast names those sessions so they can be selected and stopped.
func (e *Editor) AcceptInterrupt() bool {
	e.syncSelection()
	if e.active == nil {
		return false
	}
	screen := e.Screen()
	if screen.AcceptInterrupt() {
		return true
	}
	var running []string
	for i, entry := range e.registry.Entries() {
		if entry.View != screen && entry.View.Status().Running {
			running = append(running, fmt.Sprintf("%d %s", i+1, entry.DisplayName()))
		}
		// A sub-agent has no tab to switch to, so it is named by the session
		// that owns it: quitting would kill work the user cannot see either way.
		for _, name := range entry.View.Family().RunningNames() {
			running = append(running, fmt.Sprintf("%d %s → %s", i+1, entry.DisplayName(), name))
		}
	}
	if len(running) == 0 {
		return false
	}
	screen.RefuseExit(
		"Background sessions still running: " + strings.Join(running, ", ") + " (switch and interrupt them first)",
	)
	return true
}

// Screen is the view the shell is drawing and routing events to: the open
// sub-agent session when there is one, the selected tab otherwise.
func (e *Editor) Screen() *sessions.View {
	if e.childScreen != nil {
		return e.childScreen
	}
	return e.active
}

// ShowChild puts a sub-agent's session on screen without giving it a tab.
// The selector keeps marking the session that owns it.
func (e *Editor) ShowChild(child *sessions.View) {
	if child == nil || child == e.childScreen || e.active == nil {
		return
	}
	if e.childScreen != nil {
		e.childScreen.SetActive(false)
	} else {
		e.active.SetActive(false)
	}
	e.childScreen = child
	child.SetActive(true)
	// The family decides which screen answers its questions, so it is told
	// the screen changed however the change was asked for — the band, the
	// browser, or the shell itself.
	child.Family().Show(child.ChildJobID())
	e.RequestRedraw()
}

// ShowMain returns to the selected session's own screen.
func (e *Editor) ShowMain() {
	if e.childScreen == nil {
		return
	}
	e.retireChildScreen()
	e.RequestRedraw()
}

// retireChildScreen puts the selected session back on screen without asking
// for a frame; the callers that need one ask for it themselves.
func (e *Editor) retireChildScreen() {
	if e.childScreen == nil {
		return
	}
	e.childScreen.SetActive(false)
	e.childScreen = nil
	if e.active != nil {
		e.active.Family().Show("")
		e.active.SetActive(true)
	}
}

// Views lists every retained view, tabs and sub-agents alike, in tab order.
// Shutdown, process-wide notifications and the shell's reconciliation of new
// sub-agents all reach the whole set through here.
func (e *Editor) Views() []*sessions.View {
	entries := e.registry.Entries()
	out := make([]*sessions.View, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.View)
		out = append(out, entry.View.Family().Views()...)
	}
	return out
}

// RequestRedraw is safe to bind to every session's shared redraw relay.
func (e *Editor) RequestRedraw() {
	if e.application != nil {
		e.application.RequestRedraw()
	}
}

// Close cancels every View, including inactive sessions, then waits for all
// of them at once so one deadline covers the shutdown rather than each view
// consuming it in turn. A failed wait retains registry membership so callers
// can wait again without losing ownership.
func (e *Editor) Close(ctx context.Context) error {
	views := e.Views()
	for _, view := range views {
		view.BeginClose()
	}
	errs := make([]error, len(views))
	var wg sync.WaitGroup
	for i, view := range views {
		wg.Go(func() { errs[i] = view.Close(ctx) })
	}
	wg.Wait()
	// A sub-agent released before the exit is closing on its own goroutine;
	// its history and shell still have to land before the process leaves.
	for _, done := range e.retiring {
		select {
		case err := <-done:
			errs = append(errs, err)
		case <-ctx.Done():
			errs = append(errs, fmt.Errorf("close sub-agent view: %w", ctx.Err()))
		}
	}
	e.retiring = nil
	return errors.Join(errs...)
}
