package layout

import (
	"slices"
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

// BorderStyle selects box-drawing characters.
type BorderStyle int

// Border styles for drawable boxes.
const (
	BorderRounded BorderStyle = iota
	BorderSquare
)

type borderChars struct {
	tl, tr, bl, br, h, v string
}

func borderGlyphs(s BorderStyle) borderChars {
	if s == BorderSquare {
		return borderChars{tl: "┌", tr: "┐", bl: "└", br: "┘", h: "─", v: "│"}
	}
	return borderChars{tl: "╭", tr: "╮", bl: "╰", br: "╯", h: "─", v: "│"}
}

// BorderLabel is text embedded into a border edge.
type BorderLabel struct {
	Text  string
	Style xui.Style
}

// DrawRoundedBorder paints a rounded (or square) box onto s and embeds labels
// into the top/bottom edges. Labels on the right are right-aligned with a 1-cell
// gap from the corner; left labels leave a 1-cell gap from the left corner.
func DrawRoundedBorder(
	s *components.Surface,
	style BorderStyle,
	borderStyle xui.Style,
	topLeft, topRight, bottomLeft, bottomRight *BorderLabel,
	method xui.WidthMethod,
) {
	w, h := s.Size.Width, s.Size.Height
	if w < 2 || h < 2 {
		return
	}
	g := borderGlyphs(style)
	bs := borderStyle

	put := func(x, y int, ch string, st xui.Style) {
		s.SetCell(x, y, xui.Cell{Char: ch, Width: 1, Style: st})
	}

	// Corners + edges
	put(0, 0, g.tl, bs)
	put(w-1, 0, g.tr, bs)
	put(0, h-1, g.bl, bs)
	put(w-1, h-1, g.br, bs)
	for x := 1; x < w-1; x++ {
		put(x, 0, g.h, bs)
		put(x, h-1, g.h, bs)
	}
	for y := 1; y < h-1; y++ {
		put(0, y, g.v, bs)
		put(w-1, y, g.v, bs)
	}

	embed := func(y int, left, right *BorderLabel) {
		avail := w - 2 // between corners
		if avail < 1 {
			return
		}
		leftW, rightW := 0, 0
		if left != nil && left.Text != "" {
			leftW = xui.StringWidth(left.Text, method)
		}
		if right != nil && right.Text != "" {
			rightW = xui.StringWidth(right.Text, method)
		}
		// Prefer right label if they collide.
		if leftW+rightW > avail {
			if rightW >= avail {
				rightW = avail
				leftW = 0
			} else {
				leftW = avail - rightW
			}
		}
		if left != nil && leftW > 0 {
			text := TruncateToWidth(left.Text, leftW, method)
			s.Print(1, y, text, left.Style, method)
		}
		if right != nil && rightW > 0 {
			text := TruncateToWidth(right.Text, rightW, method)
			tw := xui.StringWidth(text, method)
			x := w - 1 - tw
			x = max(x, 1)
			s.Print(x, y, text, right.Style, method)
		}
	}
	embed(0, topLeft, topRight)
	embed(h-1, bottomLeft, bottomRight)
}

// TruncateToWidth returns the longest prefix of s that fits within max columns.
func TruncateToWidth(s string, maxInt int, method xui.WidthMethod) string {
	if maxInt <= 0 {
		return ""
	}
	if xui.StringWidth(s, method) <= maxInt {
		return s
	}
	var b strings.Builder
	w := 0
	rest := s
	for rest != "" {
		cluster, cw, next := xui.FirstGrapheme(rest, method)
		rest = next
		if cw < 1 {
			cw = 1
		}
		if w+cw > maxInt {
			break
		}
		b.WriteString(cluster)
		w += cw
	}
	return b.String()
}

// EdgeInsets (padding/margin).
type EdgeInsets struct {
	Top, Right, Bottom, Left int
}

// InsetsAll returns insets with v on all four sides.
func InsetsAll(v int) EdgeInsets { return EdgeInsets{v, v, v, v} }

// InsetsSymmetric returns insets with h on the left/right and v on the top/bottom.
func InsetsSymmetric(h, v int) EdgeInsets { return EdgeInsets{v, h, v, h} }

// InsetsHorizontal returns insets with h on the left and right sides.
func InsetsHorizontal(h int) EdgeInsets { return EdgeInsets{0, h, 0, h} }

// InsetsVertical returns insets with v on the top and bottom sides.
func InsetsVertical(v int) EdgeInsets { return EdgeInsets{v, 0, v, 0} }

// InsetsOnly returns insets with explicit per-side values.
func InsetsOnly(top, right, bottom, left int) EdgeInsets {
	return EdgeInsets{top, right, bottom, left}
}

// Horizontal returns the combined left and right padding.
func (e EdgeInsets) Horizontal() int { return e.Left + e.Right }

// Vertical returns the combined top and bottom padding.
func (e EdgeInsets) Vertical() int { return e.Top + e.Bottom }

// Padding wraps a child with insets.
type Padding struct {
	Insets EdgeInsets
	Child  components.Widget
}

// Handle forwards the event to the child.
func (p *Padding) Handle(ctx *components.EventContext, ev xui.Event) {
	if p.Child != nil {
		p.Child.Handle(ctx, ev)
	}
}

// Draw renders the child offset by the padding insets.
func (p *Padding) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	innerW := maxW - p.Insets.Horizontal()
	innerH := maxH - p.Insets.Vertical()
	if maxW <= 0 || innerW < 0 {
		innerW = 0
	}
	if maxH <= 0 || innerH < 0 {
		innerH = 0
	}
	var child components.Surface
	if p.Child != nil {
		cmax := components.Size{Width: innerW, Height: innerH}
		if maxH <= 0 {
			cmax.Height = 10000
		}
		child = p.Child.Draw(ctx.WithConstraints(components.Size{}, cmax))
	}
	w := child.Size.Width + p.Insets.Horizontal()
	h := child.Size.Height + p.Insets.Vertical()
	if maxW > 0 && w > maxW {
		w = maxW
	}
	if maxH > 0 && h > maxH {
		h = maxH
	}
	s := components.Surface{Size: components.Size{Width: w, Height: h}, Widget: p}
	if p.Child != nil {
		s.Children = []components.SubSurface{{
			Origin:  components.Point{X: p.Insets.Left, Y: p.Insets.Top},
			Surface: child,
		}}
	}
	return s
}

