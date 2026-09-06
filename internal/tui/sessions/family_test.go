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
	assert.Empty(t, f.Current(), "releasing the open child puts the parent back on screen")
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
	assert.Empty(t, f.Current())
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
	_, ok := f.footerHint()
	assert.False(t, ok, "a working child says nothing on the footer")

	row.State, row.Ended = agentpanel.StateDone, clock
	assert.False(t, f.panel.Visible(), "a success leaves the band at once")
	hint, ok := f.footerHint()
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

// The /agents browser draws over the session: ShowAgents opens it, the rows it
// draws are the family's own children, and Escape gives the screen back.
func TestShowAgentsDrawsTheBrowserOverTheSession(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))

	require.NotNil(t, parent.agents)
	assert.False(t, parent.agents.Visible())
	parent.ShowAgents()
	require.True(t, parent.agents.Visible())

	text := components.SurfaceText(parent.Draw(components.DrawContext{
		Max: components.Size{Width: 100, Height: 30}, Method: xui.WidthUnicode,
	}))
	assert.Contains(t, text, "Agents  1 running · 1 total")
	assert.Contains(t, text, "explore(read the loader)")

	require.True(t, parent.agents.HandleEvent(&components.EventContext{},
		xui.KeyEvent{Press: true, Code: xui.KeyEscape}))
	assert.False(t, parent.agents.Visible())
}

// A child's browser lists its parent's family, not the empty one it was built
// with: the view is adopted after construction, and the seam is read late.
func TestAChildsBrowserListsItsParentsFamily(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	child := newChild()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", child))

	rows := child.family.Agents()
	require.Len(t, rows, 1)
	assert.Equal(t, "job-1", rows[0].JobID)
	assert.True(t, rows[0].Retained, "a child this session still holds can be opened")
}

// A retained child the job manager does not list still gets a row, built from
// what this session knows: the child's own state, and nothing invented.
func TestAgentsRowsFallBackToTheRetainedChildsOwnState(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	waiting, stopped, failed := newChild(), newChild(), newChild()
	waiting.lifetime.status = Status{Waiting: "permission"}
	stopped.lifetime.status = Status{Stopped: true}
	failed.lifetime.status = Status{Error: "the child could not read the file"}
	require.NoError(t, f.Adopt("job-wait", "review(check the gate)", waiting))
	require.NoError(t, f.Adopt("job-stop", "build(rename the seam)", stopped))
	require.NoError(t, f.Adopt("job-fail", "worker(patch the seam)", failed))

	rows := f.Agents()
	require.Len(t, rows, 3)
	assert.Equal(t, "permission", rows[0].Waiting)
	assert.Empty(t, rows[0].Status, "a job the manager never listed claims no status of its own")
	assert.Equal(t, job.StatusCancelled, rows[1].Status)
	assert.Equal(t, job.StatusFailed, rows[2].Status)
	assert.Equal(t, "the child could not read the file", rows[2].Error)
	assert.Zero(t, rows[0].Tools, "nothing is invented: the counts come from the parent's store")
	assert.Empty(t, rows[0].ResultPath)
}

// The result location is the file the job wrote, else the directory that keeps
// its record — never a path nobody recorded.
func TestResultLocationPrefersTheResultFile(t *testing.T) {
	assert.Equal(t, "/jobs/j1/result.md",
		resultLocation(job.Meta{Dir: "/jobs/j1", ResultPath: "/jobs/j1/result.md"}))
	assert.Equal(t, "/jobs/j1", resultLocation(job.Meta{Dir: "/jobs/j1"}))
	assert.Empty(t, resultLocation(job.Meta{}))
}

