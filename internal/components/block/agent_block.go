package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/components/text"
	"github.com/pulseaiclub/phi/internal/components/tree"
)

// ChildTool is one nested tool row under an AgentBlock (sub-agent tree).
type ChildTool struct {
	Name   string
	Detail string
	Status status.ToolStatus
}

// AgentBlock renders agent_spawn / agent_wait with an optional
// nested tool tree and a terminal markdown summary (not raw JSON).
type AgentBlock struct {
	Name     string
	Detail   string
	Status   status.ToolStatus
	Children []ChildTool
	Summary  string // markdown; shown only when set
	Error    string
	Expanded bool
	Theme    components.Theme
	Spinner  *status.Spinner
	OnToggle func(expanded bool)

	titleH int
}

func (a *AgentBlock) theme() components.Theme {
	if a.Theme.Success.Fg.Kind == 0 && a.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return a.Theme
}

func (a *AgentBlock) hasBody() bool {
	return len(a.Children) > 0 || strings.TrimSpace(a.Summary) != "" || strings.TrimSpace(a.Error) != ""
}

// Handle toggles expansion on Enter/space or a left-click on the title row.
func (a *AgentBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if !a.hasBody() {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			a.Expanded = !a.Expanded
			if a.OnToggle != nil {
				a.OnToggle(a.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < a.titleH {
			a.Expanded = !a.Expanded
			if a.OnToggle != nil {
				a.OnToggle(a.Expanded)
			}
			ctx.ConsumeAndRedraw()
		}
	}
}

// PointerShape offers the hand exactly where a click acts — the title row of
// an agent run with content — and a text beam over the rest.
func (a *AgentBlock) PointerShape(_, y int) string {
	if a.hasBody() && y >= 0 && y < a.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// CopyText returns name, detail, child lines, and summary.
func (a *AgentBlock) CopyText() string {
	var b strings.Builder
	b.WriteString(a.Name)
	if a.Detail != "" {
		b.WriteByte(' ')
		b.WriteString(a.Detail)
	}
	st := tree.DefaultStyle()
	for i, c := range a.Children {
		b.WriteByte('\n')
		b.WriteString(tree.PrefixForSiblings(len(a.Children), i, st))
		b.WriteString(childIcon(c.Status))
		b.WriteByte(' ')
		b.WriteString(c.Name)
		if c.Detail != "" {
			b.WriteByte(' ')
			b.WriteString(c.Detail)
		}
	}
	if sum := strings.TrimSpace(a.Summary); sum != "" {
		b.WriteByte('\n')
		b.WriteString(sum)
	}
	if err := strings.TrimSpace(a.Error); err != "" {
		b.WriteByte('\n')
		b.WriteString("Error: ")
		b.WriteString(err)
	}
	return b.String()
}

// Draw renders the agent title (icon + name + detail), the nested tool tree,
// and the markdown summary / error body when expanded.
func (a *AgentBlock) Draw(ctx components.DrawContext) components.Surface {
	th := a.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}

	icon, iconSt := toolIcon(a.Status, th, a.Spinner)
	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
		{Text: a.Name, Style: th.ToolName},
	}
	if a.Detail != "" {
		spans = append(spans, components.Span{Text: " " + a.Detail, Style: th.Muted})
	}
	switch a.Status {
	case status.ToolCancelled:
		spans = append(spans, components.Span{Text: " (cancelled)", Style: th.Muted})
	case status.ToolRejected:
		spans = append(spans, components.Span{Text: " (rejected)", Style: th.Muted})
	}
	if a.hasBody() {
		arrow := " ▶"
		if a.Expanded {
			arrow = " ▼"
		}
		spans = append(spans, components.Span{Text: arrow, Style: th.Muted})
	}

	titleLines := components.WrapSpans(spans, max(w-messageIndent, 1), ctx.Method)
	a.titleH = len(titleLines)

	var treeLines []components.RichLine
	var footLines []components.RichLine
	if a.Expanded && a.hasBody() {
		bodyW := w
		if bodyW > 2 {
			bodyW -= 2
		}
		bodyW = max(bodyW-messageIndent, 1)
		st := tree.DefaultStyle()
		n := len(a.Children)
		for i, c := range a.Children {
			prefix := tree.PrefixForSiblings(n, i, st)
			cIcon, cSt := toolIcon(c.Status, th, a.Spinner)
			row := []components.Span{
				{Text: prefix, Style: th.Muted},
				{Text: cIcon + " ", Style: cSt},
				{Text: c.Name, Style: th.ToolName},
			}
			if c.Detail != "" {
				row = append(row, components.Span{Text: " " + c.Detail, Style: th.Muted})
			}
			treeLines = append(treeLines, components.WrapSpans(row, max(w-messageIndent, 1), ctx.Method)...)
		}
		if err := strings.TrimSpace(a.Error); err != "" {
			footLines = append(footLines, components.WrapSpans([]components.Span{
				{Text: "Error: " + err, Style: th.Destructive},
			}, bodyW, ctx.Method)...)
		}
		if sum := strings.TrimSpace(a.Summary); sum != "" {
			md := text.RenderMarkdown(sum, th)
			footLines = append(footLines, components.WrapSpans(md, bodyW, ctx.Method)...)
		}
	}

	h := len(titleLines) + len(treeLines) + len(footLines)
	h = max(h, 1)
	s := components.NewSurface(w, h, a)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range treeLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range footLines {
		components.PaintSpans(&s, messageIndent+2, y, line, ctx.Method)
		y++
	}
	return s
}

func toolIcon(st status.ToolStatus, th components.Theme, spin *status.Spinner) (string, xui.Style) {
	icon := "✓"
	iconSt := th.Success
	switch st {
	case status.ToolRunning, status.ToolQueued:
		icon = "⋯"
		iconSt = th.ToolName
		if spin != nil {
			icon = spin.Glyph()
		}
	case status.ToolError:
		icon = "✗"
		iconSt = th.Destructive
	case status.ToolCancelled, status.ToolRejected:
		icon = "⊘"
		iconSt = th.Muted
	}
	return icon, iconSt
}

func childIcon(st status.ToolStatus) string {
	switch st {
	case status.ToolRunning, status.ToolQueued:
		return "⋯"
	case status.ToolError:
		return "✗"
	case status.ToolCancelled, status.ToolRejected:
		return "⊘"
	default:
		return "✓"
	}
}
