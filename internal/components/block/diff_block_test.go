package block_test

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/block"
	"github.com/alvnukov/cozyphi/internal/components/status"
)

const sampleDiff = "--- a/pane.go\n" +
	"+++ b/pane.go\n" +
	"@@ -1,3 +1,4 @@\n" +
	" kept line\n" +
	"-removed line\n" +
	"+added line\n" +
	"+second added\n"

func drawDiff(d *block.DiffBlock) string {
	s := d.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 40}})
	return components.SurfaceText(s)
}

func TestDiffStatsCountsHunkLinesOnly(t *testing.T) {
	added, removed := block.DiffStats(sampleDiff)
	if added != 2 || removed != 1 {
		t.Fatalf("stats: got +%d −%d, want +2 −1", added, removed)
	}
}

func TestDiffBlockTitleCarriesStatsWhenCollapsed(t *testing.T) {
	d := &block.DiffBlock{
		Name:   "edit",
		Path:   "pane.go",
		Diff:   sampleDiff,
		Status: status.ToolDone,
		Theme:  components.DefaultTheme(),
	}
	txt := drawDiff(d)
	for _, want := range []string{"edit", "pane.go", "+2", "−1", "▶"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("collapsed title missing %q: %q", want, txt)
		}
	}
	if strings.Contains(txt, "added line") {
		t.Fatalf("collapsed card must not show hunks: %q", txt)
	}
}

func TestDiffBlockExpandedShowsHunksWithoutFileHeader(t *testing.T) {
	d := &block.DiffBlock{
		Name:     "edit",
		Path:     "pane.go",
		Diff:     sampleDiff,
		Status:   status.ToolDone,
		Expanded: true,
		Theme:    components.DefaultTheme(),
	}
	txt := drawDiff(d)
	for _, want := range []string{"@@ -1,3 +1,4 @@", "-removed line", "+added line", "▼"} {
		if !strings.Contains(txt, want) {
			t.Fatalf("expanded body missing %q: %q", want, txt)
		}
	}
	if strings.Contains(txt, "--- a/") || strings.Contains(txt, "+++ b/") {
		t.Fatalf("file header duplicates the title path and must be dropped: %q", txt)
	}
}

func TestDiffBlockErrorVisibleCollapsed(t *testing.T) {
	d := &block.DiffBlock{
		Name:   "edit",
		Path:   "pane.go",
		Error:  "hash mismatch: the file changed on disk\nsecond error line",
		Status: status.ToolError,
		Theme:  components.DefaultTheme(),
	}
	txt := drawDiff(d)
	if !strings.Contains(txt, "Error: hash mismatch") {
		t.Fatalf("collapsed card must show the failure: %q", txt)
	}
	if strings.Contains(txt, "second error line") {
		t.Fatalf("collapsed card shows only the first error line: %q", txt)
	}
}

func TestToolBlockErrorVisibleCollapsed(t *testing.T) {
	b := &block.ToolBlock{
		Name:   "grep",
		Detail: `"pat" — 0 matches`,
		Error:  "ripgrep exited with code 2\ndetails follow",
		Status: status.ToolError,
		Theme:  components.DefaultTheme(),
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "Error: ripgrep exited with code 2") {
		t.Fatalf("collapsed tool row must show the failure: %q", txt)
	}
	if strings.Contains(txt, "details follow") {
		t.Fatalf("collapsed tool row shows only the first error line: %q", txt)
	}
}

func TestBashBlockErrorShowsFinalLineCollapsed(t *testing.T) {
	b := &block.BashBlock{
		Command: "make test",
		Output:  "long build log\nFAIL: TestX broke\n",
		Status:  block.BashError,
		Theme:   components.DefaultTheme(),
	}
	s := b.Draw(components.DrawContext{Max: components.Size{Width: 60, Height: 20}})
	txt := components.SurfaceText(s)
	if !strings.Contains(txt, "FAIL: TestX broke") {
		t.Fatalf("collapsed failed command must show its last line: %q", txt)
	}
	if strings.Contains(txt, "long build log") {
		t.Fatalf("collapsed failed command shows only the tail: %q", txt)
	}
}
