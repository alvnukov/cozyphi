package compaction

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/pulseaiclub/phi/internal/llm"
)

var (
	//go:embed compaction-summary.tmpl
	compactionSummaryPrompt string
	//go:embed compaction-update-summary.tmpl
	compactionUpdateSummaryPrompt string
)

// generateSummary generates a summary of the conversation history using an LLM.
// If a previousSummary is provided, it will update the existing summary with new messages.
// Otherwise, it generates an initial summary from the currentMessages.
func generateSummary(
	ctx context.Context,
	llm llm.Compactor,
	currentMessages []llm.Message,
	previousSummary string,
) (string, error) {
	basePrompt := compactionSummaryPrompt
	if previousSummary != "" {
		basePrompt = compactionUpdateSummaryPrompt
	}

	conversation := SerializeConversation(currentMessages)
	promptText := fmt.Sprintf("<conversation>\n%s\n</conversation>", conversation)
	if previousSummary != "" {
		promptText += fmt.Sprintf("<previous-summary>\n%s\n</previous-summary>", previousSummary)
	}
	promptText += basePrompt
	return llm.Compact(ctx, promptText)
}

// generateTurnPrefixSummary generates a summary specifically for turn prefixes (prefix-based compaction).
// It creates a summary from the provided messages without considering any previous summary.
func generateTurnPrefixSummary(
	ctx context.Context,
	llm llm.Compactor,
	messages []llm.Message,
) (string, error) {
	conversation := SerializeConversation(messages)
	promptText := fmt.Sprintf("<conversation>\n%s\n</conversation>", conversation)
	return llm.Compact(ctx, promptText)
}
