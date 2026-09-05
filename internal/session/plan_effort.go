package session

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// NormalizePlanEffort validates an independent step override; empty inherits.
func NormalizePlanEffort(value string) (string, error) {
	effort, ok := llm.ParseReasoningEffort(value)
	if !ok {
		return "", fmt.Errorf(
			"plan effort %q is invalid; use none, minimal, low, medium, high, xhigh, max, or clear it",
			value,
		)
	}
	return string(effort), nil
}
