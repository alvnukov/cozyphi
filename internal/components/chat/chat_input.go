package chat

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/text"
	"github.com/pulseaiclub/phi/internal/debuglog"
)

// ChatInput is a composer: rounded border, edge labels, multiline editor.
//
// Layout (minBodyRows=3 → total height 5; +1 when PendingSkills set):
//
//	╭────────────────────────────── model-name───────╮
//	│ Skills: building-plugins                       │
//	│█                                               │
//	│                                                │
//	╰─ ↑1.2k ↓800 C900 Σ2.0k 5% of 128k ── ~/path ───╯
type ChatInput struct {
	// Value is the current editor text (may contain newlines).
	Value string
	// Cursor is a byte offset into Value.
	Cursor int

	MinBodyRows int // default 3
	MaxBodyRows int // default 12; height grows with content up to this

	TopLeftLabel     layout.BorderLabel
	TopRightLabel    layout.BorderLabel
	BottomLeftLabel  layout.BorderLabel
	BottomRightLabel layout.BorderLabel

	BorderStyle    xui.Style
	TextStyle      xui.Style
	CursorStyle    xui.Style // visual block when terminal cursor unavailable
	UseBlockCursor bool      // paint reverse cell in addition to terminal cursor

	// Theme styles the pending-skills chip row (Muted label, Success names).
	Theme components.Theme

	// PendingSkills are skill names shown inside the bordered editor as the
	// first content row: "Skills: name1 name2".
	PendingSkills []string

	PaddingX int // horizontal inner padding; default 1

	// OnSubmit is called when Enter is pressed (without modifiers).
	OnSubmit func(text string)
	// OnChange is called after Value mutates.
	OnChange func(text string)
	// OnPendingSkillsChange is called after PendingSkills mutates.
	OnPendingSkillsChange func(skills []string)
	// OnMentionChange is called after Value or Cursor changes that may
	// activate/deactivate an @-file mention. active is false when none.
	OnMentionChange func(active bool, query string)
	// OnSlashChange is called after Value or Cursor changes that may
	// activate/deactivate a leading /command. active is false when none.
	OnSlashChange func(active bool, query string)

	// MentionOpen is set by the editor while the @-file picker is visible.
	// When true, Up/Down/Tab/Enter are left unconsumed so the picker can
	// handle navigation (focus stays on the composer for typing).
	MentionOpen bool
	// SlashOpen is set while the /command picker is visible (same nav deferral).
	SlashOpen bool

	// dumpNextDraw is set on paste/insert when PHI_DEBUG=1.
	dumpNextDraw bool
}

func (c *ChatInput) completerOpen() bool {
	return c.MentionOpen || c.SlashOpen
}

func (c *ChatInput) bodyRows(width int, method xui.WidthMethod) int {
	minR := c.MinBodyRows
	if minR < 1 {
		minR = 3
	}
	maxR := c.MaxBodyRows
	if maxR < 1 {
		maxR = 12
	}
	if maxR < minR {
		maxR = minR
	}
	pad := c.padX()
	innerW := width - 2 - pad*2
	innerW = max(innerW, 1)
	n := len(text.WrapEditorLines(c.Value, innerW, method))
	n = max(n, 1)
	n = max(n, minR)
	n = min(n, maxR)
	return n
}

// PreferredHeight returns total height (optional skills row + body + borders),
// growing with content up to MaxBodyRows so the composer cannot expand forever.
func (c *ChatInput) PreferredHeight(width int, method xui.WidthMethod) int {
	return c.pendingSkillsHeight() + c.bodyRows(width, method) + 2
}

func (c *ChatInput) pendingSkillsHeight() int {
	if len(c.PendingSkills) == 0 {
		return 0
	}
	return 1
}

// AddPendingSkill appends name if not already pending.
func (c *ChatInput) AddPendingSkill(name string) {
	name = strings.TrimSpace(name)
	if name == "" {
		return
	}
	if slices.Contains(c.PendingSkills, name) {
		return
	}
	c.PendingSkills = append(c.PendingSkills, name)
	c.notifyPendingSkills()
}

