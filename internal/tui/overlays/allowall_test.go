package overlays

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func askBodyText(o *Overlays) string {
	body, _ := o.perm.askRows(o.theme, askInnerWidth(80), 0)
	parts := make([]string, 0, len(body))
	for _, row := range body {
		parts = append(parts, lineText(row))
	}
	return strings.Join(parts, "\n")
}

// TestAskExplainsTheHighlightedOption: every option carries its fine
// print — the explain row follows the ring, so select-then-activate
// always reads what the choice actually creates.
func TestAskExplainsTheHighlightedOption(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request:     permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:       reply,
		PersistPath: "/home/u/.cozyphi/config.yaml",
	})

	if got := askBodyText(o); !strings.Contains(got, "Runs this call once") {
		t.Fatalf("option 1 must explain itself, got:\n%s", got)
	}
	o.perm.ring.Step(1)
	if got := askBodyText(o); !strings.Contains(got, "Stops asking for every tool and command") {
		t.Fatalf("the session allow-all must say it covers everything, got:\n%s", got)
	}
	o.perm.ring.Step(1)
	got := askBodyText(o)
	if !strings.Contains(got, "permissions.dangerously_allow_all") {
		t.Fatalf("the persistent allow-all must name the rule, got:\n%s", got)
	}
	if !strings.Contains(got, "config.yaml") {
		t.Fatalf("the persistent allow-all must name the file, got:\n%s", got)
	}
}

// TestAskPersistentChoiceArmsAConfirm: the permanent grant is guarded by
// the standard's armed y/n question — it names the file, y writes, n and
// Esc keep the ask open, and any other key withdraws and still acts.
func TestAskPersistentChoiceArmsAConfirm(t *testing.T) {
	press := func(o *Overlays, code xui.KeyCode, r rune) {
		o.handlePermissionKey(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
	}
	begin := func() (*Overlays, chan controller.AskReply) {
		o := testOverlays(controller.NewActivityHandler(nil))
		reply := make(chan controller.AskReply, 1)
		o.beginPermissionAsk(controller.PermissionAskMsg{
			Request:     permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
			Reply:       reply,
			PersistPath: "/home/u/.cozyphi/config.yaml",
		})
		return o, reply
	}

	o, reply := begin()
	press(o, xui.KeyRune, '3')
	if o.perm == nil || !o.perm.confirm.Armed() {
		t.Fatal("the persistent option must arm a confirm, not resolve")
	}
	if got := askBodyText(o); !strings.Contains(got, "(y/n)") || !strings.Contains(got, "config.yaml") {
		t.Fatalf("the armed question must render with its target, got:\n%s", got)
	}
	press(o, xui.KeyRune, 'y')
	select {
	case r := <-reply:
		if !r.Approved || !r.AllowPersistent {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("y must fire the armed write")
	}

	// n keeps the ask open with nothing armed.
	o, reply = begin()
	press(o, xui.KeyRune, '3')
	press(o, xui.KeyRune, 'n')
	if o.perm == nil || o.perm.confirm.Armed() {
		t.Fatal("n must disarm and keep the ask")
	}

	// Esc withdraws the question — it must not deny the whole ask.
	press(o, xui.KeyRune, '3')
	press(o, xui.KeyEscape, 0)
	if o.perm == nil {
		t.Fatal("Esc on an armed question must only withdraw it")
	}
	select {
	case <-reply:
		t.Fatal("no reply may be sent while withdrawing")
	default:
	}

	// Any other key withdraws and still acts: 1 approves this call once.
	press(o, xui.KeyRune, '3')
	press(o, xui.KeyRune, '1')
	select {
	case r := <-reply:
		if !r.Approved || r.AllowPersistent || r.AllowSession {
			t.Fatalf("got %+v", r)
		}
	default:
		t.Fatal("a digit must withdraw the question and answer")
	}
}

// TestAskMouseArmsAndWithdrawsThePersistentConfirm: the mouse passes
// through the same question — activating the persistent option arms it,
// wheeling withdraws it.
func TestAskMouseArmsAndWithdrawsThePersistentConfirm(t *testing.T) {
	o := testOverlays(controller.NewActivityHandler(nil))
	reply := make(chan controller.AskReply, 1)
	o.beginPermissionAsk(controller.PermissionAskMsg{
		Request:     permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:       reply,
		PersistPath: "/home/u/.cozyphi/config.yaml",
	})
	drawTestPanel(t, o)
	body, _ := o.perm.askRows(o.theme, askInnerWidth(80), 0)
	y := rowContaining(t, body, "Every Session")

	o.HandleAskMouse(&components.EventContext{}, click(4, y))
	if o.perm.ring.Selected() != int(askOptAllowPersistent) {
		t.Fatal("the first click must select the persistent option")
	}
	// A frame passes between the clicks: the explain row grew, so the
	// panel re-measures and the option lands on a fresh row.
	drawTestPanel(t, o)
	body, _ = o.perm.askRows(o.theme, askInnerWidth(80), 0)
	o.HandleAskMouse(&components.EventContext{}, click(4, rowContaining(t, body, "Every Session")))
	if o.perm == nil || !o.perm.confirm.Armed() {
		t.Fatal("activating the persistent option by mouse must arm the confirm")
	}
	select {
	case <-reply:
		t.Fatal("arming must not resolve")
	default:
	}

	wheel := xui.MouseEvent{X: 40, Y: 12, Button: xui.MouseWheelDown, Action: xui.MousePress}
	o.HandleAskMouse(&components.EventContext{}, wheel)
	if o.perm == nil || o.perm.confirm.Armed() {
		t.Fatal("wheeling is acting elsewhere: it must withdraw the question")
	}
}
