package main

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/xui"
)

// Minimal immediate-mode example: draw text and quit on Ctrl+C / q.
func main() {
	vx, err := xui.New(xui.Options{})
	if err != nil {
		panic(err)
	}
	defer vx.Close()

	if err := vx.EnterAltScreen(); err != nil {
		panic(err)
	}
	loop := xui.NewLoop(vx)
	loop.Start()
	defer loop.Stop()
	vx.NotifyWinsize(loop)
	vx.QueryTerminal(300 * time.Millisecond)

	style := xui.Style{Fg: xui.RGBColor(0x7a, 0xaa, 0xf7), Bold: true}
	dim := xui.Style{Fg: xui.IndexedColor(8)}
	n := 0
	redraw := true

	for {
		if redraw {
			win := vx.Window()
			win.Clear()
			win.Print(2, 1, "xui hello", style)
			win.Print(2, 3, fmt.Sprintf("frames: %d  (q / Ctrl+C to quit)", n), dim)
			if err := vx.Render(); err != nil {
				panic(err)
			}
			redraw = false
			n++
		}
		ev := loop.NextEvent()
		switch e := ev.(type) {
		case xui.KeyEvent:
			if e.CtrlC() || (e.Code == xui.KeyRune && (e.Rune == 'q' || e.Rune == 'Q')) {
				return
			}
			redraw = true
		case xui.ResizeEvent:
			vx.Resize(e.Cols, e.Rows)
			redraw = true
		}
	}
}
