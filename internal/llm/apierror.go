package llm

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// ErrContextOverflow marks a provider rejection caused by the request
// exceeding the model's context window. The engine can recover from it by
// compacting the session and retrying the request.
var ErrContextOverflow = errors.New("context window exceeded")

// IsContextOverflow reports whether err (or any wrapped cause) is a
// context-overflow rejection.
func IsContextOverflow(err error) bool {
	return errors.Is(err, ErrContextOverflow)
}

// overflowMarkers are provider phrases observed in context-overflow rejection
// bodies. Matching is case-insensitive and substring-based, and the markers
// are deliberately narrow so an ordinary error is never misclassified.
var overflowMarkers = []string{
	"context_length_exceeded",
	"maximum context length",
	"prompt is too long",
	"input is too long",
	"too many tokens",
	"exceeds the maximum context",
	"exceeds the context",
}

// MarkContextOverflow wraps err as a context-overflow rejection when message
// matches a known provider phrase; otherwise it returns err unchanged. It is
// the single seam providers use to attach the recoverable-error signal without
// losing the original error text.
func MarkContextOverflow(err error, message string) error {
	if err == nil {
		return nil
	}
	if looksLikeOverflow(message) {
		return fmt.Errorf("%w: %w", ErrContextOverflow, err)
	}
	return err
}

// APIError builds a provider API error from an HTTP status and response body.
// A 413 (Request Entity Too Large) is always treated as overflow; other
// statuses are classified from the body text.
func APIError(prefix string, status int, body []byte) error {
	err := fmt.Errorf("%s: (%d) %s", prefix, status, body)
	if status == http.StatusRequestEntityTooLarge {
		return fmt.Errorf("%w: %w", ErrContextOverflow, err)
	}
	return MarkContextOverflow(err, string(body))
}

// MaxErrorBodyBytes caps how much of a non-200 response body is read before
// it reaches APIError: a hostile endpoint must not be able to size an error
// page that OOMs the harness.
const MaxErrorBodyBytes = 64 * 1024

// MaxResponseBytes caps a non-streaming response body read (compaction
// summaries): a hostile endpoint streaming an endless document must not OOM
// the reader either.
const MaxResponseBytes = 16 * 1024 * 1024

// ReadErrorBody reads up to MaxErrorBodyBytes of an error response body.
func ReadErrorBody(body io.Reader) []byte {
	raw, _ := io.ReadAll(io.LimitReader(body, MaxErrorBodyBytes))
	return raw
}

func looksLikeOverflow(message string) bool {
	lower := strings.ToLower(message)
	for _, marker := range overflowMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