// SizedBox forces exact or minimum dimensions.
type SizedBox struct {
	Width, Height int // 0 = use child / unconstrained in that axis
	Child         components.Widget
}

// Handle forwards the event to the child.
func (s *SizedBox) Handle(ctx *components.EventContext, ev xui.Event) {
	if s.Child != nil {
		s.Child.Handle(ctx, ev)
	}
}

// Draw renders the child constrained to the box's fixed size.
func (s *SizedBox) Draw(ctx components.DrawContext) components.Surface {
	cw, ch := s.Width, s.Height
	if cw <= 0 {
		cw = ctx.Max.Width
	}
	if ch <= 0 {
		ch = ctx.Max.Height
	}
	if cw <= 0 {
		cw = 1
	}
	if ch <= 0 {
		ch = 1
	}
	out := components.Surface{Size: components.Size{Width: cw, Height: ch}, Widget: s}
	if s.Child != nil {
		child := s.Child.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: cw, Height: ch}))
		out.Children = []components.SubSurface{{Origin: components.Point{}, Surface: child}}
	}
	return out
}

// Spacer takes remaining space in a flex layout. Prefer inside FlexRow/Column via Flexible.
type Spacer struct {
	Width, Height int // fixed spacer size; if both 0, draws 1×1 empty
}

// Handle is a no-op; Spacer is not interactive.
func (*Spacer) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders an empty surface of the spacer's size.
func (s *Spacer) Draw(_ components.DrawContext) components.Surface {
	w, h := s.Width, s.Height
	if w <= 0 && h <= 0 {
		w, h = 1, 1
	}
	if w <= 0 {
		w = 1
	}
	if h <= 0 {
		h = 1
	}
	return components.Surface{Size: components.Size{Width: w, Height: h}, Widget: s}
}

