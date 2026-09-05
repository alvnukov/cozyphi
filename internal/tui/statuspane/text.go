package statuspane

import (
	"strings"

	"github.com/pulseaiclub/xui"
)

// wrapLines preserves long paths and counters on narrow screens. Terminal control
// characters are replaced rather than interpreted, even in injected metadata.
func wrapLines(lines []string, width int, method xui.WidthMethod) []string {
	if width <= 0 {
		return nil
	}
	var out []string
	for _, line := range lines {
		var chunk strings.Builder
		cells := 0
		for _, r := range safeText(line) {
			n := xui.StringWidth(string(r), method)
			if cells+n > width && chunk.Len() > 0 {
				out = append(out, chunk.String())
				chunk.Reset()
				cells = 0
			}
			if n > width {
				r, n = '?', 1
			}
			chunk.WriteRune(r)
			cells += n
		}
		out = append(out, chunk.String())
	}
	return out
}
