package block

import (
	"fmt"
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// TurnSummaryBlock is the fold handle of a condensed transcript turn: one
// muted line standing in for the working rows between a user prompt and the
// turn's final reply —
//
//	▸ worked 42s · 7 tools · pane.go, mapper.go · 1 failed
//
// Expanding it brings the hidden rows back (the mapper re-emits them); the
// row itself stays, arrow flipped, as the handle that folds the turn again.
type TurnSummaryBlock struct {
	Duration time.Duration
	Tools    int
	Failed   int
	// Rows is how many rows the fold hides — the label of last resort when
	// the turn had no timed work and no tool calls.
	Rows     int
	Files    []string
	Expanded bool
	Theme    components.Theme
	OnToggle func(expanded bool)

	lineH int
}

func (b *TurnSummaryBlock) theme() components.Theme {
	if b.Theme.Muted.Fg.Kind == 0 && b.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return b.Theme
}

// Handle toggles the fold on Enter/space or a left-click on the row.
func (b *TurnSummaryBlock) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			b.toggle(ctx)
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft && e.Y >= 0 && e.Y < max(b.lineH, 1) {
			b.toggle(ctx)
		}
	}
}

func (b *TurnSummaryBlock) toggle(ctx *components.EventContext) {
	b.Expanded = !b.Expanded
	if b.OnToggle != nil {
		b.OnToggle(b.Expanded)
	}
	ctx.ConsumeAndRedraw()
}

// PointerShape offers the hand everywhere: the whole row is the fold handle.
func (*TurnSummaryBlock) PointerShape(_, _ int) string {
	return components.ShapePointer
}

// Draw renders the one-line summary, muted except the failure count.
func (b *TurnSummaryBlock) Draw(ctx components.DrawContext) components.Surface {
	th := b.theme()
	arrow := "▸"
	if b.Expanded {
		arrow = "▾"
	}
	spans := []components.Span{
		{Text: arrow + " ", Style: th.Muted},
		{Text: b.label(), Style: th.Muted},
	}
	if b.Failed > 0 {
		spans = append(spans, components.Span{
			Text:  fmt.Sprintf(" · %d failed", b.Failed),
			Style: th.Destructive,
		})
	}
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	lines := components.WrapSpans(spans, max(w-messageIndent, 1), ctx.Method)
	b.lineH = len(lines)
	s := components.NewSurface(w, max(len(lines), 1), b)
	for y, line := range lines {
		components.PaintSpans(&s, messageIndent, y, line, ctx.Method)
	}
	return s
}

func (b *TurnSummaryBlock) label() string {
	var parts []string
	if b.Duration >= time.Second {
		parts = append(parts, "worked "+components.FormatDuration(b.Duration))
	}
	switch {
	case b.Tools == 1:
		parts = append(parts, "1 tool")
	case b.Tools > 1:
		parts = append(parts, fmt.Sprintf("%d tools", b.Tools))
	}
	if len(b.Files) > 0 {
		parts = append(parts, joinFiles(b.Files))
	}
	if len(parts) == 0 {
		word := "steps"
		if b.Rows == 1 {
			word = "step"
		}
		parts = append(parts, fmt.Sprintf("%d %s", b.Rows, word))
	}
	return strings.Join(parts, " · ")
}

// joinFiles names up to three touched files and counts the rest.
func joinFiles(files []string) string {
	if len(files) <= 3 {
		return strings.Join(files, ", ")
	}
	return strings.Join(files[:3], ", ") + fmt.Sprintf(" +%d", len(files)-3)
}
