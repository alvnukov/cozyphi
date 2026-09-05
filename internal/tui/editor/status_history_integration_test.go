package editor

import (
	"context"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

func TestStatusDashboardWiresPreferencesAndHistory(t *testing.T) {
	e := newEditorWithSettings(t)
	require.NoError(t, e.ctrl.RecordStatusClose(statuspane.Stats))
	bus := controller.NewBus(nil)
	loaded := make(chan string, 2)
	history := controller.NewStatusHistory(
		bus,
		e.cwd,
		func(_ context.Context, cwd string, _ time.Time) (session.HistoryStatistics, error) {
			loaded <- cwd
			return session.HistoryStatistics{Sessions: 42}, nil
		},
	)
	defer history.Close()
	e.ConfigureStatusDashboard(history)
	e.ShowStatus()
	require.Equal(t, statuspane.Stats, e.status.Tab())
	require.Equal(t, e.cwd, <-loaded)
	select {
	case <-bus.Chan():
	case <-time.After(time.Second):
		t.Fatal("missing result")
	}
	result := bus.Drain()[0].(controller.StatusHistoryMsg)
	e.Update(result)
	root := e.Draw(components.DrawContext{Max: components.Size{Width: 120, Height: 45}, Method: xui.WidthUnicode})
	require.True(t, surfaceContains(root, "42"))
	e.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyF2})
	e.status.Hide()
	e.status.Hide()
	// Stats and Usage now tie. Duplicate Hide must not make Usage win by count.
	tab, err := e.ctrl.PreferredStatusTab()
	require.NoError(t, err)
	require.Equal(t, statuspane.Usage, tab)
	require.NoError(t, e.ctrl.RecordStatusClose(statuspane.Stats))
	tab, err = e.ctrl.PreferredStatusTab()
	require.NoError(t, err)
	require.Equal(t, statuspane.Stats, tab)
	e.ShowStatus()
	e.Update(result)
	root = e.Draw(components.DrawContext{Max: components.Size{Width: 120, Height: 45}, Method: xui.WidthUnicode})
	require.True(t, surfaceContains(root, "Loading history"), "prior opening cannot populate history")
	e.status.Hide()
}

func TestStatusHistorySnapshotPreservesUncertainty(t *testing.T) {
	totals := session.HistoryTotals{
		Rounds:        5,
		InputTokens:   10,
		OutputTokens:  3,
		CachedTokens:  2,
		TotalTokens:   13,
		UnknownUsage:  1,
		UnknownDates:  2,
		UnknownModels: 3,
	}
	h := statusHistorySnapshot(controller.StatusHistoryMsg{Statistics: session.HistoryStatistics{
		HistoryTotals: totals,
		Sessions:      2,
		Partial:       true,
		Warnings:      []string{"partial scan"},
		Days: []session.HistoryDay{
			{Day: "unknown", HistoryTotals: totals},
			{Day: "2026-01-01", HistoryTotals: totals},
		},
		Models: []session.HistoryModel{{Model: "unknown", HistoryTotals: totals}},
	}})
	require.Equal(t, 2, h.Totals.Sessions)
	require.EqualValues(t, 13, h.Totals.Total)
	require.True(t, h.Partial)
	require.Equal(t, 1, h.UnknownUsage)
	require.Equal(t, 2, h.UnknownDates)
	require.Equal(t, 3, h.UnknownModels)
	require.Len(t, h.Days, 1)
	require.Equal(t, "unknown", h.Models[0].Name)
	require.Equal(t, []string{"partial scan"}, h.Warnings)
	require.Contains(t, statusHistorySnapshot(controller.StatusHistoryMsg{Failed: true}).Unavailable, "retry with r")
}
