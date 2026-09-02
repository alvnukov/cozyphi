//go:build darwin

package notify

import "context"

// DefaultSound is what a notification plays unless the config picks another
// sound: a soft system sound that reads as "done", not as an alarm. Any name
// from /System/Library/Sounds or ~/Library/Sounds works.
const DefaultSound = "Purr"

// The scripts are fixed templates: title, body and sound name arrive as argv
// after `--` (on run argv), never interpolated into the AppleScript source —
// notification text comes from tool calls and transcripts and must not be
// parsed as script.
const (
	osascriptScript = `on run argv
	display notification (item 2 of argv) with title (item 1 of argv)
end run`
	osascriptSoundScript = `on run argv
	display notification (item 2 of argv) with title (item 1 of argv) sound name (item 3 of argv)
end run`
)

func platformSender(sound string) sendFunc {
	return darwinSender(runCommand, sound)
}

// darwinSender renders one notification through osascript; an empty sound
// keeps it silent.
func darwinSender(run commandRunner, sound string) sendFunc {
	return func(ctx context.Context, title, body string) error {
		if sound == "" {
			return run(ctx, "osascript", "-e", osascriptScript, "--", title, body)
		}
		return run(ctx, "osascript", "-e", osascriptSoundScript, "--", title, body, sound)
	}
}
