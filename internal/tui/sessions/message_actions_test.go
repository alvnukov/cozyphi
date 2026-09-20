package sessions

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

func newActionsEditor(t *testing.T) *View {
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

	e := NewView(nil, bus, ctrl, nil, nil, components.DefaultTheme(), cwd, "m", "", 0, nil, nil)
	e.App = app.NewApp(nil, components.DefaultTheme())
	return e
}

// The whole path a click takes: the strip is drawn into the frame, the hit
// test finds the block under the button, the block offers the hand there,
// and the press reaches the Host method with the id of the session entry the
// row stands for.
func TestClickingRewindReachesTheHostWithTheEntryID(t *testing.T) {
	e := newActionsEditor(t)
	// The rows below are drawn straight into the pane and the session never
	// receives them, so it would rightly say a cut at them leads nowhere and
	// the button would not be drawn at all. This test is about the path a
	// click takes, so it says the cut is on offer and leaves the session out
	// of it. Which rows really offer one is pinned in rewind_test.go.
	e.transcript.SetCanRewind(func(string) bool { return true })
	e.transcript.ApplySession(session.UserAppend{Text: "hello"})
	e.transcript.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateComplete,
		Text:  "hi there",
	}})
	e.transcript.Sync()

	prompt := e.transcript.Snapshot().Messages[0].ID
	require.NotEmpty(t, prompt)

	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 140, Height: 40},
		Method: xui.WidthUnicode,
	})
	x, y, ok := controlTextPosition(root, nil, "rewind", components.Point{})
	require.True(t, ok, "the prompt draws a rewind button")

	hit, lx, ly := root.HitTestAt(x+1, y)
	require.NotNil(t, hit)
	shaper, ok := hit.(components.PointerShaper)
	require.True(t, ok)
	require.Equal(t, components.ShapePointer, shaper.PointerShape(lx, ly))

	press := &components.EventContext{}
	hit.Handle(press, xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MousePress})
	require.True(t, press.Consume, "the press must not fall through to text selection")
	require.Empty(t, e.toast.History(), "the press alone must not act")

	ctx := &components.EventContext{}
	hit.Handle(ctx, xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MouseRelease})
	require.True(t, ctx.Consume, "the release that acted was not consumed")

	// This test is about the path a click takes, and it pins it as tightly as
	// it ever did: the strip is drawn, the hit test lands on it, the press
	// arms and the release acts, carrying this row's entry id and no other.
	// It can go no further, because the rows here were drawn straight into
	// the pane and the session has never heard of them, so the rewind is
	// refused by name. The click on a row the session did record, and the cut
	// it performs, are pinned in rewind_test.go by
	// TestTheStripActsAgainOnceTheTurnIsOver, which drives a real turn.
	history := e.toast.History()
	require.NotEmpty(t, history)
	assert.Contains(t, history[0].Message, prompt)
	assert.Contains(t, history[0].Message, "Cannot rewind")
}

// A press one column left of the strip belongs to the transcript, not to the
// buttons: it starts a selection and tells the shell nothing.
func TestClickingBesideTheStripActsOnNothing(t *testing.T) {
	e := newActionsEditor(t)
	e.transcript.ApplySession(session.UserAppend{Text: "hello"})
	e.transcript.Sync()

	root := e.Draw(components.DrawContext{
		Max:    components.Size{Width: 140, Height: 40},
		Method: xui.WidthUnicode,
	})
	// Any button of the strip anchors this: fork is the one every boundary
	// row offers regardless of where the cursor stands.
	x, y, ok := controlTextPosition(root, nil, "fork", components.Point{})
	require.True(t, ok)

	hit, lx, ly := root.HitTestAt(x-4, y)
	require.NotNil(t, hit)
	shaper, ok := hit.(components.PointerShaper)
	require.True(t, ok)
	assert.Equal(t, components.ShapeText, shaper.PointerShape(lx, ly))

	ctx := &components.EventContext{}
	hit.Handle(ctx, xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MousePress})
	assert.False(t, ctx.Consume)
	assert.Empty(t, e.toast.History())
}
