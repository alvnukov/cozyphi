package notify

import (
	"context"
	"fmt"
	"os/exec"
	"sync/atomic"
	"time"

	"github.com/alvnukov/cozyphi/internal/debuglog"
)

// sendTimeout bounds one notification subprocess: a stuck notification daemon
// must not pile up child processes for the rest of the session.
const sendTimeout = 2 * time.Second

// Mode decides when notifications reach the user.
type Mode uint8

const (
	// ModeOff disables notifications.
	ModeOff Mode = iota
	// ModeAlways notifies regardless of terminal focus.
	ModeAlways
	// ModeUnfocused notifies only while the terminal is not focused.
	ModeUnfocused
)

// ParseMode converts a notifications.mode config value into a Mode. The empty
// string is invalid here; the config layer decides what an absent key means.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "off":
		return ModeOff, nil
	case "always":
		return ModeAlways, nil
	case "unfocused":
		return ModeUnfocused, nil
	}
	return ModeOff, fmt.Errorf("invalid notifications.mode %q: want off, always or unfocused", s)
}

// sendFunc delivers one notification.
type sendFunc func(ctx context.Context, title, body string) error

// commandRunner runs one external command; a seam the sender tests use to
// observe the exact argv without spawning processes.
type commandRunner func(ctx context.Context, name string, args ...string) error

// runCommand runs an external command to completion under ctx.
func runCommand(ctx context.Context, name string, args ...string) error {
	if err := exec.CommandContext(ctx, name, args...).Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// Notifier sends desktop notifications about agent state changes without
// blocking the UI goroutine. A nil Notifier is valid and does nothing.
type Notifier struct {
	mode Mode
	send sendFunc
	// focusTrusted turns on the first time the terminal reports losing focus.
	// Until then a "focused" report may just be the synthetic one every
	// session starts with, and a terminal that never sends focus changes
	// would go silent forever in ModeUnfocused.
	focusTrusted atomic.Bool
	focused      atomic.Bool
	inflight     atomic.Bool
	broken       atomic.Bool
	onFailure    atomic.Pointer[func(error)]
}

// Option customizes a Notifier at construction.
type Option func(*Notifier)

// WithSender overrides the platform sender; used by tests.
func WithSender(sender func(ctx context.Context, title, body string) error) Option {
	return func(n *Notifier) { n.send = sender }
}

// New returns a Notifier for the given mode with the platform sender.
func New(mode Mode, opts ...Option) *Notifier {
	n := &Notifier{mode: mode, send: platformSender()}
	for _, opt := range opts {
		opt(n)
	}
	// Optimistic default: assume the terminal is focused until it reports
	// otherwise, so terminals without focus reporting never get spammed.
	n.focused.Store(true)
	return n
}

// SetFocused records whether the terminal window has focus.
func (n *Notifier) SetFocused(focused bool) {
	if n == nil {
		return
	}
	if !focused {
		n.focusTrusted.Store(true)
	}
	n.focused.Store(focused)
}

// SetOnFailure registers the callback used when the platform sender fails and
// notifications switch off. It is called once, from the sending goroutine, so
// a UI must marshal it onto its own thread.
func (n *Notifier) SetOnFailure(handle func(error)) {
	if n == nil {
		return
	}
	if handle == nil {
		n.onFailure.Store(nil)
		return
	}
	n.onFailure.Store(&handle)
}

// TurnEnded notifies that the model finished or stopped and waits for input.
func (n *Notifier) TurnEnded() {
	n.dispatch("cozyphi", "Turn finished — waiting for input")
}

// NeedsAttention notifies that the model is waiting for the user — a
// permission request, a question, or a continue prompt. An empty detail
// yields a generic body.
func (n *Notifier) NeedsAttention(detail string) {
	body := detail
	if body == "" {
		body = "The model is waiting for your input"
	}
	n.dispatch("cozyphi", body)
}

// dispatch applies mode gating and hands the send to a background goroutine.
func (n *Notifier) dispatch(title, body string) {
	if n == nil || n.mode == ModeOff || n.send == nil || n.broken.Load() {
		return
	}
	if n.mode == ModeUnfocused && n.focusTrusted.Load() && n.focused.Load() {
		return
	}
	// One notification in flight at a time: while a send is running, a newer
	// notification is dropped rather than queued behind it — they arrive
	// within seconds of each other and say much the same thing.
	if !n.inflight.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer n.inflight.Store(false)
		ctx, cancel := context.WithTimeout(context.Background(), sendTimeout)
		defer cancel()
		if err := n.send(ctx, title, body); err != nil {
			n.broken.Store(true)
			debuglog.Logf("notify: sender failed, disabling notifications: %v", err)
			if handle := n.onFailure.Load(); handle != nil {
				(*handle)(err)
			}
		}
	}()
}
