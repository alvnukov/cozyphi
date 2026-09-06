package agentlist

import (
	"errors"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/job"
)

// clock pins the wall clock so an elapsed time is a fact the test wrote.
var clock = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

// fixtureAgents is a session's whole sub-agent history: one working, one
// blocked on an ask, one done, one failed and one the user stopped — every
// state the browser must tell apart, deliberately out of order.
func fixtureAgents() []Agent {
	return []Agent{
		{
			JobID: "j-done", Title: "explore(read the loader)", Status: job.StatusCompleted,
			Tools: 3, Created: clock.Add(-9 * time.Minute), Started: clock.Add(-9 * time.Minute),
			Finished: clock.Add(-8 * time.Minute), Summary: "the loader reads ~/.cozyphi first",
			ResultPath: "/jobs/j-done/result.md",
		},
		{
			JobID: "j-run", Title: "worker(fix the lexer)", Status: job.StatusRunning,
			Tools: 7, Created: clock.Add(-3 * time.Minute), Started: clock.Add(-3 * time.Minute),
			Retained: true,
		},
		{
			JobID: "j-fail", Title: "worker(patch the seam)", Status: job.StatusFailed,
			Created: clock.Add(-6 * time.Minute), Started: clock.Add(-6 * time.Minute),
			Finished: clock.Add(-5 * time.Minute), Error: "exit 1",
			ResultPath: "/jobs/j-fail",
		},
		{
			JobID: "j-wait", Title: "review(check the gate)", Status: job.StatusRunning,
			Waiting: "permission", Created: clock.Add(-2 * time.Minute),
			Started: clock.Add(-2 * time.Minute), Retained: true,
		},
		{
			JobID: "j-stop", Title: "explore(find the caller)", Status: job.StatusCancelled,
			Created: clock.Add(-20 * time.Minute), Started: clock.Add(-20 * time.Minute),
			Finished: clock.Add(-19 * time.Minute),
		},
	}
}

type harness struct {
	pane    *Pane
	opened  *[]string
	stopped *[]string
	closed  *int
}

func newHarness(as []Agent, stopErr error) *harness {
	var opened, stopped []string
	closed := 0
	p := New(
		components.DefaultTheme(),
		func() []Agent { return as },
		func(id string) { opened = append(opened, id) },
		func(id string) error { stopped = append(stopped, id); return stopErr },
		func() { closed++ },
	)
	p.SetClock(func() time.Time { return clock })
	return &harness{pane: p, opened: &opened, stopped: &stopped, closed: &closed}
}

func press(t *testing.T, p *Pane, code xui.KeyCode, r rune) bool {
	t.Helper()
	return p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func drawText(t *testing.T, p *Pane) string {
	t.Helper()
	return components.SurfaceText(p.Draw(components.DrawContext{
		Max:    components.Size{Width: 100, Height: 24},
		Method: xui.WidthUnicode,
	}))
}

// The working children come first in the order they were started, and the
// finished ones below them, newest first.
func TestPaneOrdersRunningAgentsAboveFinishedOnes(t *testing.T) {
	got := order(fixtureAgents())
	ids := make([]string, 0, len(got))
	for _, a := range got {
		ids = append(ids, a.JobID)
	}
	assert.Equal(t, []string{"j-run", "j-wait", "j-fail", "j-done", "j-stop"}, ids)
}

// Every row names the state, the child, what it did and what it came back
// with — the facts the job recorded, and nothing beyond them.
func TestPaneDrawsEachAgentsFacts(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()

	text := drawText(t, h.pane)
	assert.Contains(t, text, "Agents  2 running · 5 total")
	assert.Contains(t, text, "⟳ worker(fix the lexer) · 7 tools · 3m 0s")
	assert.Contains(t, text, "⏸ review(check the gate) · waiting: permission · 2m 0s")
	assert.Contains(t, text, "✓ explore(read the loader) · 3 tools · 1m 0s · the loader reads ~/.cozyphi first")
	assert.Contains(t, text, "✗ worker(patch the seam) · 1m 0s · exit 1")
	assert.Contains(t, text, "■ explore(find the caller) · 1m 0s")
	assert.NotContains(t, text, "0 tools", "a run the parent never watched says nothing about tools")
}

// A job that never ran has no elapsed time and no outcome, so the row is the
// glyph and the title: the browser omits what the record does not hold.
func TestPaneOmitsFactsTheJobNeverRecorded(t *testing.T) {
	h := newHarness([]Agent{{JobID: "j1", Title: "explore(look around)", Status: job.StatusStarting}}, nil)
	h.pane.Show()

	assert.Contains(t, drawText(t, h.pane), "⟳ explore(look around)")
	assert.NotContains(t, drawText(t, h.pane), "tools")
}

// Enter on a retained child closes the browser and hands its id to the shell,
// which is the same path the band under the composer takes.
func TestPaneEnterOpensARetainedAgent(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show() // the cursor starts on j-run, the oldest running child

	require.True(t, press(t, h.pane, xui.KeyEnter, 0))
	assert.Equal(t, []string{"j-run"}, *h.opened)
	assert.False(t, h.pane.Visible(), "the browser gets out of the way of the screen it opened")
	assert.Equal(t, 1, *h.closed)
}

// Enter on a child this session no longer holds opens nothing: the row says
// where the answer was written instead of pretending to have a session.
func TestPaneEnterOnAReleasedAgentNamesItsResult(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()
	h.pane.cursor.Select(3) // j-done, finished and released

	require.True(t, press(t, h.pane, xui.KeyEnter, 0))
	assert.Empty(t, *h.opened)
	assert.True(t, h.pane.Visible())
	assert.Contains(t, drawText(t, h.pane), "/jobs/j-done/result.md")
}

// Stopping asks first: x arms a y/n confirmation naming the child, y stops it
// through the seam.
func TestPaneStopAsksBeforeStopping(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyRune, 'x'))
	require.True(t, h.pane.confirm.Armed())
	assert.Empty(t, *h.stopped, "x alone must not stop anything")
	assert.Contains(t, drawText(t, h.pane), `stop agent "worker(fix the lexer)"`)

	require.True(t, press(t, h.pane, xui.KeyRune, 'y'))
	assert.Equal(t, []string{"j-run"}, *h.stopped)
	assert.False(t, h.pane.confirm.Armed())
}

