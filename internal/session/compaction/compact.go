package compaction

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

// CompactionPreparation holds everything Compact needs: the entries to
// summarize, the recent messages kept, and the file operations captured
// from the summarized history.
type CompactionPreparation struct {
	FirstKeptEntryId     string
	MessagesToSummarize  []llm.Message
	TurnPrefixMessages   []llm.Message
	RecentMessages       []llm.Message
	IsSplitTurn          bool
	TokensBefore         int
	PreviousSummary      string
	PreviousPreserveData map[string]any
	FileOps              FileOperation
}

// PrepareCompact analyzes pathEntries and settings to decide what to
// summarize and what to keep. It returns an empty preparation when the
// session was already compacted.
func PrepareCompact(
	pathEntries []session.MessageEntry,
	settings Settings,
) (*CompactionPreparation, error) {
	// already compacted, skip
	if len(pathEntries) > 0 && pathEntries[len(pathEntries)-1].GetType() == session.EntryCompaction {
		return &CompactionPreparation{}, nil
	}

	// find the last compaction entry
	preCompactionIndex := -1
	for i := range slices.Backward(pathEntries) {
		entry := pathEntries[i]
		if entry.GetType() == session.EntryCompaction {
			preCompactionIndex = i
			break
		}
	}

	// [preCompactionIndex + 1, end)
	start := preCompactionIndex + 1
	end := len(pathEntries)

	lastUsage := getLastAssistantUsage(pathEntries)
	tokenBefore := lastUsage.TotalTokens
	keepRecentTokens := settings.keepRecentTokens

	cutPoint := findCutPoint(pathEntries, start, end, keepRecentTokens)

	firstKeptEntry := pathEntries[cutPoint.firstKeptEntryIndex]
	if firstKeptEntry.GetID() == "" {
		return nil, errors.New("session needs migration")
	}

	firstKeptEntryID := firstKeptEntry.GetID()

	historyEnd := cutPoint.firstKeptEntryIndex
	if cutPoint.isSplitTurn {
		historyEnd = cutPoint.turnStartIndex
	}

	var messagesToSummarize []llm.Message
	for i := start; i < historyEnd; i++ {
		msg := getMessageFromEntry(pathEntries[i])
		if msg != nil {
			messagesToSummarize = append(messagesToSummarize, *msg)
		}
	}

	// Messages for turn prefix summary (if splitting a turn)
	var turnPrefixMessages []llm.Message
	if cutPoint.isSplitTurn {
		for i := cutPoint.turnStartIndex; i < cutPoint.firstKeptEntryIndex; i++ {
			msg := getMessageFromEntry(pathEntries[i])
			if msg != nil {
				turnPrefixMessages = append(turnPrefixMessages, *msg)
			}
		}
	}

	var recentMessages []llm.Message
	for i := cutPoint.firstKeptEntryIndex; i < end; i++ {
		msg := getMessageFromEntry(pathEntries[i])
		if msg != nil {
			recentMessages = append(recentMessages, *msg)
		}
	}

	previousSummary := ""
	var previousPreserveData map[string]any
	if preCompactionIndex >= 0 {
		prevCompaction := pathEntries[preCompactionIndex].(session.CompactionEntry)
		previousSummary = prevCompaction.Compaction.Summary
		previousPreserveData = prevCompaction.Compaction.PreserveData
	}

	fileOps := extractFileOperations(messagesToSummarize, pathEntries, preCompactionIndex)

	return &CompactionPreparation{
		FirstKeptEntryId:     firstKeptEntryID,
		MessagesToSummarize:  messagesToSummarize,
		TurnPrefixMessages:   turnPrefixMessages,
		RecentMessages:       recentMessages,
		TokensBefore:         tokenBefore,
		PreviousSummary:      previousSummary,
		PreviousPreserveData: previousPreserveData,
		FileOps:              *fileOps,
		IsSplitTurn:          cutPoint.isSplitTurn,
	}, nil
}

// CompactionResult is the outcome of a compaction run: the generated
// summary plus the bookkeeping needed to persist the compaction entry.
type CompactionResult struct {
	Summary          string
	FirstKeptEntryID string
	TokensBefore     int
	// HookDefinition-specific data (e.g., ArtifactIndex, version markers for structured compaction)
	Details any
	// HookDefinition-provided data to persist alongside compaction entry.
	PreserveData map[string]any
}

