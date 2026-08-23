package components

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
)

func lineText(l RichLine) string {
	var b strings.Builder
	for _, sp := range l {
		b.WriteString(sp.Text)
	}
	return b.String()
}

func linesText(lines []RichLine) []string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = lineText(l)
	}
	return out
}

// TestWrapSpansBreaksAtWordBoundaries: wrapping must move whole words to the
// next line instead of splitting them mid-grapheme.
func TestWrapSpansBreaksAtWordBoundaries(t *testing.T) {
	got := linesText(WrapSpans([]Span{{Text: "alpha beta gamma"}}, 11, xui.WidthUnicode))
	want := []string{"alpha beta", "gamma"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// TestWrapSpansKeepsCyrillicWordsWhole is the screenshot regression: a long
// Russian word must move to the next line, not split into "пра|вильный".
func TestWrapSpansKeepsCyrillicWordsWhole(t *testing.T) {
	got := linesText(WrapSpans([]Span{{Text: "правильный deep module"}}, 12, xui.WidthUnicode))
	want := []string{"правильный", "deep module"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// TestWrapSpansHardBreaksOverlongWord: a single word wider than the line
// still breaks at grapheme boundaries instead of overflowing.
func TestWrapSpansHardBreaksOverlongWord(t *testing.T) {
	got := linesText(WrapSpans([]Span{{Text: "abcdefghij"}}, 4, xui.WidthUnicode))
	want := []string{"abcd", "efgh", "ij"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// TestWrapSpansTrimsBreakWhitespace: the spaces at a wrap point are dropped,
// so the next line starts with the word, not with leading blanks.
func TestWrapSpansTrimsBreakWhitespace(t *testing.T) {
	got := linesText(WrapSpans([]Span{{Text: "alpha   beta"}}, 5, xui.WidthUnicode))
	want := []string{"alpha", "beta"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// TestWrapSpansPreservesStyleBoundaries: a word split across two spans keeps
// each part's style on every wrapped line.
func TestWrapSpansPreservesStyleBoundaries(t *testing.T) {
	bold := xui.Style{Bold: true}
	lines := WrapSpans([]Span{
		{Text: "plain ", Style: xui.Style{}},
		{Text: "boldword", Style: bold},
	}, 8, xui.WidthUnicode)
	if len(lines) != 2 {
		t.Fatalf("lines = %d, want 2 (%q)", len(lines), linesText(lines))
	}
	if got := lineText(lines[0]); got != "plain" {
		t.Fatalf("line0 = %q, want %q", got, "plain")
	}
	if len(lines[1]) != 1 || !lines[1][0].Style.Bold || lines[1][0].Text != "boldword" {
		t.Fatalf("line1 = %+v, want single bold boldword", lines[1])
	}
}

// TestWrapSpansKeepsHardNewlines: explicit newlines still produce line breaks
// and empty lines (paragraph rhythm comes from the markdown renderer).
func TestWrapSpansKeepsHardNewlines(t *testing.T) {
	got := linesText(WrapSpans([]Span{{Text: "a\n\nb"}}, 10, xui.WidthUnicode))
	want := []string{"a", "", "b"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}