// Flexible marks a flex child with a weight.
// Place as a direct child of FlexRow/FlexColumn.
type Flexible struct {
	Flex  int // weight; 0 means intrinsic size only
	Tight bool
	Child components.Widget
}

// Handle forwards the event to the child.
func (f *Flexible) Handle(ctx *components.EventContext, ev xui.Event) {
	if f.Child != nil {
		f.Child.Handle(ctx, ev)
	}
}

// Draw renders the child at its intrinsic size.
func (f *Flexible) Draw(ctx components.DrawContext) components.Surface {
	if f.Child == nil {
		return components.Surface{Size: components.Size{Width: 0, Height: 0}, Widget: f}
	}
	return f.Child.Draw(ctx)
}

// Divider is a horizontal rule.
type Divider struct {
	Style xui.Style
	Char  string // default "─"
}

// Handle is a no-op; Divider is not interactive.
func (*Divider) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders a horizontal rule across the available width.
func (d *Divider) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	ch := d.Char
	if ch == "" {
		ch = "─"
	}
	st := d.Style
	if st == (xui.Style{}) {
		st = components.DefaultTheme().Border
	}
	s := components.NewSurface(w, 1, d)
	for x := 0; x < w; x++ {
		s.SetCell(x, 0, xui.Cell{Char: ch, Width: 1, Style: st})
	}
	return s
}

// Stack overlays children; later children paint above.
type Stack struct {
	Children []components.Widget
	// Width/Height force stack size; 0 = max of children / constraints.
	Width, Height int
}

// Handle forwards the event to children topmost-first, stopping when one consumes it.
func (s *Stack) Handle(ctx *components.EventContext, ev xui.Event) {
	for i := range slices.Backward(s.Children) {
		s.Children[i].Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
}

// Draw overlays all children within the stack bounds.
func (s *Stack) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if s.Width > 0 {
		maxW = s.Width
	}
	if s.Height > 0 {
		maxH = s.Height
	}
	if maxW <= 0 {
		maxW = 40
	}
	if maxH <= 0 {
		maxH = 10
	}
	out := components.Surface{Size: components.Size{Width: maxW, Height: maxH}, Widget: s}
	for i, ch := range s.Children {
		var origin components.Point
		var child components.Surface
		if p, ok := ch.(*Positioned); ok {
			cw := maxW
			chh := maxH
			if p.Width > 0 {
				cw = p.Width
			}
			if p.Height > 0 {
				chh = p.Height
			}
			child = p.Child.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: cw, Height: chh}))
			origin = resolvePositioned(p, maxW, maxH, child.Size)
		} else {
			child = ch.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxW, Height: maxH}))
		}
		out.Children = append(out.Children, components.SubSurface{Origin: origin, Z: i, Surface: child})
	}
	return out
}

// Positioned places a child inside a Stack.
type Positioned struct {
	Left, Top, Right, Bottom *int
	Width, Height            int
	Child                    components.Widget
}

// Handle forwards the event to the child.
func (p *Positioned) Handle(ctx *components.EventContext, ev xui.Event) {
	if p.Child != nil {
		p.Child.Handle(ctx, ev)
	}
}

// Draw renders the child; Stack resolves the final position.
func (p *Positioned) Draw(ctx components.DrawContext) components.Surface {
	if p.Child == nil {
		return components.Surface{Widget: p}
	}
	return p.Child.Draw(ctx)
}

func resolvePositioned(p *Positioned, sw, sh int, cs components.Size) components.Point {
	x, y := 0, 0
	if p.Left != nil {
		x = *p.Left
	} else if p.Right != nil {
		x = sw - cs.Width - *p.Right
	}
	if p.Top != nil {
		y = *p.Top
	} else if p.Bottom != nil {
		y = sh - cs.Height - *p.Bottom
	}
	return components.Point{X: x, Y: y}
}

