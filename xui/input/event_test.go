package input

import "testing"

func TestKeyEventHotkeyRune(t *testing.T) {
	cases := []struct {
		name string
		ev   KeyEvent
		want rune
	}{
		{"russian plain k", KeyEvent{Code: KeyRune, Rune: 'л'}, 'k'},
		{"russian capital K", KeyEvent{Code: KeyRune, Rune: 'Л'}, 'K'},
		{"kitty alternate wins", KeyEvent{Code: KeyRune, Rune: 'л', AltRune: 'k'}, 'k'},
		{"latin passthrough", KeyEvent{Code: KeyRune, Rune: 'K'}, 'K'},
		{"non-letter untouched", KeyEvent{Code: KeyRune, Rune: '1'}, '1'},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.ev.HotkeyRune(); got != tc.want {
				t.Fatalf("HotkeyRune() = %q, want %q", got, tc.want)
			}
		})
	}
}
