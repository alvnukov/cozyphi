package chat

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
	"github.com/alvnukov/cozyphi/internal/components/text"
)

// TestChatInputOpencodeFrame pins the composer frame against the opencode
// prompt (packages/tui/src/component/prompt/index.tsx): a left ┃ bar in the
// posture color around a backgroundElement panel, text inset two columns
// past the bar, the meta row inside the frame bottom, and the ╹▀ fade tail.
func TestChatInputOpencodeFrame(t *testing.T) {
	th := components.DefaultTheme()
	c := &ChatInput{
		MinBodyRows: 1,
		Theme:       th,
		AgentLabel:  layout.BorderLabel{Text: "⏵⏵ build", Style: th.Secondary},
		ModelLabel:  "deepseek-chat",
		Value:       "hello",
		Cursor:      5,
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	if s.Size.Height != 6 {
		t.Fatalf("height = %d, want 6 (pad + editor + gap + meta + tail + hints)", s.Size.Height)
	}
	for y := range 4 {
		cell := s.Buffer[y*60]
		if cell.Char != "┃" || !cell.Style.Equal(th.Secondary) {
			t.Fatalf("bar row %d = %q %+v", y, cell.Char, cell.Style)
		}
	}
	if bg := s.Buffer[60+1].Style.Bg; !bg.Equal(th.BackgroundElement.Bg) {
		t.Fatalf("panel bg = %v", bg)
	}
	if ch := s.Buffer[60+3].Char; ch != "h" {
		t.Fatalf("text must start two columns past the bar, got %q", ch)
	}
	if fg := s.Buffer[60+3].Style.Fg; !fg.Equal(th.Foreground.Fg) {
		t.Fatalf("text style = %v", fg)
	}
	meta := rowString(s, 3)
	if !strings.Contains(meta, "⏵⏵ build · deepseek-chat") {
		t.Fatalf("meta row = %q", meta)
	}
	// Everything painted inside the frame carries the panel background:
	// Print replaces cell styles wholesale, so a bgless span would punch a
	// default-background hole into the element panel.
	wantLead := th.Secondary
	wantLead.Bg = th.BackgroundElement.Bg
	if st := s.Buffer[3*60+3].Style; !st.Equal(wantLead) {
		t.Fatalf("meta lead style = %+v", st)
	}
	if x := strings.Index(meta, "deepseek-chat"); !s.Buffer[3*60+x].Style.Fg.Equal(th.Foreground.Fg) {
		t.Fatalf("model label not foreground at col %d", x)
	}
	tail := s.Buffer[4*60]
	if tail.Char != "╹" || !tail.Style.Equal(th.Secondary) {
		t.Fatalf("tail corner = %q %+v", tail.Char, tail.Style)
	}
	if fade := s.Buffer[4*60+4]; fade.Char != "▀" || !fade.Style.Fg.Equal(th.BackgroundElement.Bg) {
		t.Fatalf("tail fade = %q %+v", fade.Char, fade.Style)
	}
	if s.Cursor == nil || s.Cursor.X != 8 || s.Cursor.Y != 1 {
		t.Fatalf("cursor = %+v, want (8,1)", s.Cursor)
	}
}

// TestChatInputHintsRow: below the frame the cwd sits muted on the left and
// usage hints on the right; with no usage set the keymap fallback shows.
func TestChatInputHintsRow(t *testing.T) {
	th := components.DefaultTheme()
	c := &ChatInput{
		MinBodyRows: 1,
		Theme:       th,
		HintsLeft:   "~/src/cozyphi",
		HintsRight:  []components.Span{{Text: "5% of 128k", Style: th.Muted}},
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	row := rowString(s, 5)
	if !strings.Contains(row, "~/src/cozyphi") {
		t.Fatalf("hints row missing cwd: %q", row)
	}
	if x := strings.Index(row, "5% of 128k"); x != 49 {
		t.Fatalf("usage must right-align with a one-column margin, got col %d: %q", x, row)
	}
	if st := s.Buffer[5*60+1].Style; !st.Equal(th.Muted) {
		t.Fatalf("cwd style = %+v", st)
	}

	c.HintsRight = nil
	s = c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	row = rowString(s, 5)
	if !strings.Contains(row, "tab mode") || !strings.Contains(row, "^k commands") {
		t.Fatalf("fallback hints missing: %q", row)
	}
	i := strings.Index(row, "tab mode")
	if fg := s.Buffer[5*60+i].Style.Fg; !fg.Equal(th.Foreground.Fg) {
		t.Fatalf("shortcut key style = %v", fg)
	}
	if fg := s.Buffer[5*60+i+3].Style.Fg; !fg.Equal(th.Muted.Fg) {
		t.Fatalf("shortcut label style = %v", fg)
	}
}

// TestChatInputPlaceholder: empty input shows the muted placeholder inside
// the frame with the cursor parked at the text origin (opencode textarea).
func TestChatInputPlaceholder(t *testing.T) {
	th := components.DefaultTheme()
	c := &ChatInput{
		MinBodyRows: 1,
		Theme:       th,
		Placeholder: `Ask anything... "how do I exit vim?"`,
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	if ch := s.Buffer[60+3].Char; ch != "A" {
		t.Fatalf("placeholder must render at the text origin, got %q", ch)
	}
	wantPlaceholder := th.Muted
	wantPlaceholder.Bg = th.BackgroundElement.Bg
	if st := s.Buffer[60+3].Style; !st.Equal(wantPlaceholder) {
		t.Fatalf("placeholder style = %+v", st)
	}
	if s.Cursor == nil || s.Cursor.X != 3 || s.Cursor.Y != 1 {
		t.Fatalf("cursor = %+v, want (3,1)", s.Cursor)
	}

	// Typed text replaces the placeholder entirely.
	c.Value, c.Cursor = "hi", 2
	s = c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	if row := rowString(s, 1); strings.Contains(row, "Ask anything") {
		t.Fatalf("placeholder leaked under typed text: %q", row)
	}
}

// TestChatInputPanelContentKeepsPanelBackground: typed text, the placeholder,
// and the meta row ride on the element panel background (opencode keeps the
// whole input on backgroundElement); only the hints row below the frame stays
// on the terminal background.
func TestChatInputPanelContentKeepsPanelBackground(t *testing.T) {
	th := components.DefaultTheme()
	panel := th.BackgroundElement.Bg
	c := &ChatInput{
		MinBodyRows: 1,
		Theme:       th,
		AgentLabel:  layout.BorderLabel{Text: "⏵⏵ build", Style: th.Secondary},
		ModelLabel:  "m",
		Value:       "hi",
		Cursor:      2,
		HintsLeft:   "~/x",
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	if bg := s.Buffer[60+3].Style.Bg; !bg.Equal(panel) {
		t.Fatalf("typed text bg = %v, want panel %v", bg, panel)
	}
	if bg := s.Buffer[3*60+3].Style.Bg; !bg.Equal(panel) {
		t.Fatalf("meta lead bg = %v, want panel %v", bg, panel)
	}
	if bg := s.Buffer[5*60+1].Style.Bg; bg.Equal(panel) {
		t.Fatal("hints row must stay on the terminal background")
	}

	c.Value, c.Cursor = "", 0
	c.Placeholder = "Ask anything..."
	s = c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 12}, Method: xui.WidthUnicode})
	if bg := s.Buffer[60+3].Style.Bg; !bg.Equal(panel) {
		t.Fatalf("placeholder bg = %v, want panel %v", bg, panel)
	}
}

// TestChatInputMinHeight pins the geometry floor: the smallest total height
// at the configured minimum body rows, so layout code clamps short screens
// against one number instead of re-deriving it.
func TestChatInputMinHeight(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3}
	if h := c.MinHeight(); h != 8 {
		t.Fatalf("min height = %d, want 8", h)
	}
	c.PendingSkills = []string{"building-plugins"}
	if h := c.MinHeight(); h != 9 {
		t.Fatalf("min height with skills = %d, want 9", h)
	}
	compact := &ChatInput{MinBodyRows: 1}
	if h := compact.MinHeight(); h != 6 {
		t.Fatalf("compact min height = %d, want 6", h)
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
	c := &ChatInput{MinBodyRows: 3, MaxBodyRows: 5}
	method := xui.WidthUnicode
	w := 40
	if h := c.PreferredHeight(w, method); h != 8 {
		t.Fatalf("empty preferred height = %d, want 8", h)
	}
	c.Value = "one\ntwo\nthree\nfour"
	if h := c.PreferredHeight(w, method); h != 9 {
		t.Fatalf("4 lines preferred height = %d, want 9", h)
	}
	c.Value = "one\ntwo\nthree\nfour\nfive\nsix\nseven"
	if h := c.PreferredHeight(w, method); h != 10 {
		t.Fatalf("over max preferred height = %d, want 10 (max body 5 + frame)", h)
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: w, Height: 20}, Method: method})
	if s.Size.Height != 10 {
		t.Fatalf("draw height = %d, want 10", s.Size.Height)
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
	if h := c.PreferredHeight(40, xui.WidthUnicode); h < 8 {
		t.Fatalf("expected grow after paste, height=%d", h)
	}
}

