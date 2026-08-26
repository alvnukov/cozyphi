//go:build windows

package app

// watchResume is a no-op on Windows: SIGCONT has no Windows equivalent, so
// there is no detach-resume signal to repaint after.
func (a *App) watchResume() {}
