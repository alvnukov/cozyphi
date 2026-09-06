package editor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

// childShell is a shell with one tab and one sub-agent adopted by it, plus
// the buses both sides publish on.
type childShell struct {
	shell      *editor.Editor
	registry   *sessions.Registry
	parent     *sessions.View
	child      *sessions.View
	parentBus  *controller.Bus
	childBus   *controller.Bus
	parentID   string
	makeTab    func(name string) (string, *sessions.View, *controller.Bus)
	appliction *app.App
}

func newChildShell(t *testing.T) *childShell {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	makeView := func() (*sessions.View, *controller.Bus) {
		bus := controller.NewBus(nil)
		return sessions.NewView(application, bus, nil, nil, nil, components.DefaultTheme(),
			t.TempDir(), "test", "", 1000, nil, nil), bus
	}
	makeTab := func(name string) (string, *sessions.View, *controller.Bus) {
		view, bus := makeView()
		id, err := registry.Open(name, view)
		require.NoError(t, err)
		return id, view, bus
	}
	parentID, parent, parentBus := makeTab("main")
	require.NoError(t, shell.Activate(parentID))
	child, childBus := makeView()
	require.NoError(t, parent.Family().Adopt("job-1", "explore(read the loader)", child))
	return &childShell{
		shell: shell, registry: registry, parent: parent, child: child,
		parentBus: parentBus, childBus: childBus, parentID: parentID,
		makeTab: makeTab, appliction: application,
	}
}

func TestASubAgentIsNeverATabButItsBusIsStillDrained(t *testing.T) {
	f := newChildShell(t)
	require.Equal(t, 1, f.registry.Len(), "a sub-agent takes no selector slot")
	require.Len(t, f.registry.Entries(), 1)
	assert.Equal(t, []*sessions.View{f.parent, f.child}, f.shell.Views(),
		"the shell still owns the child: it built it and it will close it")

	f.childBus.Publish(controller.SetActivityMsg{Activity: controller.ActivityStreaming})
	f.shell.DrainNow()
	assert.True(t, f.child.Status().Running,
		"nothing else drains a child's bus: the shell does it for every family")
	f.shell.ShowChild(f.child)
	assert.Contains(t, drawText(f.shell), "#1 explore(read the loader)",
		"a child is labeled with the opening number of the session that owns it")
}

func TestShowChildSwapsTheScreenWithoutMovingTheSelector(t *testing.T) {
	f := newChildShell(t)
	f.shell.ShowChild(f.child)
	require.Same(t, f.child, f.shell.Screen())
	active, ok := f.registry.Active()
	require.True(t, ok)
	assert.Equal(t, f.parentID, active.ID, "the selector keeps marking the session that owns the child")

	for _, ch := range "childdraft" {
		f.shell.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: ch, Press: true})
	}
	assert.Contains(t, drawText(f.shell), "childdraft", "keys reach the session on screen")

	f.shell.ShowMain()
	require.Same(t, f.parent, f.shell.Screen())
	assert.NotContains(t, drawText(f.shell), "childdraft", "the parent's composer kept its own draft")
}

// TestTheShellsOwnScreenChangeMovesThePendingAsk: the band is not the only
// door onto a sub-agent's screen — the shell opens and leaves one itself — so
// the family's open question has to follow the user through this door too,
// unanswered and never twice.
func TestTheShellsOwnScreenChangeMovesThePendingAsk(t *testing.T) {
	f := newChildShell(t)
	reply := make(chan controller.AskReply, 1)
	f.childBus.Publish(controller.PermissionAskMsg{
		Request: permission.Request{Tool: "bash", Action: permission.ActionBash, Command: "curl https://x"},
		Reply:   reply,
	})
	f.shell.DrainNow()
	require.Contains(t, drawText(f.shell), "[explore(read the loader)] Run this command?")

	f.shell.ShowChild(f.child)
	text := drawText(f.shell)
	assert.Contains(t, text, "Run this command?", "the question opens on the screen the user opened")
	assert.NotContains(t, text, "[explore(read the loader)] Run this command?",
		"a session looking at its own ask needs no label")

	f.shell.ShowMain()
	assert.Contains(t, drawText(f.shell), "[explore(read the loader)] Run this command?",
		"and it comes back with the user")
	assert.Empty(t, reply, "moving a question answers nothing")
}

