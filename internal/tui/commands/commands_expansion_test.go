package commands

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components/toast"
)

func TestDispatchNewClearsSession(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}

	require.True(t, r.DispatchSlash("/new", CommandContext{Host: host}))
	assert.Equal(t, 1, host.cleared)
}

func TestDispatchThemeArgs(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	require.True(t, r.DispatchSlash("/theme opencode", ctx))
	assert.Equal(t, "opencode", host.theme)

	host.theme = ""
	require.True(t, r.DispatchSlash("/theme nope", ctx))
	assert.Empty(t, host.theme, "unknown theme must not be applied")
	assert.Contains(t, host.toastMsg, "unknown theme")
	assert.Equal(t, toast.ToastError, host.toastKind)

	host.toastMsg = ""
	require.True(t, r.DispatchSlash("/theme", ctx))
	assert.Empty(t, host.theme)
	assert.Contains(t, host.toastMsg, "usage:")
}

func TestDispatchSlashToleratesWhitespace(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	require.True(t, r.DispatchSlash("  /theme   opencode-light  ", ctx))
	assert.Equal(t, "opencode-light", host.theme)
}

func TestDispatchSlashIgnoresSlashTextNotMeantAsCommand(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	assert.False(t, r.DispatchSlash("hello /clear", ctx), "mid-text /clear is a prompt, not a command")
	assert.Equal(t, 0, host.cleared)

	assert.False(t, r.DispatchSlash("/etc/hosts is the file", ctx), "unknown leading path falls through to the model")
	assert.Equal(t, 0, host.cleared)
}

func TestDispatchExport(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	require.True(t, r.DispatchSlash("/export", ctx))
	assert.Equal(t, 1, host.exports)
	assert.Empty(t, host.exportPath, "no arg → the host picks the default path")

	require.True(t, r.DispatchSlash("/export /tmp/chat.md", ctx))
	assert.Equal(t, 2, host.exports)
	assert.Equal(t, "/tmp/chat.md", host.exportPath)
}

func TestDispatchCompact(t *testing.T) {
	r := NewBuiltinRegistry()
	host := &fakeHost{}

	require.True(t, r.DispatchSlash("/compact", CommandContext{Host: host}))
	assert.Equal(t, 1, host.compacted)
}

func TestCompleteSlashArg(t *testing.T) {
	r := NewBuiltinRegistry()

	items, ok := r.CompleteSlashArg("theme", "open")
	require.True(t, ok)
	require.Len(t, items, 2, "opencode and opencode-light")
	assert.Equal(t, "opencode", items[0].Path)

	items, ok = r.CompleteSlashArg("theme", "zzz")
	require.True(t, ok, "the completer answers even with no matches")
	assert.Empty(t, items)

	_, ok = r.CompleteSlashArg("clear", "")
	assert.False(t, ok, "commands without a completer report none")

	_, ok = r.CompleteSlashArg("unknown", "")
	assert.False(t, ok)
}

func TestModelSlashCommand(t *testing.T) {
	r := NewCommandRegistry()
	r.Register(ModelSlashCommand([]string{"deepseek-chat", "gpt-4o"}))
	host := &fakeHost{}
	ctx := CommandContext{Host: host}

	require.True(t, r.DispatchSlash("/model GPT-4O", ctx))
	assert.Equal(t, "gpt-4o", host.model, "model names resolve case-insensitively")

	require.True(t, r.DispatchSlash("/model nope", ctx))
	assert.Contains(t, host.toastMsg, "unknown model")

	require.True(t, r.DispatchSlash("/model", ctx))
	assert.Contains(t, host.toastMsg, "usage:")

	items, ok := r.CompleteSlashArg("model", "gp")
	require.True(t, ok)
	require.Len(t, items, 1)
	assert.Equal(t, "gpt-4o", items[0].Path)
}
