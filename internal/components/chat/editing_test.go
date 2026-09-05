package chat

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

func editKey(c *ChatInput, key rune, mods xui.Modifiers) bool {
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: key, Mods: mods})
	return ctx.Consume
}

func editEscape(c *ChatInput) bool {
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEscape})
	return ctx.Consume
}

func TestReadlineEditing(t *testing.T) {
	for _, tc := range []struct {
		name, value string
		cursor      int
		key         rune
		mods        xui.Modifiers
		want        string
		pos         int
	}{
		{"start of logical line", "first\nпривет мир\nlast", len("first\nпривет"), 'a', xui.ModCtrl, "first\nпривет мир\nlast", len("first\n")},
		{"end of logical line", "first\nпривет мир\nlast", len("first\nпри"), 'e', xui.ModCtrl, "first\nпривет мир\nlast", len("first\nпривет мир")},
		{"unicode backward", "a界🙂b", len("a界🙂"), 'b', xui.ModCtrl, "a界🙂b", len("a界")},
		{"unicode forward", "a界🙂b", len("a界"), 'f', xui.ModCtrl, "a界🙂b", len("a界🙂")},
		{"kill word keeps separator", "one two", 7, 'w', xui.ModCtrl, "one ", 4},
		{"kill whitespace word", "one foo/bar  ", 13, 'w', xui.ModCtrl, "one ", 4},
		{"kill rest", "first\nhello world\nlast", 11, 'k', xui.ModCtrl, "first\nhello\nlast", 11},
		{"kill newline at end", "hello\nworld", 5, 'k', xui.ModCtrl, "helloworld", 5},
		{"empty ctrl d does not quit", "", 0, 'd', xui.ModCtrl, "", 0},
		{"alt word", "one two", 7, 'b', xui.ModAlt, "one two", 4},
		{"kill before", "first\nпривет мир", len("first\nпривет"), 'u', xui.ModCtrl, "first\n мир", 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ChatInput{Value: tc.value, Cursor: tc.cursor}
			c.SetEditingMode(editmode.Readline)
			require.True(t, editKey(c, tc.key, tc.mods))
			require.Equal(t, tc.want, c.Value)
			require.Equal(t, tc.pos, c.Cursor)
			require.True(t, utf8.ValidString(c.Value))
		})
	}
}

func TestEditingUndoAndYank(t *testing.T) {
	c := &ChatInput{Value: "one привет", Cursor: len("one привет")}
	c.SetEditingMode(editmode.Readline)
	editKey(c, 'w', xui.ModCtrl)
	require.Equal(t, "one ", c.Value)
	editKey(c, 'y', xui.ModCtrl)
	require.Equal(t, "one привет", c.Value)
	editKey(c, 'z', xui.ModCtrl)
	require.Equal(t, "one ", c.Value)
	editKey(c, 'Z', xui.ModCtrl|xui.ModShift)
	require.Equal(t, "one привет", c.Value)

	c.SetSelection(0, len(c.Value))
	editKey(c, 'x', 0)
	require.Equal(t, "x", c.Value)
	editKey(c, 'z', xui.ModCtrl)
	require.Equal(t, "one привет", c.Value, "selection replacement is one edit")
	editKey(c, '!', 0)
	editKey(c, 'Z', xui.ModCtrl|xui.ModShift)
	require.Equal(t, "one привет!", c.Value, "a new edit discards the redo branch")
}

func TestEditingExternalReplacementInvalidatesUndo(t *testing.T) {
	c := &ChatInput{Value: "old", Cursor: 3}
	editKey(c, '!', 0)
	c.Value, c.Cursor = "new draft", 9
	editKey(c, 'z', xui.ModCtrl)
	require.Equal(t, "new draft", c.Value)
}

