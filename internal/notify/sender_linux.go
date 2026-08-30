//go:build linux

package notify

import "context"

func platformSender() sendFunc {
	return linuxSender(runCommand)
}

// linuxSender renders one notification through notify-send; title and body
// ride as plain argv, never through a shell.
func linuxSender(run commandRunner) sendFunc {
	return func(ctx context.Context, title, body string) error {
		return run(ctx, "notify-send", title, body)
	}
}