// PopPendingSkill removes the last pending skill. Returns false if none.
func (c *ChatInput) PopPendingSkill() bool {
	if len(c.PendingSkills) == 0 {
		return false
	}
	c.PendingSkills = c.PendingSkills[:len(c.PendingSkills)-1]
	c.notifyPendingSkills()
	return true
}

// ClearPendingSkills removes all pending skills.
func (c *ChatInput) ClearPendingSkills() {
	if len(c.PendingSkills) == 0 {
		return
	}
	c.PendingSkills = nil
	c.notifyPendingSkills()
}

func (c *ChatInput) notifyPendingSkills() {
	if c.OnPendingSkillsChange != nil {
		c.OnPendingSkillsChange(c.PendingSkills)
	}
}

func (c *ChatInput) padX() int {
	if c.PaddingX <= 0 {
		return 1
	}
	return c.PaddingX
}

func (c *ChatInput) clampCursor() {
	if c.Cursor < 0 {
		c.Cursor = 0
	}
	if c.Cursor > len(c.Value) {
		c.Cursor = len(c.Value)
	}
}

// Handle edits the composer value: typing, navigation, submit on Enter,
// and pending-skill backspace removal.
func (c *ChatInput) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		if !e.Press {
			return
		}
		c.clampCursor()
		switch e.Code {
		case xui.KeyEnter:
			if c.completerOpen() {
				// Let the @-file / slash picker accept / consume Enter.
				return
			}
			// Shift+Enter / Alt+Enter insert newline; bare Enter submits.
			if e.Mods.Has(xui.ModShift) || e.Mods.Has(xui.ModAlt) || e.Mods.Has(xui.ModCtrl) {
				c.insert("\n")
				ctx.ConsumeAndRedraw()
				return
			}
			if c.OnSubmit != nil {
				c.OnSubmit(c.Value)
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyBackspace:
			if c.Cursor > 0 {
				_, size := utf8.DecodeLastRuneInString(c.Value[:c.Cursor])
				deleted := c.Value[c.Cursor-size : c.Cursor]
				c.Value = c.Value[:c.Cursor-size] + c.Value[c.Cursor:]
				c.Cursor -= size
				c.notifyChange()
				debuglog.Logf("chat backspace deleted=%q cursor=%d value_len=%d", deleted, c.Cursor, len(c.Value))
				if debuglog.Enabled() {
					c.dumpNextDraw = true
				}
			} else if c.PopPendingSkill() {
				debuglog.Logf("chat backspace popped pending skill remaining=%d", len(c.PendingSkills))
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyDelete:
			if c.Cursor < len(c.Value) {
				_, size := utf8.DecodeRuneInString(c.Value[c.Cursor:])
				deleted := c.Value[c.Cursor : c.Cursor+size]
				c.Value = c.Value[:c.Cursor] + c.Value[c.Cursor+size:]
				c.notifyChange()
				debuglog.Logf("chat delete deleted=%q cursor=%d value_len=%d", deleted, c.Cursor, len(c.Value))
				if debuglog.Enabled() {
					c.dumpNextDraw = true
				}
			}
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyLeft:
			if c.Cursor > 0 {
				_, size := utf8.DecodeLastRuneInString(c.Value[:c.Cursor])
				c.Cursor -= size
			}
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyRight:
			if c.Cursor < len(c.Value) {
				_, size := utf8.DecodeRuneInString(c.Value[c.Cursor:])
				c.Cursor += size
			}
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyHome:
			c.Cursor = lineStart(c.Value, c.Cursor)
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyEnd:
			c.Cursor = lineEnd(c.Value, c.Cursor)
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyUp:
			if c.completerOpen() {
				return
			}
			c.moveVert(-1)
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyDown:
			if c.completerOpen() {
				return
			}
			c.moveVert(1)
			c.notifyCompleters()
			ctx.ConsumeAndRedraw()
			return
		case xui.KeyTab:
			if c.completerOpen() {
				return
			}
			return
		case xui.KeyRune:
			if e.Mods.Has(xui.ModCtrl) || e.Mods.Has(xui.ModAlt) || e.Mods.Has(xui.ModSuper) {
				return
			}
			if e.Rune >= 0x20 || e.Rune == '\t' {
				c.insert(string(e.Rune))
				ctx.ConsumeAndRedraw()
			}
			return
		}
	case xui.PasteEvent:
		debuglog.Logf("chat paste raw bytes=%d", len(e.Text))
		debuglog.DumpRunes("chat paste raw", e.Text)
		c.insert(e.Text)
		debuglog.DumpRunes("chat value after paste", c.Value)
		debuglog.Logf("chat cursor=%d", c.Cursor)
		ctx.ConsumeAndRedraw()
	}
}

