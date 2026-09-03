//go:build darwin

package notify

import (
	"context"
	"strings"
	"testing"
)

// recordArgs returns a runner that captures the argv it was handed.
func recordArgs(got *[]string) commandRunner {
	return func(_ context.Context, _ string, args ...string) error {
		*got = args
		return nil
	}
}

func TestDarwinSenderPassesTextAsArgv(t *testing.T) {
	// A hostile title that would break out of an interpolated AppleScript
	// string; it must survive as an inert argv element instead.
	title := "cozyphi\") with title \"evil"
	body := "line one\nline two"

	var gotArgs []string
	if err := darwinSend(t.Context(), recordArgs(&gotArgs), "", title, body); err != nil {
		t.Fatalf("darwin sender: %v", err)
	}

	assertArgs(t, gotArgs, []string{"-e", osascriptScript, "--", title, body})
	if strings.Contains(osascriptScript, title) || strings.Contains(osascriptScript, body) {
		t.Fatal("notification text interpolated into the AppleScript template")
	}
}

// The sound name rides as argv too, read by a template that names its
// position; the silent sender keeps the template without one.
func TestDarwinSenderPlaysTheSoundFromArgv(t *testing.T) {
	sound := "Purr\") sound name \"evil"

	var gotArgs []string
	if err := darwinSend(t.Context(), recordArgs(&gotArgs), sound, "cozyphi", "done"); err != nil {
		t.Fatalf("darwin sender: %v", err)
	}

	assertArgs(t, gotArgs, []string{"-e", osascriptSoundScript, "--", "cozyphi", "done", sound})
	if strings.Contains(osascriptSoundScript, sound) {
		t.Fatal("sound name interpolated into the AppleScript template")
	}
	if !strings.Contains(osascriptSoundScript, "sound name (item 3 of argv)") {
		t.Fatalf("sound template does not read the sound from argv: %q", osascriptSoundScript)
	}
}

func assertArgs(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
