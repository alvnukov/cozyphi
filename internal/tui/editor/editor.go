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
	application  *app.App
	registry     *sessions.Registry
	active       *sessions.View
	syncSessions func()
}

// NewEditor binds an already assembled registry to the terminal application.
// Registry operations and View presentation run exclusively on the UI goroutine.
func NewEditor(application *app.App, registry *sessions.Registry) *Editor {
	e := &Editor{application: application, registry: registry}
	e.syncSelection()
	return e
}

// SetSessionSync installs cmd's retained-session reconciliation on the UI
// goroutine. It runs only during a scheduled draw, not in a second event loop.
func (e *Editor) SetSessionSync(syncSessions func()) { e.syncSessions = syncSessions }

func (e *Editor) syncSelection() {
	entry, _ := e.registry.Active()
	if e.active == entry.View {
		return
	}
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
	if err := e.registry.Activate(id); err != nil {
		return err
	}
	e.syncSelection()
	return nil
}

// Jump selects the 1-based opening position, also used by /switch.
func (e *Editor) Jump(n int) error {
	if err := e.registry.Jump(n); err != nil {
		return err
	}
	e.syncSelection()
	return nil
}

// Capture claims only session navigation, before a focused input or modal can
// consume it. Ordinary editing, interrupts and mouse hit testing stay in App.
func (e *Editor) Capture(ctx *components.EventContext, ev xui.Event) {
	e.syncSelection()
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
		err = e.registry.Next()
	case keys.CmdSessionPrev:
		err = e.registry.Prev()
	case keys.CmdSessionBack:
		err = e.registry.Back()
	default:
		return
	}
	e.syncSelection()
	if e.active != nil {
		if err != nil {
			e.active.Toast(err.Error(), toast.ToastWarning, 3*time.Second)
		} else if entry, ok := e.registry.Active(); ok {
			e.active.Toast("Session: "+entry.Name, toast.ToastSuccess, 2*time.Second)
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
		for _, entry := range e.registry.Entries() {
			if entry.View != e.active {
				entry.View.Handle(&components.EventContext{}, ev)
			}
		}
	}
	if e.active != nil && ctx.DeliveredTo != e.active {
		e.active.Handle(ctx, ev)
	}
}

// Draw drains inactive sessions too, without animating their hidden widgets.
func (e *Editor) Draw(ctx components.DrawContext) components.Surface {
	e.DrainNow()
	if e.active == nil {
		return components.Surface{}
	}
	return e.active.Draw(ctx)
}

// DrainNow projects every queued session update without changing selection.
func (e *Editor) DrainNow() {
	if e.syncSessions != nil {
		e.syncSessions()
	}
	e.syncSelection()
	for _, entry := range e.registry.Entries() {
		entry.View.DrainNow()
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
	if e.active.AcceptInterrupt() {
		return true
	}
	var running []string
	for i, entry := range e.registry.Entries() {
		if entry.View != e.active && entry.View.Status().Running {
			running = append(running, fmt.Sprintf("%d %s", i+1, entry.Name))
		}
	}
	if len(running) == 0 {
		return false
	}
	e.active.RefuseExit(
		"Background sessions still running: " + strings.Join(running, ", ") + " (switch and interrupt them first)",
	)
	return true
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
	entries := e.registry.Entries()
	for _, entry := range entries {
		entry.View.BeginClose()
	}
	errs := make([]error, len(entries))
	var wg sync.WaitGroup
	for i, entry := range entries {
		wg.Go(func() { errs[i] = entry.View.Close(ctx) })
	}
	wg.Wait()
	return errors.Join(errs...)
}
