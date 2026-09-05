package controller

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestStatusHistoryGenerationAndCancellation(t *testing.T) {
	bus := NewBus(nil)
	started := make(chan context.Context, 4)
	release := make(chan struct{}, 4)
	h := NewStatusHistory(
		bus,
		t.TempDir(),
		func(ctx context.Context, _ string, _ time.Time) (session.HistoryStatistics, error) {
			started <- ctx
			select {
			case <-ctx.Done():
				return session.HistoryStatistics{}, ctx.Err()
			case <-release:
			}
			return session.HistoryStatistics{Sessions: 3}, nil
		},
	)
	defer h.Close()
	h.Request(0)
	first := <-started
	generation := h.generation
	h.Request(7)
	require.ErrorIs(t, first.Err(), context.Canceled)
	second := <-started
	require.False(t, h.Accept(StatusHistoryMsg{Generation: generation}))
	release <- struct{}{}
	select {
	case <-bus.Chan():
	case <-time.After(time.Second):
		t.Fatal("result not delivered")
	}
	result := bus.Drain()[0].(StatusHistoryMsg)
	require.True(t, h.Accept(result))
	require.Equal(t, 3, result.Statistics.Sessions)
	h.Cancel()
	require.ErrorIs(t, second.Err(), context.Canceled)
	require.False(t, h.Accept(result))
	h.Request(30)
	third := <-started
	require.False(t, h.Accept(result), "closed/reopened pane rejects queued results")
	h.Close()
	require.ErrorIs(t, third.Err(), context.Canceled)
	h.Request(0)
	require.False(t, h.Accept(StatusHistoryMsg{Generation: h.generation}))
}

func TestHistorySinceUTCInclusive(t *testing.T) {
	now := time.Date(2026, 3, 1, 23, 30, 0, 0, time.FixedZone("west", -2*3600))
	require.Equal(t, "2026-02-24", HistorySince(now, 7).Format("2006-01-02"))
	require.Equal(t, "2026-02-01", HistorySince(now, 30).Format("2006-01-02"))
	require.True(t, HistorySince(now, 0).IsZero())
	require.Equal(t, time.UTC, HistorySince(now, 7).Location())
}

func TestControllerStatusPreferences(t *testing.T) {
	proj, _ := newEffortProject(t)
	c := &Controller{proj: proj}
	tab, err := c.PreferredStatusTab()
	require.NoError(t, err)
	require.Equal(t, "usage", tab)
	require.NoError(t, c.RecordStatusClose("stats"))
	tab, err = c.PreferredStatusTab()
	require.NoError(t, err)
	require.Equal(t, "stats", tab)
	require.NoError(t, c.RecordStatusClose("usage"))
	tab, err = c.PreferredStatusTab()
	require.NoError(t, err)
	require.Equal(t, "usage", tab)
	state, err := project.LoadUIState(proj.Global())
	require.NoError(t, err)
	require.EqualValues(t, 1, state.StatusCloses["stats"])
	path := proj.Global().UIStateFile()
	require.NoError(t, os.WriteFile(path, []byte("{"), 0o600))
	tab, err = c.PreferredStatusTab()
	require.Error(t, err)
	require.Equal(t, "usage", tab)
	require.Error(t, c.RecordStatusClose("stats"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "{", string(data))
}
