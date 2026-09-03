package block_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
)

func drawSummary(b *block.TurnSummaryBlock) string {
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 10}})
	return components.SurfaceText(s)
}

func TestTurnSummaryNamesWorkToolsFilesAndFailures(t *testing.T) {
	b := &block.TurnSummaryBlock{
		Duration: 42 * time.Second,
		Tools:    7,
		Failed:   1,
		Files:    []string{"pane.go", "mapper.go"},
		Theme:    components.DefaultTheme(),
	}
	txt := drawSummary(b)
	for _, want := range []string{"▸", "worked 42s", "7 tools", "pane.go, mapper.go", "1 failed"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("summary missing %q: %q", want, txt)
		}
	}
}

func TestTurnSummaryFallsBackToStepCount(t *testing.T) {
	b := &block.TurnSummaryBlock{Rows: 3, Theme: components.DefaultTheme()}
	if txt := drawSummary(b); !strings.Contains(txt, "3 steps") {
		t.Fatalf("an untimed toolless turn falls back to its row count: %q", txt)
	}
}

func TestTurnSummaryClipsTheFileList(t *testing.T) {
	b := &block.TurnSummaryBlock{
		Tools: 5,
		Files: []string{"a.go", "b.go", "c.go", "d.go", "e.go"},
		Theme: components.DefaultTheme(),
	}
	txt := drawSummary(b)
	if !strings.Contains(txt, "a.go, b.go, c.go +2") {
		t.Fatalf("more than three files clip to a count: %q", txt)
	}
	if strings.Contains(txt, "d.go") {
		t.Fatalf("clipped files are not named: %q", txt)
	}
}

func TestTurnSummaryToggleFlipsTheArrow(t *testing.T) {
	toggled := -1
	b := &block.TurnSummaryBlock{
		Rows:  2,
		Theme: components.DefaultTheme(),
		OnToggle: func(expanded bool) {
			if expanded {
				toggled = 1
			} else {
				toggled = 0
			}
		},
	}
	ctx := &components.EventContext{}
	b.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	if toggled != 1 || !b.Expanded || !ctx.Consume {
		t.Fatalf("Enter expands: toggled=%d expanded=%v consume=%v", toggled, b.Expanded, ctx.Consume)
	}
	if txt := drawSummary(b); !strings.Contains(txt, "▾") {
		t.Fatalf("expanded summary flips its arrow: %q", txt)
	}
	b.Handle(&components.EventContext{}, xui.MouseEvent{Action: xui.MousePress, Button: xui.MouseLeft, Y: 0})
	if toggled != 0 || b.Expanded {
		t.Fatalf("a click on the row folds it back: toggled=%d expanded=%v", toggled, b.Expanded)
	}
}
