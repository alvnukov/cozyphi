//go:build linux

package notify

import (
	"context"
	"testing"
)

// recordArgs returns a runner that captures the command and argv it was handed.
func recordArgs(gotName *string, gotArgs *[]string) commandRunner {
	return func(_ context.Context, name string, args ...string) error {
		*gotName = name
		*gotArgs = args
		return nil
	}
}

func TestLinuxSenderPassesTextAsArgv(t *testing.T) {
	title := "cozyphi; rm -rf /"
	body := "waiting for input"

	var gotName string
	var gotArgs []string
	if err := linuxSend(t.Context(), recordArgs(&gotName, &gotArgs), "", title, body); err != nil {
		t.Fatalf("linux sender: %v", err)
	}
	if gotName != "notify-send" {
		t.Errorf("command = %q, want notify-send", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != title || gotArgs[1] != body {
		t.Errorf("argv = %q, want [%q %q]", gotArgs, title, body)
	}
}

// A sound becomes the daemon's sound-name hint, ahead of the text.
func TestLinuxSenderAsksForTheSoundByHint(t *testing.T) {
	var gotName string
	var gotArgs []string
	if err := linuxSend(
		t.Context(),
		recordArgs(&gotName, &gotArgs),
		"message-new-instant",
		"cozyphi",
		"done",
	); err != nil {
		t.Fatalf("linux sender: %v", err)
	}
	want := []string{"--hint=string:sound-name:message-new-instant", "cozyphi", "done"}
	if len(gotArgs) != len(want) {
		t.Fatalf("argv = %q, want %q", gotArgs, want)
	}
	for i := range want {
		if gotArgs[i] != want[i] {
			t.Errorf("argv[%d] = %q, want %q", i, gotArgs[i], want[i])
		}
	}
}
