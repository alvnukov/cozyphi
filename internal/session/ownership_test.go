package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestOpenSessionRejectsActiveOwner(t *testing.T) {
	path := writeSessionFixture(t, "")
	first, err := OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, first.Close()) })
	second, err := OpenSession(path)
	require.Nil(t, second)
	require.ErrorIs(t, err, ErrBusy)
	require.NoError(t, first.Close())
	require.NoError(t, first.Close())
	second, err = OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, second.Close()) })
	// A repeated close on the old manager cannot release the new owner's lock.
	require.NoError(t, first.Close())
	_, err = OpenSession(path)
	require.ErrorIs(t, err, ErrBusy)
}

func TestNewSessionOwnsBeforeAndAfterFlush(t *testing.T) {
	dir := t.TempDir()
	m, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	require.NoFileExists(t, m.File())
	_, err = OpenSession(m.File())
	require.ErrorIs(t, err, ErrBusy, "ownership starts before the JSONL is created")
	p := startOwnerProcess(t, m.File())
	p.send(t, "open")
	require.Equal(t, "busy", p.line(t))
	require.NoError(t, p.wait(t))

	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "persist"})
	require.NoError(t, err)
	_, err = OpenSession(m.File())
	require.ErrorIs(t, err, ErrBusy, "temp+rename must not drop ownership")
	p = startOwnerProcess(t, m.File())
	p.send(t, "open")
	require.Equal(t, "busy", p.line(t))
	require.NoError(t, p.wait(t))
}

func TestClosedSessionCannotWrite(t *testing.T) {
	for _, flushed := range []bool{false, true} {
		t.Run(map[bool]string{false: "buffered", true: "flushed"}[flushed], func(t *testing.T) {
			dir := t.TempDir()
			m, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
			require.NoError(t, err)
			if flushed {
				_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "persist"})
				require.NoError(t, err)
			}
			before := m.Len()
			require.NoError(t, m.Close())
			_, err = m.Append(llm.Message{Role: llm.RoleUser, Content: "not allowed"})
			require.ErrorIs(t, err, os.ErrClosed)
			_, err = m.AppendAssistant(llm.Message{Role: llm.RoleAssistant, Content: "no"}, "model", "")
			require.ErrorIs(t, err, os.ErrClosed)
			_, err = m.AppendCompaction(Compaction{Summary: "no"})
			require.ErrorIs(t, err, os.ErrClosed)
			_, err = m.ReplacePlan([]PlanItem{{Content: "no", Status: PlanPending, Type: StepEdit}})
			require.ErrorIs(t, err, os.ErrClosed)
			require.Equal(t, before, m.Len())
			require.Empty(t, m.Plan().Items)
			if flushed {
				loaded, err := reopenSession(t, m)
				require.NoError(t, err)
				require.Equal(t, before, loaded.Len())
			} else {
				require.NoFileExists(t, m.File())
			}
		})
	}
}

func TestBusyOpenDoesNotRepair(t *testing.T) {
	for _, tail := range []string{
		`{"type":"EntryMessage","id":"torn`,
		`{"type":"EntryMessage","id":"m2","message":{"role":"assistant","content":"ok"}}`,
	} {
		t.Run(tail, func(t *testing.T) {
			path := writeSessionFixture(t, "")
			owner, err := OpenSession(path)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, owner.Close()) })
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
			require.NoError(t, err)
			_, err = f.WriteString(tail)
			require.NoError(t, err)
			require.NoError(t, f.Close())
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			info, err := os.Stat(path)
			require.NoError(t, err)
			_, err = OpenSession(path)
			require.ErrorIs(t, err, ErrBusy)
			list, err := ListSessions(filepath.Dir(path))
			require.NoError(t, err)
			require.Len(t, list, 1)
			require.True(t, list[0].Active)
			_, err = ReadSessionModel(path)
			require.NoError(t, err)
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			afterInfo, err := os.Stat(path)
			require.NoError(t, err)
			require.Equal(t, info.ModTime(), afterInfo.ModTime())
		})
	}
}

