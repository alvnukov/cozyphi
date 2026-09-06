package agentpanel

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

// base is the fake clock's origin: every fixture times off it, so a test can
// walk the 30-second windows without sleeping.
var base = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)

type harness struct {
	panel *Panel
	rows  []Row
	now   time.Time

	opened  []string
	stopped []string
	leaves  int
	stopErr error

	// wake records what the last Draw asked the scheduler for.
	wake time.Time
}

func newHarness(rows ...Row) *harness {
	h := &harness{rows: rows, now: base}
	h.panel = New(
		components.DefaultTheme(),
		func() []Row { return h.rows },
		func() time.Time { return h.now },
		Actions{
			Open:  func(id string) { h.opened = append(h.opened, id) },
			Stop:  func(id string) error { h.stopped = append(h.stopped, id); return h.stopErr },
			Leave: func() { h.leaves++ },
		},
	)
	return h
}

// draw renders the band and returns its rows as plain text, one per line.
func (h *harness) draw(t *testing.T, width int) []string {
	t.Helper()
	h.wake = time.Time{}
	s := h.panel.Draw(components.DrawContext{Method: xui.WidthUnicode, Wake: &h.wake}, width)
	text := strings.TrimSuffix(components.SurfaceText(s), "\n")
	if text == "" {
		return nil
	}
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " ")
	}
	return lines
}

// selectedRow is the band row painted with the selection style, or -1. The
// assertion goes through the surface on purpose: it is what the user sees.
func (h *harness) selectedRow(t *testing.T, width int) int {
	t.Helper()
	s := h.panel.Draw(components.DrawContext{Method: xui.WidthUnicode}, width)
	for y := range s.Size.Height {
		if s.Buffer[y*s.Size.Width].Style.Reverse {
			return y
		}
	}
	return -1
}

func (h *harness) press(code xui.KeyCode, r rune) bool {
	return h.panel.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func (h *harness) key(r rune) bool { return h.press(xui.KeyRune, r) }

func running(id, title string, tools int, age time.Duration) Row {
	return Row{ID: id, Title: title, State: StateRunning, Tools: tools, Started: base.Add(-age)}
}

// Every state renders its own glyph and its own words, and the tools and
// elapsed tail follows them all.
func TestRowGlyphsAndText(t *testing.T) {
	cases := []struct {
		name string
		row  Row
		want string
	}{
		{
			name: "running",
			row:  running("a", "explore(find the config loader)", 3, 80*time.Second),
			want: "○ ⟳ explore(find the config loader) · 3 tools · 1m 20s",
		},
		{
			name: "waiting",
			row: Row{
				ID: "a", Title: "explore(desc)", State: StateWaiting, Waiting: "permission",
				Tools: 3, Started: base.Add(-80 * time.Second),
			},
			want: "○ ⏸ explore(desc) · waiting: permission · 3 tools · 1m 20s",
		},
		{
			name: "failed",
			row: Row{
				ID: "a", Title: "explore(desc)", State: StateFailed, Tools: 3,
				Started: base.Add(-80 * time.Second), Ended: base.Add(-2 * time.Second),
			},
			want: "○ ✗ explore(desc) · failed · 3 tools · 1m 18s",
		},
		{
			name: "stopped",
			row: Row{
				ID: "a", Title: "explore(desc)", State: StateStopped, Tools: 3,
				Started: base.Add(-80 * time.Second), Ended: base.Add(-2 * time.Second),
			},
			want: "○ ■ explore(desc) · stopped · 3 tools · 1m 18s",
		},
		{
			name: "nothing to report yet",
			row:  Row{ID: "a", Title: "explore(desc)", State: StateRunning},
			want: "○ ⟳ explore(desc)",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(tc.row)
			lines := h.draw(t, 80)
			require.Len(t, lines, 2)
			assert.Equal(t, "● main", lines[0], "the parent screen is current by default")
			assert.Equal(t, tc.want, lines[1])
		})
	}
}

