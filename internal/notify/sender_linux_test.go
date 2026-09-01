//go:build linux

package notify

import (
	"context"
	"testing"
)

func TestLinuxSenderPassesTextAsArgv(t *testing.T) {
	title := "cozyphi; rm -rf /"
	body := "waiting for input"

	var gotName string
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) error {
		gotName = name
		gotArgs = args
		return nil
	}
	if err := linuxSender(run)(t.Context(), title, body); err != nil {
		t.Fatalf("linux sender: %v", err)
	}
	if gotName != "notify-send" {
		t.Errorf("command = %q, want notify-send", gotName)
	}
	if len(gotArgs) != 2 || gotArgs[0] != title || gotArgs[1] != body {
		t.Errorf("argv = %q, want [%q %q]", gotArgs, title, body)
	}
}
