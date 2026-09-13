package components

import "testing"

func TestPlainTextStripsTerminalSequences(t *testing.T) {
	cases := map[string]struct{ input, want string }{
		"CSI":            {"a\x1b[31mred\x1b[0mb", "aredb"},
		"OSC BEL":        {"a\x1b]52;c;c2VjcmV0\ab", "ab"},
		"OSC ST":         {"a\x1b]8;;https://example.test\x1b\\link\x1b]8;;\x1b\\b", "alinkb"},
		"DCS":            {"a\x1bPpayload\x1b\\b", "ab"},
		"C1":             {"a\u009b31mred\u009dtitle\u009cb", "aredb"},
		"incomplete CSI": {"a\x1b[31", "a"},
		"incomplete OSC": {"a\x1b]52;payload", "a"},
		"incomplete DCS": {"a\x1bPpayload\x1b", "a"},
		"controls":       {"a\rb\a\tc\n", "ab c\n"},
		"Unicode":        {"Привет 👩‍💻 é 漢字", "Привет 👩‍💻 é 漢字"},
	}
	for name, tt := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PlainText(tt.input); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