// Clickable wraps a child with mouse/key activation.
type Clickable struct {
	Child   components.Widget
	OnClick func()
}

// Handle triggers OnClick on Enter/Space or a left click, otherwise forwards the event to the child.
func (c *Clickable) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			if c.OnClick != nil {
				c.OnClick()
			}
			ctx.ConsumeAndRedraw()
			return
		}
	case xui.MouseEvent:
		if e.Button == xui.MouseLeft && e.Action == xui.MousePress {
			if c.OnClick != nil {
				c.OnClick()
			}
			ctx.ConsumeAndRedraw()
			return
		}
	}
	if c.Child != nil {
		c.Child.Handle(ctx, ev)
	}
}

// Draw renders the child and re-tags its surface so hit tests land on the Clickable.
func (c *Clickable) Draw(ctx components.DrawContext) components.Surface {
	if c.Child == nil {
		return components.Surface{Size: components.Size{Width: 1, Height: 1}, Widget: c}
	}
	child := c.Child.Draw(ctx)
	// Re-tag so hit testing lands on Clickable.
	child.Widget = c
	return child
}

// BoxDecoration holds a background fill and optional border styles.
type BoxDecoration struct {
	Background xui.Style // uses Bg primarily; Fg ignored for fill
	Border     xui.Style
	BorderKind BorderStyle
	Bordered   bool
}

// Container lays out a single child with optional size, padding, and decoration.
type Container struct {
	Width, Height int // 0 = hug child / fill max
	Padding       EdgeInsets
	Decoration    BoxDecoration
	Child         components.Widget

	// Labels embedded in border when Bordered.
	TopLeft, TopRight, BottomLeft, BottomRight BorderLabel
}

// Handle forwards the event to the child.
func (c *Container) Handle(ctx *components.EventContext, ev xui.Event) {
	if c.Child != nil {
		c.Child.Handle(ctx, ev)
	}
}

// Draw renders the child with padding, background fill, and optional border.
func (c *Container) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	border := 0
	if c.Decoration.Bordered {
		border = 1
	}
	pad := c.Padding
	innerMaxW := maxW - pad.Horizontal() - border*2
	innerMaxH := maxH - pad.Vertical() - border*2
	if maxW <= 0 || innerMaxW < 0 {
		innerMaxW = 0
	}
	if maxH <= 0 {
		innerMaxH = 10000
	} else if innerMaxH < 0 {
		innerMaxH = 0
	}

	var child components.Surface
	if c.Child != nil {
		child = c.Child.Draw(
			ctx.WithConstraints(components.Size{}, components.Size{Width: innerMaxW, Height: innerMaxH}),
		)
	}

	contentW := child.Size.Width + pad.Horizontal() + border*2
	contentH := child.Size.Height + pad.Vertical() + border*2
	w, h := contentW, contentH
	if c.Width > 0 {
		w = c.Width
	} else if maxW > 0 {
		w = maxW
	}
	// Height: >0 explicit, <0 fill max, 0 hug content.
	if c.Height > 0 {
		h = c.Height
	} else if maxH > 0 && c.Height < 0 {
		h = maxH
	}
	if h < 1 {
		h = contentH
		h = max(h, 1)
	}
	if w < 1 {
		w = 1
	}

	s := components.NewSurface(w, h, c)
	// Fill background
	bg := c.Decoration.Background
	if bg.Bg.Kind != 0 || bg.Fg.Kind != 0 {
		fill := xui.Cell{Char: " ", Width: 1, Style: bg}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				s.SetCell(x, y, fill)
			}
		}
	}
	if c.Decoration.Bordered {
		var tl, tr, bl, br *BorderLabel
		if c.TopLeft.Text != "" {
			tl = &c.TopLeft
		}
		if c.TopRight.Text != "" {
			tr = &c.TopRight
		}
		if c.BottomLeft.Text != "" {
			bl = &c.BottomLeft
		}
		if c.BottomRight.Text != "" {
			br = &c.BottomRight
		}
		bs := c.Decoration.Border
		if bs == (xui.Style{}) {
			bs = components.DefaultTheme().Border
		}
		DrawRoundedBorder(&s, c.Decoration.BorderKind, bs, tl, tr, bl, br, ctx.Method)
	}
	if c.Child != nil {
		ox := border + pad.Left
		oy := border + pad.Top
		s.Children = []components.SubSurface{{Origin: components.Point{X: ox, Y: oy}, Surface: child}}
	}
	return s
}

