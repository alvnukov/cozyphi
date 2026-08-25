package app

import (
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
