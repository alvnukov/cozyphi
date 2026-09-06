package session

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestTitleRoundTripBeforeAssistant(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	_, err = m.Append(llm.Message{Role: llm.RoleUser, Content: "Первая цель"})
	require.NoError(t, err)
	leaf, before := m.LeafID(), m.BuildContext()
	require.NoError(t, m.SetTitle("  Разобрать   имена сессий  ", "model"))
	require.Equal(t, leaf, m.LeafID())
	require.Equal(t, before, m.BuildContext())
	rows, err := ListSessions(dir)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "Первая цель", rows[0].FirstPrompt)
	require.Equal(t, "Разобрать имена сессий", DisplayTitle(rows[0]))
	require.Equal(t, "model", rows[0].TitleSource)
	require.NoError(t, m.SetTitle("Моё имя", "user"))
	count := m.Len()
	require.ErrorIs(t, m.SetTitle("late model", "model"), ErrTitlePinned)
	require.Equal(t, count, m.Len())
	path := m.File()
	require.NoError(t, m.Close())
	loaded, err := OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, loaded.Close()) })
	require.Equal(t, "Моё имя", loaded.DisplayTitle())
	require.Equal(t, leaf, loaded.LeafID())
	after := loaded.BuildContext()
	require.Len(t, after, len(before))
	for i := range before {
		require.Equal(t, before[i].GetID(), after[i].GetID())
		require.Equal(t, before[i].(SessionMessageEntry).Message, after[i].(SessionMessageEntry).Message)
	}
	require.ErrorIs(t, loaded.SetTitle("late", "model"), ErrTitlePinned)
	require.NoError(t, loaded.SetTitle("Новое имя", "user"))
	require.ErrorIs(t, m.SetTitle("closed", "user"), os.ErrClosed)
}

func TestTitleValidationAndFallback(t *testing.T) {
	m := NewManager(t.TempDir())
	for _, invalid := range []string{"", "  ", "one\ntwo", "tab\there", "\x1b]2;bad", "\a", "\u202eRTL", "\u2028", string([]byte{0xff}), strings.Repeat("界", 61)} {
		t.Run(string([]rune(invalid)), func(t *testing.T) {
			require.Error(t, m.SetTitle(invalid, "model"))
			require.Equal(t, 1, m.Len())
		})
	}
	require.Error(t, m.SetTitle("valid", "other"))
	require.Equal(t, ShortID(m.ID()), m.DisplayTitle())
	_, err := m.AppendDelivery("event", llm.Message{Role: llm.RoleUser, Content: "not a user prompt"})
	require.NoError(t, err)
	_, err = m.Append(
		llm.Message{
			Role:    llm.RoleUser,
			Content: "<system-reminder>noise</system-reminder>\n" + strings.Repeat("界", 60),
		},
	)
	require.NoError(t, err)
	_, err = m.Append(llm.Message{Role: llm.RoleUser, Content: "last prompt"})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("界", 48), m.DisplayTitle())
	require.NoError(t, m.SetTitle(strings.Repeat("界", 60), "model"))
	require.Equal(t, 60, utf8.RuneCountInString(m.DisplayTitle()))
	require.NotContains(t, DisplayTitle(SessionMeta{FirstPrompt: "evil\x1b]2;\a\u009c\u202e"}), "\x1b")
}

func TestTitleFailedFlushRollsBack(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	m.sessionFile = filepath.Join(dir, "missing", "session.jsonl")
	require.Error(t, m.SetTitle("not saved", "user"))
	title, source := m.Title()
	require.Empty(t, title)
	require.Empty(t, source)
	require.Equal(t, 1, m.Len())
	require.Empty(t, m.LeafID())
}

func TestTitleUserPinIsAtomic(t *testing.T) {
	m := NewManager(t.TempDir())
	var wg sync.WaitGroup
	for range 20 {
		wg.Go(func() { _ = m.SetTitle("model", "model") })
	}
	require.NoError(t, m.SetTitle("user", "user"))
	wg.Wait()
	title, source := m.Title()
	require.Equal(t, "user", title)
	require.Equal(t, "user", source)
}

func TestTitleLegacyListFallback(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	for _, msg := range []llm.Message{
		{Role: llm.RoleUser, Content: "first prompt"},
		{Role: llm.RoleAssistant, Content: "answer"},
		{Role: llm.RoleUser, Content: "last prompt"},
	} {
		_, err = m.Append(msg)
		require.NoError(t, err)
	}
	rows, err := ListSessions(dir)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.Equal(t, "first prompt", DisplayTitle(rows[0]))
	require.Equal(t, "last prompt", rows[0].Preview)
}
