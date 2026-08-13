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
	got := components.ExtractSurfaceText(s, 0, 0, 59, 0)
	if strings.Contains(got, "▎") {
		t.Fatalf("selection must not include rule chrome: %q", got)
	}
	if got != ub.Text {
		t.Fatalf("got %q, want %q", got, ub.Text)
	}
}
