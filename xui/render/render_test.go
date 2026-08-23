package render

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui/cell"
)

func TestRendererStripsTabControl(t *testing.T) {
	r := NewRenderer()
	var buf mockWriter
	dirty := []cell.DirtyCell{
		{X: 0, Y: 0, Cell: cell.Cell{Char: "A", Width: 1}},
		{X: 1, Y: 0, Cell: cell.Cell{Char: "\t", Width: 1}},
		{X: 2, Y: 0, Cell: cell.Cell{Char: "B", Width: 1}},
	}
	_, err := r.RenderDiff(&buf, dirty, 0, 0, false, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := string(buf.b)
	if strings.Contains(out, "\t") {
		t.Fatalf("raw tab emitted: %q", out)
	}
	if !strings.Contains(out, "A") || !strings.Contains(out, "B") {
		t.Fatalf("missing glyphs in %q", out)
	}
}

type mockWriter struct{ b []byte }

func (m *mockWriter) Write(p []byte) (int, error) {
	m.b = append(m.b, p...)
	return len(p), nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return sub == ""
}
