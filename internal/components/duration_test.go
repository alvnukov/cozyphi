package components

import (
	"testing"
	"time"
)

// TestFormatDuration pins opencode-style wall-time rendering shared by the
// turn footer and the thinking header: 4s, 1m 4s, 1h 2m; nothing to say
// for zero or negative spans.
func TestFormatDuration(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{0, ""},
		{-time.Second, ""},
		{500 * time.Millisecond, "0s"},
		{4 * time.Second, "4s"},
		{59 * time.Second, "59s"},
		{time.Minute + 4*time.Second, "1m 4s"},
		{94 * time.Second, "1m 34s"},
		{time.Hour + 2*time.Minute, "1h 2m"},
	}
	for _, tc := range cases {
		if got := FormatDuration(tc.d); got != tc.want {
			t.Errorf("FormatDuration(%v) = %q, want %q", tc.d, got, tc.want)
		}
	}
}
