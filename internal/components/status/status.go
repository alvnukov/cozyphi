package status

import (
	"strings"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// Expandable is a titled container whose Child is shown when expanded.
type Expandable struct {
	Title      components.Widget
	Child      components.Widget
	Expanded   bool
	Expandable bool // if false, title only
	Theme      components.Theme
	OnChanged  func(expanded bool)
}

func (e *Expandable) theme() components.Theme {
	if e.Theme.Muted.Fg.Kind == 0 && e.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return e.Theme
}

// Handle toggles expansion on Enter/space or a title click and forwards
// other events to the title and child widgets.
func (e *Expandable) Handle(ctx *components.EventContext, ev xui.Event) {
	if !e.Expandable {
		if e.Expanded && e.Child != nil {
			e.Child.Handle(ctx, ev)
		}
		if e.Title != nil {
			e.Title.Handle(ctx, ev)
		}
		return
	}
	switch ev := ev.(type) {
	case xui.KeyEvent:
		if ev.Code == xui.KeyEnter || (ev.Code == xui.KeyRune && ev.Rune == ' ') {
			e.Expanded = !e.Expanded
			if e.OnChanged != nil {
				e.OnChanged(e.Expanded)
			}
			ctx.ConsumeAndRedraw()
			return
		}
	case xui.MouseEvent:
		if ev.Action == xui.MousePress && ev.Button == xui.MouseLeft {
			// Clicks on y==0 (title) toggle; body forwarded when expanded.
			if ev.Y == 0 || !e.Expanded {
				e.Expanded = !e.Expanded
				if e.OnChanged != nil {
					e.OnChanged(e.Expanded)
				}
				ctx.ConsumeAndRedraw()
				return
			}
		}
	}
	if e.Expanded && e.Child != nil {
		e.Child.Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
	if e.Title != nil {
		e.Title.Handle(ctx, ev)
	}
}

// PointerShape offers the hand over the title — and anywhere while
// collapsed, mirroring where a click toggles — and a text beam over the
// expanded body.
func (e *Expandable) PointerShape(_, y int) string {
	if !e.Expandable {
		return ""
	}
	if y == 0 || !e.Expanded {
		return components.ShapePointer
	}
	return components.ShapeText
}

// Draw renders the title row (with expand arrow) and the child below it
// when expanded.
func (e *Expandable) Draw(ctx components.DrawContext) components.Surface {
	th := e.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	var titleSurf components.Surface
	if e.Title != nil {
		titleSurf = e.Title.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: w - 2, Height: 1}))
	} else {
		titleSurf = components.NewSurface(0, 1, nil)
	}
	arrow := ""
	if e.Expandable {
		if e.Expanded {
			arrow = "▼"
		} else {
			arrow = "▶"
		}
	}
	titleH := titleSurf.Size.Height
	titleH = max(titleH, 1)

	var body components.Surface
	bodyH := 0
	if e.Expanded && e.Child != nil {
		body = e.Child.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: w, Height: 10000}))
		bodyH = body.Size.Height
	}

	h := titleH + bodyH
	s := components.NewSurface(w, h, e)
	// paint title
	s.Children = append(s.Children, components.SubSurface{Origin: components.Point{X: 0, Y: 0}, Surface: titleSurf})
	if arrow != "" {
		ax := titleSurf.Size.Width + 1
		if ax >= w {
			ax = w - 1
		}
		if ax < 0 {
			ax = 0
		}
		s.SetCell(ax, 0, xui.Cell{Char: arrow, Width: 1, Style: th.Muted})
	}
	// The visual hover affordance mirrors the pointer: the title row while
	// expanded, the whole (title-only) block while collapsed. Both layers
	// need it — the title glyphs live in the child surface, the arrow cell
	// in the parent.
	if e.Expandable && components.Hovering(ctx, e) && (e.Expanded || ctx.Hover.Y == 0) {
		components.ApplyHoverRows(&s.Children[0].Surface, 0, titleH, th.BackgroundElement)
		components.ApplyHoverRows(&s, 0, titleH, th.BackgroundElement)
	}
	if bodyH > 0 {
		s.Children = append(
			s.Children,
			components.SubSurface{Origin: components.Point{X: 0, Y: titleH}, Surface: body, Z: 1},
		)
	}
	return s
}

