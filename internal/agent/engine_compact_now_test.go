package agent

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/llm"
	"github.com/pulseaiclub/phi/internal/session"
)

func TestEngineCompactNowNothingToCompact(t *testing.T) {
	engine := newContextTestEngine(t, "http://127.0.0.1:1", 100000)
	err := engine.CompactNow(t.Context(), func(session.Event) bool { return true })
	require.ErrorContains(t, err, "nothing to compact")
}

func TestEngineCompactNowAfterResumeUsesPersistedUsage(t *testing.T) {
	server, _, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	dir := t.TempDir()
	manager, err := session.NewSessionManager(
		dir,
		session.WithSessionDir(dir),
		session.WithShouldFlush(true),
	)
	require.NoError(t, err)
	for _, msg := range []llm.Message{
		{Role: llm.RoleUser, Content: "first question"},
		{Role: llm.RoleAssistant, Content: "first answer"},
		{Role: llm.RoleUser, Content: "current question"},
		{
			Role:    llm.RoleAssistant,
			Content: "current answer",
			Usage:   llm.Usage{PromptTokens: 1200, TotalTokens: 1280},
		},
	} {
		_, err = manager.Append(msg)
		require.NoError(t, err)
	}

	engine, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{
			Name:          "fake",
			BaseURL:       server.URL,
			APIKey:        "x",
			ContextWindow: 100000,
		},
		SessionOpts: SessionOpts{ResumePath: manager.File()},
	})
	require.NoError(t, err)
	require.Equal(t, 1200, engine.contextStats().ContextTokens)

	err = engine.CompactNow(t.Context(), func(session.Event) bool { return true })
	require.NoError(t, err)
	compacted := engine.session.PathEntries()[0].(session.CompactionEntry)

	reopened, err := NewEngine(EngineOpts{
		Model: llm.ModelConfig{
			Name:          "fake",
			BaseURL:       server.URL,
			APIKey:        "x",
			ContextWindow: 100000,
		},
		SessionOpts: SessionOpts{ResumePath: manager.File()},
	})
	require.NoError(t, err)
	stats := reopened.contextStats()
	require.Equal(t, "estimate", stats.TokenSource)
	require.Equal(t, compacted.Compaction.TokensAfter, stats.ContextTokens)
}

func TestEngineCompactNowCompactsOlderTurnsBelowAutomaticThreshold(t *testing.T) {
	server, streams, _ := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "first question"},
		llm.Message{Role: llm.RoleAssistant, Content: "first answer"},
		llm.Message{Role: llm.RoleUser, Content: "current question"},
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: "current answer",
			Usage:   llm.Usage{PromptTokens: 1200, TotalTokens: 1280},
		},
	))

	var complete session.CompactionComplete
	err := engine.CompactNow(t.Context(), func(ev session.Event) bool {
		if done, ok := ev.(session.CompactionComplete); ok {
			complete = done
		}
		return true
	})

	require.NoError(t, err)
	require.False(t, complete.Failed)
	require.Contains(t, complete.Compaction.Report(), "Compacted 2 messages")
	require.Contains(t, complete.Compaction.Report(), "2 kept")
	require.Zero(t, streams.Load(), "manual compact must not start chat rounds")
}

func TestEngineCompactNowRunsCompaction(t *testing.T) {
	server, streams, bodies := fakeContextServer(t, "SUMMARY-OF-OLD-HISTORY", func(int32) string { return "" })
	engine := newContextTestEngine(t, server.URL, 100000)
	seedTwoTurnHistory(t, engine)

	var events []session.Event
	err := engine.CompactNow(t.Context(), func(ev session.Event) bool {
		events = append(events, ev)
		return true
	})
	require.NoError(t, err)
	require.Len(t, events, 2, "Started then Complete")
	_, started := events[0].(session.CompactionStarted)
	require.True(t, started)
	complete, ok := events[1].(session.CompactionComplete)
	require.True(t, ok)
	require.False(t, complete.Failed)
	report := complete.Compaction.Report()
	require.Contains(t, report, "Compacted 2 messages")
	require.Contains(t, report, "2 kept")
	require.Contains(t, report, "25k")
	require.Positive(t, complete.Compaction.TokensAfter)
	require.Less(t, complete.Compaction.TokensAfter, complete.Compaction.TokensBefore)
	stats := engine.contextStats()
	require.Equal(t, "estimate", stats.TokenSource)
	require.Equal(t, complete.Compaction.TokensAfter, stats.ContextTokens)
	require.NoError(t, engine.session.Append(
		llm.Message{Role: llm.RoleUser, Content: "after compaction"},
		llm.Message{
			Role:    llm.RoleAssistant,
			Content: "fresh answer",
			Usage:   llm.Usage{PromptTokens: 3333, TotalTokens: 3400},
		},
	))
	stats = engine.contextStats()
	require.Equal(t, "provider", stats.TokenSource)
	require.Equal(t, 3333, stats.ContextTokens)

	// Manual compaction preserves the latest complete turn and summarizes
	// older history in one request, regardless of cumulative provider usage.
	require.Zero(t, streams.Load(), "compact must not start chat rounds")
	require.Len(t, bodies(), 1)

	var found bool
	for _, entry := range engine.session.PathEntries() {
		if _, ok := entry.(session.CompactionEntry); ok {
			found = true
		}
	}
	require.True(t, found, "compaction entry must land in the session")
}
