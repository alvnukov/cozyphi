package tui

import (
	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/xui"
)

func (editor *Editor) drawFooter(ctx components.DrawContext, width int) components.Surface {
	footer := components.NewSurface(width, 1, nil)
	dim := editor.theme.Muted
	msg := editor.activity.Label(editor.snap)

	x := 1
	if msg != "" {
		if editor.activity.ShowSpinner() && editor.spin != nil {
			g := editor.spin.Glyph()
			footer.Print(x, 0, g, editor.theme.ToolName, ctx.Method)
			x += xui.StringWidth(g, ctx.Method)
			footer.Print(x, 0, " ", dim, ctx.Method)
			x += xui.StringWidth(" ", ctx.Method)
		}
		footer.Print(x, 0, msg, dim, ctx.Method)
		x += xui.StringWidth(msg, ctx.Method)
	}

	stats := editor.usageStats
	if stats != "" {
		sw := xui.StringWidth(stats, ctx.Method)
		sx := width - sw - 1
		if sx < x+2 {
			sx = x + 2
		}
		if sx+sw <= width {
			footer.Print(sx, 0, stats, dim, ctx.Method)
		}
	}
	return footer
}
