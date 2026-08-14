package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/layout"
	"github.com/pulseaiclub/phi/internal/components/text"
)

func TestChatInputBorderLabels(t *testing.T) {
	c := &ChatInput{
		MinBodyRows: 3,
		TopRightLabel: layout.BorderLabel{
			Text:  "nostromo—1—skill",
			Style: xui.Style{Fg: xui.RGBColor(0x5f, 0xc2, 0xc2)},
		},
		BottomLeftLabel: layout.BorderLabel{
			Text:  "↑1.2k ↓800 Σ2.0k 5% of 128k",
			Style: xui.Style{Fg: xui.RGBColor(0x7d, 0xc3, 0xff)},
		},
		BottomRightLabel: layout.BorderLabel{
			Text:  "~/Desktop/../examples/hello",
			Style: xui.Style{Fg: xui.IndexedColor(250)},
		},
		UseBlockCursor: true,
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 10}})
	if s.Size.Width != 60 || s.Size.Height != 5 {
		t.Fatalf("size = %+v", s.Size)
	}
	// Corners
	if s.Buffer[0].Char != "╭" || s.Buffer[59].Char != "╮" {
		t.Fatalf("top corners %q %q", s.Buffer[0].Char, s.Buffer[59].Char)
	}
	bottom := 4 * 60
	if s.Buffer[bottom].Char != "╰" || s.Buffer[bottom+59].Char != "╯" {
		t.Fatalf("bottom corners")
	}
	top := rowString(s, 0)
	if !strings.Contains(top, "nostromo") {
		t.Fatalf("top row missing model: %q", top)
	}
	bot := rowString(s, 4)
	if !strings.Contains(bot, "5% of 128k") {
		t.Fatalf("bottom row missing context/token: %q", bot)
	}
	if !strings.Contains(bot, "examples") {
		t.Fatalf("bottom row missing path: %q", bot)
	}
	if s.Cursor == nil {
		t.Fatal("expected cursor")
	}
}

func TestChatInputTyping(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3}
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'h', Press: true})
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'i', Press: true})
	if c.Value != "hi" || c.Cursor != 2 {
		t.Fatalf("value=%q cursor=%d", c.Value, c.Cursor)
	}
	submitted := ""
	c.OnSubmit = func(s string) { submitted = s }
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	if submitted != "hi" {
		t.Fatalf("submit = %q", submitted)
	}
}

func TestChatInputMentionOpenDefersNav(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, Value: "@a\nb", Cursor: 2, MentionOpen: true}
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyDown, Press: true})
	if ctx.Consume {
		t.Fatal("Down should bubble when MentionOpen")
	}
	if c.Cursor != 2 {
		t.Fatalf("cursor should stay put, got %d", c.Cursor)
	}
	submitted := false
	c.OnSubmit = func(string) { submitted = true }
	ctx = &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	if ctx.Consume || submitted {
		t.Fatal("Enter should bubble to picker when MentionOpen")
	}
}

func TestChatInputNewlineModifiers(t *testing.T) {
	for _, mods := range []xui.Modifiers{xui.ModShift, xui.ModAlt, xui.ModCtrl} {
		c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 8}
		ctx := &components.EventContext{}
		c.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: 'a', Press: true})
		submitted := false
		c.OnSubmit = func(string) { submitted = true }
		c.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Mods: mods, Press: true})
		if submitted {
			t.Fatalf("mods=%v should insert newline, not submit", mods)
		}
		if c.Value != "a\n" {
			t.Fatalf("mods=%v value=%q", mods, c.Value)
		}
	}
}

func TestChatInputGrowsUntilMax(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 5, PaddingX: 1}
	method := xui.WidthUnicode
	w := 40
	if h := c.PreferredHeight(w, method); h != 5 {
		t.Fatalf("empty preferred height = %d, want 5", h)
	}
	c.Value = "one\ntwo\nthree\nfour"
	if h := c.PreferredHeight(w, method); h != 6 {
		t.Fatalf("4 lines preferred height = %d, want 6", h)
	}
	c.Value = "one\ntwo\nthree\nfour\nfive\nsix\nseven"
	if h := c.PreferredHeight(w, method); h != 7 {
		t.Fatalf("over max preferred height = %d, want 7 (max body 5 + borders)", h)
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: w, Height: 20}, Method: method})
	if s.Size.Height != 7 {
		t.Fatalf("draw height = %d, want 7", s.Size.Height)
	}
}

