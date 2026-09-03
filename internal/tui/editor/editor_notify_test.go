package editor

import (
	"errors"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// fakeNotifier records the attention pings the editor sends.
type fakeNotifier struct {
	focused    bool
	focusCalls int
	turns      int
	attention  []string
	onFailure  func(error)
	reconfigs  []notify.Mode
	sounds     []string
}

func (f *fakeNotifier) SetFocused(focused bool)    { f.focusCalls++; f.focused = focused }
func (f *fakeNotifier) SetOnFailure(h func(error)) { f.onFailure = h }
func (f *fakeNotifier) TurnEnded()                 { f.turns++ }
func (f *fakeNotifier) NeedsAttention(d string)    { f.attention = append(f.attention, d) }

func (f *fakeNotifier) Reconfigure(mode notify.Mode, sound string) {
	f.reconfigs = append(f.reconfigs, mode)
	f.sounds = append(f.sounds, sound)
}

func newNotifyTestEditor(t *testing.T) (*Editor, *fakeNotifier) {
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

	e := NewEditor(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
	n := &fakeNotifier{}
	e.SetAttentionNotifier(n)
	return e, n
}

func TestEditorNotifiesOnRunEnded(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.Update(controller.RunEndedMsg{})

	assert.Equal(t, 1, n.turns, "one ping when the run pipeline goes idle")
	assert.Empty(t, n.attention)
}

// A running watch wakes the session by itself, so the end of a turn is not
// a wait for input: the turn ping stays quiet until the last watch is gone,
// while asks still ping — the user has to answer those.
func TestEditorStaysQuietOnRunEndedWhileAWatchRuns(t *testing.T) {
	e, n := newNotifyTestEditor(t)
	watches := []watch.Watch{{ID: "w1", Label: "edge logs", Live: true}}
	e.watchList = func() []watch.Watch { return watches }

	e.Update(controller.RunEndedMsg{})
	assert.Zero(t, n.turns, "no turn ping while a watch runs")

	e.Update(controller.PermissionAskMsg{Request: permission.Request{Tool: "bash"}})
	assert.Equal(t, []string{"bash"}, n.attention, "asks still ping")

	watches[0].Live = false
	e.Update(controller.RunEndedMsg{})
	assert.Equal(t, 1, n.turns, "the ping returns once the watch is gone")
}

func TestEditorNotifiesOnAsksWithDetail(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.Update(controller.PermissionAskMsg{Request: permission.Request{Tool: "bash"}})
	e.Update(controller.ContinueAskMsg{MaxRounds: 5})
	e.Update(controller.QuestionAskMsg{Questions: []questiontool.Question{{Header: "Database"}}})

	require.Len(t, n.attention, 3)
	assert.Equal(t, "bash", n.attention[0])
	assert.Contains(t, n.attention[1], "5")
	assert.Equal(t, "Database", n.attention[2])
}

func TestEditorAsksWithoutDetailUseDefaultBody(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.Update(controller.PermissionAskMsg{})

	require.Len(t, n.attention, 1)
	assert.Empty(t, n.attention[0])
}

func TestEditorDismissMessagesDoNotNotify(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.Update(controller.PermissionDismissMsg{})
	e.Update(controller.ContinueDismissMsg{})
	e.Update(controller.QuestionDismissMsg{})

	assert.Zero(t, n.turns)
	assert.Empty(t, n.attention)
}

func TestEditorForwardsFocusEventsToNotifier(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.Handle(&components.EventContext{}, xui.FocusEvent{Focused: false})
	assert.False(t, n.focused)
	e.Handle(&components.EventContext{}, xui.FocusEvent{Focused: true})
	assert.True(t, n.focused)
	assert.Equal(t, 2, n.focusCalls)
}

func TestEditorWithoutNotifierIsSafe(t *testing.T) {
	e, _ := newNotifyTestEditor(t)
	e.notifier = nil

	assert.NotPanics(t, func() {
		e.Update(controller.RunEndedMsg{})
		e.Update(controller.PermissionAskMsg{})
		e.Handle(&components.EventContext{}, xui.FocusEvent{Focused: false})
	})
}

// A sender that fails switches notifications off for the session; the user has
// to be told, or the missing pings read as a still-running turn.
func TestEditorToastsWhenTheNotifierFails(t *testing.T) {
	e, n := newNotifyTestEditor(t)
	require.NotNil(t, n.onFailure, "the editor must subscribe to sender failures")

	n.onFailure(errors.New("osascript: exit status 1"))
	e.drainBus()

	assert.True(t, e.toast.Visible())
	assert.Contains(t, e.toast.Message, "Desktop notifications are off")
	assert.Contains(t, e.toast.Message, "osascript: exit status 1")
}

// A committed settings snapshot reconfigures the live notifier, so toggling
// the General checkboxes takes effect without a restart.
func TestEditorAppliesNotificationSettingsLive(t *testing.T) {
	e, n := newNotifyTestEditor(t)

	e.applySettings(harnesssettings.Snapshot{
		Notifications: harnesssettings.Notifications{Mode: notify.ModeOff, Sound: "Glass"},
	})

	require.Len(t, n.reconfigs, 1)
	assert.Equal(t, notify.ModeOff, n.reconfigs[0])
	assert.Equal(t, "Glass", n.sounds[0])
}

// A committed snapshot also carries permissions.tasks: the level reaches the
// controller's gate at once, so a row toggled in the General tab decides
// the model's very next task write without a restart.
func TestEditorAppliesTaskAccessLive(t *testing.T) {
	e, _ := newNotifyTestEditor(t)
	require.Equal(t, tasks.AccessWrite, e.ctrl.TasksAccess(), "the default is write")

	e.applySettings(harnesssettings.Snapshot{Tasks: tasks.AccessRead})
	assert.Equal(t, tasks.AccessRead, e.ctrl.TasksAccess())

	e.applySettings(harnesssettings.Snapshot{})
	assert.Equal(t, tasks.AccessWrite, e.ctrl.TasksAccess(), "an unset level is the default again")
}
