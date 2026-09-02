package permission

import (
	"context"
	"sync/atomic"
)

// BypassGate wraps an inner Gate and allows everything when Enabled is true
// ("Allow All for This Session").
type BypassGate struct {
	Inner   Gate
	Enabled *atomic.Bool
}

// Check returns Allow whenever the bypass is explicitly enabled; otherwise
// it defers to the inner gate. An incomplete assembly — a nil receiver or a
// missing inner — denies with an actionable reason: a half-built boundary
// must never permit.
func (g *BypassGate) Check(ctx context.Context, req Request) (Decision, string) {
	if g == nil {
		return Deny, "permission gate unavailable: request denied (gate not assembled)"
	}
	if g.Enabled != nil && g.Enabled.Load() {
		return Allow, ""
	}
	if g.Inner == nil {
		return Deny, "permission gate unavailable: request denied (inner gate missing)"
	}
	return g.Inner.Check(ctx, req)
}
