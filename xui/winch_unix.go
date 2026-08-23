//go:build unix

package xui

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/pulseaiclub/xui/input"
)

func (vx *XUI) startWinchSignals(loop *Loop) {
	vx.stopWinchSignals()
	vx.winchCh = make(chan os.Signal, 1)
	vx.stopWinch = make(chan struct{})
	signal.Notify(vx.winchCh, syscall.SIGWINCH)
	go func() {
		for {
			select {
			case <-vx.winchCh:
				cols, rows, err := vx.tty.Size()
				if err != nil {
					continue
				}
				loop.Post(input.ResizeEvent{Cols: cols, Rows: rows})
			case <-vx.stopWinch:
				signal.Stop(vx.winchCh)
				return
			}
		}
	}()
}

func (vx *XUI) stopWinchSignals() {
	if vx.stopWinch != nil {
		select {
		case <-vx.stopWinch:
		default:
			close(vx.stopWinch)
		}
	}
	if vx.winchCh != nil {
		signal.Stop(vx.winchCh)
	}
}
