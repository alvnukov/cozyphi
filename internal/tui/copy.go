package tui

import (
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/toast"
)

// textSel tracks drag selection over the transcript.
// Coordinates are content-space (relative to MessageList content origin),
// so the highlight stays on the selected text when the list scrolls.
type textSel struct {
	pending  bool
	dragging bool
	active   bool
	ax, ay   int
	ex, ey   int
}

func (s *textSel) clear() {
	*s = textSel{}
}

// viewSel returns viewport (list-surface) coordinates for the current content origin.
func (editor *Editor) viewSel() (ax, ay, ex, ey int) {
	ox := editor.list.ContentOrigin()
	return editor.sel.ax, editor.sel.ay + ox, editor.sel.ex, editor.sel.ey + ox
}

func (editor *Editor) toContentY(viewY int) int {
	return viewY - editor.list.ContentOrigin()
}

func (editor *Editor) copyResult(text, okMsg, failMsg string) {
	if text == "" {
		return
	}
	ok := false
	if editor.vx != nil {
		ok = editor.vx.CopyToClipboard(text) == nil
	}
	if ok {
		editor.toast.Show(okMsg, toast.ToastSuccess, 2*time.Second)
	} else {
		editor.toast.Show(failMsg, toast.ToastError, 2*time.Second)
	}
}

func (editor *Editor) copyBlock(text string) {
	editor.copyResult(text, "Copied to clipboard", "Failed to copy")
}

func (editor *Editor) handleCopyKey(ctx *components.EventContext, e xui.KeyEvent) bool {
	if !e.Press {
		return false
	}
	// Ctrl+Shift+C / Super+C — copy selected (or last) message block.
	copyChord := false
	if e.Code == xui.KeyRune && (e.Rune == 'c' || e.Rune == 'C') {
		if e.Mods.Has(xui.ModCtrl) && e.Mods.Has(xui.ModShift) {
			copyChord = true
		}
		if e.Mods.Has(xui.ModSuper) && !e.Mods.Has(xui.ModCtrl) {
			copyChord = true
		}
	}
	if !copyChord {
		return false
	}
	text := editor.list.SelectedCopyText()
	if text == "" {
		text = editor.list.LastCopyText()
	}
	if text == "" {
		return true
	}
	editor.copyBlock(text)
	ctx.ConsumeAndRedraw()
	return true
}

func (editor *Editor) handleListMouse(ctx *components.EventContext, e xui.MouseEvent) {
	if e.Button == xui.MouseWheelUp || e.Button == xui.MouseWheelDown {
		editor.list.Handle(ctx, e)
		return
	}
	if e.Button != xui.MouseLeft && e.Button != 0 {
		// Some terminals send motion with button 0.
		if e.Action != xui.MouseMotion && e.Action != xui.MouseDrag {
			return
		}
	}

	inList := e.Y >= 0 && e.Y < editor.listH && editor.listH > 0 && len(editor.list.Entries) > 0

	switch e.Action {
	case xui.MousePress:
		if e.Button != xui.MouseLeft {
			return
		}
		if !inList {
			editor.sel.clear()
			ctx.Redraw = true
			return
		}
		cy := editor.toContentY(e.Y)
		editor.sel = textSel{
			pending: true,
			ax:      e.X,
			ay:      cy,
			ex:      e.X,
			ey:      cy,
		}
		ctx.RequestFocus(&editor.Chat)
		ctx.Redraw = true
		return

	case xui.MouseDrag, xui.MouseMotion:
		if !editor.sel.pending && !editor.sel.dragging {
			return
		}
		// Only treat as drag when primary button is held (or Button==Left on Drag).
		if e.Action == xui.MouseMotion && e.Button != xui.MouseLeft {
			return
		}
		editor.sel.dragging = true
		editor.sel.active = true
		editor.sel.ex = e.X
		editor.sel.ey = editor.toContentY(e.Y)
		ctx.ConsumeAndRedraw()
		return

	case xui.MouseRelease:
		if e.Button != xui.MouseLeft {
			return
		}
		if !editor.sel.pending && !editor.sel.dragging {
			return
		}
		editor.sel.ex = e.X
		editor.sel.ey = editor.toContentY(e.Y)
		if editor.sel.dragging && (editor.sel.ax != editor.sel.ex || editor.sel.ay != editor.sel.ey) {
			ax, ay, ex, ey := editor.viewSel()
			text := components.ExtractSurfaceText(editor.lastListSurf, ax, ay, ex, ey)
			editor.sel.active = true
			if text != "" {
				editor.copyResult(text, "Selection copied to clipboard", "Failed to copy selection")
			}
			editor.sel.pending = false
			editor.sel.dragging = false
			// Leave active highlight until next click.
			ctx.ConsumeAndRedraw()
			return
		}
		// Click without drag → select message block.
		idx := editor.list.IndexAtPoint(e.X, e.Y)
		if idx >= 0 {
			editor.list.Selected = idx
		}
		editor.sel.clear()
		ctx.RequestFocus(&editor.Chat)
		ctx.ConsumeAndRedraw()
	}
}
