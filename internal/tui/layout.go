package tui

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

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

	e.tick++
	if e.activity.ShowSpinner() && e.tick%4 == 0 {
		e.spin.Tick()
	}
	_ = e.toast.Visible()

	maxSize := ctx.Max
	root := components.Surface{Size: maxSize, Widget: e}

	footerH := 1
	var chatH int
	if e.permAsk != nil {
		chatH = e.permAsk.preferredAskHeight(maxSize.Width, ctx.Method)
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else if e.continueAsk != nil {
		chatH = e.continueAsk.preferredAskHeight()
		maxChatH := maxSize.Height - footerH - 3
		if chatH > maxChatH {
			chatH = maxChatH
		}
		if chatH < 8 {
			chatH = 8
		}
	} else {
		chatH = e.Chat.PreferredHeight(maxSize.Width, ctx.Method)
		minChatH := 5
		if len(e.Chat.PendingSkills) > 0 {
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
	if e.permAsk != nil {
		chatSurf = e.drawPermissionAsk(ctx, maxSize.Width, chatH)
	} else if e.continueAsk != nil {
		chatSurf = e.drawContinueAsk(ctx, maxSize.Width, chatH)
	} else {
		chatSurf = e.Chat.Draw(
			ctx.WithConstraints(components.Size{}, components.Size{Width: maxSize.Width, Height: chatH}),
		)
	}
	footer := l.drawFooter(ctx, maxSize.Width)

	root.Children = []components.SubSurface{
		{Origin: components.Point{X: 0, Y: 0}, Surface: listSurf},
		{Origin: components.Point{X: 0, Y: listH}, Surface: chatSurf, Z: 1},
		{Origin: components.Point{X: 0, Y: maxSize.Height - footerH}, Surface: footer, Z: 2},
	}
	if e.permAsk == nil && e.continueAsk == nil {
		if e.slash.Open {
			e.slash.AnchorBottomY = listH
			e.slash.AnchorX = 0
			e.slash.AnchorWidth = maxSize.Width
			panel := e.slash.Draw(ctx)
			root.Children = append(root.Children, components.SubSurface{
				Origin:  components.Point{X: 0, Y: 0},
				Surface: panel,
				Z:       15,
			})
		}
		if e.mention.Open {
			e.mention.AnchorBottomY = listH
			e.mention.AnchorX = 0
			e.mention.AnchorWidth = maxSize.Width
			men := e.mention.Draw(ctx)
			root.Children = append(root.Children, components.SubSurface{
				Origin:  components.Point{X: 0, Y: 0},
				Surface: men,
				Z:       15,
			})
		}
	}
	if e.palette.Open {
		pal := e.palette.Draw(ctx)
		root.Children = append(root.Children, components.SubSurface{
			Origin:  components.Point{X: 0, Y: 0},
			Surface: pal,
			Z:       20,
		})
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

func (l *EditorLayout) drawFooter(ctx components.DrawContext, width int) components.Surface {
	e := l.e
	footer := components.NewSurface(width, 1, nil)
	dim := e.theme.Muted
	msg := e.activity.Label(e.transcript.Snapshot())
	if hs := strings.TrimSpace(e.hookStatus); hs != "" {
		if msg == "" {
			msg = hs
		} else {
			msg = hs + " · " + msg
		}
	}
	if e.ctrl != nil {
		if n := e.ctrl.LiveJobCount(); n > 0 {
			jobBit := fmt.Sprintf("%d job", n)
			if n != 1 {
				jobBit += "s"
			}
			if msg == "" {
				msg = jobBit
			} else {
				msg = msg + " · " + jobBit
			}
		}
	}

	x := 1
	if msg != "" {
		if e.activity.ShowSpinner() && e.spin != nil {
			x += e.spin.PaintScan(&footer, x, 0, e.theme.ToolName, dim, ctx.Method)
			footer.Print(x, 0, " ", dim, ctx.Method)
			x += xui.StringWidth(" ", ctx.Method)
		}
		footer.Print(x, 0, msg, dim, ctx.Method)
		x += xui.StringWidth(msg, ctx.Method)
	}

	hint := strings.TrimSpace(e.updateHint)
	if hint != "" {
		hw := xui.StringWidth(hint, ctx.Method)
		hx := width - hw - 1
		hx = max(hx, x+2)
		if hx+hw <= width {
			st := e.theme.Warning
			st.Bold = false
			footer.Print(hx, 0, hint, st, ctx.Method)
		}
	}
	return footer
}
