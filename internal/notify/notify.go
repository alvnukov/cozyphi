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
// string is invalid here; DecodeConfig decides what an absent key means.
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

// String returns the notifications.mode key that parses back into the mode.
func (m Mode) String() string {
	switch m {
	case ModeAlways:
		return "always"
	case ModeUnfocused:
		return "unfocused"
	default:
		return "off"
	}
}

// DecodeConfig interprets the raw notifications.mode and notifications.sound
// keys. It is the one reading of the section in the process: the project
// loader and the settings manager both call it, so a settings checkbox and a
// boot-time load cannot disagree. Absent keys (empty strings) keep the
// documented defaults — unfocused mode, the platform default sound; "off"
// sound keeps notifications silent.
func DecodeConfig(mode, sound string) (Mode, string, error) {
	parsed := ModeUnfocused
	if mode != "" {
		var err error
		parsed, err = ParseMode(mode)
		if err != nil {
			return ModeOff, "", err
		}
	}
	switch sound {
	case "":
		return parsed, DefaultSound, nil
	case "off":
		return parsed, "", nil
	default:
		return parsed, sound, nil
	}
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
	// mode and sound live in atomics so a live Reconfigure from the UI
	// goroutine races no dispatch; sound is what the platform sender is
	// asked to play with each notification — empty keeps them silent.
	mode  atomic.Uint32
	sound atomic.Pointer[string]
	send  sendFunc
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

// WithSound names the sound the platform sender plays with each
// notification; DefaultSound unless set, "" for silence.
func WithSound(name string) Option {
	return func(n *Notifier) { n.sound.Store(&name) }
}

// New returns a Notifier for the given mode. Without WithSender it delivers
// through the platform sender, asking for the sound the notifier carries at
// dispatch time — a later Reconfigure changes it mid-session.
func New(mode Mode, opts ...Option) *Notifier {
	sound := DefaultSound
	n := &Notifier{}
	n.mode.Store(uint32(mode))
	n.sound.Store(&sound)
	for _, opt := range opts {
		opt(n)
	}
	if n.send == nil {
		n.send = func(ctx context.Context, title, body string) error {
			return platformSend(ctx, n.Sound(), title, body)
		}
	}
	// Optimistic default: assume the terminal is focused until it reports
	// otherwise, so terminals without focus reporting never get spammed.
	n.focused.Store(true)
	return n
}

// Sound returns the sound the next notification will ask for; empty keeps
// it silent.
func (n *Notifier) Sound() string {
	if s := n.sound.Load(); s != nil {
		return *s
	}
	return ""
}

// Reconfigure swaps the delivery mode and the sound mid-session — the
// settings pane applies a saved draft through here, so a checkbox takes
// effect without a restart. It does not resurrect a sender that already
// failed.
func (n *Notifier) Reconfigure(mode Mode, sound string) {
	if n == nil {
		return
	}
	n.mode.Store(uint32(mode))
	n.sound.Store(&sound)
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

// currentMode reads the atomic mode. Only Mode values are ever stored; the
// mask keeps the uint32→Mode narrowing well-defined regardless.
func (n *Notifier) currentMode() Mode { return Mode(n.mode.Load() & 0xff) }

// dispatch applies mode gating and hands the send to a background goroutine.
func (n *Notifier) dispatch(title, body string) {
	if n == nil || n.currentMode() == ModeOff || n.send == nil || n.broken.Load() {
		return
	}
	if n.currentMode() == ModeUnfocused && n.focusTrusted.Load() && n.focused.Load() {
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