func (c *ChatInput) insert(s string) {
	before := s
	s = sanitizeComposerText(s)
	if s != before {
		debuglog.Logf("chat sanitize changed text bytes %d -> %d", len(before), len(s))
	}
	if s == "" {
		return
	}
	c.clampCursor()
	c.Value = c.Value[:c.Cursor] + s + c.Value[c.Cursor:]
	c.Cursor += len(s)
	if debuglog.Enabled() {
		c.dumpNextDraw = true
	}
	c.notifyChange()
}

// sanitizeComposerText keeps the composer free of terminal-breaking controls.
// Tabs become spaces; other C0 controls (except newline) are dropped. Raw tabs
// painted into the tty expand to tab-stops and desync the cell renderer.
// Block-element UI chrome (e.g. ▎ from transcript selection) is also stripped —
// those glyphs can disagree with tty ambiguous-width and shift the caret.
func sanitizeComposerText(s string) string {
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	if !strings.ContainsFunc(s, func(r rune) bool {
		return r == '\t' || (r < 0x20 && r != '\n') || r == 0x7f || isComposerChrome(r)
	}) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n':
			_ = b.WriteByte('\n')
		case r == '\t':
			_, _ = b.WriteString("    ")
		case r < 0x20, r == 0x7f:
			// drop
		case isComposerChrome(r):
			// drop transcript chrome
		default:
			_, _ = b.WriteRune(r)
		}
	}
	return b.String()
}

func isComposerChrome(r rune) bool {
	switch r {
	case '▎', '▌', '┃':
		return true
	default:
		return false
	}
}

func (c *ChatInput) notifyChange() {
	if c.OnChange != nil {
		c.OnChange(c.Value)
	}
	c.notifyCompleters()
}

func (c *ChatInput) notifyCompleters() {
	c.notifyMention()
	c.notifySlash()
}

func (c *ChatInput) notifyMention() {
	if c.OnMentionChange == nil {
		return
	}
	q, _, _, ok := ActiveMention(c.Value, c.Cursor)
	c.OnMentionChange(ok, q)
}

func (c *ChatInput) notifySlash() {
	if c.OnSlashChange == nil {
		return
	}
	q, _, _, ok := ActiveSlash(c.Value, c.Cursor)
	c.OnSlashChange(ok, q)
}

// ReplaceRange replaces value[start:end] with text and places the cursor after it.
func (c *ChatInput) ReplaceRange(start, end int, text string) {
	if start < 0 {
		start = 0
	}
	if end > len(c.Value) {
		end = len(c.Value)
	}
	if start > end {
		start, end = end, start
	}
	text = sanitizeComposerText(text)
	c.Value = c.Value[:start] + text + c.Value[end:]
	c.Cursor = start + len(text)
	c.notifyChange()
}

func (c *ChatInput) moveVert(delta int) {
	start := lineStart(c.Value, c.Cursor)
	col := utf8.RuneCountInString(c.Value[start:c.Cursor])
	if delta < 0 {
		if start == 0 {
			c.Cursor = 0
			return
		}
		prevEnd := start - 1 // newline
		prevStart := lineStart(c.Value, prevEnd)
		c.Cursor = runeIndex(c.Value[prevStart:prevEnd], col) + prevStart
		return
	}
	end := lineEnd(c.Value, c.Cursor)
	if end >= len(c.Value) {
		c.Cursor = len(c.Value)
		return
	}
	nextStart := end + 1
	nextEnd := lineEnd(c.Value, nextStart)
	c.Cursor = runeIndex(c.Value[nextStart:nextEnd], col) + nextStart
}

