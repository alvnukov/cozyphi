package editor

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

// sessionLink is a shell-owned hit target. It selects only; permission replies
// and input continue to belong to the destination View.
type sessionLink struct {
	label      string
	selectView func()
}

func (b *sessionLink) Handle(ctx *components.EventContext, ev xui.Event) {
	activate := false
	switch v := ev.(type) {
	case xui.MouseEvent:
		activate = v.Button == xui.MouseLeft && v.Action == xui.MousePress
	case xui.KeyEvent:
		activate = v.Press && v.Code == xui.KeyEnter
	}
	if activate {
		b.selectView()
		ctx.ConsumeAndRedraw()
	}
}

func (b *sessionLink) Draw(ctx components.DrawContext) components.Surface {
	s := components.NewSurface(max(0, ctx.Max.Width), 1, b)
	s.Print(
		0,
		0,
		layout.EllipsizeToWidth(b.label, s.Size.Width, ctx.Method),
		components.DefaultTheme().Foreground,
		ctx.Method,
	)
	return s
}

func cleanName(name string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, name)
}

func sessionMarks(s sessions.Status) string {
	var marks []string
	if s.Running {
		marks = append(marks, "running")
	}
	if s.Waiting != "" {
		marks = append(marks, "waiting:"+s.Waiting)
	}
	if s.Interrupted {
		marks = append(marks, "interrupted")
	}
	if s.Stopped {
		marks = append(marks, "stopped")
	}
	if s.Error != "" {
		marks = append(marks, "error")
	}
	if s.Unread > 0 {
		marks = append(marks, fmt.Sprintf("unread:%d", s.Unread))
	}
	if s.LiveJobs > 0 {
		marks = append(marks, fmt.Sprintf("⚙%d", s.LiveJobs))
	}
	if len(marks) == 0 {
		return ""
	}
	return " [" + strings.Join(marks, " ") + "]"
}

func (e *Editor) drawSessions(ctx components.DrawContext) components.Surface {
	root := components.Surface{Size: ctx.Max, Widget: e}
	entries := e.registry.Entries()
	active, _ := e.registry.Active()
	// Keep the current destination visible even when all names do not fit.
	start := 0
	total := 0
	for i, entry := range entries {
		total += xui.StringWidth(cleanName(entry.DisplayName())+sessionMarks(entry.View.Status()), ctx.Method) + 6
		if entry.ID == active.ID {
			start = i
		}
	}
	if total <= ctx.Max.Width-4 {
		start = 0
	}
	x := 0
	add := func(label string, width int, action func()) {
		link := &sessionLink{label: label, selectView: action}
		root.Children = append(
			root.Children,
			components.SubSurface{
				Origin:  components.Point{X: x},
				Surface: link.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: width, Height: 1})),
			},
		)
		x += width
	}
	if len(entries) > 1 && ctx.Max.Width >= 6 {
		add("‹ ", 2, func() {
			if e.registry.Prev() == nil {
				e.syncSelection()
			}
		})
		add("› ", 2, func() {
			if e.registry.Next() == nil {
				e.syncSelection()
			}
		})
	}
	for i := start; i < len(entries) && x < ctx.Max.Width; i++ {
		entry := entries[i]
		dot := "○"
		if entry.ID == active.ID {
			dot = "●"
		}
		label := dot + " " + cleanName(entry.DisplayName()) + sessionMarks(entry.View.Status())
		width := min(ctx.Max.Width-x, xui.StringWidth(label, ctx.Method)+3)
		add(label+" / ", width, func() { _ = e.Activate(entry.ID) })
	}
	return root
}

func (e *Editor) drawShell(ctx components.DrawContext) components.Surface {
	e.bodyRows = 0
	if ctx.Max.Height < 3 || ctx.Max.Width < 1 {
		return e.active.Draw(ctx)
	}
	rows := 1
	var notice *sessionLink
	for i, entry := range e.registry.Entries() {
		status := entry.View.Status()
		if entry.View == e.active || status.Attention == "" {
			continue
		}
		shortcut := fmt.Sprintf("/switch %d", i+1)
		if next := keys.Label(keys.CmdSessionNext); next != "" {
			shortcut += " · " + next + " next"
		}
		notice = &sessionLink{
			label:      fmt.Sprintf("#%d %s: %s — %s", i+1, cleanName(entry.DisplayName()), status.Attention, shortcut),
			selectView: func() { _ = e.Activate(entry.ID) },
		}
		break
	}
	if notice != nil && ctx.Max.Height >= 8 {
		rows++
	}
	e.bodyRows = rows
	body := e.active.Draw(
		ctx.WithConstraints(components.Size{}, components.Size{Width: ctx.Max.Width, Height: ctx.Max.Height - rows}),
	)
	root := components.Surface{Size: ctx.Max, Widget: e}
	root.Children = append(
		root.Children,
		components.SubSurface{
			Surface: e.drawSessions(
				ctx.WithConstraints(components.Size{}, components.Size{Width: ctx.Max.Width, Height: 1}),
			),
		},
	)
	if rows == 2 {
		root.Children = append(
			root.Children,
			components.SubSurface{Origin: components.Point{Y: 1}, Surface: notice.Draw(ctx)},
		)
	}
	root.Children = append(root.Children, components.SubSurface{Origin: components.Point{Y: rows}, Surface: body})
	return root
}