func TestFailedOpenReleasesOwnership(t *testing.T) {
	path := writeSessionFixture(t, "{bad}\n")
	for range 2 {
		_, err := OpenSession(path)
		require.Error(t, err)
		require.NotErrorIs(t, err, ErrBusy)
	}
	// Replace corrupt data, then prove a separate process can acquire the lock.
	require.NoError(t, os.WriteFile(path, []byte(`{"type":"EntrySession","id":"fixed"}`+"\n"), 0o600))
	p := startOwnerProcess(t, path)
	p.send(t, "open")
	require.Equal(t, "owned", p.line(t))
	p.send(t, "close")
	require.Equal(t, "closed", p.line(t))
	p.send(t, "exit")
	require.NoError(t, p.wait(t))

	missing := filepath.Join(t.TempDir(), "missing.jsonl")
	for range 2 {
		_, err := OpenSession(missing)
		require.ErrorIs(t, err, os.ErrNotExist)
	}
}

func TestListSessionsProbesWithoutCreatingLocks(t *testing.T) {
	path := writeSessionFixture(t, `{"torn`)
	dir := filepath.Dir(path)
	before, err := os.ReadFile(path)
	require.NoError(t, err)
	list, err := ListSessions(dir)
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.False(t, list[0].Active)
	require.NoDirExists(t, filepath.Join(dir, ".locks"))
	after, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, before, after)

	owner, err := OpenSession(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	for range 3 {
		list, err = ListSessions(dir)
		require.NoError(t, err)
		require.True(t, list[0].Active)
		_, err = OpenSession(path)
		require.ErrorIs(t, err, ErrBusy, "a probe must not release the existing owner")
	}
	require.NoError(t, owner.Close())
	list, err = ListSessions(dir)
	require.NoError(t, err)
	require.False(t, list[0].Active)
}

func TestOpenLatestSessionSkipsBusy(t *testing.T) {
	dir := t.TempDir()
	_, err := OpenLatestSession(filepath.Join(dir, "absent"))
	require.ErrorIs(t, err, os.ErrNotExist)
	_, err = OpenLatestSession(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
	var sessions []*Manager
	for i := range 3 {
		m, err := newTestSessionManager(t, dir, WithSessionDir(dir), WithShouldFlush(true))
		require.NoError(t, err)
		_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "persist"})
		require.NoError(t, err)
		stamp := time.Unix(int64(i+1), 0)
		require.NoError(t, os.Chtimes(m.File(), stamp, stamp))
		sessions = append(sessions, m)
	}
	_, err = OpenLatestSession(dir)
	require.ErrorIs(t, err, os.ErrNotExist, "all active")
	require.NoError(t, sessions[0].Close())
	require.NoError(t, sessions[1].Close())
	for _, index := range []int{1, 0} {
		m, err := OpenLatestSession(dir)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, m.Close()) })
		require.Equal(t, sessions[index].ID(), m.ID())
	}
	_, err = OpenLatestSession(dir)
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestReadSessionModelReadsOnlyHeader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "model.jsonl")
	content := "\n" + `{"type":"EntrySession","id":"model","model":"header-model"}` + "\n{broken}\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o400))
	model, err := ReadSessionModel(path)
	require.NoError(t, err)
	require.Equal(t, "header-model", model)
	require.NoDirExists(t, filepath.Join(filepath.Dir(path), ".locks"))
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, content, string(data))
	_, err = ReadSessionModel(filepath.Join(t.TempDir(), "missing"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCloseInMemorySession(t *testing.T) {
	m := NewManager(t.TempDir())
	require.NoError(t, m.Close())
	require.NoError(t, m.Close())
	_, err := m.Append(llm.Message{Role: llm.RoleUser, Content: "no"})
	require.ErrorIs(t, err, os.ErrClosed)
	_, err = m.ClearPlan()
	require.ErrorIs(t, err, os.ErrClosed)
}

func TestSessionOwnershipResolvesSymlinkAliases(t *testing.T) {
	path := writeSessionFixture(t, "")
	alias := filepath.Join(t.TempDir(), "alias.jsonl")
	if err := os.Symlink(path, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	m, err := OpenSession(alias)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, m.Close()) })
	_, err = OpenSession(path)
	require.ErrorIs(t, err, ErrBusy)
	m, err = reopenSession(t, m)
	require.NoError(t, err)
	_, err = OpenSession(alias)
	require.ErrorIs(t, err, ErrBusy)
}
