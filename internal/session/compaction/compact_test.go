package compaction

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func msgEntry(id string, role llm.Role, tokens int) session.MessageEntry {
	return session.SessionMessageEntry{
		SessionBaseEntry: session.SessionBaseEntry{ID: id},
		Message: llm.Message{
			Role:  role,
			Usage: llm.Usage{TotalTokens: tokens},
		},
	}
}

func TestPrepareCompact_AlreadyCompacted_ReturnsEmptyPreparation(t *testing.T) {
	entries := []session.MessageEntry{
		msgEntry("e1", llm.RoleUser, 10),
		session.CompactionEntry{
			SessionBaseEntry: session.SessionBaseEntry{ID: "comp1"},
			Compaction: session.Compaction{
				Summary: "prev",
			},
		},
	}
	settings := Settings{keepRecentTokens: 100}

	prep, err := PrepareCompact(entries, settings)

	assert.NoError(t, err)
	assert.NotNil(t, prep)
	assert.Empty(t, prep.FirstKeptEntryId)
	assert.Nil(t, prep.MessagesToSummarize)
	assert.Nil(t, prep.RecentMessages)
}

func TestPrepareCompact_EmptySession_ReturnsEmptyPreparation(t *testing.T) {
	prep, err := PrepareCompact(nil, Settings{keepRecentTokens: 100})

	assert.NoError(t, err)
	assert.NotNil(t, prep)
	assert.Empty(t, prep.FirstKeptEntryId)
	assert.Nil(t, prep.MessagesToSummarize)
}

func TestPrepareCompact_SessionNeedsMigration_ReturnsError(t *testing.T) {
	entries := []session.MessageEntry{
		session.SessionMessageEntry{
			SessionBaseEntry: session.SessionBaseEntry{ID: ""},
			Message: llm.Message{
				Role:  llm.RoleUser,
				Usage: llm.Usage{TotalTokens: 10},
			},
		},
	}
	settings := Settings{keepRecentTokens: 100}

	prep, err := PrepareCompact(entries, settings)

	assert.Error(t, err)
	assert.Nil(t, prep)
	assert.Contains(t, err.Error(), "migration")
}

func TestPrepareCompact_NoPreviousCompaction_SplitsByKeepRecentTokens(t *testing.T) {
	entries := []session.MessageEntry{
		msgEntry("e1", llm.RoleUser, 10),
		msgEntry("e2", llm.RoleAssistant, 20),
		msgEntry("e3", llm.RoleUser, 30),
	}
	settings := Settings{keepRecentTokens: 25}

	prep, err := PrepareCompact(entries, settings)

	assert.NoError(t, err)
	assert.NotNil(t, prep)
	assert.Equal(t, "e3", prep.FirstKeptEntryId)
	assert.False(t, prep.IsSplitTurn)
	assert.Len(t, prep.MessagesToSummarize, 2)
	assert.Equal(t, llm.RoleUser, prep.MessagesToSummarize[0].Role)
	assert.Equal(t, llm.RoleAssistant, prep.MessagesToSummarize[1].Role)
	assert.Len(t, prep.RecentMessages, 1)
	assert.Equal(t, llm.RoleUser, prep.RecentMessages[0].Role)
	assert.Equal(t, 20, prep.TokensBefore)
	assert.Empty(t, prep.PreviousSummary)
	assert.Nil(t, prep.PreviousPreserveData)
}

func TestPrepareCompact_KeepAll_WhenUnderTokenLimit(t *testing.T) {
	entries := []session.MessageEntry{
		msgEntry("e1", llm.RoleUser, 10),
		msgEntry("e2", llm.RoleAssistant, 20),
	}
	settings := Settings{keepRecentTokens: 100}

	prep, err := PrepareCompact(entries, settings)

	assert.NoError(t, err)
	assert.NotNil(t, prep)
	assert.Equal(t, "e1", prep.FirstKeptEntryId)
	assert.Empty(t, prep.MessagesToSummarize)
	assert.Len(t, prep.RecentMessages, 2)
	assert.Equal(t, 20, prep.TokensBefore)
}

func TestPrepareCompact_WithPreviousCompaction_SetsSummaryAndPreserveData(t *testing.T) {
	entries := []session.MessageEntry{
		session.CompactionEntry{
			SessionBaseEntry: session.SessionBaseEntry{ID: "comp1"},
			Compaction: session.Compaction{
				Summary:      "old summary",
				PreserveData: map[string]any{"k": "v"},
			},
		},
		msgEntry("e1", llm.RoleUser, 10),
		msgEntry("e2", llm.RoleAssistant, 20),
	}
	settings := Settings{keepRecentTokens: 100}

	prep, err := PrepareCompact(entries, settings)

	assert.NoError(t, err)
	assert.NotNil(t, prep)
	assert.Equal(t, "old summary", prep.PreviousSummary)
	assert.Equal(t, map[string]any{"k": "v"}, prep.PreviousPreserveData)
	assert.Equal(t, "e1", prep.FirstKeptEntryId)
	assert.Empty(t, prep.MessagesToSummarize)
	assert.Len(t, prep.RecentMessages, 2)
}