func TestChatInputCJKPasteNoContinuationReverse(t *testing.T) {
	c := &ChatInput{
		MinBodyRows:    3,
		MaxBodyRows:    8,
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
	if h := c.PreferredHeight(60, method); h != 9 {
		t.Fatalf("preferred height with pending skill = %d, want 9", h)
	}
	s := c.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 10}, Method: method})
	if s.Size.Height != 9 {
		t.Fatalf("draw height = %d, want 9", s.Size.Height)
	}
	if s.Buffer[0].Char != "┃" {
		t.Fatalf("bar = %q, want ┃ (skills must be inside the frame)", s.Buffer[0].Char)
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
	if h := c.PreferredHeight(60, method); h != 8 {
		t.Fatalf("preferred height after clear = %d, want 8", h)
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
	c := &ChatInput{MinBodyRows: 3, UseBlockCursor: false}
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

func TestChatInputPointerShapeText(t *testing.T) {
	c := &ChatInput{MinBodyRows: 3}
	if got := c.PointerShape(0, 0); got != components.ShapeText {
		t.Fatalf("composer shape = %q, want text", got)
	}
}

// TestChatInputSingleLineUpDownNoCaretBounce: on a single-line draft without
// history there is no other line to move to, so Up/Down must not bounce the
// caret to Home/End (the old behavior read as "left/right" and confused users).
func TestChatInputSingleLineUpDownNoCaretBounce(t *testing.T) {
	c := &ChatInput{MinBodyRows: 1, Value: "hello", Cursor: 5}
	ctx := &components.EventContext{}
	c.Handle(ctx, key(xui.KeyUp))
	if c.Cursor != 5 {
		t.Fatalf("Up on a single line must not bounce to 0, got %d", c.Cursor)
	}
	c.Handle(ctx, key(xui.KeyDown))
	if c.Cursor != 5 {
		t.Fatalf("Down on a single line must not bounce to end, got %d", c.Cursor)
	}
}
