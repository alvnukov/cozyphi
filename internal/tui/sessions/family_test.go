package sessions

import (
	"fmt"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tui/agentpanel"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// familyFixture builds one parent view and a maker for the child views it
// adopts. Everything shares a single controller: these tests are about the
// family's own bookkeeping, and each view carries its own status.
func familyFixture(t *testing.T) (*View, func() *View) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())

	bus := controller.NewBus(nil)
	ctrl, err := controller.NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	t.Cleanup(ctrl.Close)

	newView := func() *View {
		return NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
	}
	return newView(), newView
}

// pressKey drives the band the way the view's key ladder does.
func pressKey(t *testing.T, f *Family, code xui.KeyCode) {
	t.Helper()
	require.True(t, f.HandleEvent(nil, xui.KeyEvent{Code: code, Press: true}))
}

func TestFamilyAdoptsChildrenAndHandsThemBackOnRelease(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.Equal(t, 0, f.Len())
	require.Same(t, parent, f.Screen(), "an empty family shows the session that owns it")

	first, second := newChild(), newChild()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", first))
	require.NoError(t, f.Adopt("job-2", "build(rename the seam)", second))
	assert.Equal(t, 2, f.Len())
	assert.True(t, f.Has("job-1"))
	assert.Equal(t, []*View{first, second}, f.Views(), "rows follow adoption order")
	assert.Equal(t, "job-2", second.ChildJobID())
	assert.Equal(t, "build(rename the seam)", second.ChildTitle())
	assert.Same(t, f, second.Family(), "a child draws the band its parent owns")

	require.Error(t, f.Adopt("job-1", "again", newChild()), "one job is retained once")
	require.Error(t, f.Adopt("", "nameless", newChild()), "a child without an assignment is refused")

	f.Show("job-2")
	assert.Same(t, second, f.Screen())
	assert.Equal(t, "job-2", f.Current())

	released, ok := f.Release("job-2")
	require.True(t, ok)
	assert.Same(t, second, released)
	assert.Equal(t, 1, f.Len())
	assert.Equal(t, "", f.Current(), "releasing the open child puts the parent back on screen")
	assert.Same(t, parent, f.Screen())

	_, ok = f.Release("job-2")
	assert.False(t, ok, "a released child is released once")
}

func TestFamilyRefusesMoreChildrenThanItRetains(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	for i := range familyCap {
		require.NoError(t, f.Adopt(fmt.Sprintf("job-%d", i), "worker", newChild()))
	}
	err := f.Adopt("job-over", "worker", newChild())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "12")
	assert.Equal(t, familyCap, f.Len())
}

func TestFamilyRowsReportEachChildsOwnState(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	running, waiting, failed, stopped := newChild(), newChild(), newChild(), newChild()
	waiting.lifetime.status = Status{Waiting: "permission"}
	failed.lifetime.status = Status{Error: "the child could not read the file"}
	stopped.lifetime.status = Status{Stopped: true}
	require.NoError(t, f.Adopt("job-run", "explore(read the loader)", running))
	require.NoError(t, f.Adopt("job-wait", "build(apply the patch)", waiting))
	require.NoError(t, f.Adopt("job-fail", "explore(find the caller)", failed))
	require.NoError(t, f.Adopt("job-stop", "build(rename the seam)", stopped))

	rows := f.rows()
	require.Len(t, rows, 4)
	assert.Equal(t, "job-run", rows[0].ID)
	assert.Equal(t, "explore(read the loader)", rows[0].Title, "a row is named the way the transcript names it")
	assert.Equal(t, agentpanel.StateRunning, rows[0].State)
	assert.Equal(t, agentpanel.StateWaiting, rows[1].State)
	assert.Equal(t, "permission", rows[1].Waiting)
	assert.Equal(t, agentpanel.StateFailed, rows[2].State)
	assert.Equal(t, agentpanel.StateStopped, rows[3].State)
	assert.Zero(t, rows[0].Tools, "nothing is invented: the counts come from the parent's store")
}

func TestChildStateFollowsTheRecordedOutcome(t *testing.T) {
	assert.Equal(t, agentpanel.StateDone, childState(job.StatusCompleted))
	assert.Equal(t, agentpanel.StateStopped, childState(job.StatusCancelled))
	assert.Equal(t, agentpanel.StateFailed, childState(job.StatusFailed))
	assert.Equal(t, agentpanel.StateFailed, childState(job.StatusTimedOut),
		"a timeout is a failure to the user: the child never came back")
}

