package block

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

// A live row — a call that finished while what it started runs on — wears
// the watch glyph instead of the checkmark, which would say it is over.
func TestToolLiveRowWearsTheWatchGlyph(t *testing.T) {
	th := components.DefaultTheme()
	ctx := components.DrawContext{Max: components.Size{Width: 40, Height: 10}}
	live := &ToolBlock{Name: "watch", Detail: "w1 edge logs", Status: status.ToolLive, Theme: th}
	if got := components.SurfaceText(live.Draw(ctx)); !containsRune(got, '⏱') {
		t.Fatalf("live row = %q, want the ⏱ glyph", got)
	}
	done := &ToolBlock{Name: "watch", Detail: "w1 edge logs", Status: status.ToolDone, Theme: th}
	if got := components.SurfaceText(done.Draw(ctx)); containsRune(got, '⏱') {
		t.Fatalf("done row = %q, must not keep the ⏱ glyph", got)
	}
}

// SetExpanded drives a row outright the way the footer's watch indicator
// does: it notifies on a real change only, and a row with nothing to show
// stays shut.
func TestSetExpandedReportsRealChangesOnly(t *testing.T) {
	var toggled []bool
	row := &ToolBlock{Name: "watch", Output: "hit", OnToggle: func(e bool) { toggled = append(toggled, e) }}
	if !row.SetExpanded(true) || !row.Expanded {
		t.Fatal("opening a folded row with a body must change it")
	}
	if row.SetExpanded(true) {
		t.Fatal("opening an open row is no change")
	}
	if !row.SetExpanded(false) || row.Expanded {
		t.Fatal("folding an open row must change it")
	}
	if len(toggled) != 2 || !toggled[0] || toggled[1] {
		t.Fatalf("OnToggle saw %v, want [true false]", toggled)
	}

	bare := &ToolBlock{Name: "watch"}
	if bare.SetExpanded(true) || bare.Expanded {
		t.Fatal("a row with nothing to show stays shut")
	}
}

func containsRune(s string, want rune) bool {
	for _, r := range s {
		if r == want {
			return true
		}
	}
	return false
}
