package app

import (
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/chat"
	"github.com/pulseaiclub/phi/internal/components/input"
	"github.com/pulseaiclub/phi/internal/components/palette"
)

// minFrame caps the frame rate: event-driven draws and animation wakes fire
// no sooner than this after the previous frame.
const minFrame = time.Second / 60

// App is the vxfw-style application runtime.
type App struct {
	vx       *xui.XUI
	loop     *xui.Loop
	root     components.Widget
	focused  components.Widget
	lastSurf components.Surface
	redraw   bool
	sched    *scheduler
	// nextWake is the earliest follow-up frame the last draw asked for.
	nextWake time.Time
	// resumeRefresh requests a full repaint on the next paint() (SIGCONT).
	resumeRefresh atomic.Bool
	// pending is a single push-back slot used when coalesceWheel peeks past a
	// non-wheel event (must not Post to the end of the queue — that reorders).
	pending xui.Event
}

// NewApp creates an App around an existing Vaxis.
func NewApp(vx *xui.XUI) *App {
	return &App{vx: vx, redraw: true, sched: newScheduler(minFrame)}
}

// RequestRedraw schedules a frame from any goroutine (stream updates, etc).
func (a *App) RequestRedraw() {
	if a == nil {
		return
	}
	if a.loop != nil {
		a.sched.Request()
		return
	}
	a.redraw = true
}

// Run starts the event loop and drives root until quit: Ctrl+C, or a widget
// setting EventContext.Quit. Quit returns nil; a paint failure returns the
// error. Either way callers release the resources the UI was wired to after
// Run returns. Frames are demand-driven: events, scheduler wakes (streaming
// bursts, animations, toast expiry) — never a free-running ticker.
func (a *App) Run(root components.Widget) error {
	a.root = root
	a.loop = xui.NewLoop(a.vx)
	a.loop.Start()
	defer a.loop.Stop()

	if err := a.vx.EnterAltScreen(); err != nil {
		return err
	}
	a.vx.NotifyWinsize(a.loop)
	a.vx.QueryTerminal(500 * time.Millisecond)
	_ = a.vx.EnableMouse()
	a.watchResume()

	// Init event
	ctx := &components.EventContext{Redraw: true}
	a.dispatch(ctx, xui.FocusEvent{Focused: true})
	if ctx.Focus != nil {
		a.focused = ctx.Focus
	}
	a.redraw = true
	if err := a.paint(); err != nil {
		return err
	}
	a.redraw = false

	for {
		var ev xui.Event
		if a.pending != nil {
			ev = a.pending
			a.pending = nil
		} else {
			select {
			case ev = <-a.loop.Events():
			case <-a.sched.Due():
				a.redraw = true
				ev = nil
			}
		}
		if ev != nil {
			ev = a.coalesceWheel(ev)
			if a.handleEvent(ev) {
				return nil
			}
		}
		if a.redraw {
			if err := a.paint(); err != nil {
				return err
			}
			a.redraw = false
		}
	}
}

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

// coalesceWheel merges back-to-back wheel events into one with a summed Wheel
// count so a fast trackpad flick triggers a single redraw instead of dozens of
// partial paints (which leave CJK/ASCII ghost columns on the TTY).
func (a *App) coalesceWheel(ev xui.Event) xui.Event {
	m, ok := ev.(xui.MouseEvent)
	if !ok || (m.Button != xui.MouseWheelUp && m.Button != xui.MouseWheelDown) {
		return ev
	}
	if m.Wheel <= 0 {
		m.Wheel = 1
	}
	for {
		next, ok := a.loop.TryEvent()
		if !ok {
			break
		}
		n, ok := next.(xui.MouseEvent)
		if !ok || (n.Button != xui.MouseWheelUp && n.Button != xui.MouseWheelDown) {
			a.pending = next
			break
		}
		step := n.Wheel
		if step <= 0 {
			step = 1
		}
		if n.Button == m.Button {
			m.Wheel += step
			continue
		}
		// Opposite direction: net the deltas onto the surviving button.
		if m.Wheel > step {
			m.Wheel -= step
			continue
		}
		if m.Wheel < step {
			m.Button = n.Button
			m.Wheel = step - m.Wheel
			continue
		}
		m.Wheel = 0
	}
	if m.Wheel == 0 {
		m.Button = xui.MouseNone
		m.Action = xui.MouseMotion
	}
	// Full refresh heals any prior TTY desync before the scrolled frame paints.
	a.vx.QueueRefresh()
	return m
}