func TestChatInputPasteMultilineDoesNotSubmit(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 8}
	submitted := false
	c.OnSubmit = func(string) { submitted = true }
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.PasteEvent{Text: "a\nb\nc"})
	if submitted {
		t.Fatal("paste must not submit")
	}
	if c.Value != "a\nb\nc" {
		t.Fatalf("value=%q", c.Value)
	}
	if h := c.PreferredHeight(40, xui.WidthUnicode); h < 5 {
		t.Fatalf("expected grow after paste, height=%d", h)
	}
}

func TestChatInputCJKPasteNoContinuationReverse(t *testing.T) {
	c := &ChatInput{
		MinBodyRows:    3,
		MaxBodyRows:    8,
		PaddingX:       1,
		UseBlockCursor: true,
		CursorStyle:    xui.Style{Reverse: true},
	}
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.PasteEvent{Text: "已修复中文粘贴"})
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}, Method: xui.WidthUnicode})
	if s.Cursor == nil {
		t.Fatal("expected cursor")
	}
	cy := s.Cursor.Y
	revPrimaries := 0
	for x := 0; x < s.Size.Width; {
		cell := s.Buffer[cy*s.Size.Width+x]
		step := int(cell.Width)
		step = max(step, 1)
		if xui.StringWidth(cell.Char, xui.WidthUnicode) == 2 && cell.Width != 2 {
			t.Fatalf("CJK %q stored with width %d at col %d", cell.Char, cell.Width, x)
		}
		if cell.Style.Reverse {
			revPrimaries++
		}
		x += step
	}
	if revPrimaries != 1 {
		t.Fatalf("expected 1 reverse primary (block cursor), got %d", revPrimaries)
	}
}

func TestCursorLineColFullWidthWrap(t *testing.T) {
	// 5 CJK chars → width 10; cursor at end of exactly-full line must wrap.
	sample := "一二三四五"
	line, col := text.CursorLineCol(sample, len(sample), 10, xui.WidthUnicode)
	if line != 1 || col != 0 {
		t.Fatalf("got line=%d col=%d, want line=1 col=0", line, col)
	}
}

func TestSnapSurfaceColToGlyphStart(t *testing.T) {
	s := components.NewSurface(6, 1, nil)
	s.SetCell(0, 0, xui.Cell{Char: "中", Width: 2})
	s.SetCell(2, 0, xui.Cell{Char: "文", Width: 2})
	if got := text.SnapSurfaceColToGlyphStart(s.Buffer, 6, 1, 0); got != 0 {
		t.Fatalf("snap 1 -> %d, want 0", got)
	}
	if got := text.SnapSurfaceColToGlyphStart(s.Buffer, 6, 3, 0); got != 2 {
		t.Fatalf("snap 3 -> %d, want 2", got)
	}
}

func TestSanitizeComposerTextDropsControls(t *testing.T) {
	in := "a\tb\r\nc\x00e\rd\n"
	got := sanitizeComposerText(in)
	want := "a    b\nce\nd\n"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := sanitizeComposerText("▎hello"); got != "hello" {
		t.Fatalf("chrome strip: got %q", got)
	}
}

