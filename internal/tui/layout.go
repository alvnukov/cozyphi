package tui

import (
	"github.com/pulseaiclub/phi/internal/components"
)

// EditorLayout owns Draw composition: transcript, composer/ask, footer, overlays.
type EditorLayout struct {
	e *Editor
}

// Draw renders the editor surface for the given draw context.
func (l *EditorLayout) Draw(ctx components.DrawContext) components.Surface {
	e := l.e
	e.drainBus()

	if e.footer != nil {
		e.footer.AdvanceTick()
	}
	_ = e.toast.Visible()

	maxSize := ctx.Max
	root := components.Surface{Size: maxSize, Widget: e}

	footerH := 1
	var chatH int
	if askH, overlay := e.overlays.PreferredBottomHeight(maxSize.Width, ctx.Method); overlay {
		chatH = askH
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else {
		chatH = e.composer.PreferredHeight(maxSize.Width, ctx.Method)
		minChatH := 5
		if len(e.composer.Chat.PendingSkills) > 0 {
			minChatH++
		}
		if chatH < minChatH {
			chatH = minChatH
		}
		maxChatH := maxSize.Height - footerH - 3
		maxChatH = max(maxChatH, minChatH)
		if chatH > maxChatH {
			chatH = maxChatH
		}
	}
	listH := maxSize.Height - chatH - footerH
	if listH < 3 {
		listH = 3
		chatH = maxSize.Height - listH - footerH
		chatH = max(chatH, 5)
	}

	listSurf := e.transcript.Draw(
		ctx,
		maxSize.Width,
		listH,
	)
	listH = e.transcript.ListHeight()

	var chatSurf components.Surface
	if surf, ok := e.overlays.DrawBottom(ctx, maxSize.Width, chatH); ok {
		chatSurf = surf
	} else {
		chatSurf = e.composer.DrawChat(ctx, maxSize.Width, chatH)
	}
	footer := e.footer.Draw(ctx, maxSize.Width)

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf},
		{Origin: components.Point{X: 0, Y: listH}, Surface: chatSurf, Z: 1},
		{Origin: components.Point{X: 0, Y: maxSize.Height - footerH}, Surface: footer, Z: 2},
	}
	if !e.overlays.Active() {
		root.Children = append(root.Children, e.composer.PickerOverlays(ctx, listH, maxSize.Width)...)
	}
	if pal, ok := e.composer.PaletteOverlay(ctx); ok {
		root.Children = append(root.Children, pal)
	}
	if e.toast.Visible() {
		toastSurf := e.toast.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: toastSurf,
			Z:       40,
		})
	}
	return root
}
