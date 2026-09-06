package sessiontool_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools/sessiontool"
)

func TestValidationAndPinAtStoreBoundary(t *testing.T) {
	dir := t.TempDir()
	store, err := session.NewSessionManager(dir, session.WithSessionDir(dir), session.WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, store.Close()) })
	tool := sessiontool.Tool(func(title string) (string, error) {
		if err := store.SetTitle(title, "model"); err != nil {
			return "", err
		}
		saved, _ := store.Title()
		return saved, nil
	})
	for _, title := range []string{"", " \t ", "one\ntwo", "one\rtwo", "bad\x1btitle", "bad\u202etitle", strings.Repeat("я", 61)} {
		input, err := json.Marshal(map[string]string{"action": "set_title", "title": title})
		require.NoError(t, err)
		before := store.Len()
		_, err = tool.Run(t.Context(), input)
		require.Error(t, err)
		require.Equal(t, before, store.Len())
	}
	_, err = tool.Run(t.Context(), []byte("{\"action\":\"set_title\",\"title\":\"\xff\"}"))
	require.ErrorContains(t, err, "UTF-8")
	result, err := tool.Run(t.Context(), []byte(`{"action":"set_title","title":"  Имя   сессии  "}`))
	require.NoError(t, err)
	require.Equal(t, "Имя сессии", result.Detail)
	require.NoError(t, store.SetTitle("Ручное имя", "user"))
	before := store.Len()
	_, err = tool.Run(t.Context(), []byte(`{"action":"set_title","title":"new name"}`))
	require.ErrorIs(t, err, session.ErrTitlePinned)
	require.Equal(t, before, store.Len())
}