// Compact generates a summary for preparation via llm and returns the
// resulting CompactionResult. Options are accepted for future extension.
func Compact(
	ctx context.Context,
	preparation CompactionPreparation,
	compactor llm.Compactor,
	_ ...CompactOption,
) (CompactionResult, error) {
	var summary string
	if preparation.IsSplitTurn && len(preparation.TurnPrefixMessages) > 0 {
		var (
			historySummary       string
			turnPrefixSummary    string
			historySummaryErr    error
			turnPrefixSummaryErr error
		)

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			if len(preparation.MessagesToSummarize) == 0 {
				historySummary = "No prior history."
				return
			}

			historySummary, historySummaryErr = generateSummary(
				ctx,
				compactor,
				preparation.MessagesToSummarize,
				preparation.PreviousSummary,
			)
		}()

		go func() {
			defer wg.Done()
			turnPrefixSummary, turnPrefixSummaryErr = generateTurnPrefixSummary(
				ctx,
				compactor,
				preparation.TurnPrefixMessages,
			)
		}()

		wg.Wait()

		if historySummaryErr != nil {
			return CompactionResult{}, historySummaryErr
		}
		if turnPrefixSummaryErr != nil {
			return CompactionResult{}, turnPrefixSummaryErr
		}

		summary = historySummary + "\n\n---\n\n**Turn Context (split turn):**\n\n" + turnPrefixSummary
	} else {
		// Just generate history summary
		if len(preparation.MessagesToSummarize) == 0 {
			summary = "No prior history."
		} else {
			var err error
			summary, err = generateSummary(
				ctx,
				compactor,
				preparation.MessagesToSummarize,
				preparation.PreviousSummary,
			)
			if err != nil {
				return CompactionResult{}, err
			}
		}
	}

	readFiles, modifiedFiles := computeFileLists(&preparation.FileOps)
	fileOperations := formatFileOperations(readFiles, modifiedFiles)
	if fileOperations != "" {
		summary += "\n\n" + fileOperations
	}

	return CompactionResult{
		Summary:          summary,
		FirstKeptEntryID: preparation.FirstKeptEntryId,
		TokensBefore:     preparation.TokensBefore,
		Details:          CompactionDetails{ReadFiles: readFiles, ModifiedFiles: modifiedFiles},
	}, nil
}

// Run prepares compaction, generates summary via llm, and appends the compaction entry to manager.
// It is intended to be called from handler/session to avoid exposing CompactionResult outside this package.
func Run(
	ctx context.Context,
	pathEntries []session.MessageEntry,
	manager *session.Manager,
	compactor llm.Compactor,
	settings Settings,
) error {
	prep, err := PrepareCompact(pathEntries, settings)
	if err != nil {
		return err
	}
	if prep.FirstKeptEntryId == "" {
		return nil
	}
	result, err := Compact(ctx, *prep, compactor)
	if err != nil {
		return err
	}
	_, err = manager.AppendCompaction(session.Compaction{
		Summary:          result.Summary,
		FirstKeptEntryID: result.FirstKeptEntryID,
		TokensBefore:     result.TokensBefore,
		Details:          result.Details,
	})
	return err
}

// CompactionDetails lists the files read and modified in the summarized
// history; it is persisted with the compaction entry.
type CompactionDetails struct {
	ReadFiles     []string
	ModifiedFiles []string
}

func getLastAssistantUsage(entries []session.MessageEntry) llm.Usage {
	for i := range slices.Backward(entries) {
		entry := entries[i]
		if entry.GetType() == session.EntryMessage {
			msgEntry := entry.(session.SessionMessageEntry)
			if msgEntry.Message.Role == llm.RoleAssistant {
				return msgEntry.Message.Usage
			}
		}
	}
	return llm.Usage{
		TotalTokens: 0,
	}
}

func getMessageFromEntry(entry session.MessageEntry) *llm.Message {
	if entry.GetType() == session.EntryMessage {
		msgEntry := entry.(session.SessionMessageEntry)
		return &msgEntry.Message
	}
	return nil
}

type compactOptions struct {
	customInstructions string
	summaryOptions     *SummaryOptions
}

// SummaryOptions holds optional knobs for summary generation.
type SummaryOptions struct{}

// CompactOption customizes a Compact call.
type CompactOption func(options *compactOptions)

// WithCustomInstructions returns a CompactOption that sets the custom
// instructions for summary generation.
func WithCustomInstructions(customInstructions string) CompactOption {
	return func(options *compactOptions) {
		options.customInstructions = customInstructions
	}
}

// WithSummaryOptions returns a CompactOption that sets the summary options.
func WithSummaryOptions(summaryOptions SummaryOptions) CompactOption {
	return func(options *compactOptions) {
		options.summaryOptions = &summaryOptions
	}
}

const toolResultMaxChars = 500

// truncateForSummary truncates content to at most maxChars runes, appending "..." if truncated.
func truncateForSummary(content string, maxChars int) string {
	runes := []rune(content)
	if len(runes) <= maxChars {
		return content
	}
	return string(runes[:maxChars]) + "..."
}

// SerializeConversation formats messages as a single string for summary prompts:
// [User]: ..., [Assistant]: ..., [Assistant tool calls]: ..., [Tool result]: ...
func SerializeConversation(messages []llm.Message) string {
	var parts []string
	for _, msg := range messages {
		switch msg.Role {
		case llm.RoleUser:
			if msg.Content != "" {
				parts = append(parts, "[User]: "+msg.Content)
			}
		case llm.RoleAssistant:
			if msg.Content != "" {
				parts = append(parts, "[Assistant]: "+msg.Content)
			}
			if len(msg.ToolCalls) > 0 {
				var callStrs []string
				for _, tc := range msg.ToolCalls {
					callStrs = append(callStrs, tc.Function.Name+"("+tc.Function.Arguments+")")
				}
				parts = append(parts, "[Assistant tool calls]: "+strings.Join(callStrs, "; "))
			}
		case llm.RoleTool:
			if msg.Content != "" {
				parts = append(parts, "[Tool result]: "+truncateForSummary(msg.Content, toolResultMaxChars))
			}
		}
	}
	return strings.Join(parts, "\n\n")
}
