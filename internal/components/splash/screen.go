package splash

import (
	"strings"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// brandName is the plain title used when the terminal is too small for the
// block wordmark.
const brandName = "CozyPhi"

// wordmark is the static CozyPhi logo in figlet-standard glyphs (the z has
// an explicit diagonal), one row per entry; every row has the same display
// width (TestWordmarkUniformWidth keeps that true).
var wordmark = []string{
	`  ____           _____  _   _   ____    _       _ `,
	` / ___|   ___   |__  / | | | | |  _ \  | |__   (_)`,
	`| |      / _ \    / /  | |_| | | |_) | | '_ \  | |`,
	`| |___  | (_) |  / /_   \__, | |  __/  | | | | | |`,
	` \____|  \___/  /____|  |___/  |_|     |_| |_| |_|`,
}

// Screen is the static welcome screen: CozyPhi wordmark, tagline, help copy.
// Shown while the transcript is empty. It requests no frames — an idle
// welcome screen costs zero terminal writes.
type Screen struct {
	Theme   components.Theme
	Version string // appended to the tagline (e.g. "v0.16.0"); empty hides it
	Hint    string // optional tip under the help line; empty uses the default
}

// Handle is a no-op: the welcome screen has no interaction.
func (*Screen) Handle(*components.EventContext, xui.Event) {}

func (w *Screen) tagline() string {
	if w.Version == "" {
		return "terminal coding agent"
	}
	return "terminal coding agent · " + w.Version
}

func (w *Screen) hintLines() []string {
	if w.Hint != "" {
		return wrapHint(w.Hint, 48)
	}
	return []string{
		"Type a message below and press Enter to start",
	}
}

func wrapHint(s string, width int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var lines []string
	var cur string
	for _, word := range words {
		if cur == "" {
			cur = word
			continue
		}
		if len(cur)+1+len(word) > width {
			lines = append(lines, cur)
			cur = word
			continue
		}
		cur += " " + word
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// Draw renders the centered wordmark (or plain title on a narrow terminal)
// plus tagline, help, and hint copy.
func (w *Screen) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 24
	}
	root := components.NewSurface(maxW, maxH, w)

	th := w.Theme
	if th == (components.Theme{}) {
		th = components.DefaultTheme()
	}

	// Wordmark near-white; only Ctrl+K / ! carry the accent punch.
	brand := xui.Style{Fg: xui.RGBColor(0xe8, 0xec, 0xf2), Bold: true}
	if th.Foreground.Fg.Kind == xui.ColorRGB {
		brand = xui.Style{Fg: th.Foreground.Fg, Bold: true}
	}
	helpKey := th.Success
	if helpKey == (xui.Style{}) {
		helpKey = th.Keybind
	}
	if helpKey == (xui.Style{}) {
		helpKey = xui.Style{Fg: xui.RGBColor(0x7d, 0xc3, 0xa0), Bold: true}
	}
	// Secondary copy: theme muted without Dim. ANSI Dim + bright-black
	// (Terminal IndexedColor 8) is nearly invisible on dark backgrounds.
	body := splashBodyStyle(th)

	type line struct {
		spans []components.Span
		logo  string
	}
	var lines []line
	if wordmarkFits(maxW, maxH, ctx.Method) {
		for _, row := range wordmark {
			lines = append(lines, line{logo: row})
		}
		lines = append(lines,
			line{},
			line{spans: []components.Span{{Text: w.tagline(), Style: body}}},
		)
	} else {
		lines = append(lines,
			line{spans: []components.Span{{Text: brandName, Style: brand}}},
			line{spans: []components.Span{{Text: w.tagline(), Style: body}}},
		)
	}
	lines = append(lines,
		line{},
		line{spans: []components.Span{
			{Text: "Ctrl+K", Style: helpKey},
			{Text: " command palette", Style: body},
			{Text: ", ", Style: body},
			{Text: "!", Style: helpKey},
			{Text: " run a shell command", Style: body},
		}},
	)
	for _, h := range w.hintLines() {
		lines = append(lines, line{spans: []components.Span{{Text: h, Style: body}}})
	}

	y0 := max((maxH-len(lines))/2, 0)
	for i, ln := range lines {
		if ln.logo != "" {
			x := (maxW - xui.StringWidth(ln.logo, ctx.Method)) / 2
			components.PaintSpans(&root, x, y0+i, []components.Span{{Text: ln.logo, Style: brand}}, ctx.Method)
			continue
		}
		if ln.spans == nil {
			continue
		}
		x := (maxW - components.MeasureSpans(ln.spans, ctx.Method)) / 2
		components.PaintSpans(&root, x, y0+i, ln.spans, ctx.Method)
	}
	return root
}

func wordmarkFits(maxW, maxH int, method xui.WidthMethod) bool {
	logoW := 0
	for _, row := range wordmark {
		if w := xui.StringWidth(row, method); w > logoW {
			logoW = w
		}
	}
	return logoW+2 <= maxW && len(wordmark)+4 <= maxH
}

// splashBodyStyle is readable secondary copy for the hero — theme muted
// without Dim, lifting ANSI bright-black to a mid gray.
func splashBodyStyle(th components.Theme) xui.Style {
	st := th.Muted
	st.Dim = false
	if st.Fg.Kind == xui.ColorIndex && st.Fg.Index <= 8 {
		st.Fg = xui.IndexedColor(245)
	}
	if st.Fg.Kind == 0 {
		st.Fg = xui.RGBColor(0xa8, 0xb2, 0xc0)
	}
	return st
}
