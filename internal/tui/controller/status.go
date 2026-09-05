package controller

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

// StatusConfigSources exposes only paths and source modes, never configuration values.
func (c *Controller) StatusConfigSources() []string {
	if c == nil || c.proj == nil {
		return nil
	}
	sources := []string{
		"Global config: " + c.proj.Global().ConfigFile(),
		"Project MCP: " + c.proj.MCPConfigFile(),
		"Global LSP: " + c.proj.Global().LSPConfigFile(),
	}
	cfg := c.proj.Config()
	switch {
	case cfg == nil:
		sources = append(sources, "Model: source unavailable")
	case cfg.ModelEnvOverride():
		sources = append(sources, "Model: COZYPHI_MODEL override")
	default:
		sources = append(sources, "Model: configured/session selection")
	}
	if cfg != nil && cfg.OpenCode.Enabled {
		sources = append(sources, "OpenCode: read-only import enabled")
	} else {
		sources = append(sources, "OpenCode: import disabled")
	}
	return sources
}

// PreferredStatusTab loads the global close-frequency preference.
func (c *Controller) PreferredStatusTab() (string, error) {
	if c == nil || c.proj == nil {
		return "usage", errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return "usage", err
	}
	return state.PreferredStatusTab(), nil
}

// RecordStatusClose preserves sibling preferences and refuses malformed state.
func (c *Controller) RecordStatusClose(tab string) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) { s.RecordStatusClose(tab) })
}

// StatusHistoryMsg carries a detached scan result, never raw error text.
type StatusHistoryMsg struct {
	Generation uint64
	Statistics session.HistoryStatistics
	Failed     bool
}

func (StatusHistoryMsg) isMsg() {}

// StatusHistory owns cancellable scans. Request, Cancel, Accept and Close are
// UI-goroutine confined; workers communicate only through the Bus.
type StatusHistory struct {
	bus        *Bus
	cwd        string
	load       func(context.Context, string, time.Time) (session.HistoryStatistics, error)
	generation uint64
	cancel     context.CancelFunc
	workers    sync.WaitGroup
	closed     bool
}

func NewStatusHistory(
	bus *Bus,
	cwd string,
	load func(context.Context, string, time.Time) (session.HistoryStatistics, error),
) *StatusHistory {
	return &StatusHistory{bus: bus, cwd: cwd, load: load}
}

// HistorySince includes today and the preceding days-1 UTC calendar days.
func HistorySince(now time.Time, days int) time.Time {
	if days != 7 && days != 30 {
		return time.Time{}
	}
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 1-days)
}

func (h *StatusHistory) Request(days int) {
	h.Cancel()
	if h.closed {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel
	generation := h.generation
	since := HistorySince(time.Now(), days)
	h.workers.Go(func() {
		stats, err := h.load(ctx, h.cwd, since)
		if ctx.Err() != nil {
			return
		}
		h.bus.Publish(StatusHistoryMsg{Generation: generation, Statistics: stats, Failed: err != nil})
	})
}

func (h *StatusHistory) Cancel() {
	if h == nil {
		return
	}
	h.generation++
	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

func (h *StatusHistory) Accept(m StatusHistoryMsg) bool {
	return h != nil && !h.closed && h.cancel != nil && m.Generation == h.generation
}

// Close cancels and joins bounded, context-aware scans on every application exit.
func (h *StatusHistory) Close() {
	if h == nil {
		return
	}
	h.closed = true
	h.Cancel()
	h.workers.Wait()
}
