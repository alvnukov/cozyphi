package llm

import "testing"

// TestSniffProtocol pins the single sniffing heuristic: it exists only as a
// compatibility fallback for models that never declared a protocol, so the
// table doubles as the contract the config-boundary warning leans on.
func TestSniffProtocol(t *testing.T) {
	tests := []struct {
		name    string
		model   string
		baseURL string
		want    Protocol
	}{
		{"claude prefix means anthropic wire", "claude-3-5-sonnet", "", ProtocolAnthropic},
		{"prefix is case- and space-insensitive", "  Claude-Opus ", "", ProtocolAnthropic},
		{"anthropic in the host means anthropic wire", "custom-name", "https://api.anthropic.com", ProtocolAnthropic},
		{"openai-compatible gateway defaults to openai", "gateway-claude", "https://llm.corp/v1", ProtocolOpenAI},
		{"plain openai setup", "gpt-4o", "https://api.openai.com/v1", ProtocolOpenAI},
		{"no hints at all", "", "", ProtocolOpenAI},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := SniffProtocol(tt.model, tt.baseURL); got != tt.want {
				t.Fatalf("SniffProtocol(%q, %q) = %q, want %q", tt.model, tt.baseURL, got, tt.want)
			}
		})
	}
}
