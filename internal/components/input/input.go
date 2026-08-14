package input

import (
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/text"
)

// TextField is a multiline editor without tui border chrome.
type TextField struct {
	Value            string
	Cursor           int
	Placeholder      string
	MaxLines         int // soft visual cap; 0 = unconstrained
	Style            xui.Style
	PlaceholderStyle xui.Style
	OnSubmit         func(string)
	OnChange         func(string)
}

func (t *TextField) clamp() {
	if t.Cursor < 0 {
		t.Cursor = 0
	}
	if t.Cursor > len(t.Value) {
		t.Cursor = len(t.Value)
	}
}

func (t *TextField) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if !e.Press {
			return
		}
		t.clamp()
		switch e.Code {
		case xui.KeyEnter:
			if e.Mods.Has(xui.ModShift) || e.Mods.Has(xui.ModCtrl) {
				t.insert("\n")
				ctx.ConsumeAndRedraw()
				return
			}
			if t.OnSubmit != nil {
				t.OnSubmit(t.Value)
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyBackspace:
			if t.Cursor > 0 {
				_, size := utf8.DecodeLastRuneInString(t.Value[:t.Cursor])
				t.Value = t.Value[:t.Cursor-size] + t.Value[t.Cursor:]
				t.Cursor -= size
				t.notify()
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyLeft:
			if t.Cursor > 0 {
				_, size := utf8.DecodeLastRuneInString(t.Value[:t.Cursor])
				t.Cursor -= size
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyRight:
			if t.Cursor < len(t.Value) {
				_, size := utf8.DecodeRuneInString(t.Value[t.Cursor:])
				t.Cursor += size
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyRune:
			if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) {
				return
			}
			if e.Rune >= 0x20 || e.Rune == '\t' {
				t.insert(string(e.Rune))
				ctx.ConsumeAndRedraw()
			}
		}
	case xui.PasteEvent:
		t.insert(e.Text)
		ctx.ConsumeAndRedraw()
	}
}

func (t *TextField) insert(s string) {
	t.clamp()
	t.Value = t.Value[:t.Cursor] + s + t.Value[t.Cursor:]
	t.Cursor += len(s)
	t.notify()
}

func (t *TextField) notify() {
	if t.OnChange != nil {
		t.OnChange(t.Value)
	}
}

func (t *TextField) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	display := t.Value
	style := t.Style
	if display == "" && t.Placeholder != "" {
		display = t.Placeholder
		style = t.PlaceholderStyle
		if style == (xui.Style{}) {
			style = components.DefaultTheme().Muted
		}
	}
	lines := text.WrapEditorLines(display, w, ctx.Method)
	h := len(lines)
	if h < 1 {
		h = 1
	}
	if t.MaxLines > 0 && h > t.MaxLines {
		h = t.MaxLines
	}
	if ctx.Max.Height > 0 && h > ctx.Max.Height {
		h = ctx.Max.Height
	}
	s := components.NewSurface(w, h, t)
	for i := 0; i < h && i < len(lines); i++ {
		s.Print(0, i, lines[i], style, ctx.Method)
	}
	// Cursor only when editing real value (not placeholder).
	if t.Value != "" || t.Placeholder == "" {
		line, col := text.CursorLineCol(t.Value, t.Cursor, w, ctx.Method)
		if line >= h {
			line = h - 1
		}
		if line < 0 {
			line = 0
		}
		s.Cursor = &components.Point{X: col, Y: line}
	} else {
		s.Cursor = &components.Point{X: 0, Y: 0}
	}
	return s
}

// DiffBlock renders a unified diff with coloring.
type DiffBlock struct {
	Diff  string
	Theme components.Theme
}

func (d *DiffBlock) theme() components.Theme {
	if d.Theme.Success.Fg.Kind == 0 && d.Theme.Destructive.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return d.Theme
}

func (*DiffBlock) Handle(_ *components.EventContext, _ xui.Event) {}

func (d *DiffBlock) Draw(ctx components.DrawContext) components.Surface {
	th := d.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	raw := strings.ReplaceAll(d.Diff, "\r", "")
	lines := strings.Split(raw, "\n")
	h := len(lines)
	if h < 1 {
		h = 1
	}
	s := components.NewSurface(w, h, d)
	for y, line := range lines {
		st := th.Foreground
		switch {
		case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
			st = th.Success
			st.Bold = false
		case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
			st = th.Destructive
		case strings.HasPrefix(line, "@@"):
			st = th.ToolName
		case strings.HasPrefix(line, "diff ") || strings.HasPrefix(line, "index ") ||
			strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++"):
			st = th.Muted
		}
		s.Print(0, y, layout.TruncateToWidth(line, w, ctx.Method), st, ctx.Method)
	}
	return s
}

