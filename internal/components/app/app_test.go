package app

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/chat"
)

// quitRoot is a stand-in root widget that requests quit on a chosen rune.
type quitRoot struct {
	quitRune rune
	handled  []xui.Event
}

func (r *quitRoot) Handle(ctx *components.EventContext, ev xui.Event) {
	if ke, ok := ev.(xui.KeyEvent); ok && ke.Press && ke.Code == xui.KeyRune && ke.Rune == r.quitRune {
		ctx.Quit = true
		return
	}
	r.handled = append(r.handled, ev)
}

func (r *quitRoot) Draw(_ components.DrawContext) components.Surface {
	return components.Surface{Widget: r}
}

func keyPress(r rune, mods xui.Modifiers) xui.KeyEvent {
	return xui.KeyEvent{Code: xui.KeyRune, Rune: r, Mods: mods, Press: true}
}

// Every quit path must return from Run: the caller releases the resources the
// UI was wired to (session hooks, job manager, MCP pool) after Run returns.
// A path that exits the loop any other way would leak them.
func TestHandleEventCtrlCQuitsWithoutDispatch(t *testing.T) {
	root := &quitRoot{}
	a := &App{root: root}
	if quit := a.handleEvent(keyPress('c', xui.ModCtrl)); !quit {
		t.Fatal("Ctrl+C must quit")
	}
	if len(root.handled) != 0 {
		t.Fatalf("Ctrl+C must not reach widgets, got %d event(s)", len(root.handled))
	}
}

func TestHandleEventRootQuitRequestQuits(t *testing.T) {
	root := &quitRoot{quitRune: 'q'}
	a := &App{root: root}
	if quit := a.handleEvent(keyPress('q', 0)); !quit {
		t.Fatal("EventContext.Quit from a widget must quit")
	}
}

func TestHandleEventRegularKeyDispatches(t *testing.T) {
	root := &quitRoot{}
	a := &App{root: root}
	if quit := a.handleEvent(keyPress('x', 0)); quit {
		t.Fatal("a regular key must not quit")
	}
	if len(root.handled) != 1 {
		t.Fatalf("root must receive the key, got %d event(s)", len(root.handled))
	}
}

// Ctrl+C over an active composer selection must copy instead of quitting —
// the focused ChatInput claims the chord through CopyKeyAcceptor.
func TestHandleEventCtrlCWithComposerSelectionCopies(t *testing.T) {
	c := &chat.ChatInput{Value: "hello world"}
	c.SetSelection(0, 5)
	copied := ""
	c.OnCopy = func(s string) bool { copied = s; return true }
	root := &quitRoot{}
	a := &App{root: root, focused: c}
	if quit := a.handleEvent(keyPress('c', xui.ModCtrl)); quit {
		t.Fatal("Ctrl+C with a composer selection must not quit")
	}
	if copied != "hello" {
		t.Fatalf("copied = %q, want %q", copied, "hello")
	}
	if len(root.handled) != 0 {
		t.Fatalf("copy chord must not bubble, got %d event(s)", len(root.handled))
	}
}

// interruptRoot claims a bounded number of Ctrl+C presses as interrupts before
// it lets the app quit, standing in for an editor with work still in flight.
type interruptRoot struct {
	quitRoot
	claims int
}

func (r *interruptRoot) AcceptInterrupt() bool {
	if r.claims == 0 {
		return false
	}
	r.claims--
	return true
}

// Ctrl+C is an interrupt for as long as the root has work to stop: the press
// must not quit and must not reach widgets as a key, and it must redraw so the
// user sees what it stopped. Only a press the root declines exits the app.
func TestHandleEventCtrlCInterruptsBeforeQuitting(t *testing.T) {
	root := &interruptRoot{claims: 1}
	a := &App{root: root}
	if quit := a.handleEvent(keyPress('c', xui.ModCtrl)); quit {
		t.Fatal("a claimed Ctrl+C must not quit")
	}
	if len(root.handled) != 0 {
		t.Fatalf("an interrupt must not reach widgets, got %d event(s)", len(root.handled))
	}
	if !a.redraw {
		t.Fatal("an interrupt must redraw so its effect is visible")
	}
	if quit := a.handleEvent(keyPress('c', xui.ModCtrl)); !quit {
		t.Fatal("a Ctrl+C the root declines must quit")
	}
}

// panicRoot panics mid-Draw, like a widget hitting bad width math on a frame.
type panicRoot struct{ draws int }

func (*panicRoot) Handle(*components.EventContext, xui.Event) {}
func (r *panicRoot) Draw(components.DrawContext) components.Surface {
	r.draws++
	panic("width math gone wrong")
}

// A panic inside Draw must not escape the draw path: the frame becomes an
// error surface naming the panic, and the widget tree stays runnable so the
// next event still paints a frame instead of killing the process.
func TestDrawTreeRecoversPanicIntoErrorSurface(t *testing.T) {
	root := &panicRoot{}
	a := &App{root: root}
	surf := a.drawTree(components.DrawContext{
		Max: components.Size{Width: 40, Height: 10},
	})
	if root.draws != 1 {
		t.Fatalf("Draw must have run, got %d calls", root.draws)
	}
	text := components.ExtractSurfaceText(surf, 0, 0, surf.Size.Width, surf.Size.Height)
	if !strings.Contains(text, "render error") || !strings.Contains(text, "width math gone wrong") {
		t.Fatalf("error surface must name the panic, got %q", text)
	}
	if surf.Size.Width != 40 || surf.Size.Height != 10 {
		t.Fatalf("error surface must cover the screen, got %dx%d", surf.Size.Width, surf.Size.Height)
	}
}

// A widget that panics once and then recovers must keep getting frames: the
// recover is per frame, not a one-shot that bricks the loop.
func TestDrawTreeSurvivesRepeatedPanics(t *testing.T) {
	root := &flakyRoot{}
	a := &App{root: root}
	for range 3 {
		a.drawTree(components.DrawContext{Max: components.Size{Width: 20, Height: 4}})
	}
	if root.draws != 3 {
		t.Fatalf("every frame must reach Draw, got %d of 3", root.draws)
	}
}

type flakyRoot struct{ draws int }

func (*flakyRoot) Handle(*components.EventContext, xui.Event) {}
func (r *flakyRoot) Draw(components.DrawContext) components.Surface {
	r.draws++
	if r.draws%2 == 1 {
		panic("flaky")
	}
	return components.Surface{Widget: r}
}
