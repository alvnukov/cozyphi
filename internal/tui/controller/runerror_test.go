package controller

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// wireErr quotes a provider or net-stack error verbatim; the fixtures keep
// the real casing and punctuation the classifier has to match.
func wireErr(s string) error { return errors.New(s) }

func TestClassifyRunErrorNamesTheCauseAndTheAction(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "context overflow points at /compact",
			err:  fmt.Errorf("anthropic: %w", llm.ErrContextOverflow),
			want: "/compact",
		},
		{
			name: "401 points at /connect",
			err:  llm.APIError("anthropic", 401, []byte("authentication_error: invalid x-api-key")),
			want: "/connect",
		},
		{
			name: "403 points at /connect",
			err:  llm.APIError("openai", 403, []byte("permission denied")),
			want: "/connect",
		},
		{
			name: "429 names the rate limit",
			err:  llm.APIError("anthropic", 429, []byte("rate_limit_error")),
			want: "rate limiting",
		},
		{
			name: "529 overloaded is a rate limit too",
			err:  llm.APIError("anthropic", 529, []byte("overloaded_error")),
			want: "rate limiting",
		},
		{
			name: "retry-after seconds surface in the headline",
			err:  llm.APIError("openai", 429, []byte("Rate limit reached, please try again in 12s.")),
			want: "retry in about 12s",
		},
		{
			name: "dns failure names the network",
			err: wireErr(
				`Post "https://api.anthropic.com/v1/messages": dial tcp: lookup api.anthropic.com: no such host`,
			),
			want: "unreachable",
		},
		{
			name: "connection refused names the network",
			err:  wireErr(`Post "http://localhost:11434": dial tcp 127.0.0.1:11434: connect: connection refused`),
			want: "unreachable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, classifyRunError(tt.err), tt.want)
		})
	}
}

func TestClassifyRunErrorLeavesTheUnknownAlone(t *testing.T) {
	assert.Empty(t, classifyRunError(errors.New("stream ended unexpectedly")))
}

func TestRunErrorTextKeepsTheRawErrorAndNamesTheRetryPath(t *testing.T) {
	text := runErrorText(llm.APIError("anthropic", 401, []byte("invalid x-api-key")))

	assert.True(t, strings.HasPrefix(text, "The provider rejected the credentials"), text)
	assert.Contains(t, text, "anthropic: (401) invalid x-api-key")
	assert.Contains(t, text, "↑")
}

func TestRunErrorTextFallsBackToAPlainHeadline(t *testing.T) {
	text := runErrorText(errors.New("stream ended unexpectedly"))

	assert.True(t, strings.HasPrefix(text, "The run failed."), text)
	assert.Contains(t, text, "stream ended unexpectedly")
}
