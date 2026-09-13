package shellpane_test

import (
	"strings"
	"testing"
	"time"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/shelltask"
	"github.com/alvnukov/cozyphi/internal/tui/shellpane"
)

func key(p *shellpane.Pane, code xui.KeyCode, r rune) {
	p.HandleEvent(&components.EventContext{}, xui.KeyEvent{Press: true, Code: code, Rune: r})
}

func draw(p *shellpane.Pane, wake *time.Time) string {
	return components.SurfaceText(p.Draw(components.DrawContext{
		Max: components.Size{Width: 120, Height: 15}, Method: xui.WidthUnicode, Wake: wake,
	}))
}

func TestShellPaneKeepsSelectedIdentityAndStopTarget(t *testing.T) {
	var promoted, stopped []string
	p := shellpane.New(components.DefaultTheme(),
		func(id string) error { promoted = append(promoted, id); return nil },
		func(id string) error { stopped = append(stopped, id); return nil }, nil)
	a := shelltask.Snapshot{ID: "a", Command: "build", State: shelltask.Running}
	b := shelltask.Snapshot{ID: "b", Command: "deploy", State: shelltask.Running}
	p.SetTasks([]shelltask.Snapshot{a, b})
	p.Show()
	key(p, xui.KeyDown, 0)
	key(p, xui.KeyRune, 's')
	require.Empty(t, stopped)
	p.SetTasks([]shelltask.Snapshot{b, a})
	key(p, xui.KeyRune, 'y')
	require.Equal(t, []string{"b"}, stopped)
	key(p, xui.KeyRune, 'b')
	require.Equal(t, []string{"b"}, promoted, "selection must follow ID after reorder")
}

func TestShellPaneLiveOutputAndTerminalState(t *testing.T) {
	shellTaskDeadline := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	exitCode := 7
	p := shellpane.New(components.DefaultTheme(), nil, nil, nil)
	p.SetSessionID("current")
	task := shelltask.Snapshot{
		ID: "s1", ParentSessionID: "other", Command: "make", State: shelltask.Running,
		Background: true, Output: "first", OutputFile: "/tmp/output.txt",
		Deadline: &shellTaskDeadline,
	}
	p.SetTasks([]shelltask.Snapshot{task})
	p.Show()
	require.Contains(t, draw(p, nil), "other session")
	key(p, xui.KeyEnter, 0)
	require.Contains(t, draw(p, nil), "first")
	task.Output = "first\n\x1b[31mfailed\x1b[0m\x1b]52;c;secret\a"
	task.State, task.ExitCode, task.Truncated = shelltask.Failed, &exitCode, true
	p.SetTasks([]shelltask.Snapshot{task})
	screen := draw(p, nil)
	require.Contains(t, screen, "failed (7)")
	require.Contains(t, screen, "expires 12:00:00")
	require.Contains(t, screen, "bounded tail")
	require.NotContains(t, screen, "secret")
	require.NotContains(t, screen, "\x1b")
	require.NotContains(t, screen, "[31m")
	key(p, xui.KeyEscape, 0)
	require.True(t, p.Visible())
	key(p, xui.KeyRune, 's')
	require.Contains(t, draw(p, nil), "already ended")
}

func TestShellPaneUsesSchedulerOnlyForVisibleElapsedTime(t *testing.T) {
	p := shellpane.New(components.DefaultTheme(), nil, nil, nil)
	p.SetTasks([]shelltask.Snapshot{{ID: "a", State: shelltask.Completed}})
	p.Show()
	var wake time.Time
	draw(p, &wake)
	require.True(t, wake.IsZero(), "terminal list must not schedule idle frames")
	p.SetTasks([]shelltask.Snapshot{{ID: "a", State: shelltask.Running, Started: time.Now()}})
	draw(p, &wake)
	require.False(t, wake.IsZero(), "running elapsed text needs scheduler wake")
	key(p, xui.KeyEnter, 0)
	wake = time.Time{}
	draw(p, &wake)
	require.True(t, wake.IsZero(), "output updates come from events")
}

func TestShellPaneSmallViewportAndUnicode(_ *testing.T) {
	p := shellpane.New(components.DefaultTheme(), nil, nil, nil)
	p.SetTasks([]shelltask.Snapshot{{ID: "a", Command: "👩‍💻 漢字", Output: strings.Repeat("x", 100)}})
	p.Show()
	p.Draw(components.DrawContext{Max: components.Size{Width: 1, Height: 1}, Method: xui.WidthUnicode})
	key(p, xui.KeyEnter, 0)
	p.Draw(components.DrawContext{Max: components.Size{Width: 1, Height: 1}, Method: xui.WidthUnicode})
}
