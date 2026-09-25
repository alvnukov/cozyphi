package block

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/text"
	"github.com/alvnukov/cozyphi/internal/session"
)

const (
	foldedArrow   = " \u25b6"
	unfoldedArrow = " \u25bc"
)

// AsideBlock renders a side question: a title row with the question, and the
// message it was asked about when that was not the end of the context, over
// the Markdown answer. The gutter and the title carry the theme's Aside color,
// so the row reads as a branch off the conversation rather than a turn of it.
// A click on the title folds the answer away and brings it back.
type AsideBlock struct {
	Question string
	// AnchorPreview names the message the question was about; empty when it
	// was about the whole context.
	AnchorPreview string
	Answer        string
	SkippedTool   string
	Error         string
	State         session.State
	Expanded      bool
	Theme         components.Theme
	Spinner       *status.Spinner
	OnToggle      func(expanded bool)

	titleH   int
	markdown text.MarkdownStream
}

func (a *AsideBlock) theme() components.Theme {
	if a.Theme.Aside.Fg.Kind == 0 && a.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return a.Theme
}

func (a *AsideBlock) hasBody() bool {
	return strings.TrimSpace(a.Answer) != "" || a.SkippedTool != "" || strings.TrimSpace(a.Error) != ""
}

// Handle toggles the answer on Enter/space or a left-click on the title.
func (a *AsideBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	if !a.hasBody() {
		return
	}
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			a.toggle(ctx)
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < a.titleH {
			a.toggle(ctx)
		}
	}
}

func (a *AsideBlock) toggle(ctx *components.EventContext) {
	a.Expanded = !a.Expanded
	if a.OnToggle != nil {
		a.OnToggle(a.Expanded)
	}
	ctx.ConsumeAndRedraw()
}

// PointerShape offers the hand where a click folds, and a text beam elsewhere.
func (a *AsideBlock) PointerShape(_, y int) string {
	if a.hasBody() && y >= 0 && y < a.titleH {
		return components.ShapePointer
	}
	return components.ShapeText
}

// HoverTooltip explains the fold on the title, where the hand shows.
func (a *AsideBlock) HoverTooltip(_, y int) (string, bool) {
	if !a.hasBody() || y < 0 || y >= a.titleH {
		return "", false
	}
	if a.Expanded {
		return "fold - hide the answer to this side question", true
	}
	return "unfold - show the answer to this side question", true
}

// CollapseOnClick folds the expanded block and reports whether it did.
func (a *AsideBlock) CollapseOnClick() bool {
	return foldExpanded(&a.Expanded, a.hasBody(), a.OnToggle)
}

// CopyText returns the question, the answer and the error that ended it.
func (a *AsideBlock) CopyText() string {
	parts := append([]string{"btw: " + a.Question}, a.bodyParts()...)
	if msg := strings.TrimSpace(a.Error); msg != "" {
		parts = append(parts, msg)
	}
	return strings.Join(parts, "\n\n")
}

func (a *AsideBlock) bodyParts() []string {
	var parts []string
	if answer := strings.TrimSpace(a.Answer); answer != "" {
		parts = append(parts, answer)
	}
	if a.SkippedTool != "" {
		parts = append(parts, session.AsideSkippedToolNote(a.SkippedTool))
	}
	return parts
}

func (a *AsideBlock) titleSpans(th components.Theme) []components.Span {
	icon := "?"
	if a.State == session.StateStreaming && a.Spinner != nil {
		icon = a.Spinner.Glyph()
	}
	label := th.Aside
	label.Bold = true
	spans := []components.Span{
		{Text: icon + " ", Style: th.Aside},
		{Text: "btw ", Style: label},
		{Text: strings.TrimSpace(a.Question), Style: th.Foreground},
	}
	if a.AnchorPreview != "" {
		spans = append(spans, components.Span{Text: "  re: " + a.AnchorPreview, Style: th.Muted})
	}
	switch a.State {
	case session.StateCancelled:
		spans = append(spans, components.Span{Text: " (cancelled)", Style: th.Warning})
	case session.StateError:
		spans = append(spans, components.Span{Text: " (failed)", Style: th.Destructive})
	}
	if a.hasBody() {
		arrow := foldedArrow
		if a.Expanded {
			arrow = unfoldedArrow
		}
		spans = append(spans, components.Span{Text: arrow, Style: th.Muted})
	}
	return spans
}

// Draw renders the title and, when expanded, the answer below it.
func (a *AsideBlock) Draw(ctx components.DrawContext) components.Surface {
	th := a.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	inner := max(w-messageIndent, 1)
	titleLines := components.WrapSpans(a.titleSpans(th), inner, ctx.Method)
	a.titleH = len(titleLines)

	var bodyLines []components.RichLine
	if a.Expanded {
		if body := strings.Join(a.bodyParts(), "\n\n"); body != "" {
			bodyLines = a.markdown.Render(body, th, inner, ctx.Method)
		}
		if msg := strings.TrimSpace(a.Error); msg != "" {
			bodyLines = append(bodyLines, components.WrapSpans(
				[]components.Span{{Text: msg, Style: th.Destructive}}, inner, ctx.Method)...)
		}
	}

	s := components.NewSurface(w, max(len(titleLines)+len(bodyLines), 1), a)
	y := 0
	for _, line := range titleLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	for _, line := range bodyLines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
		y++
	}
	components.HoverTitleRows(ctx, &s, a, a.titleH, th.BackgroundElement, a.hasBody())
	gutterBar(&s, th.Aside)
	return s
}