// The current screen's row wears ● and every other row ○, so the band always
// says where the user is.
func TestCurrentMarkerFollowsSetCurrent(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))

	lines := h.draw(t, 60)
	require.Len(t, lines, 2)
	assert.Equal(t, "● main", lines[0], "the parent screen is current by default")
	assert.True(t, strings.HasPrefix(lines[1], "○ "), "row %q", lines[1])

	h.panel.SetCurrent("a")
	lines = h.draw(t, 60)
	assert.Equal(t, "○ main", lines[0])
	assert.True(t, strings.HasPrefix(lines[1], "● "), "row %q", lines[1])

	h.panel.SetCurrent("")
	lines = h.draw(t, 60)
	assert.Equal(t, "● main", lines[0])
}

// A title too long for the terminal is ellipsized; the state word and the
// counts survive, because they are the facts a narrow band must keep.
func TestLongTitleEllipsizes(t *testing.T) {
	h := newHarness(running("a", strings.Repeat("long ", 30), 3, 5*time.Second))

	lines := h.draw(t, 40)
	require.Len(t, lines, 2)
	assert.LessOrEqual(t, xui.StringWidth(lines[1], xui.WidthUnicode), 40)
	assert.Contains(t, lines[1], "…")
	assert.True(t, strings.HasSuffix(lines[1], "· 3 tools · 5s"), "row %q", lines[1])
}

// An empty seam is an invisible panel: no rows, no height, nothing drawn.
func TestNoChildrenIsInvisible(t *testing.T) {
	h := newHarness()
	assert.False(t, h.panel.Visible())
	assert.Equal(t, 0, h.panel.Height())
	assert.Empty(t, h.draw(t, 80))
}

func sixChildren() []Row {
	rows := make([]Row, 0, 6)
	for _, id := range []string{"c1", "c2", "c3", "c4", "c5", "c6"} {
		rows = append(rows, running(id, "explore("+id+")", 1, time.Second))
	}
	return rows
}

// Seven list rows in a three-row window: the indicators count what is hidden
// on each side and move with the cursor, and the band never grows past five.
func TestViewportOfThreeWithIndicators(t *testing.T) {
	h := newHarness(sixChildren()...)
	require.True(t, h.panel.Visible())
	h.panel.Focus()

	// At the top: main plus two children, four rows hidden below.
	assert.Equal(t, 4, h.panel.Height())
	lines := h.draw(t, 40)
	require.Len(t, lines, 4)
	assert.Equal(t, "● main", lines[0])
	assert.Equal(t, "○ ⟳ explore(c1) · 1 tools · 1s", lines[1])
	assert.Equal(t, "○ ⟳ explore(c2) · 1 tools · 1s", lines[2])
	assert.Equal(t, "↓ 4 more", lines[3])

	// Three steps down and the window has slid by one: an indicator on each
	// side, five rows in all.
	for range 3 {
		require.True(t, h.press(xui.KeyDown, 0))
	}
	assert.Equal(t, 5, h.panel.Height())
	lines = h.draw(t, 40)
	require.Len(t, lines, 5)
	assert.Equal(t, "↑ 1 more", lines[0])
	assert.Equal(t, "○ ⟳ explore(c1) · 1 tools · 1s", lines[1])
	assert.Equal(t, "○ ⟳ explore(c3) · 1 tools · 1s", lines[3])
	assert.Equal(t, "↓ 3 more", lines[4])

	// G lands on the last row: everything above is counted, nothing below.
	require.True(t, h.key('G'))
	assert.Equal(t, 4, h.panel.Height())
	lines = h.draw(t, 40)
	require.Len(t, lines, 4)
	assert.Equal(t, "↑ 4 more", lines[0])
	assert.Equal(t, "○ ⟳ explore(c6) · 1 tools · 1s", lines[3])

	// gg goes back to main.
	require.True(t, h.key('g'))
	require.True(t, h.key('g'))
	lines = h.draw(t, 40)
	assert.Equal(t, "● main", lines[0])
}