// Draw renders the bordered composer with edge labels, skills row, editor
// text, and the block/terminal cursor.
func (c *ChatInput) Draw(ctx components.DrawContext) components.Surface {
	w := ctx.Max.Width
	if w <= 0 {
		w = 40
	}
	pendingH := c.pendingSkillsHeight()
	editorRows := c.bodyRows(w, ctx.Method)
	body := pendingH + editorRows // inner rows inside the border
	h := body + 2                 // + borders
	if ctx.Max.Height > 0 && h > ctx.Max.Height {
		h = ctx.Max.Height
		body = h - 2
		if body < 1+pendingH {
			body = 1 + pendingH
			h = body + 2
		}
		editorRows = body - pendingH
		if editorRows < 1 {
			editorRows = 1
			body = pendingH + editorRows
			h = body + 2
		}
	}

	borderSt := c.BorderStyle
	if borderSt == (xui.Style{}) {
		borderSt = xui.Style{Fg: xui.IndexedColor(240)}
	}
	textSt := c.TextStyle
	if textSt == (xui.Style{}) {
		textSt = xui.Style{Fg: xui.DefaultColor()}
	}
	cursorSt := c.CursorStyle
	if cursorSt == (xui.Style{}) {
		cursorSt = xui.Style{Reverse: true}
	}

	s := components.NewSurface(w, h, c)
	var tl, tr, bl, br *layout.BorderLabel
	if c.TopLeftLabel.Text != "" {
		tl = &c.TopLeftLabel
	}
	if c.TopRightLabel.Text != "" {
		tr = &c.TopRightLabel
	}
	if c.BottomLeftLabel.Text != "" {
		bl = &c.BottomLeftLabel
	}
	if c.BottomRightLabel.Text != "" {
		br = &c.BottomRightLabel
	}
	layout.DrawRoundedBorder(&s, layout.BorderRounded, borderSt, tl, tr, bl, br, ctx.Method)

	pad := c.padX()
	innerW := w - 2 - pad*2
	innerW = max(innerW, 1)

	// Fill body background (non-default spaces so Clear+diff works cleanly).
	for y := 1; y <= body && y < h-1; y++ {
		for x := 1; x < w-1; x++ {
			s.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: textSt})
		}
	}

	if pendingH > 0 {
		c.paintPendingSkills(&s, 1+pad, 1, innerW, ctx.Method)
	}

	lines := text.WrapEditorLines(c.Value, innerW, ctx.Method)
	// Scroll so cursor line is visible within the editor region.
	curLine, curCol := text.CursorLineCol(c.Value, c.Cursor, innerW, ctx.Method)
	scroll := 0
	if curLine >= editorRows {
		scroll = curLine - editorRows + 1
	}
	editorTop := 1 + pendingH
	for i := 0; i < editorRows; i++ {
		li := i + scroll
		if li < 0 || li >= len(lines) {
			continue
		}
		s.Print(1+pad, editorTop+i, lines[li], textSt, ctx.Method)
	}

	// Cursor position in surface coords (editor region, below skills).
	visLine := curLine - scroll
	visLine = max(visLine, 0)
	if visLine >= editorRows {
		visLine = editorRows - 1
	}
	cx := 1 + pad + curCol
	cy := editorTop + visLine
	if cx >= w-1 {
		cx = w - 2
	}
	if cy >= h-1 {
		cy = h - 2
	}
	// Never place the block cursor on a wide-glyph continuation column —
	// that paints a Width=1 reverse space into the second half of a CJK cell
	// and shows up as phantom "cursor blocks".
	cx = text.SnapSurfaceColToGlyphStart(s.Buffer, w, cx, cy)

	if c.UseBlockCursor {
		existing := s.Buffer[cy*w+cx]
		ch := existing.Char
		width := existing.Width
		if ch == "" {
			ch = " "
		}
		if width < 1 {
			width = 1
		}
		// If this cell is somehow still a trail, don't reverse-paint it.
		if width == 1 && cx > 0 {
			prev := s.Buffer[cy*w+cx-1]
			if prev.Width > 1 {
				cx--
				existing = s.Buffer[cy*w+cx]
				ch = existing.Char
				width = existing.Width
				if ch == "" {
					ch = " "
				}
				if width < 1 {
					width = 1
				}
			}
		}
		s.SetCell(cx, cy, xui.Cell{Char: ch, Width: width, Style: cursorSt})
	}
	s.Cursor = &components.Point{X: cx, Y: cy}

	if debuglog.Enabled() && c.dumpNextDraw {
		c.dumpNextDraw = false
		debuglog.Logf(
			"chat draw w=%d innerW=%d body=%d editorRows=%d pending=%d lines=%d curLine=%d curCol=%d scroll=%d cx=%d cy=%d",
			w,
			innerW,
			body,
			editorRows,
			pendingH,
			len(lines),
			curLine,
			curCol,
			scroll,
			cx,
			cy,
		)
		if visLine >= 0 && curLine-scroll < len(lines) && curLine-scroll >= 0 {
			li := curLine - scroll
			debuglog.Logf("chat draw focus line %q width=%d", lines[li], xui.StringWidth(lines[li], ctx.Method))
		}
		cell := s.Buffer[cy*w+cx]
		debuglog.Logf("chat cursor cell char=%q width=%d reverse=%v", cell.Char, cell.Width, cell.Style.Reverse)
		dumpSurfaceRow("chat row", s.Buffer, w, cy)
	}
	return s
}

