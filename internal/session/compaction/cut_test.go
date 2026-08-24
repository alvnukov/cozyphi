package compaction

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func TestFindCutIndex_ExceedsTokensChoosesNearestCutPoint(t *testing.T) {
	entries := []session.MessageEntry{
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleUser, Content: strings.Repeat("a", 10)},
		},
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleAssistant, Content: strings.Repeat("b", 20)},
		},
		session.BranchSummaryEntry{}, // non-message, should be skipped for token accumulation
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleUser, Content: strings.Repeat("c", 30)},
		},
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleAssistant, Content: strings.Repeat("d", 40)},
		},
	}

	startIndex := 0
	endIndex := len(entries)
	keepRecentTokens := estimateMessageTokens(entries[4].(session.SessionMessageEntry).Message)
	cutPoints := []int{1, 3, 4}

	cutIndex := findCutIndex(entries, startIndex, endIndex, keepRecentTokens, cutPoints)

	assert.Equal(t, 3, cutIndex)
}

func TestFindCutIndexDoesNotSumCumulativeProviderUsage(t *testing.T) {
	entries := []session.MessageEntry{
		session.SessionMessageEntry{Message: llm.Message{
			Role: llm.RoleUser, Content: "short question",
		}},
		session.SessionMessageEntry{Message: llm.Message{
			Role: llm.RoleAssistant, Content: "short answer",
			Usage: llm.Usage{PromptTokens: 90000, TotalTokens: 90100},
		}},
		session.SessionMessageEntry{Message: llm.Message{
			Role: llm.RoleUser, Content: "another short question",
		}},
		session.SessionMessageEntry{Message: llm.Message{
			Role: llm.RoleAssistant, Content: "another short answer",
			Usage: llm.Usage{PromptTokens: 91000, TotalTokens: 91100},
		}},
	}

	cutIndex := findCutIndex(entries, 0, len(entries), 1000, []int{0, 1, 2, 3})

	assert.Equal(t, 0, cutIndex, "provider usage describes the whole prompt, not this message's size")
}

func TestFindCutIndex_NotExceedTokensReturnsFirstCutPoint(t *testing.T) {
	entries := []session.MessageEntry{
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleUser, Content: "one"},
		},
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleAssistant, Content: "two"},
		},
		session.SessionMessageEntry{
			Message: llm.Message{Role: llm.RoleUser, Content: "three"},
		},
	}

	startIndex := 0
	endIndex := len(entries)
	keepRecentTokens := 200
	cutPoints := []int{0, 1, 2}

	cutIndex := findCutIndex(entries, startIndex, endIndex, keepRecentTokens, cutPoints)

	assert.Equal(t, cutPoints[0], cutIndex)
}