// A stop the wiring refuses is the wiring's to report; the pane asked, and the
// question is gone either way.
func TestPaneStopSurvivesARefusal(t *testing.T) {
	h := newHarness(fixtureAgents(), errors.New("no such job"))
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyRune, 'x'))
	require.True(t, press(t, h.pane, xui.KeyRune, 'y'))
	assert.Equal(t, []string{"j-run"}, *h.stopped)
	assert.False(t, h.pane.confirm.Armed())
}

// x on a child that already ended is a dead key: it says so instead of posing a
// question nothing could answer.
func TestPaneStopOnFinishedAgentSaysSo(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()
	h.pane.cursor.Select(2) // j-fail

	require.True(t, press(t, h.pane, xui.KeyRune, 'x'))
	assert.False(t, h.pane.confirm.Armed())
	assert.Empty(t, *h.stopped)
	assert.Contains(t, drawText(t, h.pane), "already finished")
}

// A key the browser has no use for answers with the keys that work, read from
// the catalog so the notice cannot drift from the help screen.
func TestPaneDeadKeyNamesTheKeysThatWork(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyRune, 'z'))
	assert.Contains(t, drawText(t, h.pane), "Enter open")
}

// Escape closes the browser and notifies the owner exactly once, no matter how
// often Hide lands.
func TestPaneCloseNotifiesOwnerOnce(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyEscape, 0))
	assert.False(t, h.pane.Visible())
	assert.Equal(t, 1, *h.closed)

	h.pane.Hide()
	assert.Equal(t, 1, *h.closed, "a second Hide stays silent")
}

// q closes it too, the way it does in the watch browser.
func TestPaneQClosesTheBrowser(t *testing.T) {
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()

	require.True(t, press(t, h.pane, xui.KeyRune, 'q'))
	assert.False(t, h.pane.Visible())
}

// A session that never spawned a child opens the browser just fine: an empty
// note, no crash, keys dead.
func TestPaneEmptySession(t *testing.T) {
	h := newHarness(nil, nil)
	h.pane.Show()

	assert.Contains(t, drawText(t, h.pane), "no sub-agents")
	require.True(t, press(t, h.pane, xui.KeyEnter, 0))
	assert.Empty(t, *h.opened)
	require.True(t, press(t, h.pane, xui.KeyRune, 'x'))
	assert.False(t, h.pane.confirm.Armed())
}

// A running row asks for the next frame itself, which is how its elapsed time
// moves without the pane owning a timer.
func TestPaneAsksForAFrameWhileAnAgentRuns(t *testing.T) {
	var woke time.Time
	ctx := components.DrawContext{
		Max:    components.Size{Width: 100, Height: 24},
		Method: xui.WidthUnicode,
		Wake:   &woke,
	}
	h := newHarness(fixtureAgents(), nil)
	h.pane.Show()
	h.pane.Draw(ctx)
	assert.False(t, woke.IsZero(), "a running row keeps the frames coming")

	done := newHarness([]Agent{{
		JobID: "j1", Title: "explore(x)", Status: job.StatusCompleted,
		Started: clock.Add(-time.Minute), Finished: clock,
	}}, nil)
	woke = time.Time{}
	done.pane.Show()
	done.pane.Draw(ctx)
	assert.True(t, woke.IsZero(), "a finished list is a still picture")
}
