package sessions

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

const childName = "explore(read the loader)"

// childAskFixture is one parent with one retained sub-agent, both wired to a
// notifier so the ping a routed ask sends can be read back.
func childAskFixture(t *testing.T) (parent, child *View, notifier *fakeNotifier) {
	t.Helper()
	parent, newChild := familyFixture(t)
	notifier = &fakeNotifier{}
	parent.SetAttentionNotifier(notifier)
	child = newChild()
	child.SetAttentionNotifier(&fakeNotifier{})
	require.NoError(t, parent.Family().Adopt("job-1", childName, child))
	return parent, child, notifier
}

func childPermAsk(reply chan controller.AskReply) controller.PermissionAskMsg {
	return controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	}
}

// viewText draws a view and returns everything on it, overlay included.
func viewText(t *testing.T, e *View) string {
	t.Helper()
	return components.SurfaceText(e.Draw(components.DrawContext{
		Max:    components.Size{Width: 120, Height: 40},
		Method: xui.WidthUnicode,
	}))
}

// pressAsk gives the permission panel one key, the way the view's ladder does.
func pressAsk(t *testing.T, e *View, ev xui.KeyEvent) {
	t.Helper()
	ev.Press = true
	require.True(t, e.overlays.HandlePermissionKey(&components.EventContext{}, ev))
}

// TestAHiddenChildsAskLandsOnTheScreenTheUserIsOn: a sub-agent has no screen
// of its own until the user opens it, so its question is asked where the user
// is, under the name of the session that raised it.
func TestAHiddenChildsAskLandsOnTheScreenTheUserIsOn(t *testing.T) {
	parent, child, notifier := childAskFixture(t)
	reply := make(chan controller.AskReply, 1)

	child.Update(childPermAsk(reply))

	assert.True(t, parent.overlays.PermissionActive(), "the ask goes to the screen in front of the user")
	assert.False(t, child.overlays.PermissionActive(), "and it is not shown twice")
	assert.Contains(t, viewText(t, parent), "["+childName+"] Run this command?",
		"the panel says whose call it is answering")

	assert.Equal(t, "permission", child.Status().Waiting, "the hidden child still records what it waits for")
	assert.Empty(t, parent.Status().Waiting, "the parent is not the one waiting")
	assert.Contains(t, parent.Status().Attention, "["+childName+"]",
		"a sub-agent has no tab, so the mark that names a session to switch to is the parent's")
	assert.Equal(t, []string{"bash"}, notifier.attention, "the parent's notifier is the one the user can find")
}

// TestAnsweringARoutedAskRepliesToTheChild: approve and deny both land in the
// sub-agent's own channel, and neither ends its assignment.
func TestAnsweringARoutedAskRepliesToTheChild(t *testing.T) {
	for _, tc := range []struct {
		name     string
		key      xui.KeyEvent
		approved bool
	}{
		{"approve", xui.KeyEvent{Code: xui.KeyRune, Rune: 'y'}, true},
		{"deny", xui.KeyEvent{Code: xui.KeyEscape}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent, child, _ := childAskFixture(t)
			child.Update(controller.SetActivityMsg{Activity: controller.ActivityTools})
			require.True(t, child.Status().Running)
			reply := make(chan controller.AskReply, 1)
			child.Update(childPermAsk(reply))

			pressAsk(t, parent, tc.key)

			select {
			case r := <-reply:
				assert.Equal(t, tc.approved, r.Approved)
				assert.False(t, r.AllowPersistent, "no rule outlives the session from here")
			default:
				t.Fatal("the answer must reach the session that asked")
			}
			assert.False(t, parent.overlays.PermissionActive())
			assert.True(t, child.Status().Running, "one denied call does not end the assignment")
			assert.False(t, child.Status().Stopped)
		})
	}
}

