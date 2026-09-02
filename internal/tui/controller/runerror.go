package controller

import (
	"github.com/alvnukov/cozyphi/internal/runerror"
)

// tuiRemedies are the fixes this surface can offer: both are slash commands,
// which exist nowhere else, so the shared classifier does not know them.
var tuiRemedies = runerror.Remedies{
	Auth:            "Run /connect to fix the API key.",
	ContextOverflow: "Run /compact to shrink the history, then retry.",
}

// runErrorText composes the transcript message for a failed run: what went
// wrong in plain language with the action that fixes it, then the raw error
// as the detail, then the retry path — the composer's ↑ history keeps the
// prompt, so nothing has to be retyped.
func runErrorText(err error) string {
	headline := runerror.Hint(err, tuiRemedies)
	if headline == "" {
		headline = "The run failed."
	}
	return headline + "\n\n" + err.Error() +
		"\n\nPress ↑ in the composer to recall the prompt and retry."
}
