//go:build darwin

package notify

import "context"

// osascriptScript is a fixed template: title and body arrive as argv after
// `--` (on run argv), never interpolated into the AppleScript source —
// notification text comes from tool calls and transcripts and must not be
// parsed as script.
const osascriptScript = `on run argv
	display notification (item 2 of argv) with title (item 1 of argv)
end run`

func platformSender() sendFunc {
	return darwinSender(runCommand)
}

// darwinSender renders one notification through osascript.
func darwinSender(run commandRunner) sendFunc {
	return func(ctx context.Context, title, body string) error {
		return run(ctx, "osascript", "-e", osascriptScript, "--", title, body)
	}
}
