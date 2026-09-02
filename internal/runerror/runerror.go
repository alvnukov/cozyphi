// Package runerror classifies a failed agent run into a cause a person can
// act on. Two surfaces report such a failure — the TUI transcript and
// `cozyphi run` on stderr — and both read the cause and its sentence from
// here, so a hint cannot drift between them.
//
// The remedy is the caller's, because it genuinely differs: /connect and
// /compact exist only in the TUI, and a headless run has no composer to
// recall a prompt from. Everything a surface would phrase the same way is
// part of the shared message.
package runerror

import (
	"regexp"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// Cause names what went wrong, for callers that branch on the failure
// rather than print it.
type Cause int

const (
	// CauseUnknown is an error that fits no known cause; its message is empty
	// and the raw error is all a surface can show.
	CauseUnknown Cause = iota
	CauseCanceled
	CauseAuth
	CauseRateLimit
	CauseUnreachable
	CauseContextOverflow
)

// Classification is the cause plus the sentence naming it. Message carries
// no surface-specific instruction, so any caller can print it as is.
type Classification struct {
	Cause   Cause
	Message string
}

// Remedies are the actions a surface can offer for the causes whose fix
// depends on where the failure is reported. A remedy left empty prints the
// bare message.
type Remedies struct {
	Auth            string
	ContextOverflow string
}

// retryAfterRe pulls a "retry after 12s" style delay out of a rate-limit
// body, whichever provider phrasing carries it.
var retryAfterRe = regexp.MustCompile(`(?i)(?:retry|try again)\D{0,20}?(\d+(?:\.\d+)?)\s*s`)

// Classify names the failure's cause. Provider rejections arrive typed —
// llm.StatusError carries the HTTP status (llm.IsRateLimited /
// llm.IsAuthFailure) and cancellation stays errors.Is-able — so the branch
// is on the code first. Only transport failures, which speak through the Go
// net stack's message text, still fall back to phrase matching.
func Classify(err error) Classification {
	if err == nil {
		return Classification{}
	}
	if llm.IsContextOverflow(err) {
		return Classification{Cause: CauseContextOverflow, Message: "The context overflowed."}
	}
	switch {
	case llm.IsCanceled(err):
		return Classification{Cause: CauseCanceled, Message: "The run was canceled."}
	case llm.IsAuthFailure(err):
		return Classification{Cause: CauseAuth, Message: "The provider rejected the credentials."}
	case llm.IsRateLimited(err):
		return rateLimited(err)
	}
	text := strings.ToLower(err.Error())
	switch {
	case containsAny(text, "invalid api key", "invalid x-api-key",
		"unauthorized", "authentication_error", "permission_error"):
		return Classification{Cause: CauseAuth, Message: "The provider rejected the credentials."}
	case containsAny(text, "rate limit", "rate_limit",
		"too many requests", "overloaded", "quota"):
		return rateLimited(err)
	case containsAny(text, "connection refused", "dial tcp", "no such host",
		"i/o timeout", "connection reset", "network is unreachable",
		"tls handshake", "context deadline exceeded"):
		return Classification{
			Cause:   CauseUnreachable,
			Message: "The provider is unreachable — check the network and the base URL, then retry.",
		}
	default:
		return Classification{}
	}
}

// Hint is the sentence a surface shows: the shared message, followed by the
// caller's remedy where the cause has one. It is empty when the error fits
// no known cause, which is the caller's cue to fall back to its own wording.
func Hint(err error, remedies Remedies) string {
	c := Classify(err)
	remedy := ""
	switch c.Cause {
	case CauseAuth:
		remedy = remedies.Auth
	case CauseContextOverflow:
		remedy = remedies.ContextOverflow
	case CauseUnknown, CauseCanceled, CauseRateLimit, CauseUnreachable:
		// The message already says everything a surface could add.
	}
	if c.Message == "" || remedy == "" {
		return c.Message
	}
	return c.Message + " " + remedy
}

// rateLimited quotes the provider's own delay when the body carries one:
// "wait a moment" is the fallback, not a better answer than a number.
func rateLimited(err error) Classification {
	if m := retryAfterRe.FindStringSubmatch(err.Error()); m != nil {
		return Classification{
			Cause:   CauseRateLimit,
			Message: "The provider is rate limiting — retry in about " + m[1] + "s.",
		}
	}
	return Classification{
		Cause:   CauseRateLimit,
		Message: "The provider is rate limiting — wait a moment and retry.",
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
