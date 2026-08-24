package block

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
)

func TestExtractUserMessageSkipsRuleChrome(t *testing.T) {
	ub := &UserBlock{Text: "13个技能 你把这个 skills挪动过去", Theme: components.DefaultTheme()}
	s := ub.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 5}, Method: xui.WidthUnicode})
	got := components.ExtractSurfaceText(s, 0, 1, 59, 1)
	if strings.Contains(got, "┃") {
		t.Fatalf("selection must not include rule chrome: %q", got)
	}
	if strings.TrimSpace(got) != ub.Text {
		t.Fatalf("got %q, want %q", got, ub.Text)
	}
}

// TestUserBlockOpencodePanel pins the user message to opencode's UserMessage:
// a full-height secondary ┃ rule, panel background (#141414) behind every
// cell right of the rule, one blank panel row above and below the text, and
// the text itself two columns right of the rule.
func TestUserBlockOpencodePanel(t *testing.T) {
	th := components.DefaultTheme()
	ub := &UserBlock{Text: "hello", Theme: th}
	s := ub.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 10}, Method: xui.WidthUnicode})

	if s.Size.Height != 3 {
		t.Fatalf("height=%d, want 3 (pad row + one text row + pad row)", s.Size.Height)
	}
	panel := xui.RGBColor(0x14, 0x14, 0x14)
	cell := func(x, y int) xui.Cell { return s.Buffer[y*s.Size.Width+x] }

	bar := cell(0, 1)
	if bar.Char != "┃" || bar.Style.Fg != th.Secondary.Fg {
		t.Fatalf("bar cell = %q %+v, want ┃ in secondary color", bar.Char, bar.Style)
	}
	for _, xy := range [][2]int{{1, 0}, {1, 1}, {10, 1}, {19, 1}, {2, 2}} {
		if c := cell(xy[0], xy[1]); c.Style.Bg != panel {
			t.Fatalf("cell %v bg=%v, want panel %v", xy, c.Style.Bg, panel)
		}
	}
	text := cell(3, 1)
	if text.Char != "h" || text.Style.Fg != th.Foreground.Fg {
		t.Fatalf("text cell = %q %+v, want 'h' in foreground color", text.Char, text.Style)
	}
}

// TestUserBlockPanelWrapsInsideRule: wrapping counts the rule and the panel
// padding, so lines fit within width-3 columns and padding rows wrap too.
func TestUserBlockPanelWrapsInsideRule(t *testing.T) {
	ub := &UserBlock{Text: strings.Repeat("a", 18), Theme: components.DefaultTheme()}
	s := ub.Draw(components.DrawContext{Max: components.Size{Width: 20, Height: 10}, Method: xui.WidthUnicode})
	if s.Size.Height != 4 {
		t.Fatalf("height=%d, want 4 (pad + 2 wrapped rows + pad)", s.Size.Height)
	}
	row := components.ExtractSurfaceText(s, 0, 1, 19, 1)
	if n := len(strings.TrimSpace(row)); n != 17 {
		t.Fatalf("first text row holds %d chars, want 17 (20 - rule - 2 padding)", n)
	}
}
