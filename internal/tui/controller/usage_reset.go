package controller

import (
	"context"
	"sync"

	"github.com/alvnukov/cozyphi/internal/provider"
)

// usageWork is protected by streamMu; selection is sampled on the UI goroutine.
// A reset invalidates every earlier read, including results already on the bus.
type usageWork struct {
	generation      uint64
	inFlight        bool
	cancel          context.CancelFunc
	resetInFlight   bool
	resetCancel     context.CancelFunc
	workers         sync.WaitGroup
	model, provider string
}

// UsageResetMsg contains only the display-safe outcome of an explicitly
// confirmed reset. Completion asks the shell for a new read, never another reset.
type UsageResetMsg struct {
	InFlight bool
	Result   provider.QuotaResetResult
	Err      error
}

func (UsageResetMsg) isMsg() {}

// SyncQuotaSelection invalidates observations when the execution model changes.
// Call from the UI goroutine, like ModelConfig and SessionStats.
func (c *Controller) SyncQuotaSelection() bool {
	if c == nil {
		return false
	}
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	if c.usageWork.model == c.modelCfg.Name && c.usageWork.provider == c.modelCfg.ProviderID {
		return false
	}
	c.usageWork.model = c.modelCfg.Name
	c.usageWork.provider = c.modelCfg.ProviderID
	c.invalidateQuotaLocked()
	return true
}

// InvalidateQuota withdraws observations after credential changes without I/O.
func (c *Controller) InvalidateQuota() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	c.invalidateQuotaLocked()
}

func (c *Controller) invalidateQuotaLocked() {
	c.usageWork.generation++
	if c.usageWork.cancel != nil {
		c.usageWork.cancel()
	}
	c.usageWork.cancel = nil
	c.usageWork.inFlight = false
}

// AcceptQuota rejects obsolete bus messages even if they arrived after a reset
// or after switching away and back to the same provider.
func (c *Controller) AcceptQuota(msg UsageQuotaMsg) bool {
	if c == nil {
		return false
	}
	c.SyncQuotaSelection()
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	return !c.closing && !c.usageWork.resetInFlight &&
		msg.Generation == c.usageWork.generation && msg.ProviderID == c.modelCfg.ProviderID
}

// ResetQuota is called only by the UI's explicit credit-spend confirmation.
// Account binding and single-use authorization remain inside the provider.
func (c *Controller) ResetQuota(ctx context.Context, target *provider.QuotaResetTarget) {
	if c == nil || c.providers == nil {
		return
	}
	if c.SyncQuotaSelection() || c.modelCfg.ProviderID != "openai" {
		c.publish(UsageResetMsg{Err: provider.ErrQuotaResetStale})
		return
	}
	c.resetQuotaWith(ctx, target, c.providers.ResetQuota)
}

func (c *Controller) resetQuotaWith(
	ctx context.Context,
	target *provider.QuotaResetTarget,
	reset func(context.Context, *provider.QuotaResetTarget) (provider.QuotaResetResult, error),
) {
	if c == nil || reset == nil {
		return
	}
	c.streamMu.Lock()
	if c.closing || c.usageWork.resetInFlight {
		c.streamMu.Unlock()
		return
	}
	c.invalidateQuotaLocked()
	c.usageWork.resetInFlight = true
	rctx, cancel := context.WithTimeout(ctx, quotaFetchTimeout)
	c.usageWork.resetCancel = cancel
	c.usageWork.workers.Add(1)
	c.streamMu.Unlock()
	c.publish(UsageResetMsg{InFlight: true})
	go func() {
		defer c.usageWork.workers.Done()
		defer cancel()
		result, err := reset(rctx, target)
		c.streamMu.Lock()
		c.usageWork.resetInFlight = false
		c.usageWork.resetCancel = nil
		closing := c.closing
		c.streamMu.Unlock()
		if !closing {
			c.publish(UsageResetMsg{Result: result, Err: err})
		}
	}()
}

func (c *Controller) closeUsage() {
	c.streamMu.Lock()
	defer c.streamMu.Unlock()
	c.invalidateQuotaLocked()
	if c.usageWork.resetCancel != nil {
		c.usageWork.resetCancel()
	}
}
