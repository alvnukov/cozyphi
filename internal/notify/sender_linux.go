//go:build linux

package notify

import "context"

// DefaultSound is the freedesktop sound-theme name a notification asks the
// daemon to play unless the config picks another; a daemon without sound
// support ignores the hint.
const DefaultSound = "message-new-instant"

func platformSend(ctx context.Context, sound, title, body string) error {
	return linuxSend(ctx, runCommand, sound, title, body)
}

// linuxSend renders one notification through notify-send; title and body
// ride as plain argv, never through a shell. A sound travels as the
// sound-name hint, an empty one asks for nothing.
func linuxSend(ctx context.Context, run commandRunner, sound, title, body string) error {
	if sound == "" {
		return run(ctx, "notify-send", title, body)
	}
	return run(ctx, "notify-send", "--hint=string:sound-name:"+sound, title, body)
}
