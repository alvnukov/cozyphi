package cell

import "testing"

// Grapheme-cluster regression cases: a terminal renders one cluster as one
// glyph, so ZWJ emoji, flags and skin-tone sequences must measure as the
// width of the glyph they draw — not as the sum of their runes' widths.
func TestStringWidthCountsClusters(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want int
	}{
		{"empty", "", 0},
		{"ascii", "ab", 2},
		{"cyrillic", "конец", 5},
		{"cjk", "日本語", 6},
		{"hangul", "한국", 4},
		{"simple emoji", "🙂😎", 4},
		{"zwj family", "👩‍👩‍👧", 2},
		{"zwj technologist with skin tone", "👩🏻‍💻", 2},
		{"flag", "🇯🇵", 2},
		{"two flags", "🇷🇺🇺🇸", 4},
		{"emoji with skin tone", "👍🏽", 2},
		{"emoji presentation default", "✅", 2},
		{"emoji presentation via VS16", "\u263a\uFE0f", 2},
		{"text presentation default", "\u263a", 1},
		{"combining mark", "e\u0301", 1},
		{"lone combining mark", "\u0301", 1},
		{"arrows and checks", "→✓★", 3},
		{"box drawing", "┃│─", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StringWidth(tt.in, WidthUnicode); got != tt.want {
				t.Errorf("StringWidth(%q) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

// The measure and paint paths must agree cell for cell: StringWidth equals
// the sum of the widths FirstGrapheme hands to Window.Print, or code-box
// borders drift from the glyphs a terminal draws.
func TestStringWidthMatchesPrintPath(t *testing.T) {
	samples := []string{
		"", "ab", "конец", "日本語", "🙂😎", "👩‍👩‍👧", "👩🏻‍💻",
		"🇷🇺🇺🇸", "👍🏽", "✅", "\u263a\uFE0f", "e\u0301", "\u0301",
		"┃│─", "a\u200db", "\x1b[0m",
	}
	for _, s := range samples {
		sum, rest := 0, s
		for rest != "" {
			var w int
			_, w, rest = FirstGrapheme(rest, WidthUnicode)
			sum += w
		}
		if got := StringWidth(s, WidthUnicode); got != sum {
			t.Errorf("StringWidth(%q) = %d, print path sums to %d", s, got, sum)
		}
	}
}

// FirstGrapheme must hand out whole clusters with their drawn width so the
// print path places one cell per glyph.
func TestFirstGraphemeReturnsClusters(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		cluster string
		rest    string
		want    int
	}{
		{"empty", "", "", "", 0},
		{"ascii", "ab", "a", "b", 1},
		{"zwj family", "👩‍👩‍👧x", "👩‍👩‍👧", "x", 2},
		{"flag pair", "🇷🇺🇯🇵", "🇷🇺", "🇯🇵", 2},
		{"skin tone", "👍🏽!", "👍🏽", "!", 2},
		{"combining mark", "e\u0301b", "e\u0301", "b", 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cluster, width, rest := FirstGrapheme(tt.in, WidthUnicode)
			if cluster != tt.cluster {
				t.Errorf("FirstGrapheme(%q) cluster = %q, want %q", tt.in, cluster, tt.cluster)
			}
			if width != tt.want {
				t.Errorf("FirstGrapheme(%q) width = %d, want %d", tt.in, width, tt.want)
			}
			if rest != tt.rest {
				t.Errorf("FirstGrapheme(%q) rest = %q, want %q", tt.in, rest, tt.rest)
			}
		})
	}
}
