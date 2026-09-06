package editor

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

// CloseCurrent requests closure of the selected membership, never its disk history.
func (e *Editor) CloseCurrent() error {
	entry, ok := e.registry.Active()
	if !ok {
		return errors.New("no session is selected")
	}
	return e.RequestClose(entry.ID)
}

// RequestClose captures a live identity before asking to stop running work.
// All membership and confirmation state belongs to the UI goroutine.
func (e *Editor) RequestClose(id string) error {
	if e.closeConfirm != nil {
		return errors.New("answer the pending close confirmation first")
	}
	entry, err := e.closeTarget(id)
	if err != nil {
		return err
	}
	entry.View.DrainNow()
	if !entry.View.BeginCloseIfIdle() {
		e.closeConfirm = &entry
		e.RequestRedraw()
		return nil
	}
	return e.beginSessionClose(id)
}

func (e *Editor) closeTarget(id string) (sessions.Entry, error) {
	if _, closing := e.closing[id]; closing {
		return sessions.Entry{}, errors.New("session is already closing: wait for cleanup")
	}
	var target sessions.Entry
	survivors := 0
	for _, entry := range e.registry.Entries() {
		if entry.ID == id {
			target = entry
		} else if _, closing := e.closing[entry.ID]; !closing {
			survivors++
		}
	}
	if target.View == nil {
		return target, errors.New("session is no longer open")
	}
	if survivors == 0 {
		return target, errors.New("cannot close the last tab: open another session first, or quit")
	}
	return target, nil
}

func (e *Editor) beginSessionClose(id string) error {
	entry, err := e.closeTarget(id) // Recheck after confirmation, not just before it.
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	e.closing[id] = done
	entry.View.BeginClose()
	e.syncSelection()
	go func() {
		// BeginClose already retired UI state. Only the cleanup wait runs here.
		done <- entry.View.Close(context.Background())
	}()
	e.RequestRedraw()
	return nil
}

func (e *Editor) finishSessionCloses() {
	for id, done := range e.closing {
		select {
		case err := <-done:
			if err != nil {
				// Retain the slot on cleanup failure; quitting can still join it.
				e.closing[id] = nil
				e.showCloseError(err)
				continue
			}
			if _, err := e.registry.Close(id); err != nil {
				e.showCloseError(err)
			}
			delete(e.closing, id)
		default:
		}
	}
}

func (e *Editor) showCloseError(err error) {
	if err != nil && e.active != nil {
		e.active.Toast(err.Error(), toast.ToastWarning, 5*time.Second)
	}
}

func (e *Editor) captureCloseConfirmation(ctx *components.EventContext, ev xui.Event) bool {
	if e.closeConfirm == nil {
		return false
	}
	switch ev.(type) {
	case xui.KeyEvent, xui.MouseEvent, xui.PasteEvent, xui.PasteRejectedEvent:
		// Capture only input. Resize and focus still belong to the application.
	default:
		return false
	}
	if key, ok := ev.(xui.KeyEvent); ok && key.Press && key.Mods == 0 {
		target := e.closeConfirm
		switch {
		case key.Code == xui.KeyRune && (key.Rune == 'y' || key.Rune == 'Y'):
			e.closeConfirm = nil
			e.showCloseError(e.beginSessionClose(target.ID))
		case key.Code == xui.KeyEscape, key.Code == xui.KeyEnter,
			key.Code == xui.KeyRune && (key.Rune == 'n' || key.Rune == 'N'):
			e.closeConfirm = nil
		}
	}
	// Never let an approval, pasted text, or composer shortcut answer this ask.
	ctx.ConsumeAndRedraw()
	return true
}

func (e *Editor) drawCloseConfirmation(ctx components.DrawContext) components.Surface {
	s := components.NewSurface(max(0, ctx.Max.Width), max(0, ctx.Max.Height), e)
	target := e.closeConfirm
	lines := []string{
		fmt.Sprintf("Stop and close %s (%s)?", cleanName(target.Name), target.ID),
		"Running work will be cancelled. Disk history and agent results are kept.",
		"[y] Stop and close    [Esc / n / Enter] Cancel",
	}
	for row, line := range lines {
		if row >= s.Size.Height {
			break
		}
		s.Print(0, row, layout.EllipsizeToWidth(line, s.Size.Width, ctx.Method),
			components.DefaultTheme().Foreground, ctx.Method)
	}
	return s
}