// The cursor never rests on an indicator row, wherever the motions leave it.
func TestCursorNeverLandsOnAnIndicator(t *testing.T) {
	h := newHarness(sixChildren()...)
	h.panel.Focus()

	walk := []func(){
		func() { h.press(xui.KeyDown, 0) },
		func() { h.press(xui.KeyDown, 0) },
		func() { h.key('j') },
		func() { h.key('3') },
		func() { h.key('j') },
		func() { h.key('G') },
		func() { h.press(xui.KeyPageUp, 0) },
		func() { h.press(xui.KeyEnd, 0) },
		func() { h.press(xui.KeyHome, 0) },
	}
	for i, step := range walk {
		step()
		lines := h.draw(t, 40)
		y := h.selectedRow(t, 40)
		require.GreaterOrEqual(t, y, 0, "step %d: nothing selected", i)
		assert.NotContains(t, lines[y], " more", "step %d: cursor on an indicator", i)
	}
}

// A success leaves the band at once and lives on for 30 seconds as the
// footer's hint, because there is no row left to say what happened.
func TestDoneRowVanishesAndArmsTheHint(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 2, time.Minute))
	require.Len(t, h.draw(t, 40), 2)

	h.rows = []Row{{
		ID: "a", Title: "explore(one)", State: StateDone, Tools: 2,
		Started: base.Add(-time.Minute), Ended: base,
	}}
	assert.False(t, h.panel.Visible(), "a finished child leaves at once")
	assert.Equal(t, 0, h.panel.Height())

	hint, ok := h.panel.Hint()
	assert.True(t, ok)
	assert.Equal(t, "/agents to see agents", hint)

	h.now = base.Add(29 * time.Second)
	_, ok = h.panel.Hint()
	assert.True(t, ok, "the hint outlives the row by 30 seconds")

	h.now = base.Add(31 * time.Second)
	_, ok = h.panel.Hint()
	assert.False(t, ok)
}

// The row of the session on screen is exempt from the whole lifecycle: it is
// the user's way back to main, so it stays whatever state it reached and for
// as long as they stand on it.
func TestCurrentRowSurvivesEveryLifecycleRule(t *testing.T) {
	cases := []struct {
		name string
		row  Row
		want string
	}{
		{
			name: "a success that would leave at once",
			row: Row{
				ID: "a", Title: "explore(one)", State: StateDone, Tools: 4,
				Started: base.Add(-80 * time.Second), Ended: base,
			},
			want: "● ✓ explore(one) · 4 tools · 1m 20s",
		},
		{
			name: "a failure whose window ran out",
			row: Row{
				ID: "a", Title: "explore(one)", State: StateFailed, Tools: 4,
				Started: base.Add(-80 * time.Second), Ended: base,
			},
			want: "● ✗ explore(one) · failed · 4 tools · 1m 20s",
		},
		{
			name: "a stop whose window ran out",
			row: Row{
				ID: "a", Title: "explore(one)", State: StateStopped, Tools: 4,
				Started: base.Add(-80 * time.Second), Ended: base,
			},
			want: "● ■ explore(one) · stopped · 4 tools · 1m 20s",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness(tc.row)
			h.panel.SetCurrent("a")
			h.now = base.Add(10 * time.Minute) // long past every window

			require.True(t, h.panel.Visible(), "the screen the user is on is always drawn")
			lines := h.draw(t, 80)
			require.Len(t, lines, 2)
			assert.Equal(t, "○ main", lines[0], "and main is one row away")
			assert.Equal(t, tc.want, lines[1])

			// Back on the parent's screen the ordinary rules take the row.
			h.panel.SetCurrent("")
			assert.False(t, h.panel.Visible(), "leaving hands the row back to its window")
			assert.Empty(t, h.draw(t, 80))
		})
	}
}

