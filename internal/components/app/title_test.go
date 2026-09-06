package app

import (
	"bytes"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
)

type titleSource struct{ title string }

func (s *titleSource) TerminalTitle() string { return s.title }

func TestTerminalTitleSafeAndOnlyChanged(t *testing.T) {
	var state terminalTitle
	var out bytes.Buffer
	source := &titleSource{title: "hello\x1b\a\n\r\x00\u009c\u202e世界\xff"}
	require.NoError(t, state.sync(source, out.Write))
	require.Equal(t, "\x1b]0;hello世界\x1b\\\x1b]2;hello世界\x1b\\", out.String())
	out.Reset()
	require.NoError(t, state.sync(source, out.Write))
	require.Empty(t, out.String())
	source.title = "resumed"
	require.NoError(t, state.sync(source, out.Write))
	require.Equal(t, "\x1b]0;resumed\x1b\\\x1b]2;resumed\x1b\\", out.String())
	out.Reset()
	require.NoError(t, state.sync(struct{}{}, out.Write))
	require.Empty(t, out.String())
}

func TestTerminalTitleRetriesFailedWrite(t *testing.T) {
	for _, failure := range []error{io.ErrClosedPipe, nil} {
		var state terminalTitle
		source := &titleSource{title: "title"}
		err := state.sync(source, func([]byte) (int, error) { return 0, failure })
		if failure != nil {
			require.ErrorIs(t, err, failure)
		} else {
			require.ErrorIs(t, err, io.ErrShortWrite)
		}
		var out bytes.Buffer
		require.NoError(t, state.sync(source, out.Write))
		require.Equal(t, "\x1b]0;title\x1b\\\x1b]2;title\x1b\\", out.String())
	}
}
