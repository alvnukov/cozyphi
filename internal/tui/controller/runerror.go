package controller

import (
	"regexp"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// runErrorText composes the transcript message for a failed run: what went
// wrong in plain language with the action that fixes it, then the raw error
// as the detail, then the retry path — the composer's ↑ history keeps the
// prompt, so nothing has to be retyped.
func runErrorText(err error) string {
	headline := classifyRunError(err)
	if headline == "" {
		headline = "The run failed."
	}
	return headline + "\n\n" + err.Error() +
		"\n\nPress ↑ in the composer to recall the prompt and retry."
}

// retryAfterRe pulls a "retry after 12s" style delay out of a rate-limit
// body, whichever provider phrasing carries it.
var retryAfterRe = regexp.MustCompile(`(?i)(?:retry|try again)\D{0,20}?(\d+(?:\.\d+)?)\s*s`)

// classifyRunError names the failure's cause and the action to take, or ""
// when the error fits no known cause. Provider rejections arrive typed —
// llm.StatusError carries the HTTP status (llm.IsRateLimited /
// llm.IsAuthFailure) and cancellation stays errors.Is-able — so the branch
// is on the code first. Only transport failures, which speak through the
// Go net stack's message text, still fall back to phrase matching.
func classifyRunError(err error) string {
	if llm.IsContextOverflow(err) {
		return "The context overflowed — run /compact to shrink the history, then retry."
	}
	switch {
	case llm.IsCanceled(err):
		return "The run was canceled."
	case llm.IsAuthFailure(err):
		return "The provider rejected the credentials — run /connect to fix the API key."
	case llm.IsRateLimited(err):
		if m := retryAfterRe.FindStringSubmatch(err.Error()); m != nil {
			return "The provider is rate limiting — retry in about " + m[1] + "s."
		}
		return "The provider is rate limiting — wait a moment and retry."
	}
	text := strings.ToLower(err.Error())
	switch {
	case containsAny(text, "invalid api key", "invalid x-api-key",
		"unauthorized", "authentication_error", "permission_error"):
		return "The provider rejected the credentials — run /connect to fix the API key."
	case containsAny(text, "rate limit", "rate_limit",
		"too many requests", "overloaded", "quota"):
		if m := retryAfterRe.FindStringSubmatch(err.Error()); m != nil {
			return "The provider is rate limiting — retry in about " + m[1] + "s."
		}
		return "The provider is rate limiting — wait a moment and retry."
	case containsAny(text, "connection refused", "dial tcp", "no such host",
		"i/o timeout", "connection reset", "network is unreachable",
		"tls handshake", "context deadline exceeded"):
		return "The provider is unreachable — check the network and the base URL, then retry."
	default:
		return ""
	}
}

func containsAny(text string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
