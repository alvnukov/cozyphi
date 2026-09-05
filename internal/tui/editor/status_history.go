package editor

import (
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/statuspane"
)

// ConfigureStatusDashboard connects controller preferences and the assembled loader.
func (e *Editor) ConfigureStatusDashboard(history *controller.StatusHistory) {
	e.statusHistory = history
	e.ConfigureStatusHistory(history.Request)
	e.ConfigureStatusTabs(func() string {
		tab, err := e.ctrl.PreferredStatusTab()
		if err != nil {
			e.toast.Show("Cannot load status preferences: "+err.Error(), toast.ToastWarning, 5*time.Second)
		}
		return tab
	}, func(tab string) {
		if err := e.ctrl.RecordStatusClose(tab); err != nil {
			e.toast.Show("Cannot save status preferences: "+err.Error(), toast.ToastWarning, 5*time.Second)
		}
	})
}

func statusHistorySnapshot(m controller.StatusHistoryMsg) statuspane.History {
	if m.Failed {
		return statuspane.History{
			Unavailable: "Cannot load history; check session directory permissions and retry with r.",
		}
	}
	s := m.Statistics
	h := statuspane.History{
		Totals: historyTotals(s.HistoryTotals), Partial: s.Partial,
		UnknownUsage: s.UnknownUsage, UnknownModels: s.UnknownModels, UnknownDates: s.UnknownDates,
		Warnings: append([]string(nil), s.Warnings...),
	}
	h.Totals.Sessions = s.Sessions
	for _, day := range s.Days {
		date, err := time.Parse("2006-01-02", day.Day)
		if err == nil {
			h.Days = append(h.Days, statuspane.Day{Date: date, Totals: historyTotals(day.HistoryTotals)})
		}
	}
	for _, model := range s.Models {
		h.Models = append(h.Models, statuspane.Model{Name: model.Model, Totals: historyTotals(model.HistoryTotals)})
	}
	return h
}

func historyTotals(t session.HistoryTotals) statuspane.Totals {
	return statuspane.Totals{
		Rounds: t.Rounds,
		Input:  int64(t.InputTokens),
		Output: int64(t.OutputTokens),
		Cached: int64(t.CachedTokens),
		Total:  int64(t.TotalTokens),
	}
}
