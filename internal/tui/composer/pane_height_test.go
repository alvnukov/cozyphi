package composer

import (
	"strings"
	"testing"

	"github.com/pulseaiclub/xui"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
)

// TestChatInputStartsSingleLine: the composer occupies one editor row while
// empty (the old floor of three wasted two rows) and grows with wrapped
// content up to MaxBodyRows.
func TestChatInputStartsSingleLine(t *testing.T) {
	t.Parallel()

	c := newChatInput(components.DefaultTheme(), "m", "/tmp")

	require.Equal(t, 1, c.MinBodyRows, "empty composer must start at one row")
	require.Equal(t, 6, c.MinHeight(), "1 editor row + 5 frame chrome rows")
	require.Equal(t, 6, c.PreferredHeight(80, xui.WidthUnicode), "empty value stays single line")

	c.Value = "a\nb\nc"
	require.Equal(t, 8, c.PreferredHeight(80, xui.WidthUnicode), "three lines grow the frame")

	c.Value = strings.Repeat("x\n", 40)
	require.Equal(t, 13, c.PreferredHeight(80, xui.WidthUnicode), "MaxBodyRows 8 caps growth: 8+5")
}