func (c *ChatInput) paintPendingSkills(s *components.Surface, x, y, width int, method xui.WidthMethod) {
	th := c.Theme
	if th.Success.Fg.Kind == 0 && th.Foreground.Fg.Kind == 0 {
		th = components.DefaultTheme()
	}
	labelSt := th.Muted
	labelSt.Dim = true
	nameSt := th.Success
	nameSt.Bold = false
	nameSt.Underline = true

	spans := []components.Span{{Text: "Skills: ", Style: labelSt}}
	for i, name := range c.PendingSkills {
		if i > 0 {
			spans = append(spans, components.Span{Text: " ", Style: labelSt})
		}
		spans = append(spans, components.Span{Text: name, Style: nameSt})
	}
	lines := components.WrapSpans(spans, width, method)
	if len(lines) == 0 {
		return
	}
	components.PaintSpans(s, x, y, lines[0], method)
}

func lineStart(s string, off int) int {
	if off > len(s) {
		off = len(s)
	}
	i := strings.LastIndexByte(s[:off], '\n')
	if i < 0 {
		return 0
	}
	return i + 1
}

func lineEnd(s string, off int) int {
	if off > len(s) {
		off = len(s)
	}
	i := strings.IndexByte(s[off:], '\n')
	if i < 0 {
		return len(s)
	}
	return off + i
}

func runeIndex(s string, n int) int {
	if n <= 0 {
		return 0
	}
	i := 0
	for pos := range s {
		if i == n {
			return pos
		}
		i++
	}
	return len(s)
}

func dumpSurfaceRow(label string, buf []xui.Cell, rowW, row int) {
	if buf == nil || row < 0 || rowW < 1 {
		return
	}
	var b strings.Builder
	_, _ = b.WriteString(label)
	_ = b.WriteByte(':')
	for x := 0; x < rowW; {
		i := row*rowW + x
		if i >= len(buf) {
			break
		}
		c := buf[i]
		step := int(c.Width)
		step = max(step, 1)
		ch := c.Char
		if ch == "" || ch == " " {
			ch = "·"
		}
		_ = b.WriteByte(' ')
		_, _ = b.WriteString(ch)
		if c.Style.Reverse {
			_, _ = b.WriteString("!R")
		}
		if step > 1 {
			_ = b.WriteByte('x')
			_ = b.WriteByte(byte('0' + min(step, 9)))
		}
		x += step
	}
	debuglog.Logf("%s", b.String())
}
