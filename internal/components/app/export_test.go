package app

import "github.com/pulseaiclub/xui"

// DispatchEvent exposes the production event path to external integration tests.
func (a *App) DispatchEvent(ev xui.Event) bool { return a.handleEvent(ev) }