// A success the user was standing on arms the footer hint when it finally
// leaves the band, not while its row is still on screen.
func TestCurrentSuccessArmsTheHintOnlyWhenItLeaves(t *testing.T) {
	h := newHarness(Row{
		ID: "a", Title: "explore(one)", State: StateDone,
		Started: base.Add(-time.Minute), Ended: base,
	})
	h.panel.SetCurrent("a")
	require.True(t, h.panel.Visible())
	_, ok := h.panel.Hint()
	assert.False(t, ok, "the row itself says the child finished")

	h.now = base.Add(5 * time.Minute)
	h.panel.SetCurrent("")
	hint, ok := h.panel.Hint()
	require.True(t, ok, "the hint starts when the row goes")
	assert.Equal(t, "/agents to see agents", hint)

	h.now = base.Add(5*time.Minute + 31*time.Second)
	_, ok = h.panel.Hint()
	assert.False(t, ok)
}

// x on the current child's row stops it and the row stays: the way back must
// not disappear under the key that ends the run.
func TestStoppingTheCurrentChildKeepsItsRow(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	h.panel.SetCurrent("a")
	h.panel.Focus()

	require.True(t, h.key('x'))
	assert.Equal(t, []string{"a"}, h.stopped)

	h.rows = []Row{{ID: "a", Title: "explore(one)", State: StateStopped, Started: base.Add(-time.Second), Ended: base}}
	h.now = base.Add(time.Minute)
	require.True(t, h.panel.Visible())
	lines := h.draw(t, 60)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[1], "stopped")
}

// A child screen whose row the seam no longer reports still draws the main
// row: the way back never depends on the seam agreeing.
func TestAChildScreenAlwaysDrawsTheWayBack(t *testing.T) {
	h := newHarness()
	h.panel.SetCurrent("a")
	assert.True(t, h.panel.Visible())
	assert.Equal(t, []string{"○ main"}, h.draw(t, 40))
}

// A failure keeps its row for 30 seconds from Ended, then goes on its own.
func TestFailedRowExpiresAfterItsWindow(t *testing.T) {
	h := newHarness(Row{
		ID: "a", Title: "explore(one)", State: StateFailed, Tools: 2,
		Started: base.Add(-time.Minute), Ended: base,
	})
	require.True(t, h.panel.Visible())

	h.now = base.Add(29 * time.Second)
	assert.True(t, h.panel.Visible())
	lines := h.draw(t, 40)
	require.Len(t, lines, 2)
	assert.Contains(t, lines[1], "failed")
	assert.Contains(t, lines[1], "1m 0s", "elapsed freezes at Ended")

	h.now = base.Add(31 * time.Second)
	assert.False(t, h.panel.Visible())
	assert.Empty(t, h.draw(t, 40))
	_, ok := h.panel.Hint()
	assert.False(t, ok, "a failure arms no success hint")
}

// A stopped row with no Ended still expires: the panel anchors the window on
// the first frame it saw the row terminal.
func TestTerminalRowWithoutEndedUsesFirstSighting(t *testing.T) {
	h := newHarness(Row{ID: "a", Title: "explore(one)", State: StateStopped, Started: base.Add(-time.Minute)})

	h.now = base.Add(10 * time.Second)
	require.True(t, h.panel.Visible(), "first sighting starts the window")

	h.now = base.Add(39 * time.Second)
	assert.True(t, h.panel.Visible())

	h.now = base.Add(41 * time.Second)
	assert.False(t, h.panel.Visible())
}

