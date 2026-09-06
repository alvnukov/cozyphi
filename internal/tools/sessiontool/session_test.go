package sessiontool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/tools/sessiontool"
)

func TestSetTitle(t *testing.T) {
	var got string
	tool := sessiontool.Tool(func(title string) (string, error) { got = title; return title, nil })
	result, err := tool.Run(t.Context(), []byte(`{"action":"set_title","title":"Имена текущих сессий"}`))
	require.NoError(t, err)
	require.Equal(t, "Имена текущих сессий", got)
	require.Equal(t, "Title: Имена текущих сессий", result.Content)
	require.Equal(t, got, result.Detail)
}

func TestInvalidActionDoesNotWrite(t *testing.T) {
	tool := sessiontool.Tool(func(string) (string, error) { t.Fatal("unexpected write"); return "", nil })
	for _, input := range []string{`{`, `{}`, `{"action":"rename","title":"name"}`} {
		_, err := tool.Run(t.Context(), []byte(input))
		require.Error(t, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := tool.Run(ctx, []byte(`{"action":"set_title","title":"name"}`))
	require.ErrorIs(t, err, context.Canceled)
}

func TestStoreFailure(t *testing.T) {
	failure := errors.New("title is pinned by the user; ask the user to run /rename")
	tool := sessiontool.Tool(func(string) (string, error) { return "", failure })
	result, err := tool.Run(t.Context(), []byte(`{"action":"set_title","title":"new name"}`))
	require.ErrorIs(t, err, failure)
	require.Empty(t, result.Content)
}
