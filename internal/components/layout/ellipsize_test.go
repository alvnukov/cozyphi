package layout

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// A cut label must read as cut: the ellipsis is the only honest way to say
// "there was more". Widths are worked by hand from the grapheme rules.
func TestEllipsizeToWidth(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"deepseek-v4-pro-max", 20, "deepseek-v4-pro-max"}, // 19 cols, fits
		{"deepseek-v4-pro-max", 16, "deepseek-v4-pro…"},    // 15 + ellipsis
		{"deepseek-v4-pro-max", 1, "…"},
		{"deepseek-v4-pro-max", 0, ""},
		{"hi", 5, "hi"},
		{"hi", 2, "hi"},
		{"你好世界", 5, "你好…"}, // 2+2 wide glyphs + ellipsis
	}
	for _, tc := range cases {
		if got := EllipsizeToWidth(tc.in, tc.max, xui.WidthUnicode); got != tc.want {
			t.Fatalf("EllipsizeToWidth(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func borderedRow(t *testing.T, width int, topLeft, topRight string) string {
	t.Helper()
	s := components.NewSurface(width, 2, nil)
	var left, right *BorderLabel
	if topLeft != "" {
		left = &BorderLabel{Text: topLeft}
	}
	if topRight != "" {
		right = &BorderLabel{Text: topRight}
	}
	DrawRoundedBorder(&s, BorderRounded, components.DefaultTheme().Border, left, right, nil, nil, xui.WidthUnicode)
	return strings.SplitN(components.SurfaceText(s), "\n", 2)[0]
}

// The model label is a border label: at 19 columns the old code rendered the
// reported "deepseek-v4-pro-" — a cut that does not look like one.
func TestBorderLabelEllipsizesWhenCut(t *testing.T) {
	row := borderedRow(t, 19, "", "deepseek-v4-pro-max")
	if !strings.Contains(row, "deepseek-v4-pro-…") {
		t.Fatalf("row = %q, want ellipsized model label", row)
	}
}

// When labels collide the right one (model) wins the whole edge; the left
// one yields entirely instead of both being mangled.
func TestBorderLabelCollisionPrefersRight(t *testing.T) {
	row := borderedRow(t, 20, "⏵⏵ build", "deepseek-v4-pro-max")
	if !strings.Contains(row, "deepseek-v4-pro-m…") {
		t.Fatalf("row = %q, want ellipsized right label", row)
	}
	if strings.Contains(row, "build") {
		t.Fatalf("row = %q, want left label dropped on collision", row)
	}
}

func TestBorderLabelFullWhenFits(t *testing.T) {
	row := borderedRow(t, 40, "⏵⏵ build", "gpt-4o")
	if strings.Contains(row, "…") {
		t.Fatalf("row = %q, want no ellipsis when everything fits", row)
	}
	if !strings.Contains(row, "⏵⏵ build") || !strings.Contains(row, "gpt-4o") {
		t.Fatalf("row = %q, want both labels intact", row)
	}
}