// x clears an ended row before its window runs out, and leaves the rest of
// the band alone.
func TestDismissWithX(t *testing.T) {
	h := newHarness(
		running("a", "explore(one)", 1, time.Second),
		Row{ID: "b", Title: "explore(two)", State: StateFailed, Ended: base},
	)
	h.panel.Focus()
	require.True(t, h.press(xui.KeyDown, 0))
	require.True(t, h.press(xui.KeyDown, 0))

	require.True(t, h.key('x'))
	assert.Empty(t, h.stopped, "an ended row is cleared, not stopped")
	lines := h.draw(t, 40)
	require.Len(t, lines, 2)
	assert.Equal(t, "● main", lines[0])
	assert.Contains(t, lines[1], "explore(one)")
}

// x on a live child asks the wiring to stop it; a refusal is a one-keypress
// notice inside the band, not a lost keypress.
func TestStopRunningChild(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	h.panel.Focus()
	require.True(t, h.press(xui.KeyDown, 0))

	require.True(t, h.key('x'))
	assert.Equal(t, []string{"a"}, h.stopped)

	h.stopErr = errors.New("already gone")
	require.True(t, h.key('x'))
	lines := h.draw(t, 60)
	assert.Contains(t, lines[len(lines)-1], "stop failed: already gone")

	require.True(t, h.press(xui.KeyDown, 0))
	lines = h.draw(t, 60)
	assert.NotContains(t, lines[len(lines)-1], "stop failed", "a notice lives one keypress")
}

// Esc and ↑ on main are the two ways back to the composer; ↑ anywhere else
// is an ordinary move.
func TestLeavingTheBand(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))

	h.panel.Focus()
	require.True(t, h.press(xui.KeyEscape, 0))
	assert.Equal(t, 1, h.leaves)
	assert.False(t, h.panel.Focused(), "leaving blurs the band")

	h.panel.Focus()
	require.True(t, h.press(xui.KeyUp, 0))
	assert.Equal(t, 2, h.leaves, "↑ on main hands the keyboard back")

	h.panel.Focus()
	require.True(t, h.press(xui.KeyDown, 0))
	require.True(t, h.press(xui.KeyUp, 0))
	assert.Equal(t, 2, h.leaves, "↑ on a child only moves")
	assert.Equal(t, 0, h.panel.cursor.Selected())

	h.panel.Focus()
	require.True(t, h.key('k'))
	assert.Equal(t, 3, h.leaves, "k on main leaves too")
}

// Enter and Space open the selected row; main opens as the empty ID.
func TestEnterAndSpaceOpen(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	h.panel.Focus()

	require.True(t, h.press(xui.KeyEnter, 0))
	require.True(t, h.press(xui.KeyDown, 0))
	require.True(t, h.key(' '))
	assert.Equal(t, []string{"", "a"}, h.opened)
}

// Focus starts on the row of the session the screen is showing.
func TestFocusLandsOnTheCurrentRow(t *testing.T) {
	h := newHarness(sixChildren()...)
	h.panel.SetCurrent("c4")
	h.panel.Focus()

	require.True(t, h.press(xui.KeyEnter, 0))
	assert.Equal(t, []string{"c4"}, h.opened)
}

// A key the band cannot use answers with the catalog's own hint row, and the
// next key clears it.
func TestDeadKeyShowsTheKeysThatWork(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	h.panel.Focus()

	require.True(t, h.key('q'))
	lines := h.draw(t, 70)
	assert.Contains(t, lines[len(lines)-1], keys.Hints(keys.ScopeAgents))
	assert.Len(t, lines, 2, "the notice takes a row, it never adds one")

	require.True(t, h.press(xui.KeyDown, 0))
	lines = h.draw(t, 70)
	assert.NotContains(t, lines[len(lines)-1], "Enter open")
}

// Keys reach the band only while it has focus; the mouse always does.
func TestKeysNeedFocus(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	assert.False(t, h.press(xui.KeyEnter, 0))
	assert.Empty(t, h.opened)
}

func (h *harness) click(y int) bool {
	return h.panel.HandleEvent(&components.EventContext{}, xui.MouseEvent{
		Y: y, Button: xui.MouseLeft, Action: xui.MousePress,
	})
}