// Spinner — animated activity indicator. Advance with Tick().
//
// Tool/thinking rows use a 1-cell braille glyph. The footer uses a
// Knight-Rider scan bar (■ trail on ⬝) so the two sites
// don't compete with the same mark.
type Spinner struct {
	Frame    int
	Style    xui.Style
	frames   []string
	scan     []string
	Interval time.Duration
	lastTick time.Time
	now      func() time.Time
}

const (
	scanOn    = '■'
	scanOff   = '⬝'
	scanW     = 6
	scanTrail = 2
)

// NewSpinner returns a spinner with a 1-cell braille glyph and a footer scan bar.
func NewSpinner(style xui.Style) *Spinner {
	return &Spinner{
		Style:    style,
		Interval: 80 * time.Millisecond,
		frames:   []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		scan:     knightRider(scanW, scanTrail),
		now:      time.Now,
	}
}

func knightRider(width, trail int) []string {
	if width < 2 {
		width = 2
	}
	if trail < 1 {
		trail = 1
	}
	type step struct{ head, dir int }
	steps := make([]step, 0, width*2)
	for i := 0; i < width; i++ {
		steps = append(steps, step{i, 1})
	}
	for i := width - 2; i >= 1; i-- {
		steps = append(steps, step{i, -1})
	}
	out := make([]string, 0, len(steps))
	var b strings.Builder
	for _, st := range steps {
		b.Reset()
		for c := 0; c < width; c++ {
			behind := (st.head - c) * st.dir
			if behind >= 0 && behind < trail {
				b.WriteRune(scanOn)
			} else {
				b.WriteRune(scanOff)
			}
		}
		out = append(out, b.String())
	}
	return out
}

// NewWaveSpinner uses ASCII-safe wave frames. Avoid East-Asian-ambiguous
// glyphs (∼ ≈ ≋) which break column tracking on some terminals.
func NewWaveSpinner(style xui.Style) *Spinner {
	return &Spinner{
		Style:    style,
		Interval: 200 * time.Millisecond,
		frames:   []string{" ", ".", "o", "O", "o", "."},
		now:      time.Now,
	}
}

// Tick advances the spinner by the frames due since the last tick, so the
// animation rate tracks wall-clock time instead of the number of draw calls.
// Without this, mouse movement (which drives extra redraws) would speed the
// spinner up. The first tick only latches the start time.
func (s *Spinner) Tick() {
	if s == nil {
		return
	}
	n := s.frameCount()
	if n == 0 {
		return
	}
	now := s.now()
	if s.lastTick.IsZero() {
		s.lastTick = now
		return
	}
	elapsed := now.Sub(s.lastTick)
	if elapsed < s.Interval {
		return
	}
	steps := int(elapsed / s.Interval)
	steps = max(steps, 1)
	s.Frame = (s.Frame + steps) % n
	s.lastTick = s.lastTick.Add(time.Duration(steps) * s.Interval)
}

func (s *Spinner) frameCount() int {
	n := len(s.frames)
	if ns := len(s.scan); ns > n {
		n = ns
	}
	return n
}

// Handle is a no-op; spinners do not take input.
func (*Spinner) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the current spinner frame as a single cell.
func (s *Spinner) Draw(_ components.DrawContext) components.Surface {
	ch := "⋯"
	if len(s.frames) > 0 {
		ch = s.frames[s.Frame%len(s.frames)]
	}
	st := s.Style
	if st == (xui.Style{}) {
		st = components.DefaultTheme().ToolName
	}
	surf := components.NewSurface(1, 1, s)
	surf.SetCell(0, 0, xui.Cell{Char: ch, Width: 1, Style: st})
	return surf
}

// Glyph returns the current 1-cell spinner character (tool / thinking rows).
func (s *Spinner) Glyph() string {
	if s == nil || len(s.frames) == 0 {
		return "⋯"
	}
	return s.frames[s.Frame%len(s.frames)]
}

// Scan returns the current footer scanner string (Knight-Rider bar).
func (s *Spinner) Scan() string {
	if s == nil || len(s.scan) == 0 {
		return s.Glyph()
	}
	return s.scan[s.Frame%len(s.scan)]
}

