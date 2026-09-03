package controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestBuildSessionStatsAggregatesSnapshot(t *testing.T) {
	start := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	cfg := llm.ModelConfig{Name: "glm-5.1", ProviderID: "zai-coding-plan", ContextWindow: 200000}
	snap := session.Snapshot{Messages: []session.Message{
		{ID: "u1", Role: session.RoleUser, Text: "hi", Started: start},
		{
			ID:      "a1",
			Role:    session.RoleAssistant,
			State:   session.StateComplete,
			Started: start,
			Ended:   start.Add(5 * time.Second),
			Usage: session.TokenUsage{
				PromptTokens:     1000,
				CompletionTokens: 100,
				CachedTokens:     400,
				TotalTokens:      1100,
			},
		},
		{
			ID:      "a2",
			Role:    session.RoleAssistant,
			State:   session.StateComplete,
			Started: start.Add(10 * time.Second),
			Ended:   start.Add(20 * time.Second),
			Usage: session.TokenUsage{
				PromptTokens:     3000,
				CompletionTokens: 200,
				CachedTokens:     2400,
				TotalTokens:      3200,
			},
		},
		// A cancelled round counts toward nothing.
		{
			ID: "a3", Role: session.RoleAssistant, State: session.StateCancelled, Started: start.Add(22 * time.Second),
			Usage: session.TokenUsage{PromptTokens: 7777, CompletionTokens: 7, TotalTokens: 7784},
		},
		// The compaction estimate supersedes earlier usage for context fill...
		{
			ID: "c1", Role: session.RoleCompaction, Started: start.Add(25 * time.Second),
			Usage: session.TokenUsage{PromptTokens: 640, Estimated: true},
		},
		// ...but must not inflate the cumulative token totals.
		{ID: "n1", Role: session.RoleNotice, Text: "compact soon"},
	}}

	stats := buildSessionStats(cfg, snap)

	require.Equal(t, "glm-5.1", stats.Model)
	require.Equal(t, "zai-coding-plan", stats.ProviderID)
	require.Equal(t, 200000, stats.ContextWindow)
	require.Equal(t, int64(4000), stats.InputTokens)
	require.Equal(t, int64(300), stats.OutputTokens)
	require.Equal(t, int64(2800), stats.CachedTokens)
	require.Equal(t, int64(4300), stats.TotalTokens)
	require.Equal(t, 2, stats.Rounds, "only completed assistant rounds count")
	require.Equal(t, start, stats.StartedAt)
	require.Equal(t, 640, stats.ContextTokens, "the latest reported usage is the context fill")
}

func TestBuildSessionStatsEmptySession(t *testing.T) {
	stats := buildSessionStats(llm.ModelConfig{}, session.Snapshot{})
	require.True(t, stats.StartedAt.IsZero())
	require.Zero(t, stats.Rounds)
	require.Zero(t, stats.ContextTokens)
	require.Zero(t, stats.TotalTokens)
}

func TestBuildSessionStatsContextFillPrefersLatestAssistantUsage(t *testing.T) {
	snap := session.Snapshot{Messages: []session.Message{
		{
			ID: "a1", Role: session.RoleAssistant, State: session.StateComplete,
			Usage: session.TokenUsage{PromptTokens: 2000, TotalTokens: 2100},
		},
		{
			ID: "a2", Role: session.RoleAssistant, State: session.StateComplete,
			Usage: session.TokenUsage{TotalTokens: 3300},
		},
	}}
	stats := buildSessionStats(llm.ModelConfig{}, snap)
	require.Equal(t, 3300, stats.ContextTokens, "falls back to total when prompt tokens are absent")
}

func TestController_FetchQuotaPublishesThroughBus(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	wantSnapshot := provider.QuotaSnapshot{ProviderID: "zai-coding-plan", PlanName: "GLM Coding Pro"}
	ctrl.modelCfg.ProviderID = "zai-coding-plan"
	ctrl.fetchQuotaWith(context.Background(), func(ctx context.Context, id string) (provider.QuotaSnapshot, error) {
		require.Equal(t, "zai-coding-plan", id, "the active provider id must reach the fetcher")
		_, hasDeadline := ctx.Deadline()
		require.True(t, hasDeadline, "the fetch must run under a timeout")
		return wantSnapshot, nil
	})
	msg := waitForUsageQuotaMsg(t, ctrl)
	require.Equal(t, "zai-coding-plan", msg.ProviderID)
	require.Equal(t, wantSnapshot, msg.Snapshot)
	require.NoError(t, msg.Err)
	require.False(t, msg.Unsupported)

	ctrl.fetchQuotaWith(context.Background(), func(_ context.Context, _ string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{}, errors.New("connection refused")
	})
	msg = waitForUsageQuotaMsg(t, ctrl)
	require.Error(t, msg.Err, "network failures surface as pane errors")
	require.False(t, msg.Unsupported)

	ctrl.fetchQuotaWith(context.Background(), func(_ context.Context, _ string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{}, provider.ErrQuotaUnsupported
	})
	msg = waitForUsageQuotaMsg(t, ctrl)
	require.ErrorIs(t, msg.Err, provider.ErrQuotaUnsupported)
	require.True(t, msg.Unsupported, "unsupported providers are distinguishable from failures")
}

func TestController_FetchQuotaSkipsWhenInFlight(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	ctrl, err := NewController(NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	calls := make(chan struct{}, 4)
	ctrl.fetchQuotaWith(ctx, func(_ context.Context, _ string) (provider.QuotaSnapshot, error) {
		calls <- struct{}{}
		<-ctx.Done()
		return provider.QuotaSnapshot{}, ctx.Err()
	})
	<-calls // first fetch is running
	ctrl.fetchQuotaWith(ctx, func(_ context.Context, _ string) (provider.QuotaSnapshot, error) {
		calls <- struct{}{}
		return provider.QuotaSnapshot{}, nil
	})
	require.Empty(t, calls, "a fetch already in flight is skipped, not queued")
	cancel()
	waitForUsageQuotaMsg(t, ctrl)
	ctrl.Close()
}

func waitForUsageQuotaMsg(t *testing.T, ctrl *Controller) UsageQuotaMsg {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		for _, m := range ctrl.bus.Drain() {
			if msg, ok := m.(UsageQuotaMsg); ok {
				return msg
			}
		}
		select {
		case <-ctrl.bus.Chan():
			continue
		case <-timeout:
			t.Fatal("timed out waiting for UsageQuotaMsg")
		}
	}
}
