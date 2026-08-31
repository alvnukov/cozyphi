package browse_test

import (
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tui/browse"
)

func TestConfirmFiresOnY(t *testing.T) {
	var c browse.Confirm
	fired := 0
	c.Arm(`Delete step 2, "wire the pane"?`, func() { fired++ })
	assert.True(t, c.Armed())
	assert.Equal(t, `Delete step 2, "wire the pane"?`, c.Label())

	assert.True(t, c.Key(key(xui.KeyRune, 'y', 0)))
	assert.Equal(t, 1, fired)
	assert.False(t, c.Armed())
	assert.Empty(t, c.Label())

	assert.False(t, c.Key(key(xui.KeyRune, 'y', 0)), "a second y answers nothing")
	assert.Equal(t, 1, fired)
}

func TestConfirmCancelsOnNAndEscape(t *testing.T) {
	for name, e := range map[string]xui.KeyEvent{
		"n":      key(xui.KeyRune, 'n', 0),
		"escape": key(xui.KeyEscape, 0, 0),
	} {
		t.Run(name, func(t *testing.T) {
			var c browse.Confirm
			fired := false
			c.Arm("Sure?", func() { fired = true })
			assert.True(t, c.Key(e), "an answer is consumed")
			assert.False(t, fired)
			assert.False(t, c.Armed())
		})
	}
}

func TestConfirmWithdrawsOnAnyOtherKey(t *testing.T) {
	var c browse.Confirm
	fired := false
	c.Arm("Sure?", func() { fired = true })

	assert.False(t, c.Key(key(xui.KeyRune, 'j', 0)), "the caller still owns the key")
	assert.False(t, fired)
	assert.False(t, c.Armed(), "acting elsewhere withdraws the question")
}

func TestConfirmRearmingReplacesTheQuestion(t *testing.T) {
	var c browse.Confirm
	var got string
	c.Arm("first?", func() { got = "first" })
	c.Arm("second?", func() { got = "second" })
	assert.Equal(t, "second?", c.Label())

	assert.True(t, c.Key(key(xui.KeyRune, 'y', 0)))
	assert.Equal(t, "second", got, "a double y can never fire two different actions")
}
