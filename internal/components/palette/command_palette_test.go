package palette

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

func TestCommandPaletteFilterAndAccept(t *testing.T) {
	accepted := ""
	p := &CommandPalette{
		Theme: components.DefaultTheme(),
		Commands: []PaletteCommand{
			{ID: "1", Noun: "mode", Verb: "use boost"},
			{ID: "2", Noun: "app", Verb: "help"},
			{ID: "3", Noun: "session", Verb: "switch", Shortcut: "Ctrl t"},
		},
		OnAccept: func(c PaletteCommand) { accepted = c.ID },
	}
	p.Show()
	if !p.Open || len(p.filtered) != 3 {
		t.Fatalf("open=%v filtered=%d", p.Open, len(p.filtered))
	}

	ctx := &components.EventContext{}
	for _, r := range "help" {
		p.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
	if len(p.filtered) != 1 || p.Commands[p.filtered[0]].ID != "2" {
		t.Fatalf("filter help → %#v", p.filtered)
	}
	p.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	if accepted != "2" || p.Open {
		t.Fatalf("accept id=%q open=%v", accepted, p.Open)
	}
}

func TestCommandPaletteDraw(t *testing.T) {
	p := &CommandPalette{
		Theme: components.DefaultTheme(),
		Commands: []PaletteCommand{
			{ID: "1", Noun: "settings", Verb: "theme"},
			{ID: "2", Noun: "plugins", Verb: "reload"},
		},
	}
	p.Show()
	s := p.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 24}, Method: xui.WidthUnicode})
	if len(s.Children) != 1 {
		t.Fatalf("children=%d", len(s.Children))
	}
	panel := s.Children[0].Surface
	var b strings.Builder
	for x := 0; x < panel.Size.Width; x++ {
		ch := panel.Buffer[x].Char
		if ch == "" {
			ch = " "
		}
		b.WriteString(ch)
	}
	top := b.String()
	if !strings.Contains(top, "Command Palette") {
		t.Fatalf("missing title: %q", top)
	}
}

func TestFuzzyMatch(t *testing.T) {
	ok, _ := fuzzyMatch("", "mode use boost")
	if !ok {
		t.Fatal("empty query should match")
	}
	ok, score := fuzzyMatch("boost", "mode use boost")
	if !ok || score < 0.15 {
		t.Fatalf("boost score=%v", score)
	}
	ok, _ = fuzzyMatch("zzz", "mode use boost")
	if ok {
		t.Fatal("zzz should not match")
	}
}

func TestCommandPaletteBlendsWeightIntoTypedQuery(t *testing.T) {
	p := &CommandPalette{
		Theme: components.DefaultTheme(),
		Commands: []PaletteCommand{
			{ID: "cold", Noun: "mode", Verb: "alpha"},
			{ID: "hot", Noun: "mode", Verb: "beta", Weight: 1},
		},
	}
	p.Show()
	p.Query = "mo"
	p.refilter()
	if len(p.filtered) != 2 {
		t.Fatalf("both rows match: %#v", p.filtered)
	}
	if p.Commands[p.filtered[0]].ID != "hot" {
		t.Fatal("equal text scores must be broken by usage weight")
	}

	// Usage never rescues a row the text filter rejected.
	p.Query = "zzz"
	p.refilter()
	if len(p.filtered) != 0 {
		t.Fatalf("garbage stays filtered out: %#v", p.filtered)
	}
}

func TestCommandPaletteNestedSubmenu(t *testing.T) {
	picked := ""
	p := &CommandPalette{
		Theme: components.DefaultTheme(),
		Commands: []PaletteCommand{
			{
				ID:           "settings-theme",
				Noun:         "settings",
				Verb:         "theme",
				SubmenuTitle: "Select Theme",
				Submenu: []PaletteCommand{
					{ID: "dark", Verb: "Dark (builtin)", Run: func() { picked = "dark" }},
					{ID: "light", Verb: "Light (builtin)", Run: func() { picked = "light" }},
				},
			},
			{ID: "other", Noun: "app", Verb: "help", Run: func() { picked = "help" }},
		},
	}
	p.Show()
	ctx := &components.EventContext{}
	// Filter to theme command
	for _, r := range "theme" {
		p.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
	p.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	if !p.Open || p.Title != "Select Theme" || len(p.stack) != 1 {
		t.Fatalf("expected nested open title=%q stack=%d open=%v", p.Title, len(p.stack), p.Open)
	}
	// Esc pops back
	p.Handle(ctx, xui.KeyEvent{Code: xui.KeyEscape, Press: true})
	if !p.Open || p.Title != "Command Palette" || len(p.stack) != 0 {
		t.Fatalf("pop failed title=%q stack=%d", p.Title, len(p.stack))
	}
	// Enter submenu again and pick Dark
	p.Query = ""
	p.Cursor = 0
	p.Selected = 0
	p.refilter()
	for _, r := range "theme" {
		p.Handle(ctx, xui.KeyEvent{Code: xui.KeyRune, Rune: r, Press: true})
	}
	p.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true})
	p.Handle(ctx, xui.KeyEvent{Code: xui.KeyEnter, Press: true}) // first theme
	if picked != "dark" || p.Open {
		t.Fatalf("pick=%q open=%v", picked, p.Open)
	}
}

func TestCommandPalettePageAndVimKeys(t *testing.T) {
	commands := make([]PaletteCommand, 10)
	for i := range commands {
		commands[i] = PaletteCommand{ID: strconv.Itoa(i), Noun: "app", Verb: fmt.Sprintf("cmd %02d", i)}
	}
	p := &CommandPalette{Theme: components.DefaultTheme(), MaxItems: 4, Commands: commands}
	p.Show()
	ctx := &components.EventContext{}
	press := func(ev xui.KeyEvent) xui.KeyEvent { ev.Press = true; return ev }

	// PageDown steps by MaxItems-1 and clamps at the last row.
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyPageDown}))
	if p.Selected != 3 {
		t.Fatalf("PageDown → %d, want 3", p.Selected)
	}
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyPageDown}))
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyPageDown}))
	if p.Selected != 9 {
		t.Fatalf("PageDown clamp → %d, want 9", p.Selected)
	}
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyPageUp}))
	if p.Selected != 6 {
		t.Fatalf("PageUp → %d, want 6", p.Selected)
	}

	// Ctrl+D/Ctrl+U move half a page; Ctrl+F/Ctrl+B match the page keys.
	p.Selected = 0
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyRune, Rune: 'd', Mods: xui.ModCtrl}))
	if p.Selected != 2 {
		t.Fatalf("Ctrl+D → %d, want 2", p.Selected)
	}
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyRune, Rune: 'u', Mods: xui.ModCtrl}))
	if p.Selected != 0 {
		t.Fatalf("Ctrl+U → %d, want 0", p.Selected)
	}
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyRune, Rune: 'f', Mods: xui.ModCtrl}))
	if p.Selected != 3 {
		t.Fatalf("Ctrl+F → %d, want 3", p.Selected)
	}
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyRune, Rune: 'b', Mods: xui.ModCtrl}))
	if p.Selected != 0 {
		t.Fatalf("Ctrl+B → %d, want 0", p.Selected)
	}

	// Plain letters still type into the query, not the selection.
	p.Handle(ctx, press(xui.KeyEvent{Code: xui.KeyRune, Rune: 'j'}))
	if p.Query != "j" || p.Selected != 0 {
		t.Fatalf("plain j typed query=%q selected=%d", p.Query, p.Selected)
	}
}
