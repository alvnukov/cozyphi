package shell

import (
	"github.com/pulseaiclub/phi/internal/components"
)

// ShellLayout owns Draw composition: transcript, composer/ask, footer, overlays.
type ShellLayout struct {
	s *Shell
}

// Draw renders the shell surface for the given draw context.
func (l *ShellLayout) Draw(ctx components.DrawContext) components.Surface {
	sh := l.s
	sh.drainBus()

	if sh.footer != nil {
		sh.footer.AdvanceTick()
	}
	_ = sh.toast.Visible()

	maxSize := ctx.Max
	root := components.Surface{Size: maxSize, Widget: sh}

	footerH := 1
	var chatH int
	if askH, overlay := sh.overlays.PreferredBottomHeight(maxSize.Width, ctx.Method); overlay {
		chatH = askH
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else {
		chatH = sh.composer.PreferredHeight(maxSize.Width, ctx.Method)
		minChatH := 5
		if len(sh.composer.Chat.PendingSkills) > 0 {
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

	listSurf := sh.transcript.Draw(
		ctx,
		maxSize.Width,
		listH,
	)
	listH = sh.transcript.ListHeight()

	var chatSurf components.Surface
	if surf, ok := sh.overlays.DrawBottom(ctx, maxSize.Width, chatH); ok {
		chatSurf = surf
	} else {
		chatSurf = sh.composer.DrawChat(ctx, maxSize.Width, chatH)
	}
	footer := sh.footer.Draw(ctx, maxSize.Width)

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf},
		{Origin: components.Point{X: 0, Y: listH}, Surface: chatSurf, Z: 1},
		{Origin: components.Point{X: 0, Y: maxSize.Height - footerH}, Surface: footer, Z: 2},
	}
	if !sh.overlays.Active() {
		root.Children = append(root.Children, sh.composer.PickerOverlays(ctx, listH, maxSize.Width)...)
	}
	if pal, ok := sh.composer.PaletteOverlay(ctx); ok {
		root.Children = append(root.Children, pal)
	}
	if sh.toast.Visible() {
		toastSurf := sh.toast.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: toastSurf,
			Z:       40,
		})
	}
	return root
}
