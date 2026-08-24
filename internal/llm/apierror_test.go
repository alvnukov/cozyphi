package llm

import (
	"errors"
	"strings"
	"testing"
)

func TestAPIErrorMarksContextOverflow(t *testing.T) {
	tests := []struct {
		name     string
		status   int
		body     string
		overflow bool
	}{
		{
			name:     "openai code",
			status:   400,
			body:     `{"error":{"message":"x","code":"context_length_exceeded"}}`,
			overflow: true,
		},
		{
			name:     "anthropic prompt too long",
			status:   400,
			body:     `{"error":{"message":"prompt is too long: 200000 tokens"}}`,
			overflow: true,
		},
		{
			name:     "openai maximum context length",
			status:   400,
			body:     `{"error":{"message":"This model's maximum context length is 128000 tokens."}}`,
			overflow: true,
		},
		{name: "entity too large", status: 413, body: `{"error":{"message":"request too large"}}`, overflow: true},
		{
			name:     "ordinary bad request",
			status:   400,
			body:     `{"error":{"message":"invalid_request_error"}}`,
			overflow: false,
		},
		{name: "unauthorized", status: 401, body: `{"error":{"message":"invalid api key"}}`, overflow: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := APIError("LLM API error", tt.status, []byte(tt.body))
			if got := IsContextOverflow(err); got != tt.overflow {
				t.Fatalf("IsContextOverflow()=%v, want %v (err=%v)", got, tt.overflow, err)
			}
		})
	}
}

func TestMarkContextOverflowPreservesOriginalError(t *testing.T) {
	base := errors.New("responses stream error (context_length_exceeded): too big")
	marked := MarkContextOverflow(base, "context_length_exceeded")
	if !IsContextOverflow(marked) {
		t.Fatalf("expected overflow error, got %v", marked)
	}
	if !strings.Contains(marked.Error(), base.Error()) {
		t.Fatalf("expected original text preserved, got %v", marked)
	}

	plain := MarkContextOverflow(errors.New("decode failure"), "decode failure")
	if IsContextOverflow(plain) {
		t.Fatalf("unexpected overflow classification for %v", plain)
	}
}
