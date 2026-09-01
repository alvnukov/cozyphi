package browse

import (
	"fmt"
	"slices"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/input"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

// Jump is the `/` fuzzy-jump machine every long list shares: it owns the
// query field, the ranked match list and the selection it started from,
// and steers the Cursor it is handed — the real selection moves live with
// the query. Drawing stays with the pane, like every other kit machine:
// the pane renders Field under a rule row labeled by Label.
type Jump struct {
	field   *input.TextField
	origin  int
	matches []int
	pos     int
}

// Open starts a jump from the cursor's current selection; the styles
// dress the query field in the pane's theme.
func (j *Jump) Open(origin int, style, placeholder xui.Style) {
	j.field = &input.TextField{
		MaxLines: 1, Style: style,
		PlaceholderStyle: placeholder, Placeholder: "type to jump",
	}
	j.origin = origin
	j.matches, j.pos = nil, 0
}

// Active reports whether the strip owns the keyboard.
func (j *Jump) Active() bool { return j != nil && j.field != nil }

// Close drops the strip without touching the selection.
func (j *Jump) Close() { j.field, j.matches = nil, nil }

// Field is the query field for the pane to draw; nil while inactive.
func (j *Jump) Field() *input.TextField { return j.field }

// JumpResult says what one event did to the strip.
type JumpResult uint8

const (
	// JumpOpen keeps the strip up; the query or the match may have moved.
	JumpOpen JumpResult = iota
	// JumpKept closed the strip on Enter, keeping the jump's selection.
	JumpKept
	// JumpBack closed the strip on Esc, restoring the origin selection.
	JumpBack
	// JumpClick closed the strip on a mouse event, keeping the selection;
	// the pane should handle the event as if the strip were never up — a
	// click is already a jump of its own.
	JumpClick
)

// Handle drives the strip for one event. rows describes the searchable
// list: row i's text, and whether the cursor may rest on it; cur is the
// cursor the jump steers.
func (j *Jump) Handle(
	ctx *components.EventContext, ev xui.Event,
	cur *Cursor, n int, row func(int) (string, bool),
) JumpResult {
	if _, ok := ev.(xui.MouseEvent); ok {
		j.Close()
		return JumpClick
	}
	if key, ok := ev.(xui.KeyEvent); ok && key.Press {
		switch key.Code {
		case xui.KeyEscape:
			cur.Select(j.origin)
			j.Close()
			return JumpBack
		case xui.KeyEnter:
			j.Close()
			return JumpKept
		case xui.KeyDown:
			j.cycle(1, cur)
			return JumpOpen
		case xui.KeyUp:
			j.cycle(-1, cur)
			return JumpOpen
		}
	}
	before := j.field.Value
	j.field.Handle(ctx, ev)
	if j.field != nil && j.field.Value != before {
		j.refresh(cur, n, row)
	}
	return JumpOpen
}

// Label is the strip's rule-row label; warn asks for warning style on a
// query with no match, so every pane names a miss the same way.
func (j *Jump) Label() (label string, warn bool) {
	switch {
	case j.field == nil || strings.TrimSpace(j.field.Value) == "":
		return " Jump ", false
	case len(j.matches) == 0:
		return " Jump · no match ", true
	case len(j.matches) == 1:
		return " Jump · 1 match ", false
	default:
		return fmt.Sprintf(" Jump · match %d/%d ", j.pos+1, len(j.matches)), false
	}
}

// StripStyle dresses the jump strip in the pane's theme.
type StripStyle struct {
	Rule   xui.Style // the rule row
	Label  xui.Style // the match label on it
	Warn   xui.Style // the label on a query with no match
	Prompt xui.Style // the "/" before the query
	// Caps are the rule row's end characters: "├","┤" inside a bordered
	// panel, "─","─" on a bare surface.
	Caps [2]string
}

// DrawStrip renders the strip into panel: a rule row at top carrying the
// match label, then the drawn query field behind a "/" prompt on the row
// below. The field is blitted at column 4; the pane places the terminal
// cursor itself, from the field surface's own cursor.
func (j *Jump) DrawStrip(
	panel *components.Surface, field components.Surface,
	method xui.WidthMethod, top, pw, ph int, st StripStyle,
) {
	if top < 1 || top >= ph-1 || pw < 2 {
		return
	}
	panel.SetCell(0, top, xui.Cell{Char: st.Caps[0], Width: 1, Style: st.Rule})
	for x := 1; x < pw-1; x++ {
		panel.SetCell(x, top, xui.Cell{Char: "─", Width: 1, Style: st.Rule})
	}
	panel.SetCell(pw-1, top, xui.Cell{Char: st.Caps[1], Width: 1, Style: st.Rule})
	label, warn := j.Label()
	style := st.Label
	if warn {
		style = st.Warn
	}
	if pw > 4 {
		panel.Print(2, top, layout.TruncateToWidth(label, pw-4, method), style, method)
		panel.Print(2, top+1, "/", st.Prompt, method)
	}
	for y := range field.Size.Height {
		for x := range field.Size.Width {
			panel.SetCell(4+x, top+1+y, field.Buffer[y*field.Size.Width+x])
		}
	}
}

// refresh recomputes the match list for the current query and parks the
// selection on the best match. No match leaves the selection where the
// last one put it; the strip label says so.
func (j *Jump) refresh(cur *Cursor, n int, row func(int) (string, bool)) {
	j.matches, j.pos = nil, 0
	query := strings.TrimSpace(j.field.Value)
	if query == "" {
		return
	}
	type match struct{ idx, score int }
	var found []match
	for i := range n {
		text, ok := row(i)
		if !ok {
			continue
		}
		if score, ok := fuzzyScore(text, query); ok {
			found = append(found, match{i, score})
		}
	}
	slices.SortStableFunc(found, func(a, b match) int { return a.score - b.score })
	for _, m := range found {
		j.matches = append(j.matches, m.idx)
	}
	if len(j.matches) > 0 {
		cur.Select(j.matches[0])
	}
}

func (j *Jump) cycle(delta int, cur *Cursor) {
	if len(j.matches) == 0 {
		return
	}
	j.pos = (j.pos + delta + len(j.matches)) % len(j.matches)
	cur.Select(j.matches[j.pos])
}

// fuzzyScore reports whether query is a case-folded subsequence of text,
// and how tight the leftmost such match is: the span it covers first,
// its start second — lower is better.
func fuzzyScore(text, query string) (int, bool) {
	q := []rune(strings.ToLower(query))
	if len(q) == 0 {
		return 0, true
	}
	start, last, qi := -1, 0, 0
	for i, r := range strings.ToLower(text) {
		if qi < len(q) && r == q[qi] {
			if qi == 0 {
				start = i
			}
			last = i
			qi++
			if qi == len(q) {
				break
			}
		}
	}
	if qi < len(q) {
		return 0, false
	}
	return (last-start)*1000 + start, true
}