func (a *App) handleEvent(ev xui.Event) (quit bool) {
	ctx := &components.EventContext{}
	switch e := ev.(type) {
	case xui.ResizeEvent:
		a.vx.Resize(e.Cols, e.Rows)
		ctx.Redraw = true
	case xui.KeyEvent:
		if e.CtrlC() {
			return true
		}
		a.dispatch(ctx, e)
	case xui.TickEvent:
		ctx.Redraw = true
	case xui.MouseEvent:
		hit, lx, ly := a.lastSurf.HitTestAt(e.X, e.Y)
		if hit != nil {
			// Only text-entry widgets take keyboard focus. Transcript blocks
			// (tool/thinking/bash headers) consume clicks to expand, and used
			// to steal focus — leaving the composer cursor visible but dead.
			if e.Action == xui.MousePress {
				if acceptsKeyboardFocus(hit) {
					a.focused = hit
				} else if a.focused != nil && !acceptsKeyboardFocus(a.focused) {
					// Drop stale focus on list rows so keys bubble to the composer.
					a.focused = a.root
				}
			}
			local := e
			local.X, local.Y = lx, ly
			hit.Handle(ctx, local)
			if ctx.Consume {
				break
			}
		}
		// Bubble unconsumed mouse (absolute coords) so root can run selection / overlays.
		a.dispatch(ctx, e)
	default:
		a.dispatch(ctx, ev)
	}
	if ctx.Focus != nil {
		a.focused = ctx.Focus
		ctx.Redraw = true
	}
	if ctx.Quit {
		return true
	}
	if ctx.Redraw {
		a.redraw = true
	}
	return false
}

func (a *App) dispatch(ctx *components.EventContext, ev xui.Event) {
	// Capture → target → bubble (simplified: focused then root)
	if a.focused != nil && a.focused != a.root {
		a.focused.Handle(ctx, ev)
		if ctx.Consume {
			return
		}
	}
	if a.root != nil {
		a.root.Handle(ctx, ev)
	}
}

// RequestFocus moves keyboard focus to w (nil = root). Safe from the UI goroutine.
func (a *App) RequestFocus(w components.Widget) {
	if a == nil {
		return
	}
	if w == nil {
		w = a.root
	}
	a.focused = w
	a.redraw = true
}

// Focused reports the widget keyboard events are dispatched to; nil means the
// root widget.
func (a *App) Focused() components.Widget {
	if a == nil {
		return nil
	}
	return a.focused
}

// acceptsKeyboardFocus reports whether a mouse-press target should become the
// keyboard focus. Message-list rows handle clicks (expand/select) but typing
// must stay on the composer / palette / text fields.
func acceptsKeyboardFocus(w components.Widget) bool {
	switch w.(type) {
	case *chat.ChatInput, *palette.CommandPalette, *input.TextField:
		return true
	default:
		return false
	}
}

// paint draws one frame and closes the scheduler bookkeeping around it:
// Frame() advances the throttle window, At(nextWake) arms the follow-up
// frame the widgets asked for during Draw (animations, toast expiry).
func (a *App) paint() error {
	if a.resumeRefresh.Swap(false) {
		a.vx.QueueRefresh()
	}
	if err := a.draw(); err != nil {
		return err
	}
	a.sched.frame()
	a.sched.At(a.nextWake)
	return nil
}

func (a *App) draw() error {
	cols, rows := a.vx.Screen().Size()
	wake := time.Time{}
	ctx := components.DrawContext{
		Min:    components.Size{},
		Max:    components.Size{Width: cols, Height: rows},
		Method: xui.WidthUnicode,
		Wake:   &wake,
	}
	surf := a.root.Draw(ctx)
	a.nextWake = wake
	a.lastSurf = surf
	win := a.vx.Window()
	win.Clear()
	if cur := surf.Render(win); cur != nil {
		a.vx.Screen().SetCursor(cur.X, cur.Y)
	} else {
		a.vx.Screen().ClearCursor()
	}
	return a.vx.Render()
}
