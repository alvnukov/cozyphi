package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestVimAlternateLayoutDeleteWord(t *testing.T) {
	c := &ChatInput{Value: "one two", Cursor: 0}
	c.SetEditingMode(editmode.Vim)
	require.True(t, editEscape(c))
	editKey(c, 'в', 0)
	editKey(c, 'ц', 0)
	require.Equal(t, "two", c.Value)
	require.Zero(t, c.Cursor)
	require.Equal(t, "VIM NORMAL", c.EditingLabel())
}

func TestVimShiftAlternateRune(t *testing.T) {
	for _, key := range []xui.KeyEvent{
		{Press: true, Code: xui.KeyRune, Rune: 'Ф', AltRune: 'a', Mods: xui.ModShift},
		{Press: true, Code: xui.KeyRune, Rune: 'Ф', AltRune: 'a'},
		{Press: true, Code: xui.KeyRune, Rune: 'a', Mods: xui.ModShift},
		{Press: true, Code: xui.KeyRune, Rune: 'A'},
	} {
		c := &ChatInput{Value: "one two", Cursor: 0}
		c.SetEditingMode(editmode.Vim)
		require.True(t, editEscape(c))
		c.Handle(&components.EventContext{}, key)
		require.Equal(t, "one two", c.Value)
		require.Equal(t, 7, c.Cursor, "Shift+A appends at line end, not after the caret")
		require.Equal(t, "VIM INSERT", c.EditingLabel())
	}
}

func TestVimAlternateLayoutCommands(t *testing.T) {
	for _, tc := range []struct {
		name, value, keys, want, mode string
		cursor, pos                   int
	}{
		{"double delete", "one\ntwo", "вв", "two", "VIM NORMAL", 0, 0},
		{"change word then type Cyrillic", "one two", "сцновый", "новый two", "VIM INSERT", 0, len("новый")},
		{"yank and put line", "one\ntwo", "ннз", "one\none\ntwo", "VIM NORMAL", 0, 4},
		{"document start", "one\ntwo", "пп", "one\ntwo", "VIM NORMAL", 7, 0},
		{"word forward", "one two", "ц", "one two", "VIM NORMAL", 0, 4},
		{"word backward", "one two", "и", "one two", "VIM NORMAL", 7, 4},
		{"character forward", "a界🙂b", "д", "a界🙂b", "VIM NORMAL", 0, 1},
		{"character backward", "a界🙂b", "р", "a界🙂b", "VIM NORMAL", len("a界🙂b"), len("a界")},
		{"line down", "one\ntwo", "о", "one\ntwo", "VIM NORMAL", 0, 4},
		{"line up", "one\ntwo", "л", "one\ntwo", "VIM NORMAL", 5, 0},
		{"uppercase append", "one two", "Ф", "one two", "VIM INSERT", 0, 7},
		{"uppercase insert", "  one", "Ш", "  one", "VIM INSERT", 5, 2},
		{"uppercase delete rest", "one two", "В", "", "VIM NORMAL", 0, 0},
		{"uppercase change rest", "one two", "С", "", "VIM INSERT", 0, 0},
		{"uppercase document end", "one\ntwo", "П", "one\ntwo", "VIM NORMAL", 0, 4},
		{"uppercase open above", "one", "Щ", "\none", "VIM INSERT", 0, 0},
		{"punctuation motion", "one two", "$", "one two", "VIM NORMAL", 0, 6},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &ChatInput{Value: tc.value, Cursor: tc.cursor}
			c.SetEditingMode(editmode.Vim)
			require.True(t, editEscape(c))
			for _, key := range tc.keys {
				require.True(t, editKey(c, key, 0))
			}
			require.Equal(t, tc.want, c.Value)
			require.Equal(t, tc.pos, c.Cursor)
			require.Equal(t, tc.mode, c.EditingLabel())
		})
	}
}

func TestVimInsertKeepsOriginalRunes(t *testing.T) {
	c := &ChatInput{}
	c.SetEditingMode(editmode.Vim)
	for _, key := range []xui.KeyEvent{
		{Press: true, Code: xui.KeyRune, Rune: 'в'},
		{Press: true, Code: xui.KeyRune, Rune: 'ц', AltRune: 'w'},
		{Press: true, Code: xui.KeyRune, Rune: 'Ф', AltRune: 'a', Mods: xui.ModShift},
	} {
		c.Handle(&components.EventContext{}, key)
	}
	require.Equal(t, "вцФ", c.Value)
	require.Equal(t, len("вцФ"), c.Cursor)
	require.Equal(t, "VIM INSERT", c.EditingLabel())
}