// Markdown renders a subset of markdown to spans.
type Markdown struct {
	Source string
	Theme  components.Theme
}

func (m *Markdown) theme() components.Theme {
	if m.Theme.Foreground.Fg.Kind == 0 && m.Theme.Warning.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return m.Theme
}

func (*Markdown) Handle(_ *components.EventContext, _ xui.Event) {}

func (m *Markdown) Draw(ctx components.DrawContext) components.Surface {
	th := m.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	spans := markdownSpans(m.Source, th)
	return components.PaintRichLines(w, components.WrapSpans(spans, w, ctx.Method), ctx.Method, m)
}

func markdownSpans(src string, th components.Theme) []components.Span {
	var out []components.Span
	lines := strings.Split(strings.ReplaceAll(src, "\r", ""), "\n")
	for i, line := range lines {
		if i > 0 {
			out = append(out, components.Span{Text: "\n", Style: th.Foreground})
		}
		switch {
		case strings.HasPrefix(line, "# "):
			out = append(
				out,
				components.Span{
					Text:  strings.TrimPrefix(line, "# "),
					Style: xui.Style{Bold: true, Fg: th.Foreground.Fg},
				},
			)
		case strings.HasPrefix(line, "## "):
			out = append(
				out,
				components.Span{
					Text:  strings.TrimPrefix(line, "## "),
					Style: xui.Style{Bold: true, Fg: th.ToolName.Fg},
				},
			)
		case strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* "):
			out = append(out, components.Span{Text: "• ", Style: th.Muted})
			out = append(out, text.HighlightAssistant(strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* "), th)...)
		case strings.HasPrefix(line, "```"):
			out = append(out, components.Span{Text: line, Style: th.Muted})
		default:
			out = append(out, text.HighlightAssistant(line, th)...)
		}
	}
	if len(out) == 0 {
		out = []components.Span{{Text: src, Style: th.Foreground}}
	}
	return out
}

// Modal is a simple centered dialog shell.
type Modal struct {
	Title   string
	Body    components.Widget
	Footer  string
	Width   int
	Theme   components.Theme
	OnClose func()
}

func (m *Modal) theme() components.Theme {
	if m.Theme.Border.Fg.Kind == 0 && m.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return m.Theme
}

func (m *Modal) Handle(ctx *components.EventContext, ev xui.Event) {
	if e, ok := ev.(xui.KeyEvent); ok {
		if e.Code == xui.KeyEscape || (e.Code == xui.KeyRune && (e.Rune == 'q' || e.Rune == 'Q')) {
			if m.OnClose != nil {
				m.OnClose()
			}
			ctx.ConsumeAndRedraw()
			return
		}
	}
	if m.Body != nil {
		m.Body.Handle(ctx, ev)
	}
}

func (m *Modal) Draw(ctx components.DrawContext) components.Surface {
	th := m.theme()
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 24
	}
	boxW := m.Width
	if boxW <= 0 {
		boxW = maxW * 3 / 4
		if boxW < 40 {
			boxW = maxW
			if boxW > 60 {
				boxW = 60
			}
		}
	}
	if boxW > maxW {
		boxW = maxW
	}

	var body components.Surface
	if m.Body != nil {
		body = m.Body.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: boxW - 4, Height: maxH - 6}))
	}
	innerH := body.Size.Height + 2 // title + footer spacing
	if m.Title != "" {
		innerH++
	}
	if m.Footer != "" {
		innerH++
	}
	boxH := innerH + 2
	if boxH > maxH {
		boxH = maxH
	}

	panel := components.NewSurface(boxW, boxH, m)
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, th.Border, nil, nil, nil, nil, ctx.Method)
	y := 1
	if m.Title != "" {
		panel.Print(2, y, m.Title, xui.Style{Bold: true, Fg: th.Foreground.Fg}, ctx.Method)
		y++
	}
	if m.Body != nil {
		panel.Children = append(
			panel.Children,
			components.SubSurface{Origin: components.Point{X: 2, Y: y}, Surface: body},
		)
	}
	if m.Footer != "" {
		panel.Print(2, boxH-2, m.Footer, th.Muted, ctx.Method)
	}

	// Center on full screen
	out := components.Surface{Size: components.Size{Width: maxW, Height: maxH}, Widget: m}
	ox := (maxW - boxW) / 2
	oy := (maxH - boxH) / 2
	if ox < 0 {
		ox = 0
	}
	if oy < 0 {
		oy = 0
	}
	out.Children = []components.SubSurface{{Origin: components.Point{X: ox, Y: oy}, Surface: panel}}
	return out
}
