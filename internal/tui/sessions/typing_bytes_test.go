package sessions

import (
	"bytes"
	"io"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/app"
)

// byteTTY is a terminal that keeps what the last frame wrote.
type byteTTY struct {
	cols, rows int
	out        bytes.Buffer
}

func (*byteTTY) Read([]byte) (int, error)      { return 0, io.EOF }
func (f *byteTTY) Write(p []byte) (int, error) { return f.out.Write(p) }
func (f *byteTTY) Size() (int, int, error)     { return f.cols, f.rows, nil }
func (*byteTTY) MakeRaw() error                { return nil }
func (*byteTTY) Restore() error                { return nil }
func (*byteTTY) Fd() uintptr                   { return 0 }
func (*byteTTY) Close() error                  { return nil }
func (*byteTTY) Interrupt()                    {}

// A typed character costs the terminal the rows it changed, not the whole
// screen. Every keystroke used to queue a full refresh, ~10 KB at 120x40; a
// terminal that drains its pty slowly (a loaded laptop) then fell behind the
// typing, while Claude Code in the next window kept up.
func TestTypedKeyWritesAFractionOfTheScreen(t *testing.T) {
	e := newTestEditor(t)
	tty := &byteTTY{cols: 120, rows: 40}
	vx, err := xui.NewWithTTY(tty, xui.Options{})
	require.NoError(t, err)
	e.vx = vx
	e.App = app.NewApp(vx, components.DefaultTheme())
	e.App.SetRoot(e)
	e.App.RequestFocus(&e.composer.Chat)

	frame := func() int {
		tty.out.Reset()
		surf := e.Draw(components.DrawContext{
			Max:    components.Size{Width: tty.cols, Height: tty.rows},
			Method: xui.WidthUnicode,
		})
		win := vx.Window()
		win.Clear()
		if cur := surf.Render(win); cur != nil {
			vx.Screen().SetCursor(cur.X, cur.Y)
		} else {
			vx.Screen().ClearCursor()
		}
		require.NoError(t, vx.Render())
		return tty.out.Len()
	}
	full := frame()
	frame()

	for _, r := range "hello" {
		dispatchControlKey(e, xui.KeyEvent{Press: true, Code: xui.KeyRune, Rune: r, Text: string(r)})
		n := frame()
		require.Less(t, n, full/4, "key %q wrote %d bytes, the whole screen is %d", r, n, full)
	}
	require.Equal(t, "hello", e.composer.Chat.Value)
}
