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
			assert.Empty(t, child.Status().Waiting,
				"an answer raises no message of its own, so the row is cleared from here")
		})
	}
}

// TestAnAskFollowsTheUserFromScreenToScreen: one ask, never two. Changing the
// screen moves the question the user has not answered yet onto the screen they
// are looking at now, and answering it anywhere resolves the same call.
func TestAnAskFollowsTheUserFromScreenToScreen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	reply := make(chan controller.AskReply, 1)
	child.Update(childPermAsk(reply))
	require.True(t, parent.overlays.PermissionActive())

	parent.Family().Show("job-1")
	assert.True(t, child.overlays.PermissionActive(), "the ask goes where the user went")
	assert.False(t, parent.overlays.PermissionActive(), "and it does not stay behind as well")
	assert.NotContains(t, viewText(t, child), "["+childName+"]",
		"a session looking at its own ask needs no label")

	parent.Family().Show("")
	assert.True(t, parent.overlays.PermissionActive(), "and it comes back with them")
	assert.False(t, child.overlays.PermissionActive())
	assert.Contains(t, viewText(t, parent), "["+childName+"] Run this command?",
		"away from home it says whose call it is again")

	pressAsk(t, parent, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y'})
	select {
	case r := <-reply:
		assert.True(t, r.Approved)
	default:
		t.Fatal("the answer must reach the child that asked, whatever screen it was given on")
	}
	assert.False(t, parent.overlays.PermissionActive())
	assert.False(t, child.overlays.PermissionActive())
	assert.Empty(t, child.Status().Waiting, "an answered ask stops the row saying the child waits")
}

// TestTheParentsOwnAskFollowsTheUserOntoAChildsScreen: the traffic goes both
// ways. The session that owns the family keeps asking while the user reads a
// sub-agent, so its question opens there, named — and it is the user's own
// session, so the rule that outlives every session is still on offer.
func TestTheParentsOwnAskFollowsTheUserOntoAChildsScreen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	parent.Family().Show("job-1")

	reply := make(chan controller.AskReply, 1)
	ask := childPermAsk(reply)
	ask.PersistPath = "/home/u/.cozyphi/config.yaml"
	parent.Update(ask)

	require.True(t, child.overlays.PermissionActive(), "the parent's ask opens where the user is")
	assert.False(t, parent.overlays.PermissionActive())
	text := viewText(t, child)
	assert.Contains(t, text, "[main] Run this command?", "an unnamed session is the main one")
	assert.Contains(t, text, "Every Session",
		"the permanent grant is only withheld from a sub-agent's ask")

	pressAsk(t, child, xui.KeyEvent{Code: xui.KeyEscape})
	select {
	case r := <-reply:
		assert.False(t, r.Approved)
	default:
		t.Fatal("the parent's own call is answered from the child's screen")
	}
	assert.Empty(t, parent.Status().Waiting)
}

// TestAnAskOnAChildsScreenWearsTheParentsRegistryName: the label names the
// session, so a renamed one is named, not called "main" regardless.
func TestAnAskOnAChildsScreenWearsTheParentsRegistryName(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	parent.SetIdentity(2, "loader")
	parent.Family().Show("job-1")

	parent.Update(childPermAsk(make(chan controller.AskReply, 1)))
	assert.Contains(t, viewText(t, child), "[loader] Run this command?")
}

// TestASecondAskWaitsItsTurnAndOpensWhenTheFirstIsAnswered: the screen holds
// one question at a time, so an arriving ask never takes the panel away from
// the one being answered — it queues, its session's row says so, and it opens
// the moment the first is out of the way.
func TestASecondAskWaitsItsTurnAndOpensWhenTheFirstIsAnswered(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	own := make(chan controller.AskReply, 1)
	parent.Update(childPermAsk(own))
	require.True(t, parent.overlays.PermissionActive())

	queued := make(chan controller.AskReply, 1)
	child.Update(childPermAsk(queued))

	assert.NotContains(t, viewText(t, parent), "["+childName+"]", "the user's own question is untouched")
	assert.False(t, child.overlays.PermissionActive(), "a waiting ask is drawn nowhere at all")
	assert.Equal(t, "permission", child.Status().Waiting, "the row says the child is waiting for the panel")

	pressAsk(t, parent, xui.KeyEvent{Code: xui.KeyRune, Rune: 'y'})
	require.NotEmpty(t, own, "the first answer went to the session that asked it")
	assert.True(t, parent.overlays.PermissionActive(), "the next question opens at once, with no timer")
	assert.Contains(t, viewText(t, parent), "["+childName+"] Run this command?")

	pressAsk(t, parent, xui.KeyEvent{Code: xui.KeyEscape})
	select {
	case r := <-queued:
		assert.False(t, r.Approved)
	default:
		t.Fatal("the queued ask is answered into its own channel")
	}
	assert.False(t, parent.overlays.PermissionActive(), "and then the panel is free")
	assert.Empty(t, child.Status().Waiting)
}

