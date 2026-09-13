package footer

import "fmt"

// SetShellActivity supplies UI-owned counts and the current keymap's hint.
func (f *FooterChrome) SetShellActivity(count int, hint string) {
	if f != nil {
		f.shellCount, f.shellHint = count, hint
	}
}

func (f *FooterChrome) shellLabel() string {
	if f.shellCount == 0 {
		return ""
	}
	return fmt.Sprintf("%d shell · /tasks", f.shellCount)
}