// Box is a bordered panel shortcut around a child.
func Box(child components.Widget, border xui.Style) *Container {
	return &Container{
		Padding: InsetsAll(1),
		Decoration: BoxDecoration{
			Bordered:   true,
			Border:     border,
			BorderKind: BorderRounded,
		},
		Child: child,
	}
}

// Text renders a static string.
type Text struct {
	Content string
	Style   xui.Style
}

// Handle is a no-op; Text is not interactive.
func (*Text) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw renders the static content string within the constraints.
func (t *Text) Draw(ctx components.DrawContext) components.Surface {
	w := xui.StringWidth(t.Content, ctx.Method)
	h := 1
	if w < ctx.Min.Width {
		w = ctx.Min.Width
	}
	if ctx.Max.Width > 0 && w > ctx.Max.Width {
		w = ctx.Max.Width
	}
	if ctx.Max.Height > 0 && h > ctx.Max.Height {
		h = ctx.Max.Height
	}
	s := components.NewSurface(w, h, t)
	s.Print(0, 0, t.Content, t.Style, ctx.Method)
	return s
}

// Button is a clickable labeled control.
type Button struct {
	Label   string
	Style   xui.Style
	Hot     xui.Style // hover/focus style
	focused bool
	hover   bool
	OnClick func()
}

// Widget returns the button itself as a components.Widget.
func (b *Button) Widget() components.Widget { return b }

// Handle triggers OnClick on Enter/Space or a left click.
func (b *Button) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if e.Code == xui.KeyEnter || (e.Code == xui.KeyRune && e.Rune == ' ') {
			if b.OnClick != nil {
				b.OnClick()
			}
			ctx.ConsumeAndRedraw()
		}
	case xui.MouseEvent:
		if e.Button == xui.MouseLeft && e.Action == xui.MousePress {
			if b.OnClick != nil {
				b.OnClick()
			}
			ctx.ConsumeAndRedraw()
		}
	}
}

// Draw renders the label with padding, applying the hot style when focused or hovered.
func (b *Button) Draw(ctx components.DrawContext) components.Surface {
	label := b.Label
	if label == "" {
		label = " "
	}
	w := xui.StringWidth(label, ctx.Method) + 2
	h := 1
	if w < ctx.Min.Width {
		w = ctx.Min.Width
	}
	if ctx.Max.Width > 0 && w > ctx.Max.Width {
		w = ctx.Max.Width
	}
	style := b.Style
	if b.focused || b.hover {
		if b.Hot != (xui.Style{}) {
			style = b.Hot
		} else {
			style.Reverse = true
		}
	}
	s := components.NewSurface(w, h, b)
	s.Print(1, 0, label, style, ctx.Method)
	// pad edges
	s.SetCell(0, 0, xui.Cell{Char: " ", Width: 1, Style: style})
	if w > 1 {
		s.SetCell(w-1, 0, xui.Cell{Char: " ", Width: 1, Style: style})
	}
	return s
}

// SetFocused updates focus visuals (called by App).
func (b *Button) SetFocused(v bool) { b.focused = v }

// SetHover updates hover visuals.
func (b *Button) SetHover(v bool) { b.hover = v }

// Center centers a single child within max constraints.
type Center struct {
	Child components.Widget
}

// Handle forwards the event to the child.
func (c *Center) Handle(ctx *components.EventContext, ev xui.Event) {
	if c.Child != nil {
		c.Child.Handle(ctx, ev)
	}
}

