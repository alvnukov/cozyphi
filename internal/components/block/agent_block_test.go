package block_test

import (
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

func drawAgent(a *block.AgentBlock) string {
	s := a.Draw(components.DrawContext{Max: components.Size{Width: 80, Height: 20}})
	return components.SurfaceText(s)
}

// TestAgentBlockTitleCountsToolsAndElapsed: a running child's title reads
// role(description) · N tools · elapsed, ticking off the wall clock.
func TestAgentBlockTitleCountsToolsAndElapsed(t *testing.T) {
	started := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	a := &block.AgentBlock{
		Name:    "worker(fix the lexer)",
		Status:  status.ToolRunning,
		Tools:   2,
		Started: started,
		Now:     func() time.Time { return started.Add(80 * time.Second) },
		Theme:   components.DefaultTheme(),
	}
	txt := drawAgent(a)
	for _, want := range []string{"worker(fix the lexer)", "2 tools", "1m 20s"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("title missing %q: %q", want, txt)
		}
	}

	// One tool is singular, and no tools at all says nothing.
	a.Tools = 1
	if txt := drawAgent(a); !strings.Contains(txt, "1 tool ·") {
		t.Fatalf("singular count: %q", txt)
	}
	a.Tools = 0
	if txt := drawAgent(a); strings.Contains(txt, "tool") {
		t.Fatalf("zero tools must stay quiet: %q", txt)
	}
}

// TestAgentBlockFreezesElapsedWhenFinished: once the child reports back, the
// clock stops where it stopped and no longer follows the wall clock.
func TestAgentBlockFreezesElapsedWhenFinished(t *testing.T) {
	started := time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	a := &block.AgentBlock{
		Name:     "worker(fix the lexer)",
		Status:   status.ToolDone,
		Tools:    3,
		Started:  started,
		Finished: started.Add(42 * time.Second),
		Now:      func() time.Time { return started.Add(time.Hour) },
		Theme:    components.DefaultTheme(),
	}
	txt := drawAgent(a)
	if !strings.Contains(txt, "3 tools · 42s") {
		t.Fatalf("frozen elapsed: %q", txt)
	}
}

// TestAgentBlockNoClockWithoutStart: a replayed outcome knows what the child
// said, not how long it took, and must not invent a duration.
func TestAgentBlockNoClockWithoutStart(t *testing.T) {
	a := &block.AgentBlock{
		Name:    "worker(fix the lexer)",
		Status:  status.ToolDone,
		Summary: "done",
		Theme:   components.DefaultTheme(),
	}
	if txt := drawAgent(a); strings.Contains(txt, "s ·") || strings.Contains(txt, "· 0s") {
		t.Fatalf("invented a duration: %q", txt)
	}
}

// TestAgentBlockGlyphSettlesOnOutcome: done, error and stopped each read
// differently, and a stopped child is not a refused call.
func TestAgentBlockGlyphSettlesOnOutcome(t *testing.T) {
	cases := []struct {
		st   status.ToolStatus
		want string
	}{
		{status.ToolDone, "✓"},
		{status.ToolError, "✗"},
		{status.ToolCancelled, "■"},
	}
	for _, tc := range cases {
		a := &block.AgentBlock{
			Name:     "worker(fix the lexer)",
			Status:   tc.st,
			Summary:  "what it found",
			Expanded: true,
			Theme:    components.DefaultTheme(),
		}
		txt := drawAgent(a)
		if !strings.Contains(txt, tc.want) {
			t.Fatalf("status %v wants glyph %q: %q", tc.st, tc.want, txt)
		}
		if !strings.Contains(txt, "what it found") {
			t.Fatalf("status %v lost the summary: %q", tc.st, txt)
		}
	}
}
