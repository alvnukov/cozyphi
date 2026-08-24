package xui

import (
	"context"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pulseaiclub/xui/input"
)

// escIdle is how long input must stay quiet before a held lone Esc is
// delivered as a key press instead of waiting for sequence bytes that are
// never coming. 50ms matches common terminal escape-time settings: instant
// for a human finger, yet far below any real sequence's byte gap.
const escIdle = 50 * time.Millisecond

// Loop reads TTY input on a background goroutine and delivers Events.
type Loop struct {
	vx      *XUI
	parser  *input.Parser
	events  chan input.Event
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	running atomic.Bool
}

// NewLoop creates an event loop bound to vx.
func NewLoop(vx *XUI) *Loop {
	return &Loop{
		vx:     vx,
		parser: input.NewParser(),
		events: make(chan input.Event, 512),
	}
}

// Start begins the reader goroutine.
func (l *Loop) Start() {
	if l.running.Swap(true) {
		return
	}
	if clearer, ok := l.vx.tty.(interface{ ClearDeadline() }); ok {
		clearer.ClearDeadline()
	}
	ctx, cancel := context.WithCancel(context.Background())
	l.cancel = cancel
	l.wg.Add(1)
	go l.readLoop(ctx)
}

// Stop stops the reader goroutine.
func (l *Loop) Stop() {
	if !l.running.Swap(false) {
		return
	}
	if l.cancel != nil {
		l.cancel()
	}
	if l.vx != nil && l.vx.tty != nil {
		l.vx.tty.Interrupt()
	}
	l.wg.Wait()
}

// NextEvent blocks until an application event is available.
func (l *Loop) NextEvent() input.Event { return <-l.events }

// TryEvent returns an event if one is pending.
func (l *Loop) TryEvent() (input.Event, bool) {
	select {
	case e := <-l.events:
		return e, true
	default:
		return nil, false
	}
}

// Events exposes the event channel.
func (l *Loop) Events() <-chan input.Event { return l.events }

// Post injects an event (e.g. Resize from SIGWINCH).
func (l *Loop) Post(ev input.Event) {
	select {
	case l.events <- ev:
	default:
	}
}

func (l *Loop) readLoop(ctx context.Context) {
	defer l.wg.Done()
	buf := make([]byte, 4096)
	var lastByte time.Time
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := l.vx.tty.Read(buf)
		if n > 0 {
			lastByte = time.Now()
			for _, ev := range l.parser.Feed(buf[:n]) {
				l.handle(ev)
			}
		}
		// A lone Esc held by the parser is a finished key press, not half a
		// sequence: once input falls quiet past escIdle, deliver it. The raw
		// TTY read returns at least every VTIME interval, so idle wakes reach
		// this check even with no input at all.
		if l.parser.Pending() && time.Since(lastByte) >= escIdle {
			for _, ev := range l.parser.FlushIdle() {
				l.handle(ev)
			}
		}
		if err != nil {
			if err == io.EOF {
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
				continue
			}
		}
	}
}

func (l *Loop) handle(ev input.Event) {
	switch e := ev.(type) {
	case input.CapEvent:
		l.vx.applyCap(e)
		return
	default:
		l.Post(e)
	}
}
