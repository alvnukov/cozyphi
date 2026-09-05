package editor_test

import (
	"context"
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/pulseaiclub/xui/input"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/editor"
	"github.com/alvnukov/cozyphi/internal/tui/sessions"
)

func TestShellRejectsOversizedPasteWithoutChangingDraft(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	application := app.NewApp(nil)
	registry := sessions.NewRegistry(12, nil)
	view := sessions.NewView(application, controller.NewBus(nil), nil, nil, nil, components.DefaultTheme(),
		t.TempDir(), "test", "", 1000, nil, nil)
	_, err := registry.Open("first", view)
	require.NoError(t, err)
	shell := editor.NewEditor(application, registry)
	t.Cleanup(func() { require.NoError(t, shell.Close(context.WithoutCancel(t.Context()))) })
	for _, ch := range "retainedDraft" {
		shell.Handle(&components.EventContext{}, xui.KeyEvent{Code: xui.KeyRune, Rune: ch, Press: true})
	}
	parser := input.NewParser()
	require.Empty(t, parser.Feed([]byte("\x1b[200~"+strings.Repeat("x", input.MaxPasteBytes+1))))
	require.Empty(t, parser.Feed([]byte("\r\nignored\x03\x1b[20")))
	events := parser.Feed([]byte("1~"))
	require.Len(t, events, 1)
	ctx := &components.EventContext{}
	shell.Capture(ctx, events[0])
	require.True(t, ctx.Consume)
	text := drawText(shell)
	require.Contains(t, text, "retainedDraft")
	require.Contains(t, text, "Paste rejected")
	require.Contains(t, text, "1 MiB")
	require.NotContains(t, text, "ignored")
}
