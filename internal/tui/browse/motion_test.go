package browse_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

func key(code xui.KeyCode, r rune, mods xui.Modifiers) xui.KeyEvent {
	return xui.KeyEvent{Press: true, Code: code, Rune: r, Mods: mods}
}

// feed runs a key sequence and returns the last parse result.
func feed(m *browse.Motions, events ...xui.KeyEvent) (browse.Motion, bool) {
	var got browse.Motion
	var ok bool
	for _, e := range events {
		got, ok = m.Key(e)
	}
	return got, ok
}

func TestKeysMapToMotions(t *testing.T) {
	cases := map[string]struct {
		events []xui.KeyEvent
		want   browse.Motion
	}{
		"up":      {[]xui.KeyEvent{key(xui.KeyUp, 0, 0)}, browse.Motion{Op: browse.OpStep, N: -1}},
		"down":    {[]xui.KeyEvent{key(xui.KeyDown, 0, 0)}, browse.Motion{Op: browse.OpStep, N: 1}},
		"j":       {[]xui.KeyEvent{key(xui.KeyRune, 'j', 0)}, browse.Motion{Op: browse.OpStep, N: 1}},
		"k":       {[]xui.KeyEvent{key(xui.KeyRune, 'k', 0)}, browse.Motion{Op: browse.OpStep, N: -1}},
		"pgup":    {[]xui.KeyEvent{key(xui.KeyPageUp, 0, 0)}, browse.Motion{Op: browse.OpPage, N: -1}},
		"pgdn":    {[]xui.KeyEvent{key(xui.KeyPageDown, 0, 0)}, browse.Motion{Op: browse.OpPage, N: 1}},
		"home":    {[]xui.KeyEvent{key(xui.KeyHome, 0, 0)}, browse.Motion{Op: browse.OpTop}},
		"end":     {[]xui.KeyEvent{key(xui.KeyEnd, 0, 0)}, browse.Motion{Op: browse.OpBottom}},
		"G":       {[]xui.KeyEvent{key(xui.KeyRune, 'G', 0)}, browse.Motion{Op: browse.OpBottom}},
		"shift+G": {[]xui.KeyEvent{key(xui.KeyRune, 'G', xui.ModShift)}, browse.Motion{Op: browse.OpBottom}},
		"ctrl+d":  {[]xui.KeyEvent{key(xui.KeyRune, 'd', xui.ModCtrl)}, browse.Motion{Op: browse.OpHalfPage, N: 1}},
		"ctrl+u":  {[]xui.KeyEvent{key(xui.KeyRune, 'u', xui.ModCtrl)}, browse.Motion{Op: browse.OpHalfPage, N: -1}},
		"gg": {
			[]xui.KeyEvent{key(xui.KeyRune, 'g', 0), key(xui.KeyRune, 'g', 0)},
			browse.Motion{Op: browse.OpTop},
		},
		"3j": {
			[]xui.KeyEvent{key(xui.KeyRune, '3', 0), key(xui.KeyRune, 'j', 0)},
			browse.Motion{Op: browse.OpStep, N: 3},
		},
		"12G": {
			[]xui.KeyEvent{key(xui.KeyRune, '1', 0), key(xui.KeyRune, '2', 0), key(xui.KeyRune, 'G', 0)},
			browse.Motion{Op: browse.OpIndex, N: 12},
		},
		"2ctrl+d": {
			[]xui.KeyEvent{key(xui.KeyRune, '2', 0), key(xui.KeyRune, 'd', xui.ModCtrl)},
			browse.Motion{Op: browse.OpHalfPage, N: 2},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var m browse.Motions
			got, ok := feed(&m, tc.events...)
			assert.True(t, ok, "the sequence belongs to the dialect")
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestPendingInputIsConsumedQuietly(t *testing.T) {
	var m browse.Motions
	for _, e := range []xui.KeyEvent{key(xui.KeyRune, '4', 0), key(xui.KeyRune, 'g', 0)} {
		got, ok := m.Key(e)
		assert.True(t, ok, "digits and a first g are consumed")
		assert.Equal(t, browse.Motion{}, got, "but produce no motion yet")
	}
}

func TestKeysOutsideTheDialectStayWithTheCaller(t *testing.T) {
	for name, e := range map[string]xui.KeyEvent{
		"letter":     key(xui.KeyRune, 'x', 0),
		"escape":     key(xui.KeyEscape, 0, 0),
		"enter":      key(xui.KeyEnter, 0, 0),
		"shift+up":   key(xui.KeyUp, 0, xui.ModShift),
		"ctrl+s":     key(xui.KeyRune, 's', xui.ModCtrl),
		"alt+letter": key(xui.KeyRune, 'j', xui.ModAlt),
		"delete":     key(xui.KeyDelete, 0, 0),
	} {
		t.Run(name, func(t *testing.T) {
			var m browse.Motions
			_, ok := m.Key(e)
			assert.False(t, ok)
		})
	}
}

func TestForeignKeysClearPendingInput(t *testing.T) {
	var m browse.Motions
	m.Key(key(xui.KeyRune, '5', 0))
	m.Key(key(xui.KeyRune, 'x', 0))
	got, ok := m.Key(key(xui.KeyRune, 'j', 0))
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{Op: browse.OpStep, N: 1}, got, "the count died with the x")

	m.Key(key(xui.KeyRune, 'g', 0))
	m.Key(key(xui.KeyEscape, 0, 0))
	got, ok = m.Key(key(xui.KeyRune, 'g', 0))
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{}, got, "escape broke the gg; this g starts a new one")
}

func TestDigitsCancelAPendingG(t *testing.T) {
	var m browse.Motions
	m.Key(key(xui.KeyRune, 'g', 0))
	m.Key(key(xui.KeyRune, '3', 0))
	got, ok := m.Key(key(xui.KeyRune, 'g', 0))
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{}, got, "g3g is a fresh pending g, not a gg")
}

func TestResetDropsTheCount(t *testing.T) {
	var m browse.Motions
	m.Key(key(xui.KeyRune, '7', 0))
	m.Reset()
	got, ok := m.Key(key(xui.KeyRune, 'j', 0))
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{Op: browse.OpStep, N: 1}, got)
}

func TestWheelScrollsThreePerNotch(t *testing.T) {
	got, ok := browse.Wheel(xui.MouseEvent{Button: xui.MouseWheelDown, Wheel: 2})
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{Op: browse.OpStep, N: 2 * browse.WheelStep}, got)

	got, ok = browse.Wheel(xui.MouseEvent{Button: xui.MouseWheelUp})
	assert.True(t, ok)
	assert.Equal(t, browse.Motion{Op: browse.OpStep, N: -browse.WheelStep}, got, "a zero notch count still means one")

	_, ok = browse.Wheel(xui.MouseEvent{Button: xui.MouseLeft})
	assert.False(t, ok, "clicks are not scrolls")
}
