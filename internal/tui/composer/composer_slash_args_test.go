package composer

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/chat"
	"github.com/alvnukov/cozyphi/internal/components/mention"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
)

func wiredCmdPane(t *testing.T) *ComposerPane {
	t.Helper()
	c := newTestPane()
	c.Wire(nil, nil, commands.NewBuiltinRegistry(), "/tmp", &fakeBus{}, &fakeFocus{})
	return c
}

// notifySlashBoth mirrors ChatInput.notifyCompleters: both slash callbacks
// fire on every edit, and the pane routes by cursor position.
func notifySlashBoth(c *ComposerPane) {
	q, _, _, nameMode := chat.ActiveSlash(c.Chat.Value, c.Chat.Cursor)
	c.onSlashChange(nameMode, q)
	name, args, partial, _, _, argMode := chat.ActiveSlashArg(c.Chat.Value, c.Chat.Cursor)
	c.onSlashArgChange(argMode, name, args, partial)
}

// TestComposerSlashArgCompletes: typing the first argument of a command
// with an arg completer offers values in the same slash picker, and
// accepting one replaces just the argument token.
func TestComposerSlashArgCompletes(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/theme open"
	c.Chat.Cursor = len(c.Chat.Value)
	notifySlashBoth(c)

	require.True(t, c.slash.Open)
	require.True(t, c.Chat.SlashOpen)
	require.NotEmpty(t, c.slash.Items)
	require.Equal(t, "opencode", c.slash.Items[0].Path)

	c.acceptSlash(c.slash.Items[0])
	require.Equal(t, "/theme opencode ", c.Chat.Value)
	require.False(t, c.slash.Open)
}

// TestComposerSlashArgNoCompleter: commands without an arg completer keep
// the picker closed once the name token is finished.
func TestComposerSlashArgNoCompleter(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/clear "
	c.Chat.Cursor = len(c.Chat.Value)
	notifySlashBoth(c)

	require.False(t, c.slash.Open)
}

// TestComposerSlashNameModeUnchanged: the command-name menu and its accept
// behavior (insert "/theme " for further typing) still work as before.
func TestComposerSlashNameModeUnchanged(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/the"
	c.Chat.Cursor = len(c.Chat.Value)
	notifySlashBoth(c)

	require.True(t, c.slash.Open)
	require.Equal(t, "theme", c.slash.Items[0].Path)

	c.acceptSlash(c.slash.Items[0])
	require.Equal(t, "/theme ", c.Chat.Value)
}

// TestComposerSlashSecondArgCompletes: /voice install completes model names
// in the second argument, and accepting one replaces only that token.
func TestComposerSlashSecondArgCompletes(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/voice install sm"
	c.Chat.Cursor = len(c.Chat.Value)
	notifySlashBoth(c)

	require.True(t, c.slash.Open)
	require.Equal(t, []string{"small"}, itemPaths(c.slash.Items))

	c.acceptSlash(c.slash.Items[0])
	require.Equal(t, "/voice install small ", c.Chat.Value)
}

// TestComposerSlashThirdArgQuiet: /voice takes no third argument, so the
// picker stays closed instead of offering an empty list.
func TestComposerSlashThirdArgQuiet(t *testing.T) {
	c := wiredCmdPane(t)
	c.Chat.Value = "/voice install small "
	c.Chat.Cursor = len(c.Chat.Value)
	notifySlashBoth(c)

	require.False(t, c.slash.Open)
}

// itemPaths lists what the picker offers, in order.
func itemPaths(items []mention.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Path)
	}
	return out
}
