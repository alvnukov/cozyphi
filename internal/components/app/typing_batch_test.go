package app

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
)

// fakeTTY is a terminal that records every write. onWrite runs inside a
// write, standing in for input that arrives while the terminal is still
// draining the frame.
type fakeTTY struct {
	cols, rows int
	out        bytes.Buffer
	writes     int
	onWrite    func()
}

func (*fakeTTY) Read([]byte) (int, error) { return 0, io.EOF }

func (f *fakeTTY) Write(p []byte) (int, error) {
	f.writes++
	f.out.Write(p)
	if hook := f.onWrite; hook != nil {
		f.onWrite = nil
		hook()
	}
	return len(p), nil
}

func (f *fakeTTY) Size() (int, int, error) { return f.cols, f.rows, nil }
func (*fakeTTY) MakeRaw() error            { return nil }
func (*fakeTTY) Restore() error            { return nil }
func (*fakeTTY) Fd() uintptr               { return 0 }
func (*fakeTTY) Close() error              { return nil }
func (*fakeTTY) Interrupt()                {}

func (f *fakeTTY) reset() {
	f.out.Reset()
	f.writes = 0
}

// typist is a one-line text field whose key handling depends on the last
// Draw, like a composer that moves its caret through wrapped rows.
type typist struct {
	text  string
	draws int
	// seen is the Draw count each key found when it arrived.
	seen  []int
	mouse int
}

func (w *typist) Handle(ctx *components.EventContext, ev xui.Event) {
	switch e := ev.(type) {
	case xui.KeyEvent:
		w.text += string(e.Rune)
		w.seen = append(w.seen, w.draws)
		ctx.Redraw = true
	case xui.MouseEvent:
		w.mouse++
		ctx.Redraw = true
		ctx.Consume = true
	}
}

func (w *typist) Draw(ctx components.DrawContext) components.Surface {
	w.draws++
	s := components.NewSurface(ctx.Max.Width, ctx.Max.Height, w)
	s.Print(0, 0, w.text, xui.Style{}, ctx.Method)
	return s
}

func newTypingApp(t *testing.T) (*App, *typist, *fakeTTY) {
	t.Helper()
	tty := &fakeTTY{cols: 40, rows: 4}
	vx, err := xui.NewWithTTY(tty, xui.Options{})
	if err != nil {
		t.Fatal(err)
	}
	root := &typist{}
	a := NewApp(vx, components.DefaultTheme())
	a.root = root
	a.loop = xui.NewLoop(vx)
	if err := a.paint(); err != nil {
		t.Fatal(err)
	}
	a.redraw = false
	tty.reset()
	return a, root, tty
}

func postKeys(a *App, text string) {
	for _, r := range text {
		a.loop.Post(keyPress(r, 0))
	}
}

func stepOnce(t *testing.T, a *App) {
	t.Helper()
	if quit, err := a.step(); quit || err != nil {
		t.Fatalf("step: quit=%v err=%v", quit, err)
	}
}

// Keys typed while the terminal is still taking the previous frame share the
// next one: a slow terminal gets one frame to catch up on, not one per key
// that it falls further behind on.
func TestKeysQueuedDuringAFrameWriteShareTheNextFrame(t *testing.T) {
	a, root, tty := newTypingApp(t)
	tty.onWrite = func() { postKeys(a, "ello") }
	postKeys(a, "h")

	stepOnce(t, a)
	if root.text != "h" || tty.writes != 1 {
		t.Fatalf("first key: text %q, %d writes", root.text, tty.writes)
	}
	tty.reset()

	stepOnce(t, a)
	if root.text != "hello" {
		t.Fatalf("queued keys not handled in one step: %q", root.text)
	}
	if tty.writes != 1 {
		t.Fatalf("four queued keys wrote %d frames, want 1", tty.writes)
	}
	if !strings.Contains(tty.out.String(), "ello") {
		t.Fatalf("shared frame lacks the typed text: %q", tty.out.String())
	}
}

// Every key in a batch sees the frame its predecessor produced.
func TestBatchedKeysSeeTheLayoutOfTheKeyBefore(t *testing.T) {
	a, root, _ := newTypingApp(t)
	postKeys(a, "abc")

	stepOnce(t, a)
	for i := 1; i < len(root.seen); i++ {
		if root.seen[i] <= root.seen[i-1] {
			t.Fatalf("key %d handled without a Draw after key %d: draws seen %v", i, i-1, root.seen)
		}
	}
}

// A mouse event is hit-tested against the frame on screen, so it ends the
// batch: the keys before it are written first, and it runs on the next step.
func TestMouseEventEndsTheTypingBatch(t *testing.T) {
	a, root, tty := newTypingApp(t)
	postKeys(a, "ab")
	a.loop.Post(xui.MouseEvent{X: 1, Y: 0, Button: xui.MouseLeft, Action: xui.MousePress})
	postKeys(a, "c")

	stepOnce(t, a)
	if root.text != "ab" || root.mouse != 0 || tty.writes != 1 {
		t.Fatalf("batch ran past the mouse event: text %q, mouse %d, %d writes", root.text, root.mouse, tty.writes)
	}
	stepOnce(t, a)
	if root.mouse != 1 || root.text != "ab" {
		t.Fatalf("mouse step: text %q, mouse %d", root.text, root.mouse)
	}
	stepOnce(t, a)
	if root.text != "abc" {
		t.Fatalf("key after the mouse event lost: %q", root.text)
	}
}

// A long burst is split so typed-in text keeps appearing while it drains.
func TestTypingBatchIsBounded(t *testing.T) {
	a, root, tty := newTypingApp(t)
	postKeys(a, strings.Repeat("x", maxTypingBatch+6))

	stepOnce(t, a)
	if len(root.text) != maxTypingBatch || tty.writes != 1 {
		t.Fatalf("first batch: %d keys, %d writes", len(root.text), tty.writes)
	}
	stepOnce(t, a)
	if len(root.text) != maxTypingBatch+6 {
		t.Fatalf("rest of the burst: %d keys", len(root.text))
	}
}
