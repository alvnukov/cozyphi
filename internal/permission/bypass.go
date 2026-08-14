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

// Check returns Allow whenever the bypass is enabled; otherwise it defers to the inner gate.
func (g *BypassGate) Check(ctx context.Context, req Request) (Decision, string) {
	if g != nil && g.Enabled != nil && g.Enabled.Load() {
		return Allow, ""
	}
	if g == nil || g.Inner == nil {
		return Allow, ""
	}
	return g.Inner.Check(ctx, req)
}