// Escape is the way out of a sub-agent's screen, and only from the screen the
// user is actually on: the parent's own ladder keeps its old last rung.
func TestEscapeLeavesTheChildScreenForTheSessionThatOwnsIt(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))
	require.NoError(t, f.Adopt("job-2", "build(rename the seam)", newChild()))
	var shown []*View
	f.SetOnShow(func(v *View) { shown = append(shown, v) })

	assert.False(t, f.escapeFrom(""), "the session that owns the family has nowhere to leave to")
	assert.False(t, f.escapeFrom("job-1"), "a child that is not on screen does not answer the key")

	f.Show("job-1")
	assert.False(t, f.escapeFrom("job-2"), "only the screen the user is on takes the key")
	require.True(t, f.escapeFrom("job-1"))
	assert.Empty(t, f.Current())
	require.Len(t, shown, 1, "Show puts the screen up itself; only the way back is announced here")
	assert.Nil(t, shown[0], "Escape goes the way the main row goes")
}

// The composer offers its exhausted Escape to the shell below it, and a child
// takes it: pressing the key on a sub-agent's screen, with nothing left to
// close, puts the session that owns it back. A parent declines, so Escape
// there still means what it always did.
func TestEscapeOnAChildScreenGoesBackToTheParent(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	child := newChild()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", child))
	var shown []*View
	f.SetOnShow(func(v *View) { shown = append(shown, v) })

	parent.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.Empty(t, shown, "the session that owns the family has nowhere to leave to")

	f.Show("job-1")
	require.Equal(t, "job-1", f.Current())
	child.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	assert.Empty(t, f.Current(), "an exhausted ladder ends on the way back")
	require.Len(t, shown, 1)
	assert.Nil(t, shown[0], "Escape goes the way the main row goes")
}

// A sub-agent's screen says on the footer how to get back, from the catalog.
func TestTheFooterOnAChildScreenShowsTheWayBack(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", newChild()))

	f.Show("job-1")
	hint, ok := f.footerHint()
	require.True(t, ok)
	assert.Equal(t, keys.Hints(keys.ScopeChild), hint)
	assert.Contains(t, hint, "main")

	f.Show("")
	hint, ok = f.footerHint()
	assert.False(t, ok, "a working family says nothing on the parent's footer")
	assert.Empty(t, hint)
}

// A child the user typed into again is working, whatever the parent recorded
// for its first assignment: the band shows ⟳ and stops claiming an end time,
// while the counts and the start it already had survive the follow-up.
func TestARowFollowsTheLiveChildAcrossAFollowUp(t *testing.T) {
	parent, newChild := familyFixture(t)
	f := parent.Family()
	child := newChild()
	require.NoError(t, f.Adopt("job-1", "explore(read the loader)", child))
	require.True(t, parent.transcript.ApplyJobProgress(job.Progress{
		JobID: "job-1", ToolUseID: "tool-1", Name: "read", Status: "done",
	}))
	require.True(t, parent.transcript.ApplyChildOutcome(job.Outcome{
		JobID: "job-1", Status: job.StatusCompleted, Summary: "read it",
	}))

	settled := f.row("job-1", child)
	require.Equal(t, agentpanel.StateDone, settled.State, "the recorded outcome settles a child at rest")
	require.Equal(t, 1, settled.Tools)
	require.False(t, settled.Ended.IsZero())

	for _, tc := range []struct {
		name   string
		status Status
		want   agentpanel.State
	}{
		{"a follow-up is running", Status{Running: true}, agentpanel.StateRunning},
		{"a follow-up is asking", Status{Waiting: "permission"}, agentpanel.StateWaiting},
		{"the user stopped the follow-up", Status{Stopped: true}, agentpanel.StateStopped},
		{"the follow-up failed", Status{Error: "the child could not read the file"}, agentpanel.StateFailed},
		{"the follow-up is over", Status{}, agentpanel.StateDone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			child.lifetime.status = tc.status
			row := f.row("job-1", child)
			assert.Equal(t, tc.want, row.State)
			assert.Equal(t, 1, row.Tools, "the counts stay the ones the parent recorded")
			assert.Equal(t, settled.Started, row.Started)
			if tc.want == agentpanel.StateRunning || tc.want == agentpanel.StateWaiting {
				assert.True(t, row.Ended.IsZero(), "a working child has not ended")
			}
		})
	}
}