// PaintScan draws Scan() with the head/trail in `on` and the rest in `off`.
func (s *Spinner) PaintScan(dst *components.Surface, x, y int, on, off xui.Style, method xui.WidthMethod) int {
	if s == nil || dst == nil {
		return 0
	}
	start := x
	for _, r := range s.Scan() {
		st := off
		if r == scanOn {
			st = on
		}
		ch := string(r)
		dst.Print(x, y, ch, st, method)
		x += xui.StringWidth(ch, method)
	}
	return x - start
}

// ScrollView is a single-child scroll viewport.
type ScrollView struct {
	Child  components.Widget
	Offset int // rows scrolled from top
	Theme  components.Theme
}

// Handle scrolls on arrow/page keys and mouse wheel, forwarding other
// events to the child.
func (s *ScrollView) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		switch e.Code {
		case xui.KeyUp, xui.KeyPageUp:
			step := 1
			if e.Code == xui.KeyPageUp {
				step = 10
			}
			s.Offset -= step
			if s.Offset < 0 {
				s.Offset = 0
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyDown, xui.KeyPageDown:
			step := 1
			if e.Code == xui.KeyPageDown {
				step = 10
			}
			s.Offset += step
			ctx.ConsumeAndRedraw()
			return
		}
	case xui.MouseEvent:
		wheel := e.Wheel
		wheel = max(wheel, 1)
		if e.Button == xui.MouseWheelUp {
			s.Offset -= 3 * wheel
			if s.Offset < 0 {
				s.Offset = 0
			}
			ctx.ConsumeAndRedraw()
			return
		}
		if e.Button == xui.MouseWheelDown {
			s.Offset += 3 * wheel
			ctx.ConsumeAndRedraw()
			return
		}
	}
	if s.Child != nil {
		s.Child.Handle(ctx, ev)
	}
}

// Draw renders the child clipped to the viewport at the current offset.
func (s *ScrollView) Draw(ctx components.DrawContext) components.Surface {
	w, h := ctx.Max.Width, ctx.Max.Height
	if w <= 0 {
		w = 40
	}
	if h <= 0 {
		h = 10
	}
	out := components.Surface{Size: components.Size{Width: w, Height: h}, Widget: s}
	if s.Child == nil {
		return out
	}
	child := s.Child.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: w, Height: 100000}))
	maxOff := child.Size.Height - h
	maxOff = max(maxOff, 0)
	if s.Offset > maxOff {
		s.Offset = maxOff
	}
	if s.Offset < 0 {
		s.Offset = 0
	}
	out.Children = []components.SubSurface{{
		Origin:  components.Point{X: 0, Y: -s.Offset},
		Surface: child,
	}}
	return out
}

// ListTile renders a selectable row: leading widget, title/subtitle text,
// and trailing widget.
type ListTile struct {
	Leading  components.Widget
	Title    string
	Subtitle string
	Trailing components.Widget
	Selected bool
	Theme    components.Theme
	OnTap    func()
}

func (l *ListTile) theme() components.Theme {
	if l.Theme.Foreground.Fg.Kind == 0 && l.Theme.Muted.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return l.Theme
}

// Handle invokes OnTap on Enter/space or a left click.
func (l *ListTile) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			if l.OnTap != nil {
				l.OnTap()
			}
			ctx.ConsumeAndRedraw()
			return
		}
	case xui.MouseEvent:
		if e.Action == xui.MousePress && e.Button == xui.MouseLeft {
			if l.OnTap != nil {
				l.OnTap()
			}
			ctx.ConsumeAndRedraw()
			return
		}
	}
}

// PointerShape offers the hand while a tap actually runs an action.
func (l *ListTile) PointerShape(_, _ int) string {
	if l.OnTap != nil {
		return components.ShapePointer
	}
	return ""
}

