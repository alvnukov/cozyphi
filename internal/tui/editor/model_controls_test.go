package editor

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
)

func dispatchControlKey(e *Editor, key xui.KeyEvent) {
	ctx := &components.EventContext{DeliveredTo: e.App.Focused()}
	if ctx.DeliveredTo != nil && ctx.DeliveredTo != e {
		ctx.DeliveredTo.Handle(ctx, key)
	}
	if !ctx.Consume {
		e.Handle(ctx, key)
	}
	if ctx.Focus != nil {
		e.App.RequestFocus(ctx.Focus)
	}
}

func TestMainScreenEffortShortcutProfiles(t *testing.T) {
	t.Cleanup(func() { require.NoError(t, keys.Rebind(nil)) })
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		t.Run(mode.String(), func(t *testing.T) {
			e := newEffortEditor(t)
			t.Cleanup(e.ctrl.Close)
			e.App = app.NewApp(nil)
			e.Focus(&e.composer.Chat)
			require.NoError(t, e.SetModelEffort("openai/gpt-5.5", "high"))
			require.NoError(t, e.applyEditingMode(mode))
			e.composer.Chat.Value, e.composer.Chat.Cursor = "keep draft", 4
			if mode == editmode.Vim {
				dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
			}
			cursorBefore := e.composer.Chat.Cursor
			dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyF5})
			p, ok := e.App.Focused().(*palette.CommandPalette)
			require.True(t, ok, "F5 opens effort picker from focused input")
			require.Contains(t, p.Title, "Select Effort")
			require.NotEmpty(t, p.Commands)
			require.Equal(t, "default", p.Commands[0].Verb)
			require.Equal(t, "keep draft", e.composer.Chat.Value)
			require.Equal(t, cursorBefore, e.composer.Chat.Cursor)
			dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
			require.Same(t, &e.composer.Chat, e.App.Focused())
			require.Equal(t, "default", e.composer.Chat.EffortLabel)
			require.Equal(t, "keep draft", e.composer.Chat.Value)
		})
	}
}

func TestMainScreenModelAndEffortMouseSelection(t *testing.T) {
	e := newEffortEditor(t)
	t.Cleanup(e.ctrl.Close)
	e.App = app.NewApp(nil)
	e.Focus(&e.composer.Chat)
	e.modelNames = []string{"openai/gpt-5.5", "openai/gpt-5.4"}
	require.NoError(t, e.SetModelEffort("openai/gpt-5.5", "high"))
	e.composer.Chat.Value, e.composer.Chat.Cursor = "draft stays", 3

	clickControlText(t, e, &e.composer.Chat, "high")
	p, ok := e.App.Focused().(*palette.CommandPalette)
	require.True(t, ok)
	require.Contains(t, p.Title, "Select Effort")
	clickControlText(t, e, p, "low")
	require.Equal(t, "low", e.ctrl.Effort())
	require.Equal(t, "low", e.composer.Chat.EffortLabel)
	require.Same(t, &e.composer.Chat, e.App.Focused())

	clickControlText(t, e, &e.composer.Chat, "openai/gpt-5.5")
	p, ok = e.App.Focused().(*palette.CommandPalette)
	require.True(t, ok)
	clickControlText(t, e, p, "openai/gpt-5.4")
	require.Contains(t, p.Title, "Select Effort")
	clickControlText(t, e, p, "medium")
	require.Equal(t, "openai/gpt-5.4", e.ctrl.ModelName())
	require.Equal(t, "medium", e.ctrl.Effort())
	require.Same(t, &e.composer.Chat, e.App.Focused())
	require.Equal(t, "draft stays", e.composer.Chat.Value)
	require.Equal(t, 3, e.composer.Chat.Cursor)
}

func TestMainScreenEffortUnavailableAndRejectedPick(t *testing.T) {
	e := newEffortEditor(t)
	t.Cleanup(e.ctrl.Close)
	e.App = app.NewApp(nil)
	e.Focus(&e.composer.Chat)
	require.NoError(t, e.SetModel("custom-no-effort"))
	require.Empty(t, e.composer.Chat.EffortLabel)
	e.openCurrentEffortPicker()
	require.Same(t, &e.composer.Chat, e.App.Focused())
	require.Contains(t, e.toast.History()[0].Message, "no selectable reasoning effort")

	require.NoError(t, e.SetModelEffort("openai/gpt-5.5", "low"))
	require.Error(t, e.pickModelEffort("openai/gpt-5.5", "invented"))
	require.Equal(t, "low", e.ctrl.Effort())
	require.Contains(t, e.toast.History()[0].Message, "does not support")

	// A change outside the picker (for example, resume) updates both fields.
	require.NoError(t, e.ctrl.SetModelEffort("openai/gpt-5.4", "medium"))
	e.Draw(components.DrawContext{Max: components.Size{Width: 120, Height: 40}, Method: xui.WidthUnicode})
	require.Equal(t, "openai/gpt-5.4", e.composer.Chat.ModelName)
	require.Equal(t, "medium", e.composer.Chat.EffortLabel)
}

func clickControlText(t *testing.T, e *Editor, target components.Widget, label string) {
	t.Helper()
	// Exercise the idle screen after transient notifications have closed.
	for e.toast.Visible() {
		e.toast.Clear()
	}
	root := e.Draw(components.DrawContext{Max: components.Size{Width: 140, Height: 40}, Method: xui.WidthUnicode})
	x, y, ok := controlTextPosition(root, target, label, components.Point{})
	require.True(t, ok, "visible control %q", label)
	hit, lx, ly := root.HitTestAt(x, y)
	require.NotNil(t, hit)
	if target == &e.composer.Chat {
		require.Same(t, target, hit, "composer control routes to %T", hit)
	}
	shaper, ok := hit.(components.PointerShaper)
	require.True(t, ok)
	require.Equal(t, components.ShapePointer, shaper.PointerShape(lx, ly))
	ctx := &components.EventContext{}
	hit.Handle(ctx, xui.MouseEvent{X: lx, Y: ly, Button: xui.MouseLeft, Action: xui.MousePress})
	require.True(t, ctx.Consume)
	if ctx.Focus != nil {
		e.App.RequestFocus(ctx.Focus)
	}
}

func controlTextPosition(
	s components.Surface,
	target components.Widget,
	label string,
	origin components.Point,
) (int, int, bool) {
	if s.Widget == target {
		target = nil // The selected widget owns its nested panel surfaces too.
	}
	if target == nil && s.Buffer != nil {
		local := s
		local.Children = nil
		for y, row := range strings.Split(components.SurfaceText(local), "\n") {
			if before, _, found := strings.Cut(row, label); found {
				return origin.X + xui.StringWidth(before, xui.WidthUnicode), origin.Y + y, true
			}
		}
	}
	for _, child := range s.Children {
		p := components.Point{X: origin.X + child.Origin.X, Y: origin.Y + child.Origin.Y}
		if x, y, ok := controlTextPosition(child.Surface, target, label, p); ok {
			return x, y, true
		}
	}
	return 0, 0, false
}
