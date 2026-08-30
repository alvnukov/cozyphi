//go:build darwin

package notify

import (
	"context"
	"strings"
	"testing"
)

func TestDarwinSenderPassesTextAsArgv(t *testing.T) {
	// A hostile title that would break out of an interpolated AppleScript
	// string; it must survive as an inert argv element instead.
	title := "cozyphi\") with title \"evil"
	body := "line one\nline two"

	var gotArgs []string
	run := func(_ context.Context, _ string, args ...string) error {
		gotArgs = args
		return nil
	}
	if err := darwinSender(run)(t.Context(), title, body); err != nil {
		t.Fatalf("darwin sender: %v", err)
	}

	want := []string{"-e", osascriptScript, "--", title, body}
	if len(gotArgs) != len(want) {
		t.Fatalf("argv = %q, want %q", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, gotArgs[i], want[i])
		}
	}
	if strings.Contains(osascriptScript, title) || strings.Contains(osascriptScript, body) {
		t.Fatal("notification text interpolated into the AppleScript template")
	}
}
