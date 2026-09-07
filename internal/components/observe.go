package components

import "github.com/alvnukov/cozyphi/internal/diag"

// ObserveTheme projects the palettes a surface paints with into the shape the
// diagnostics registry reports. Only names travel: a palette is a table of
// colors and attributes, and none of it is anybody's business outside the
// renderer.
//
// It observes and returns. It resolves no palette, repaints nothing and
// queues no refresh — asking which theme is in force must never be the thing
// that changes it.
func ObserveTheme(boot, live Theme) diag.ThemeFacts {
	return diag.ThemeFacts{
		Known:   true,
		Builtin: ThemeNames(),
		Boot:    boot.Name,
		Live:    live.Name,
	}
}
