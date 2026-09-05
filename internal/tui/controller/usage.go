package controller

import (
	"context"
	"errors"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
)

// quotaFetchTimeout bounds one subscription quota request.
const quotaFetchTimeout = 15 * time.Second

// SessionStats is the usage pane's Session section: cumulative token totals
// over completed rounds, the round count, the session start and the current
// context fill. Display-safe by construction — no secrets live here.
type SessionStats struct {
	Model         string
	ProviderID    string
	ContextWindow int
	InputTokens   int64
	OutputTokens  int64
	CachedTokens  int64
	TotalTokens   int64
	Rounds        int
	StartedAt     time.Time
	ContextTokens int
}

// SessionStats aggregates the live session for the usage pane.
func (c *Controller) SessionStats() SessionStats {
	if c == nil {
		return SessionStats{}
	}
	return buildSessionStats(c.modelCfg, c.ReplaySnapshot())
}

// buildSessionStats folds a session snapshot into pane totals. Only completed
// assistant rounds feed the cumulative counters; cancelled rounds do not, and
// compaction estimates never inflate them. The context fill is the latest
// reported usage (assistant or compaction estimate), matching the composer
// hint.
func buildSessionStats(cfg llm.ModelConfig, snap session.Snapshot) SessionStats {
	stats := SessionStats{
		Model:         cfg.Name,
		ProviderID:    cfg.ProviderID,
		ContextWindow: cfg.ContextWindow,
	}
	for _, msg := range snap.Messages {
		if stats.StartedAt.IsZero() && !msg.Started.IsZero() {
			stats.StartedAt = msg.Started
		}
		if msg.Usage.Reported() {
			// The latest reported usage wins as the context fill, so a later
			// compaction estimate supersedes earlier assistant counts.
			stats.ContextTokens = msg.Usage.ContextTokens()
		}
		if msg.Role != session.RoleAssistant || msg.State != session.StateComplete {
			continue
		}
		stats.Rounds++
		stats.InputTokens += int64(msg.Usage.PromptTokens)
		stats.OutputTokens += int64(msg.Usage.CompletionTokens)
		stats.CachedTokens += int64(msg.Usage.CachedTokens)
		stats.TotalTokens += int64(msg.Usage.TotalTokens)
	}
	return stats
}

// UsageQuotaMsg delivers a background subscription quota fetch to the usage
// pane. Unsupported tells providers without a quota adapter apart from
// transport failures; Err is safe to display (the API key never rides it).
type UsageQuotaMsg struct {
	Generation  uint64
	ProviderID  string
	Snapshot    provider.QuotaSnapshot
	Err         error
	Unsupported bool
}

func (UsageQuotaMsg) isMsg() {}

// FetchQuota starts a background subscription quota fetch for the active
// provider and publishes the result as UsageQuotaMsg. The caller never
// blocks; a fetch already in flight is skipped rather than queued.
func (c *Controller) FetchQuota(ctx context.Context) {
	if c != nil && c.providers != nil {
		c.fetchQuotaWith(ctx, c.providers.QuotaSnapshot)
	}
}

// fetchQuotaWith runs one quota fetch against an injectable fetcher so tests
// can drive the publish path without a real provider connection.
func (c *Controller) fetchQuotaWith(
	ctx context.Context,
	fetch func(context.Context, string) (provider.QuotaSnapshot, error),
) {
	if c == nil || fetch == nil {
		return
	}
	c.SyncQuotaSelection()
	c.streamMu.Lock()
	if c.closing || c.usageWork.inFlight || c.usageWork.resetInFlight {
		c.streamMu.Unlock()
		return
	}
	c.usageWork.inFlight = true
	c.usageWork.generation++
	generation := c.usageWork.generation
	fctx, cancel := context.WithTimeout(ctx, quotaFetchTimeout)
	c.usageWork.cancel = cancel
	id := c.modelCfg.ProviderID
	c.usageWork.workers.Add(1)
	c.streamMu.Unlock()

	go func() {
		defer c.usageWork.workers.Done()
		defer cancel()
		snapshot, err := fetch(fctx, id)
		c.streamMu.Lock()
		current := !c.closing && generation == c.usageWork.generation
		if current {
			c.usageWork.inFlight = false
			c.usageWork.cancel = nil
		}
		c.streamMu.Unlock()
		if current {
			c.publish(UsageQuotaMsg{
				Generation:  generation,
				ProviderID:  id,
				Snapshot:    snapshot,
				Err:         err,
				Unsupported: errors.Is(err, provider.ErrQuotaUnsupported),
			})
		}
	}()
}
