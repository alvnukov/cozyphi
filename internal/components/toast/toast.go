package toast

import (
	"slices"
	"time"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/layout"
)

// ToastKind selects border/icon colors for a toast.
type ToastKind int

// Toast kinds select the border and icon colors for a notification.
const (
	ToastSuccess ToastKind = iota
	ToastError
	ToastWarning
)

// queued is one notification waiting for the slot.
type queued struct {
	message string
	kind    ToastKind
	d       time.Duration
}

// Entry is one reported notification, kept for the history view.
type Entry struct {
	Message string
	Kind    ToastKind
	At      time.Time
}

const (
	// queueCap bounds the waiting line; past it the oldest waiter drops.
	queueCap = 8
	// historyCap is how many reported notifications History remembers.
	historyCap = 20
)

// Toast is a top overlay notification (green border + ✓ for success).
// One message owns the slot at a time; another Show waits its turn in a
// queue instead of overwriting, so an error is never cut short by the
// info toast that follows it.
type Toast struct {
	Message string
	Kind    ToastKind
	Until   time.Time
	Theme   components.Theme

	queue   []queued
	history []Entry
}

// Show reports message for d (default 2s): shown now if the slot is free,
// queued behind the current message otherwise. Every message lands in the
// history either way.
func (t *Toast) Show(message string, kind ToastKind, d time.Duration) {
	if d <= 0 {
		d = 2 * time.Second
	}
	t.remember(message, kind)
	if t.Visible() {
		if len(t.queue) >= queueCap {
			t.queue = t.queue[1:]
		}
		t.queue = append(t.queue, queued{message, kind, d})
		return
	}
	t.display(message, kind, d)
}

// Clear dismisses the current toast; queued messages still take their turn.
func (t *Toast) Clear() {
	t.Message = ""
	t.Until = time.Time{}
	t.advance()
}

// Visible reports whether a toast should be drawn, letting the next queued
// message take an expired slot first.
func (t *Toast) Visible() bool {
	t.advance()
	return t.Message != "" && time.Now().Before(t.Until)
}

// History returns the reported notifications, newest first.
func (t *Toast) History() []Entry {
	out := make([]Entry, 0, len(t.history))
	for _, e := range slices.Backward(t.history) {
		out = append(out, e)
	}
	return out
}

// advance hands the slot to the next queued message once the current one
// expires: the queue drains one full lifetime per message, never by
// overwrite.
func (t *Toast) advance() {
	for (t.Message == "" || !time.Now().Before(t.Until)) && len(t.queue) > 0 {
		next := t.queue[0]
		t.queue = t.queue[1:]
		t.display(next.message, next.kind, next.d)
	}
}

func (t *Toast) display(message string, kind ToastKind, d time.Duration) {
	t.Message = message
	t.Kind = kind
	t.Until = time.Now().Add(d)
}

func (t *Toast) remember(message string, kind ToastKind) {
	t.history = append(t.history, Entry{Message: message, Kind: kind, At: time.Now()})
	if len(t.history) > historyCap {
		t.history = t.history[len(t.history)-historyCap:]
	}
}

func (t *Toast) theme() components.Theme {
	if t.Theme.Success.Fg.Kind == 0 && t.Theme.Foreground.Fg.Kind == 0 {
		return components.DefaultTheme()
	}
	return t.Theme
}

// Handle is a no-op; toasts do not take input.
func (*Toast) Handle(_ *components.EventContext, _ xui.Event) {}

// Draw paints a full-screen transparent host with the toast bar near the top.
func (t *Toast) Draw(ctx components.DrawContext) components.Surface {
	maxW, maxH := ctx.Max.Width, ctx.Max.Height
	if maxW <= 0 {
		maxW = 80
	}
	if maxH <= 0 {
		maxH = 24
	}
	out := components.Surface{Size: components.Size{Width: maxW, Height: maxH}, Widget: t}
	if !t.Visible() {
		return out
	}

	th := t.theme()
	border, icon := toastChrome(t.Kind, th)
	fg := th.Foreground

	msg := t.Message
	inner := " " + icon
	if icon != "" {
		inner += " "
	}
	inner += msg + " "

	// width = max(floor(screen*0.95), 40), shrink to content.
	boxW := maxW * 95 / 100
	if boxW < 40 {
		boxW = maxW
		boxW = min(boxW, 40)
	}
	if boxW > maxW-2 {
		boxW = maxW - 2
		if boxW < 8 {
			boxW = maxW
		}
	}
	tw := xui.StringWidth(inner, ctx.Method) + 2 // + borders
	if tw < boxW {
		boxW = tw
	}
	if boxW < 8 {
		boxW = 8
	}
	boxH := 3
	boxH = min(boxH, maxH)

	panel := components.NewSurface(boxW, boxH, t)
	fill := xui.Style{Fg: fg.Fg}
	for y := 0; y < boxH; y++ {
		for x := 0; x < boxW; x++ {
			panel.SetCell(x, y, xui.Cell{Char: " ", Width: 1, Style: fill})
		}
	}
	layout.DrawRoundedBorder(&panel, layout.BorderRounded, border, nil, nil, nil, nil, ctx.Method)

	textX := 1
	if icon != "" {
		panel.Print(textX, 1, icon+" ", border, ctx.Method)
		textX += xui.StringWidth(icon+" ", ctx.Method)
	}
	avail := boxW - 1 - textX
	avail = max(avail, 1)
	panel.Print(textX, 1, layout.TruncateToWidth(msg, avail, ctx.Method), fg, ctx.Method)

	ox := (maxW - boxW) / 2
	oy := 1
	if oy+boxH > maxH {
		oy = 0
	}
	if ox < 0 {
		ox = 0
	}
	out.Children = []components.SubSurface{{
		Origin:  components.Point{X: ox, Y: oy},
		Surface: panel,
		Z:       50,
	}}
	return out
}

func toastChrome(kind ToastKind, th components.Theme) (border xui.Style, icon string) {
	switch kind {
	case ToastError:
		return th.Destructive, "✕"
	case ToastWarning:
		return th.Warning, ""
	default:
		return th.Success, "✓"
	}
}