// Draw centers the child within the maximum constraints.
func (c *Center) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 24
	}
	s := components.Surface{Size: components.Size{Width: maxW, Height: maxH}, Widget: c}
	if c.Child == nil {
		return s
	}
	childCtx := ctx.WithConstraints(components.Size{}, components.Size{Width: maxW, Height: maxH})
	child := c.Child.Draw(childCtx)
	ox := (maxW - child.Size.Width) / 2
	oy := (maxH - child.Size.Height) / 2
	if ox < 0 {
		ox = 0
	}
	if oy < 0 {
		oy = 0
	}
	s.Children = []components.SubSurface{{Origin: components.Point{X: ox, Y: oy}, Surface: child}}
	return s
}

// FlexRow lays out children horizontally.
type FlexRow struct {
	Children []components.Widget
	Gap      int
}

// Handle forwards the event to children, stopping when one consumes it.
func (f *FlexRow) Handle(ctx *components.EventContext, ev xui.Event) {
	for _, ch := range f.Children {
		ch.Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
}

// Draw lays out children horizontally with the configured gap.
func (f *FlexRow) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 1
	}
	return flexLayout(f, f.Children, f.Gap, maxW, maxH, flexAxisRow, ctx)
}

// FlexColumn lays out children vertically.
type FlexColumn struct {
	Children []components.Widget
	Gap      int
}

// Handle forwards the event to children, stopping when one consumes it.
func (f *FlexColumn) Handle(ctx *components.EventContext, ev xui.Event) {
	for _, ch := range f.Children {
		ch.Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
}

// Draw lays out children vertically with the configured gap.
func (f *FlexColumn) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 24
	}
	return flexLayout(f, f.Children, f.Gap, maxW, maxH, flexAxisColumn, ctx)
}

// flexAxis selects the main axis of a flex layout: row (X) or column (Y).
type flexAxis int

const (
	flexAxisRow flexAxis = iota
	flexAxisColumn
)

// flexLayout is the shared engine behind FlexRow and FlexColumn. The axis
// parameter selects the main axis: flexAxisRow (X) or flexAxisColumn (Y).
func flexLayout(
	parent components.Widget,
	children []components.Widget,
	gap, maxW, maxH int,
	ax flexAxis,
	ctx components.DrawContext,
) components.Surface {
	s := components.Surface{Size: components.Size{Width: maxW, Height: maxH}, Widget: parent}

	main := func(size components.Size) int {
		if ax == flexAxisColumn {
			return size.Height
		}
		return size.Width
	}

	type slot struct {
		w     components.Widget
		flex  int
		surf  components.Surface
		fixed int
	}
	slots := make([]slot, len(children))
	fixedTotal, flexSum := 0, 0
	for i, ch := range children {
		slots[i].w = ch
		if fl, ok := ch.(*Flexible); ok && fl.Flex > 0 {
			slots[i].flex = fl.Flex
			flexSum += fl.Flex
			continue
		}
		child := ch.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxW, Height: maxH}))
		slots[i].surf = child
		slots[i].fixed = main(child.Size)
		fixedTotal += slots[i].fixed
		if i > 0 {
			fixedTotal += gap
		}
	}

	mainMax := maxW
	if ax == flexAxisColumn {
		mainMax = maxH
	}
	remain := mainMax - fixedTotal
	remain = max(remain, 0)

	pos := 0
	for i, sl := range slots {
		if i > 0 {
			pos += gap
		}
		origin := components.Point{X: pos, Y: 0}
		if ax == flexAxisColumn {
			origin = components.Point{X: 0, Y: pos}
		}
		if sl.flex > 0 {
			share := remain
			if flexSum > 0 {
				share = remain * sl.flex / flexSum
			}
			var child components.Surface
			if ax == flexAxisColumn {
				child = sl.w.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: maxW, Height: share}))
			} else {
				child = sl.w.Draw(ctx.WithConstraints(components.Size{}, components.Size{Width: share, Height: maxH}))
			}
			s.Children = append(s.Children, components.SubSurface{Origin: origin, Surface: child})
			pos += share
		} else {
			s.Children = append(s.Children, components.SubSurface{Origin: origin, Surface: sl.surf})
			pos += sl.fixed
		}
	}
	return s
}