func TestEditingModePreservesDraftAndDefaultSelection(t *testing.T) {
	c := &ChatInput{Value: "hello", Cursor: 2}
	editKey(c, 'a', xui.ModCtrl)
	require.Equal(t, "hello", c.SelectedText())
	for _, mode := range []editmode.Mode{editmode.Readline, editmode.Vim, editmode.Standard} {
		c.SetEditingMode(mode)
		require.Equal(t, "hello", c.Value)
		require.Equal(t, len("hello"), c.Cursor)
	}
	c.SetEditingMode(editmode.Readline)
	editKey(c, 'a', xui.ModSuper)
	require.Equal(t, "hello", c.SelectedText(), "Cmd+A remains native select-all")
}

func TestVimEditing(t *testing.T) {
	for _, tc := range []struct{ name, value, keys, want string }{
		{"delete word", "one two", "0dw", "two"},
		{"change word", "one two", "0cwnew", "new two"},
		{"delete first line", "one\ntwo", "ggdd", "two"},
		{"delete final line", "one\ntwo", "Gdd", "one"},
		{"yank put line", "one\ntwo", "gg yyp", "one\none\ntwo"},
		{"delete unicode", "a界🙂b", "0lx", "a🙂b"},
		{"append", "one", "A!", "one!"},
		{"open below", "one\ntwo", "ggohello", "one\nhello\ntwo"},
		{"open above", "one", "Ohello", "hello\none"},
		{"change line", "one\ntwo", "ggccnew", "new\ntwo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ChatInput{Value: tc.value, Cursor: len(tc.value)}
			c.SetEditingMode(editmode.Vim)
			require.True(t, editEscape(c))
			for _, key := range tc.keys {
				editKey(c, key, 0)
			}
			require.Equal(t, tc.want, c.Value)
			require.True(t, utf8.ValidString(c.Value))
			require.True(t, utf8.ValidString(c.Value[:c.Cursor]))
		})
	}
}

func TestVimNormalRouting(t *testing.T) {
	submits := 0
	c := &ChatInput{Value: "hello", Cursor: 5, OnSubmit: func(string) { submits++ }}
	c.SetEditingMode(editmode.Vim)
	require.Equal(t, "VIM INSERT", c.EditingLabel())
	require.True(t, editEscape(c))
	require.Equal(t, "VIM NORMAL", c.EditingLabel())
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.Zero(t, submits)
	editKey(c, 'x', 0)
	require.Equal(t, "hell", c.Value)
	editKey(c, 'u', 0)
	require.Equal(t, "hello", c.Value)
	editKey(c, 'r', xui.ModCtrl)
	require.Equal(t, "hell", c.Value)
	editKey(c, 'i', 0)
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyEnter})
	require.Equal(t, 1, submits)
}

func TestVimPickerAndPaste(t *testing.T) {
	c := &ChatInput{Value: "/help", Cursor: 5, SlashOpen: true}
	c.SetEditingMode(editmode.Vim)
	require.False(t, editEscape(c), "picker must close before changing mode")
	c.SlashOpen = false
	require.True(t, editEscape(c))
	c.Handle(&components.EventContext{}, xui.PasteEvent{Text: "🙂"})
	require.Equal(t, "VIM INSERT", c.EditingLabel())
	require.Contains(t, c.Value, "🙂")
	c.VoiceMode = true
	require.False(t, editEscape(c), "voice must close before changing mode")
}

func TestEditingModeRenderedAtNarrowWidths(t *testing.T) {
	c := &ChatInput{Value: "hello", Cursor: 5}
	c.SetEditingMode(editmode.Vim)
	for _, width := range []int{18, 40, 80} {
		s := drawEditor(c, width)
		var b strings.Builder
		for _, cell := range s.Buffer {
			b.WriteString(cell.Char)
		}
		require.Contains(t, b.String(), "VIM INSERT")
	}
	editEscape(c)
	s := drawEditor(c, 40)
	var b strings.Builder
	for _, cell := range s.Buffer {
		b.WriteString(cell.Char)
	}
	require.Contains(t, b.String(), "VIM NORMAL")
}
