package controller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/provider"
)

func TestResetQuotaSingleFlightAndFreshRead(t *testing.T) {
	c := &Controller{bus: NewBus(nil), modelCfg: llm.ModelConfig{Name: "codex", ProviderID: "openai"}}
	defer c.Close()
	c.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{}, nil
	})
	old := waitForUsageQuotaMsg(t, c)
	require.True(t, c.AcceptQuota(old))

	readStarted, oldReadCanceled := make(chan struct{}), make(chan struct{})
	c.fetchQuotaWith(t.Context(), func(ctx context.Context, _ string) (provider.QuotaSnapshot, error) {
		close(readStarted)
		<-ctx.Done()
		close(oldReadCanceled)
		return provider.QuotaSnapshot{}, ctx.Err()
	})
	<-readStarted
	started, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	reset := func(ctx context.Context, _ *provider.QuotaResetTarget) (provider.QuotaResetResult, error) {
		calls.Add(1)
		close(started)
		select {
		case <-finish:
		case <-ctx.Done():
		}
		return provider.QuotaResetResult{Code: provider.QuotaResetDone, WindowsReset: 1}, nil
	}
	c.resetQuotaWith(t.Context(), nil, reset)
	<-started
	<-oldReadCanceled
	require.False(t, c.AcceptQuota(old), "queued pre-reset reads must be rejected")
	c.resetQuotaWith(t.Context(), nil, reset)
	c.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		t.Error("a read must not run during the mutation")
		return provider.QuotaSnapshot{}, nil
	})
	require.Equal(t, int32(1), calls.Load())
	close(finish)
	msg := waitForUsageResetMsg(t, c)
	require.NoError(t, msg.Err)
	require.Equal(t, provider.QuotaResetDone, msg.Result.Code)

	// The shell responds to completion with this read. The canceled old read
	// cannot hold its single-flight slot or overwrite the new observation.
	c.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{PlanName: "fresh"}, nil
	})
	fresh := waitForUsageQuotaMsg(t, c)
	require.True(t, c.AcceptQuota(fresh))
	require.Equal(t, "fresh", fresh.Snapshot.PlanName)
}

func TestResetQuotaUnknownIsNotRetried(t *testing.T) {
	c := &Controller{bus: NewBus(nil)}
	defer c.Close()
	var calls atomic.Int32
	c.resetQuotaWith(
		t.Context(),
		nil,
		func(context.Context, *provider.QuotaResetTarget) (provider.QuotaResetResult, error) {
			calls.Add(1)
			return provider.QuotaResetResult{}, provider.ErrQuotaResetUnknown
		},
	)
	msg := waitForUsageResetMsg(t, c)
	require.ErrorIs(t, msg.Err, provider.ErrQuotaResetUnknown)
	require.Equal(t, int32(1), calls.Load())
}

func TestQuotaSelectionRoundTripRejectsStaleMessage(t *testing.T) {
	c := &Controller{bus: NewBus(nil), modelCfg: llm.ModelConfig{Name: "a", ProviderID: "openai"}}
	defer c.Close()
	c.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		return provider.QuotaSnapshot{}, nil
	})
	old := waitForUsageQuotaMsg(t, c)
	c.modelCfg.Name = "b"
	require.True(t, c.SyncQuotaSelection())
	c.modelCfg.Name = "a"
	require.True(t, c.SyncQuotaSelection())
	require.False(t, c.AcceptQuota(old))
	require.False(t, c.SyncQuotaSelection(), "ordinary UI redraws keep current observations")
}

func TestCloseCancelsResetAndRejectsNewUsageWork(t *testing.T) {
	c := &Controller{bus: NewBus(nil)}
	started, stopped := make(chan struct{}), make(chan struct{})
	c.resetQuotaWith(
		t.Context(),
		nil,
		func(ctx context.Context, _ *provider.QuotaResetTarget) (provider.QuotaResetResult, error) {
			close(started)
			<-ctx.Done()
			close(stopped)
			return provider.QuotaResetResult{}, provider.ErrQuotaResetUnknown
		},
	)
	<-started
	c.Close()
	select {
	case <-stopped:
	default:
		t.Fatal("Close returned without canceling and joining reset")
	}
	c.resetQuotaWith(
		t.Context(),
		nil,
		func(context.Context, *provider.QuotaResetTarget) (provider.QuotaResetResult, error) {
			t.Error("reset started after shutdown")
			return provider.QuotaResetResult{}, nil
		},
	)
	c.fetchQuotaWith(t.Context(), func(context.Context, string) (provider.QuotaSnapshot, error) {
		t.Error("read started after shutdown")
		return provider.QuotaSnapshot{}, nil
	})
}

func waitForUsageResetMsg(t *testing.T, c *Controller) UsageResetMsg {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for {
		for _, m := range c.bus.Drain() {
			if msg, ok := m.(UsageResetMsg); ok && !msg.InFlight {
				return msg
			}
		}
		select {
		case <-c.bus.Chan():
		case <-timeout:
			t.Fatal("timed out waiting for reset completion")
		}
	}
}
