package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
)

// headerLine + one message line, the raw material for the torn-tail fixtures.
func writeSessionFixture(t *testing.T, tail string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "2026-01-01T00-00-00_torntail0000000.jsonl")
	content := `{"type":"EntrySession","id":"torntail0000000","timestamp":"t","cwd":"/tmp"}
{"type":"EntryMessage","id":"m1","message":{"role":"user","content":"hi"},"ts":"t"}
` + tail
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// A crash mid-append leaves one unterminated torn line at EOF: the session
// must load without it and the file must be trimmed so the next append never
// buries the torn line mid-file.
func TestOpenSessionDropsTornTail(t *testing.T) {
	path := writeSessionFixture(t, `{"type":"EntryMessage","id":"m2","mess`)
	want := `{"type":"EntrySession","id":"torntail0000000","timestamp":"t","cwd":"/tmp"}
{"type":"EntryMessage","id":"m1","message":{"role":"user","content":"hi"},"ts":"t"}
`

	m, err := OpenSession(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"user:hi"}, messageContents(m.BuildContext()))

	got, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, want, string(got), "torn tail should be trimmed from the file")

	// Appends continue cleanly on the trimmed file.
	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "ok"})
	require.NoError(t, err)
	reloaded, err := OpenSession(path)
	require.NoError(t, err)
	assert.Equal(t, messageContents(m.BuildContext()), messageContents(reloaded.BuildContext()))
}

// An undecodable line that carries its newline is corruption, not a crash
// signature: the load must fail closed.
func TestOpenSessionFailFastTerminatedBadLine(t *testing.T) {
	path := writeSessionFixture(t, "{not-json}\n")
	_, err := OpenSession(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "line 3")

	// A torn line buried mid-file (a good line follows it) fails too.
	path = writeSessionFixture(
		t,
		"{not-json}\n"+`{"type":"EntryMessage","id":"m3","message":{"role":"user","content":"x"},"ts":"t"}`,
	)
	_, err = OpenSession(path)
	require.Error(t, err)
}

// A final entry whose newline never landed still counts: keep the entry and
// restore the terminator so the next append starts on a fresh line.
func TestOpenSessionTerminatesUnterminatedFinalEntry(t *testing.T) {
	path := writeSessionFixture(
		t,
		`{"type":"EntryMessage","id":"m2","parentID":"m1","message":{"role":"assistant","content":"done"},"ts":"t"}`,
	)

	m, err := OpenSession(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"user:hi", "assistant:done"}, messageContents(m.BuildContext()))

	_, err = m.Append(llm.Message{Role: llm.RoleUser, Content: "next"})
	require.NoError(t, err)
	reloaded, err := OpenSession(path)
	require.NoError(t, err)
	assert.Equal(t, messageContents(m.BuildContext()), messageContents(reloaded.BuildContext()))
}

// A blank tail without a newline is junk from a crashed append: drop it.
func TestOpenSessionDropsUnterminatedBlankTail(t *testing.T) {
	path := writeSessionFixture(t, "   ")
	m, err := OpenSession(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"user:hi"}, messageContents(m.BuildContext()))
}

// The first full flush rewrites through temp+rename: a failed flush leaves the
// previous file intact and no temp litter behind.
func TestFlushAllEntriesLeavesNoTempAndKeepsPerms(t *testing.T) {
	dir := t.TempDir()
	m, err := NewSessionManager(dir, WithSessionDir(dir), WithShouldFlush(true))
	require.NoError(t, err)
	_, err = m.Append(llm.Message{Role: llm.RoleUser, Content: "one"})
	require.NoError(t, err)
	_, err = m.Append(llm.Message{Role: llm.RoleAssistant, Content: "two"})
	require.NoError(t, err)

	_, err = os.Stat(m.File())
	require.NoError(t, err)

	// Exactly one session file, no flush temps.
	files, err := filepath.Glob(filepath.Join(dir, "*"))
	require.NoError(t, err)
	assert.Len(t, files, 1)

	info, err := os.Stat(m.File())
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}
