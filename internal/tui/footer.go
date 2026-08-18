package tui

import (
	"fmt"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

func (editor *Editor) drawFooter(ctx components.DrawContext, width int) components.Surface {
	footer := components.NewSurface(width, 1, nil)
	dim := editor.theme.Muted
	msg := editor.activity.Label(editor.snap)
	if editor.ctrl != nil {
		if n := editor.ctrl.LiveJobCount(); n > 0 {
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
		if editor.activity.ShowSpinner() && editor.spin != nil {
			x += editor.spin.PaintScan(&footer, x, 0, editor.theme.ToolName, dim, ctx.Method)
			footer.Print(x, 0, " ", dim, ctx.Method)
			x += xui.StringWidth(" ", ctx.Method)
		}
		footer.Print(x, 0, msg, dim, ctx.Method)
		x += xui.StringWidth(msg, ctx.Method)
	}

	hint := strings.TrimSpace(editor.updateHint)
	if hint != "" {
		hw := xui.StringWidth(hint, ctx.Method)
		hx := width - hw - 1
		hx = max(hx, x+2)
		if hx+hw <= width {
			st := editor.theme.Warning
			st.Bold = false
			footer.Print(hx, 0, hint, st, ctx.Method)
		}
	}
	return footer
}
