//go:build !windows

package app

import (
	"os"
	"os/signal"
	"syscall"
)

// watchResume asks for a full repaint after SIGCONT: the terminal state
// (cursor position, alt-screen contents) is unknown after a detach. The
// flag is acted on by paint() on the UI goroutine — QueueRefresh is not
// safe off it.
func (a *App) watchResume() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGCONT)
	go func() {
		for range ch {
			a.resumeRefresh.Store(true)
			a.sched.Request()
		}
	}()
}