func TestChatInputPendingSkills(t *testing.T) {
	c := &ChatInput{
		MinBodyRows:   3,
		PendingSkills: []string{"building-plugins"},
		Theme:         components.DefaultTheme(),
	}
	method := xui.WidthUnicode
	if h := c.PreferredHeight(60, method); h != 6 {
		t.Fatalf("preferred height with pending skill = %d, want 6", h)
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 10}, Method: method})
	if s.Size.Height != 6 {
		t.Fatalf("draw height = %d, want 6", s.Size.Height)
	}
	if s.Buffer[0].Char != "╭" {
		t.Fatalf("top-left = %q, want ╭ (skills must be inside the border)", s.Buffer[0].Char)
	}
	inner := rowString(s, 1)
	if !strings.Contains(inner, "Skills:") || !strings.Contains(inner, "building-plugins") {
		t.Fatalf("pending skills row missing inside border: %q", inner)
	}
	underlined := false
	row := 1 * s.Size.Width
	for x := 0; x < s.Size.Width; x++ {
		if s.Buffer[row+x].Style.Underline {
			underlined = true
			break
		}
	}
	if !underlined {
		t.Fatal("expected underlined skill name")
	}
	// Cursor sits on the editor line below the skills chip.
	if s.Cursor == nil || s.Cursor.Y != 2 {
		t.Fatalf("cursor = %+v, want y=2 (below skills)", s.Cursor)
	}

	// Backspace on empty input pops the pending skill.
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Code: xui.KeyBackspace, Press: true})
	if len(c.PendingSkills) != 0 {
		t.Fatalf("expected pending skills cleared, got %v", c.PendingSkills)
	}
	if h := c.PreferredHeight(60, method); h != 5 {
		t.Fatalf("preferred height after clear = %d, want 5", h)
	}
}

func TestChatInputAddPendingSkillDedup(t *testing.T) {
	c := &ChatInput{}
	c.AddPendingSkill("building-plugins")
	c.AddPendingSkill("building-plugins")
	c.AddPendingSkill("example-skill")
	if len(c.PendingSkills) != 2 {
		t.Fatalf("got %v", c.PendingSkills)
	}
	if !c.PopPendingSkill() || c.PendingSkills[0] != "building-plugins" {
		t.Fatalf("pop left %v", c.PendingSkills)
	}
}

func rowString(s components.Surface, y int) string {
	var b strings.Builder
	for x := 0; x < s.Size.Width; x++ {
		ch := s.Buffer[y*s.Size.Width+x].Char
		if ch == "" {
			ch = " "
		}
		b.WriteString(ch)
	}
	return b.String()
}

func TestChatInputCJKBlockCursorKeepsWidth(t *testing.T) {
	c := &ChatInput{
		MinBodyRows:    3,
		PaddingX:       1,
		Value:          "中",
		Cursor:         0,
		UseBlockCursor: true,
		CursorStyle:    xui.Style{Reverse: true},
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 40, Height: 10}, Method: xui.WidthUnicode})
	if s.Cursor == nil {
		t.Fatal("expected cursor")
	}
	cx, cy := s.Cursor.X, s.Cursor.Y
	cell := s.Buffer[cy*s.Size.Width+cx]
	if cell.Char != "中" || cell.Width != 2 {
		t.Fatalf("block cursor cell = %+v, want 中 width 2", cell)
	}
}

func TestCursorAfterCJKPasteAtTextEnd(t *testing.T) {
	sample := "13个技能 你把这个 skills挪动过去"
	c := &ChatInput{MinBodyRows: 3, PaddingX: 1, UseBlockCursor: false}
	c.Handle(&components.EventContext{}, xui.PasteEvent{Text: sample})
	w := 80
	s := c.Draw(components.DrawContext{Max: components.Size{Width: w, Height: 10}, Method: xui.WidthUnicode})
	if s.Cursor == nil {
		t.Fatal("nil cursor")
	}
	cy := s.Cursor.Y
	var lastContentEnd int
	for x := 0; x < w; {
		cell := s.Buffer[cy*w+x]
		step := int(cell.Width)
		step = max(step, 1)
		if !cell.Trail && cell.Char != "" && cell.Char != " " && cell.Char != "│" {
			lastContentEnd = x + step
		}
		x += step
	}
	if s.Cursor.X != lastContentEnd {
		t.Fatalf("cursorX=%d want text end %d", s.Cursor.X, lastContentEnd)
	}
	// Insertion caret must not sit on the last CJK primary (IME would overlay it).
	cell := s.Buffer[cy*w+s.Cursor.X]
	if xui.StringWidth(cell.Char, xui.WidthUnicode) == 2 {
		t.Fatalf("cursor on wide glyph %q", cell.Char)
	}
}
