package chat

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/editmode"
)

func TestEditingGraphemeDeleteAcrossProfiles(t *testing.T) {
	for _, mode := range []editmode.Mode{editmode.Standard, editmode.Readline, editmode.Vim} {
		for _, cluster := range []string{"e\u0301", "👩‍💻", "🇫🇷", "👍🏽"} {
			t.Run(mode.String()+"/"+cluster, func(t *testing.T) {
				c := &ChatInput{Value: cluster + "x", Cursor: len(cluster)}
				c.SetEditingMode(mode)
				c.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyBackspace})
				require.Equal(t, "x", c.Value)
				editKey(c, 'z', xui.ModCtrl)
				require.Equal(t, cluster+"x", c.Value)
				c.Cursor = 0
				c.Handle(&components.EventContext{}, xui.KeyEvent{Press: true, Code: xui.KeyDelete})
				require.Equal(t, "x", c.Value)
			})
		}
	}
}

func TestVimNormalBackspaceIsMotion(t *testing.T) {
	c := &ChatInput{Value: "abc", Cursor: 3}
	c.SetEditingMode(editmode.Vim)
	editEscape(c)
	ctx := &components.EventContext{}
	c.Handle(ctx, xui.KeyEvent{Press: true, Code: xui.KeyBackspace})
	require.True(t, ctx.Consume)
	require.Equal(t, "abc", c.Value)
	require.Equal(t, 1, c.Cursor)
}

func TestVimLineBoundaries(t *testing.T) {
	for _, tc := range []struct{ value, keys, want string }{
		{"one", "ddp", "one"},
		{"one\n", "Gdd", "one"},
		{"one\ntwo", "Gddp", "one\ntwo"},
		{"one\n", "Gddp", "one\n"},
		{"", "ddp", ""},
		{"one", "ccnew", "new"},
	} {
		t.Run(tc.value+"/"+tc.keys, func(t *testing.T) {
			c := &ChatInput{Value: tc.value, Cursor: len(tc.value)}
			c.SetEditingMode(editmode.Vim)
			editEscape(c)
			for _, key := range tc.keys {
				editKey(c, key, 0)
			}
			require.Equal(t, tc.want, c.Value)
		})
	}
}

func TestVimCharactersAreGraphemes(t *testing.T) {
	for _, cluster := range []string{"e\u0301", "👩‍💻", "🇫🇷", "👍🏽"} {
		t.Run(cluster, func(t *testing.T) {
			c := &ChatInput{Value: cluster + "x", Cursor: len(cluster) + 1}
			c.SetEditingMode(editmode.Vim)
			editEscape(c)
			editKey(c, 'h', 0)
			require.Zero(t, c.Cursor)
			editKey(c, 'l', 0)
			require.Equal(t, len(cluster), c.Cursor)
			editKey(c, '0', 0)
			editKey(c, 'x', 0)
			require.Equal(t, "x", c.Value)
			editKey(c, 'u', 0)
			require.Equal(t, cluster+"x", c.Value)
		})
	}
}
