package llm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// The typed-status contract: providers speak through APIError, and consumers
// (TUI run errors, `phi run`) must branch on what happened — cancel, rate
// limit, auth — without grepping message text. Text matching is the fallback
// for transport failures, not the primary signal.

func TestAPIErrorExposesStatus(t *testing.T) {
	err := APIError("LLM API error", http.StatusTooManyRequests, []byte("slow down"))

	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("expected *StatusError in the chain, got %T", err)
	}
	if se.Status != http.StatusTooManyRequests {
		t.Fatalf("status: got %d, want 429", se.Status)
	}
	if !strings.Contains(err.Error(), "429") {
		t.Fatalf("status must stay readable in the text, got %q", err.Error())
	}
}

func TestRateLimitAndAuthClassification(t *testing.T) {
	rate := APIError("LLM API error", http.StatusTooManyRequests, []byte("rate limit"))
	overloaded := APIError("anthropic API error", 529, []byte("overloaded"))
	auth := APIError("anthropic API error", http.StatusUnauthorized, []byte("bad key"))
	forbidden := APIError("LLM API error", http.StatusForbidden, []byte("no"))
	other := APIError("LLM API error", http.StatusInternalServerError, []byte("boom"))
	wrapped := fmt.Errorf("round: %w", rate)

	if !IsRateLimited(rate) || !IsRateLimited(overloaded) || !IsRateLimited(wrapped) {
		t.Fatalf("rate-limit classification lost: %v / %v / %v", rate, overloaded, wrapped)
	}
	if !IsAuthFailure(auth) || !IsAuthFailure(forbidden) {
		t.Fatalf("auth classification lost: %v / %v", auth, forbidden)
	}
	if IsRateLimited(auth) || IsAuthFailure(rate) {
		t.Fatal("429 and 401 must not cross-classify")
	}
	if IsRateLimited(other) || IsAuthFailure(other) {
		t.Fatalf("a 500 is neither rate limit nor auth: %v", other)
	}
}

func TestCancelClassification(t *testing.T) {
	// The transport wraps cancellation; errors.Is must still see it.
	inner := fmt.Errorf("send request: %w", context.Canceled)
	if !IsCanceled(inner) {
		t.Fatalf("canceled chain lost: %v", inner)
	}
	if IsCanceled(errors.New("unrelated")) {
		t.Fatal("unrelated error classified as cancel")
	}
}

func TestStatusErrorPreservesOverflow(t *testing.T) {
	err := APIError("LLM API error", http.StatusRequestEntityTooLarge, []byte("context_length_exceeded"))
	if !IsContextOverflow(err) {
		t.Fatalf("overflow marker must survive the status wrapper: %v", err)
	}
}
