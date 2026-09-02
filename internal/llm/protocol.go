package llm

import "strings"

// SniffProtocol is the one compatibility heuristic that guesses a wire
// protocol for a model that never declared one: a "claude*" name or an
// "anthropic" base URL means the Anthropic wire format, anything else means
// OpenAI. It exists only as a fallback behind a warning — an OpenAI-compatible
// gateway can serve a model named claude-* and must stay on the OpenAI wire
// format when the config says so explicitly.
func SniffProtocol(model, baseURL string) Protocol {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(model)), "claude") ||
		strings.Contains(strings.ToLower(baseURL), "anthropic") {
		return ProtocolAnthropic
	}
	return ProtocolOpenAI
}
