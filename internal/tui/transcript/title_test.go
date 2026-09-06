package transcript

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
)

func TestTitleRowStaysCompact(t *testing.T) {
	m := &Mapper{}
	it := session.Item{
		ToolName:  "session",
		ToolInput: `{"title":"raw input"}`,
		ToolRun:   session.ToolRun{Status: session.ToolInProgress},
	}
	b := m.titleWidget(it)
	require.Equal(t, "set_title", b.Detail)
	require.Empty(t, b.Output)
	require.Nil(t, b.OnToggle)
	it.ToolRun = session.ToolRun{
		Status: session.ToolDone,
		Detail: "Имена текущих сессий",
		Output: "Title: Имена текущих сессий",
	}
	ok, dirty := m.patchTitle(b, it)
	require.True(t, ok)
	require.True(t, dirty)
	require.Equal(t, "Title", b.Name)
	require.Equal(t, "Имена текущих сессий", b.Detail)
	require.Empty(t, b.Output)
	require.False(t, b.Expanded)
	it.ToolRun = session.ToolRun{Status: session.ToolError, Error: "title is pinned by the user"}
	ok, dirty = m.patchTitle(b, it)
	require.True(t, ok)
	require.True(t, dirty)
	require.Contains(t, b.Detail, "pinned")
	require.Empty(t, b.Output)
}