// TestOpeningTheChildLeavesItsAskWhereItWasAsked: switching screens while a
// question is up neither loses nor copies it — it stays on the screen that
// asked it, and answering it there still resolves.
func TestOpeningTheChildLeavesItsAskWhereItWasAsked(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	reply := make(chan controller.AskReply, 1)
	child.Update(childPermAsk(reply))

	parent.Family().Show("job-1")
	assert.True(t, parent.overlays.PermissionActive(), "the ask stays where the user saw it")
	assert.False(t, child.overlays.PermissionActive(), "opening the child does not clone it")

	pressAsk(t, parent, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y'})
	select {
	case r := <-reply:
		assert.True(t, r.Approved)
	default:
		t.Fatal("the answer must still reach the child after the screen moved")
	}

	// On its own screen the child keeps its next ask, unlabeled.
	child.Update(childPermAsk(make(chan controller.AskReply, 1)))
	assert.True(t, child.overlays.PermissionActive())
	assert.False(t, parent.overlays.PermissionActive())
	assert.NotContains(t, viewText(t, child), "["+childName+"]",
		"a session looking at its own ask needs no label")
}

// TestAnArrivingChildAskNeverCancelsTheOneOnScreen: the panel holds one
// question at a time, so a sub-agent that asks while the user is answering
// something waits on its own screen instead of taking the panel away.
func TestAnArrivingChildAskNeverCancelsTheOneOnScreen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	own := make(chan controller.AskReply, 1)
	parent.Update(childPermAsk(own))
	require.True(t, parent.overlays.PermissionActive())

	child.Update(childPermAsk(make(chan controller.AskReply, 1)))

	assert.NotContains(t, viewText(t, parent), "["+childName+"]", "the user's own question is untouched")
	assert.True(t, child.overlays.PermissionActive(), "the child holds its ask until the panel is free")
	assert.Equal(t, "permission", child.Status().Waiting)
}

// TestWithdrawingAChildsAskFindsItOnAnotherScreen: the runtime drops an ask
// it can no longer use, and the panel closes wherever it was shown.
func TestWithdrawingAChildsAskFindsItOnAnotherScreen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	child.Update(childPermAsk(make(chan controller.AskReply, 1)))
	require.True(t, parent.overlays.PermissionActive())

	child.Update(controller.PermissionDismissMsg{})

	assert.False(t, parent.overlays.PermissionActive(), "the withdrawal reaches the screen that showed it")
	assert.Empty(t, child.Status().Waiting, "and the row stops saying the child is waiting")
}

// TestReleasingAChildDeniesTheQuestionItLeftOpen: the call the question
// guarded is gone with the sub-agent, so it is answered no — never yes — and
// the panel does not outlive the session it belongs to.
func TestReleasingAChildDeniesTheQuestionItLeftOpen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	reply := make(chan controller.AskReply, 1)
	child.Update(childPermAsk(reply))
	require.True(t, parent.overlays.PermissionActive())

	released, ok := parent.Family().Release("job-1")
	require.True(t, ok)
	require.Same(t, child, released)

	assert.False(t, parent.overlays.PermissionActive(), "a released child takes its panel with it")
	select {
	case r := <-reply:
		assert.False(t, r.Approved, "a question nobody can answer for is denied")
		assert.False(t, r.AllowSession)
	default:
		t.Fatal("the released child's call must not be left waiting")
	}
}

// TestAChildTurnEndingRaisesNoDesktopPing: the parent's own turn end is what
// the user is waiting on; a sub-agent finishing is progress, not a prompt.
func TestAChildTurnEndingRaisesNoDesktopPing(t *testing.T) {
	parent, child, notifier := childAskFixture(t)
	childNotifier := &fakeNotifier{}
	child.SetAttentionNotifier(childNotifier)

	child.Update(controller.RunEndedMsg{})
	assert.Zero(t, childNotifier.turns, "a sub-agent's turn end stays inside the panel")

	parent.Update(controller.RunEndedMsg{})
	assert.Equal(t, 1, notifier.turns, "the session the user talks to keeps its ping")
}

// TestARoutedQuestionAndContinueAskAlsoNameTheirSession: the label is not a
// permission-only affordance; every ask a hidden child raises wears it.
func TestARoutedQuestionAndContinueAskAlsoNameTheirSession(t *testing.T) {
	parent, child, _ := childAskFixture(t)

	child.Update(controller.ContinueAskMsg{MaxRounds: 40, Reply: make(chan controller.ContinueReply, 1)})
	require.True(t, parent.overlays.ContinueActive())
	text := viewText(t, parent)
	assert.Contains(t, text, "["+childName+"] Reached max tool rounds")
	assert.Equal(t, "continue", child.Status().Waiting)
}