// A press selects the row and opens it; a press on an indicator does nothing
// but stay inside the band.
func TestClickSelectsAndOpens(t *testing.T) {
	h := newHarness(sixChildren()...)
	require.Equal(t, 4, h.panel.Height())

	require.True(t, h.click(1))
	assert.Equal(t, []string{"c1"}, h.opened)
	assert.Equal(t, 1, h.selectedRow(t, 40))

	require.True(t, h.click(3), "the indicator eats the click")
	assert.Equal(t, []string{"c1"}, h.opened)

	require.True(t, h.click(0))
	assert.Equal(t, []string{"c1", ""}, h.opened, "main opens as the empty id")
}

// The wheel moves the window and leaves the cursor where it was, as the
// dialect says.
func TestWheelScrollsWithoutMovingTheCursor(t *testing.T) {
	h := newHarness(sixChildren()...)
	h.panel.Focus()
	require.Equal(t, 0, h.panel.cursor.Selected())

	ok := h.panel.HandleEvent(&components.EventContext{}, xui.MouseEvent{Button: xui.MouseWheelDown})
	require.True(t, ok)
	assert.Equal(t, 0, h.panel.cursor.Selected(), "the wheel never moves the cursor")

	lines := h.draw(t, 40)
	require.NotEmpty(t, lines)
	assert.Equal(t, "↑ 3 more", lines[0])
	assert.Equal(t, "○ ⟳ explore(c3) · 1 tools · 1s", lines[1])
}

// Draw asks for the next frame it needs: a second while a child runs, the
// window's end once it has stopped.
func TestDrawSchedulesWakes(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	h.draw(t, 40)
	assert.False(t, h.wake.IsZero(), "a running row must keep the clock moving")
	assert.LessOrEqual(t, time.Until(h.wake), time.Second+time.Millisecond)

	h.rows = []Row{{
		ID: "a", Title: "explore(one)", State: StateFailed,
		Started: base.Add(-time.Minute), Ended: base,
	}}
	h.draw(t, 40)
	assert.Equal(t, base.Add(window), h.wake, "an ended row wakes when its window closes")
}

// Blur drops the pending count and the notice, so a half-typed motion never
// survives a trip through the composer.
func TestBlurClearsPendingInput(t *testing.T) {
	h := newHarness(sixChildren()...)
	h.panel.Focus()
	require.True(t, h.key('q')) // a notice
	require.True(t, h.key('3')) // a pending count

	h.panel.Blur()
	assert.False(t, h.panel.Focused())
	lines := h.draw(t, 60)
	assert.NotContains(t, lines[len(lines)-1], "Enter open")

	h.panel.Focus()
	require.True(t, h.key('j'))
	assert.Equal(t, 1, h.panel.cursor.Selected(), "the dropped count must not repeat the move")
}

// The panel forgets IDs the seam stopped returning, so its memory cannot
// grow with the session.
func TestForgetsIDsTheSeamDropped(t *testing.T) {
	h := newHarness(running("a", "explore(one)", 1, time.Second))
	require.True(t, h.panel.Visible())
	require.Len(t, h.panel.seen, 1)

	h.rows = nil
	assert.False(t, h.panel.Visible())
	assert.Empty(t, h.panel.seen)
}

// A nil panel and a nil seam answer instead of panicking: the wiring builds
// the band before it has anything to put in it.
func TestZeroValuesAreSafe(t *testing.T) {
	var p *Panel
	assert.False(t, p.Visible())
	assert.Equal(t, 0, p.Height())
	assert.False(t, p.Focused())
	_, ok := p.Hint()
	assert.False(t, ok)
	assert.False(t, p.HandleEvent(nil, xui.KeyEvent{Press: true}))

	q := New(components.Theme{}, nil, nil, Actions{})
	assert.False(t, q.Visible())
	assert.Empty(t, components.SurfaceText(q.Draw(components.DrawContext{}, 40)))
}
