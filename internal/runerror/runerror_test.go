package runerror

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// wireErr quotes a provider or net-stack error verbatim; the fixtures keep
// the real casing and punctuation the classifier has to match.
func wireErr(s string) error { return errors.New(s) }

func TestClassifyNamesTheCause(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want Cause
		says string
	}{
		{
			name: "context overflow",
			err:  fmt.Errorf("anthropic: %w", llm.ErrContextOverflow),
			want: CauseContextOverflow,
			says: "context overflowed",
		},
		{
			name: "401 is a credential rejection",
			err:  llm.APIError("anthropic", 401, []byte("authentication_error: invalid x-api-key")),
			want: CauseAuth,
			says: "rejected the credentials",
		},
		{
			name: "403 is a credential rejection",
			err:  llm.APIError("openai", 403, []byte("permission denied")),
			want: CauseAuth,
			says: "rejected the credentials",
		},
		{
			name: "429 is a rate limit",
			err:  llm.APIError("anthropic", 429, []byte("rate_limit_error")),
			want: CauseRateLimit,
			says: "rate limiting",
		},
		{
			name: "529 overloaded is a rate limit too",
			err:  llm.APIError("anthropic", llm.StatusOverloaded, []byte("overloaded_error")),
			want: CauseRateLimit,
			says: "rate limiting",
		},
		{
			name: "retry-after seconds surface in the message",
			err:  llm.APIError("openai", 429, []byte("Rate limit reached, please try again in 12s.")),
			want: CauseRateLimit,
			says: "retry in about 12s",
		},
		{
			name: "dns failure names the network",
			err: wireErr(
				`Post "https://api.anthropic.com/v1/messages": dial tcp: lookup api.anthropic.com: no such host`,
			),
			want: CauseUnreachable,
			says: "unreachable",
		},
		{
			name: "connection refused names the network",
			err:  wireErr(`Post "http://localhost:11434": dial tcp 127.0.0.1:11434: connect: connection refused`),
			want: CauseUnreachable,
			says: "unreachable",
		},
		{
			name: "an untyped credential rejection still classifies",
			err:  wireErr("LLM API error: invalid api key provided"),
			want: CauseAuth,
			says: "rejected the credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.err)
			assert.Equal(t, tt.want, got.Cause)
			assert.Contains(t, got.Message, tt.says)
		})
	}
}

func TestClassifyLeavesTheUnknownAlone(t *testing.T) {
	got := Classify(errors.New("stream ended unexpectedly"))

	assert.Equal(t, CauseUnknown, got.Cause)
	assert.Empty(t, got.Message)
	assert.Empty(t, Classify(nil).Message)
}

// The shared message never names a slash command: a surface that has none
// must be able to print it as it stands.
func TestClassifyMessagesCarryNoSurfaceSpecificAction(t *testing.T) {
	errs := []error{
		fmt.Errorf("anthropic: %w", llm.ErrContextOverflow),
		llm.APIError("anthropic", 401, []byte("invalid x-api-key")),
		llm.APIError("anthropic", 429, []byte("rate_limit_error")),
		wireErr("dial tcp: no such host"),
	}
	for _, err := range errs {
		assert.NotContains(t, Classify(err).Message, "/")
	}
}

func TestHintAppendsTheCallersRemedy(t *testing.T) {
	remedies := Remedies{Auth: "Run /connect.", ContextOverflow: "Run /compact."}

	assert.Equal(t,
		"The provider rejected the credentials. Run /connect.",
		Hint(llm.APIError("anthropic", 401, []byte("nope")), remedies))
	assert.Equal(t,
		"The context overflowed. Run /compact.",
		Hint(fmt.Errorf("anthropic: %w", llm.ErrContextOverflow), remedies))
}

// A cause the caller offered no remedy for prints the bare message rather
// than a sentence with a hole in it.
func TestHintWithoutARemedyIsTheBareMessage(t *testing.T) {
	assert.Equal(t,
		"The provider rejected the credentials.",
		Hint(llm.APIError("anthropic", 401, []byte("nope")), Remedies{}))
	assert.Equal(t,
		"The run was canceled.",
		Hint(fmt.Errorf("wrapped: %w", context.Canceled), Remedies{Auth: "unused"}))
	assert.Empty(t, Hint(errors.New("stream ended unexpectedly"), Remedies{Auth: "unused"}))
}