// TestWithdrawingAChildsAskFindsItOnAnotherScreen: the runtime drops an ask
// it can no longer use, and it goes wherever the family put it — on the screen
// or still waiting behind another question.
func TestWithdrawingAChildsAskFindsItOnAnotherScreen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	child.Update(childPermAsk(make(chan controller.AskReply, 1)))
	require.True(t, parent.overlays.PermissionActive())

	child.Update(controller.PermissionDismissMsg{})

	assert.False(t, parent.overlays.PermissionActive(), "the withdrawal reaches the screen that showed it")
	assert.Empty(t, child.Status().Waiting, "and the row stops saying the child is waiting")

	// The same message finds an ask that never reached a screen.
	own := make(chan controller.AskReply, 1)
	parent.Update(childPermAsk(own))
	child.Update(childPermAsk(make(chan controller.AskReply, 1)))
	require.Equal(t, "permission", child.Status().Waiting)

	child.Update(controller.PermissionDismissMsg{})
	assert.Empty(t, child.Status().Waiting, "a queued ask is dropped where it waited")

	pressAsk(t, parent, xui.KeyEvent{Code: xui.KeyEscape})
	assert.False(t, parent.overlays.PermissionActive(), "and nothing takes its place")
}

// TestReleasingAChildDeniesTheQuestionItLeftOpen: the call the question
// guarded is gone with the sub-agent, so it is answered no — never yes — on
// screen or in the queue, and the panel does not outlive the session it
// belongs to.
func TestReleasingAChildDeniesTheQuestionItLeftOpen(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	shown := make(chan controller.AskReply, 1)
	queued := make(chan controller.ContinueReply, 1)
	child.Update(childPermAsk(shown))
	child.Update(controller.ContinueAskMsg{MaxRounds: 40, Reply: queued})
	require.True(t, parent.overlays.PermissionActive())

	released, ok := parent.Family().Release("job-1")
	require.True(t, ok)
	require.Same(t, child, released)

	assert.False(t, parent.overlays.Active(), "a released child takes its panel with it")
	select {
	case r := <-shown:
		assert.False(t, r.Approved, "a question nobody can answer for is denied")
		assert.False(t, r.AllowSession)
	default:
		t.Fatal("the released child's call must not be left waiting")
	}
	select {
	case r := <-queued:
		assert.False(t, r.Continue, "the one that never reached a screen is denied too")
	default:
		t.Fatal("a queued ask must not be left waiting either")
	}
}

// TestClosingTheParentDeniesEveryAskTheFamilyHolds: the session the user was
// talking to is going away, so every blocked call under it — its own and its
// sub-agents', on screen and queued — is answered no rather than left waiting
// for a screen that will not come back.
func TestClosingTheParentDeniesEveryAskTheFamilyHolds(t *testing.T) {
	parent, child, _ := childAskFixture(t)
	own := make(chan controller.AskReply, 1)
	queued := make(chan controller.AskReply, 1)
	parent.Update(childPermAsk(own))
	child.Update(childPermAsk(queued))
	require.True(t, parent.overlays.PermissionActive())

	parent.BeginClose()

	for name, reply := range map[string]chan controller.AskReply{"own": own, "queued": queued} {
		select {
		case r := <-reply:
			assert.False(t, r.Approved, "%s: a closing session grants nothing", name)
		default:
			t.Fatalf("%s: the call must not be left waiting on a closed session", name)
		}
	}
	assert.False(t, parent.overlays.Active())
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
