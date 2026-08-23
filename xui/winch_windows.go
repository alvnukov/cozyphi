//go:build windows

package xui

func (vx *XUI) startWinchSignals(loop *Loop) {
	// Windows: rely on in-band resize or polling; no SIGWINCH.
	_ = loop
}

func (vx *XUI) stopWinchSignals() {}