// Draw renders the leading, title, subtitle, and trailing within one or
// two rows, with a selection marker when Selected.
func (l *ListTile) Draw(ctx components.DrawContext) components.Surface {
	th := l.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	h := 1
	if l.Subtitle != "" {
		h = 2
	}
	s := components.NewSurface(w, h, l)
	x := 0
	if l.Selected {
		s.SetCell(0, 0, xui.Cell{Char: "‣", Width: 1, Style: th.Success})
		x = 2
	}
	if l.Leading != nil {
		lead := l.Leading.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: 4, Height: 1}))
		s.Children = append(s.Children, components.SubSurface{Origin: components.Point{X: x, Y: 0}, Surface: lead})
		x += lead.Size.Width + 1
	}
	titleStyle := th.Foreground
	if l.Selected {
		titleStyle.Bold = true
	}
	s.Print(x, 0, l.Title, titleStyle, ctx.Method)
	if l.Subtitle != "" {
		s.Print(x, 1, l.Subtitle, th.Muted, ctx.Method)
	}
	if l.Trailing != nil {
		trail := l.Trailing.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: 10, Height: 1}))
		tx := w - trail.Size.Width
		tx = max(tx, x)
		s.Children = append(
			s.Children,
			components.SubSurface{Origin: components.Point{X: tx, Y: 0}, Surface: trail, Z: 1},
		)
	}
	// Painted last: the hover affordance must survive the title prints,
	// which replace cell styles wholesale.
	if components.Hovering(ctx, l) && l.OnTap != nil {
		components.ApplyHoverRows(&s, 0, h, th.BackgroundElement)
	}
	return s
}

// StatusLine — bottom hint row.
type StatusLine struct {
	Left    string
	Right   string
	Theme   components.Theme
	Spinner *Spinner // optional leading spinner on Left
}

// Handle is a no-op; the status line is read-only.
func (*StatusLine) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the left hint (with optional spinner) and right-aligned text.
func (s *StatusLine) Draw(ctx components.DrawContext) components.Surface {
	th := components.DefaultTheme()
	if s.Theme.Muted.Fg.Kind != 0 || s.Theme.Foreground.Fg.Kind != 0 {
		th = s.Theme
	}
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	surf := components.NewSurface(w, 1, s)
	x := 0
	if s.Spinner != nil {
		g := s.Spinner.Glyph()
		surf.SetCell(0, 0, xui.Cell{Char: g, Width: 1, Style: th.ToolName})
		x = 2
	}
	if s.Left != "" {
		surf.Print(x, 0, s.Left, th.Muted, ctx.Method)
	}
	if s.Right != "" {
		rw := xui.StringWidth(s.Right, ctx.Method)
		surf.Print(w-rw, 0, s.Right, th.Success, ctx.Method)
	}
	return surf
}

// ToolHeader renders a status glyph, tool name, and optional detail on one row.
type ToolHeader struct {
	Name    string
	Detail  string
	Status  ToolStatus
	Theme   components.Theme
	Spinner *Spinner
}

// ToolStatus mirrors tool status icons.
type ToolStatus int

// Tool status values for tool header rows.
const (
	ToolDone ToolStatus = iota
	ToolRunning
	ToolError
	ToolCancelled
	ToolQueued
	ToolRejected
	// ToolLive marks a call that finished while what it started runs on in
	// the background — a watch that has not ended. The row keeps a pulse
	// instead of the checkmark, because the checkmark would say it is over.
	ToolLive
)

func (t *ToolHeader) theme() components.Theme {
	if t.Theme.Success.Fg.Kind == 0 && t.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return t.Theme
}

// Handle is a no-op; tool headers are read-only.
func (*ToolHeader) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the status glyph, tool name, and detail spans.
func (t *ToolHeader) Draw(ctx components.DrawContext) components.Surface {
	th := t.theme()
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	icon := "✓"
	iconSt := th.Success
	switch t.Status {
	case ToolRunning, ToolQueued:
		icon = "⋯"
		iconSt = th.ToolName
		if t.Spinner != nil {
			icon = t.Spinner.Glyph()
		}
	case ToolError:
		icon = "✗"
		iconSt = th.Destructive
	case ToolCancelled, ToolRejected:
		icon = "⊘"
		iconSt = th.Muted
	}
	spans := []components.Span{
		{Text: icon + " ", Style: iconSt},
		{Text: t.Name, Style: th.ToolName},
	}
	if t.Detail != "" {
		spans = append(spans, components.Span{Text: " " + t.Detail, Style: th.Muted})
	}
	return components.PaintRichLines(w, components.WrapSpans(spans, w, ctx.Method), ctx.Method, t)
}
