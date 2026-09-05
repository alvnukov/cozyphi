package chat

import (
	"strconv"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestVimInsertSessionUndoesAsOneChange(t *testing.T) {
	for _, tc := range []struct {
		name, value, keys, changed string
	}{
		{name: "insert", value: "one", keys: "iXY", changed: "onXYe"},
		{name: "change word", value: "one two", keys: "0cwnew", changed: "new two"},
		{name: "change line", value: "one\ntwo", keys: "ggccnew", changed: "new\ntwo"},
		{name: "open line", value: "one\ntwo", keys: "ggohello", changed: "one\nhello\ntwo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ChatInput{Value: tc.value, Cursor: len(tc.value)}
			c.SetEditingMode(editmode.Vim)
			require.True(t, editEscape(c))
			for _, key := range tc.keys {
				require.True(t, editKey(c, key, 0))
			}
			require.True(t, editEscape(c))

			require.True(t, editKey(c, 'u', 0))
			require.Equal(t, tc.value, c.Value)
			require.True(t, editKey(c, 'r', xui.ModCtrl))
			require.Equal(t, tc.changed, c.Value)
		})
	}
}

func TestReplaceRangeIsAnAtomicUndoableEdit(t *testing.T) {
	c := &ChatInput{Value: "say @fi", Cursor: len("say @fi")}
	c.SetEditingMode(editmode.Standard)

	c.ReplaceRange(4, len(c.Value), "@file ")
	require.Equal(t, "say @file ", c.Value)

	require.True(t, editKey(c, 'z', xui.ModCtrl))
	require.Equal(t, "say @fi", c.Value)
	require.True(t, editKey(c, 'Z', xui.ModCtrl|xui.ModShift))
	require.Equal(t, "say @file ", c.Value)
}

func TestReplaceRangePreservesBoundedPriorUndo(t *testing.T) {
	c := &ChatInput{Value: "seed", Cursor: len("seed")}
	c.SetEditingMode(editmode.Standard)

	for i := 0; i <= 100; i++ {
		next := strconv.Itoa(i)
		c.ReplaceRange(0, len(c.Value), next)
	}
	require.Equal(t, "100", c.Value)

	for range 100 {
		require.True(t, editKey(c, 'z', xui.ModCtrl))
	}
	require.Equal(t, "0", c.Value)
	require.True(t, editKey(c, 'z', xui.ModCtrl))
	require.Equal(t, "0", c.Value, "the oldest revision is discarded at the 100-entry bound")
}

type undoSearchHistory struct {
	match string
}

func (*undoSearchHistory) Prev(string) (string, bool) { return "", false }
func (*undoSearchHistory) Next(string) (string, bool) { return "", false }
func (h *undoSearchHistory) Search(string) []string   { return []string{h.match} }

func TestSearchPasteUndoesAsOneEvent(t *testing.T) {
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline} {
		t.Run(mode.String(), func(t *testing.T) {
			c := &ChatInput{Value: "draft", Cursor: 5, History: &undoSearchHistory{match: "history"}}
			c.SetEditingMode(mode)
			require.True(t, c.BeginSearch())
			editKey(c, 'h', 0) // resolve a match before accepting it with the paste
			c.Handle(&components.EventContext{}, xui.PasteEvent{Text: " pasted"})
			require.Equal(t, "history pasted", c.Value)
			editKey(c, 'z', xui.ModCtrl)
			require.Equal(t, "draft", c.Value)
			editKey(c, 'Z', xui.ModCtrl|xui.ModShift)
			require.Equal(t, "history pasted", c.Value)
		})
	}
}

func TestAcceptedHistorySearchIsUndoable(t *testing.T) {
	c := &ChatInput{
		Value:   "draft",
		Cursor:  len("draft"),
		History: &undoSearchHistory{match: "historical prompt"},
	}
	c.SetEditingMode(editmode.Standard)
	require.True(t, c.BeginSearch())

	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: 'h'})
	require.True(t, ctx.Consume)
	ctx = &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	require.True(t, ctx.Consume)
	require.Equal(t, "historical prompt", c.Value)

	require.True(t, editKey(c, 'z', xui.ModCtrl))
	require.Equal(t, "draft", c.Value)
}