func TestTheComposerLeavesDownIntoTheBandAndComesBack(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NotNil(t, parent.composer.Chat.OnLeaveDown, "the composer is wired to the band it draws")
	assert.False(t, parent.composer.Chat.OnLeaveDown(), "an empty band refuses the key it cannot use")

	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))
	require.True(t, parent.composer.Chat.OnLeaveDown())
	require.True(t, f.Focused())

	pressKey(t, f, xui.KeyUp) // ↑ on the main row is the way back
	assert.False(t, f.Focused())

	require.True(t, parent.composer.Chat.OnLeaveDown())
	require.True(t, f.Focused())
	pressKey(t, f, xui.KeyEscape)
	assert.False(t, f.Focused(), "Esc in the band means the composer, not an interrupt")
}

func TestEnterOpensAChildScreenAndMainReturns(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	child := newChild()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", child))
	var shown []*View
	f.SetOnShow(func(v *View) { shown = append(shown, v) })

	require.True(t, f.enter())
	pressKey(t, f, xui.KeyDown)
	pressKey(t, f, xui.KeyEnter)
	require.Equal(t, []*View{child}, shown, "Enter on a child row shows that child's screen")
	assert.Equal(t, "job-1", f.Current())
	assert.False(t, f.Focused(), "opening a session means talking to it")

	require.True(t, f.enter())
	pressKey(t, f, xui.KeyUp)
	pressKey(t, f, xui.KeyEnter)
	require.Len(t, shown, 2)
	assert.Nil(t, shown[1], "Enter on main puts the session that owns the child back")
	assert.Equal(t, "", f.Current())
}

func TestPressingXStopsAChildThroughTheManagerPath(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))
	require.ErrorIs(t, f.stop("job-1"), job.ErrNotFound,
		"x asks the job manager, the way agent_cancel does")

	require.True(t, f.enter())
	pressKey(t, f, xui.KeyDown)
	require.True(t, f.HandleEvent(nil, xui.KeyEvent{Code: xui.KeyRune, Rune: 'x', Press: true}))

	// The refusal the manager returned is what proves the press left the
	// widget: nothing inside the panel knows this job by name.
	surface := f.Draw(components.DrawContext{
		Max: components.Size{Width: 90, Height: 10}, Method: xui.WidthUnicode,
	}, 90)
	assert.Contains(t, components.SurfaceText(surface), "stop failed: "+job.ErrNotFound.Error())
}

// panelOverRows swaps the family's panel for one reading a seam the test
// drives, on a clock the test moves. The actions stay the family's own.
func panelOverRows(f *Family, rows func() []agentpanel.Row, now func() time.Time) {
	f.panel = agentpanel.New(f.parent.theme, rows, now, agentpanel.Actions{
		Open: f.open, Stop: f.stop, Leave: f.leave,
	})
}

func TestASuccessLeavesTheBandAndArmsTheFooterHint(t *testing.T) {
	parent, _ := familyFixture(t)
	f := parent.Family()
	clock := time.Now()
	row := agentpanel.Row{ID: "job-1", Title: "explore(read the loader)", State: agentpanel.StateRunning}
	panelOverRows(f, func() []agentpanel.Row { return []agentpanel.Row{row} }, func() time.Time { return clock })

	require.True(t, f.panel.Visible())
	hint, ok := f.footerHint()
	assert.False(t, ok, "a working child says nothing on the footer")

	row.State, row.Ended = agentpanel.StateDone, clock
	assert.False(t, f.panel.Visible(), "a success leaves the band at once")
	hint, ok = f.footerHint()
	require.True(t, ok)
	assert.Equal(t, "/agents to see agents", hint)

	clock = clock.Add(29 * time.Second)
	_, ok = f.footerHint()
	assert.True(t, ok, "the hint outlives the row for its window")

	clock = clock.Add(2 * time.Second)
	_, ok = f.footerHint()
	assert.False(t, ok, "and then the footer goes quiet again")
}

func TestAFailedRowStaysForItsWindow(t *testing.T) {
	parent, _ := familyFixture(t)
	f := parent.Family()
	clock := time.Now()
	row := agentpanel.Row{ID: "job-1", Title: "explore(read the loader)", State: agentpanel.StateFailed, Ended: clock}
	panelOverRows(f, func() []agentpanel.Row { return []agentpanel.Row{row} }, func() time.Time { return clock })

	require.True(t, f.panel.Visible())
	clock = clock.Add(29 * time.Second)
	assert.True(t, f.panel.Visible(), "a failure is readable for thirty seconds")
	clock = clock.Add(2 * time.Second)
	assert.False(t, f.panel.Visible())
}

func TestTheFooterShowsTheBandsKeysWhileItHoldsTheKeyboard(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))
	require.True(t, f.enter())

	hint, ok := f.footerHint()
	require.True(t, ok)
	assert.Equal(t, keys.Hints(keys.ScopeAgents), hint, "hints come from the catalog, never from here")
}