func TestSelectingAnotherTabDropsTheChildScreen(t *testing.T) {
	f := newChildShell(t)
	f.shell.ShowChild(f.child)
	otherID, other, _ := f.makeTab("second")
	require.NoError(t, f.shell.Activate(otherID))
	assert.Same(t, other, f.shell.Screen(), "a selection change ends the sub-agent detour")
}

func TestCloseOnAChildScreenOnlyReturnsToTheParent(t *testing.T) {
	f := newChildShell(t)
	f.shell.ShowChild(f.child)
	require.NoError(t, f.shell.CloseCurrent())
	assert.Same(t, f.parent, f.shell.Screen())
	assert.Equal(t, 1, f.registry.Len(), "/close on a sub-agent closes nothing")
	assert.Equal(t, 1, f.parent.Family().Len(), "and retains the sub-agent itself")
}

func TestQuittingIsRefusedWhileASubAgentStillRuns(t *testing.T) {
	f := newChildShell(t)
	f.childBus.Publish(controller.SetActivityMsg{Activity: controller.ActivityStreaming})
	f.shell.DrainNow()
	require.True(t, f.child.Status().Running)

	require.True(t, f.shell.AcceptInterrupt(), "the first Ctrl+C arms the exit on the screen")
	require.True(t, f.shell.AcceptInterrupt(), "the second is refused: a sub-agent is still working")
	text := drawText(f.shell)
	assert.Contains(t, text, "Background sessions still running")
	assert.Contains(t, text, "main → explore(read the loader)",
		"a sub-agent has no tab, so it is named by the session that owns it")
}

func TestTheAttentionNoticeIgnoresSubAgents(t *testing.T) {
	f := newChildShell(t)
	otherID, other, _ := f.makeTab("second")
	require.NoError(t, f.shell.Activate(otherID))
	require.NotNil(t, other)

	f.childBus.Publish(controller.RunEndedMsg{})
	f.shell.DrainNow()
	require.NotEmpty(t, f.child.Status().Attention, "the child did record its own attention")
	assert.NotContains(t, drawText(f.shell), "explore(read the loader): turn ended",
		"a sub-agent cannot be reached with /switch, so it never asks for it")

	f.parentBus.Publish(controller.RunEndedMsg{})
	f.shell.DrainNow()
	assert.Contains(t, strings.ReplaceAll(drawText(f.shell), "  ", " "), "#1 main: turn ended",
		"a session with a tab still asks")
}

func TestClosingTheShellClosesTheSubAgentsToo(t *testing.T) {
	f := newChildShell(t)
	require.NoError(t, f.shell.Close(context.WithoutCancel(t.Context())))
	assert.True(t, f.child.Closed(), "a sub-agent has no tab, so only the family can close it")
	assert.True(t, f.parent.Closed())
}

// A sub-agent has no tab of its own, so the tab that owns it says whose screen
// this is: "● main › explore(read the loader)". Going back clears the mark.
func TestTheSelectorMarksTheTabWhoseSubAgentIsOnScreen(t *testing.T) {
	f := newChildShell(t)
	assert.NotContains(t, drawText(f.shell), "› explore(read the loader)",
		"the session on its own screen is marked by nobody")

	f.shell.ShowChild(f.child)
	assert.Contains(t, drawText(f.shell), "● main › explore(read the loader)")

	otherID, _, _ := f.makeTab("second")
	require.NoError(t, f.shell.Activate(otherID))
	assert.NotContains(t, drawText(f.shell), "› explore(read the loader)",
		"a selection change ends the detour, and the mark with it")

	require.NoError(t, f.shell.Activate(f.parentID))
	f.shell.ShowChild(f.child)
	require.Contains(t, drawText(f.shell), "● main › explore(read the loader)")
	f.shell.ShowMain()
	assert.NotContains(t, drawText(f.shell), "› explore(read the loader)")
}
