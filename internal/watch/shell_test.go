package watch_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/watch"
)

// TestTheRealShellRunsWhereTheSessionIs is the one test that does not stub the
// shell. Everything else here proves the manager's logic; this proves the
// default wiring — the bash tool's shell, the session's working directory —
// which is the part a fake can never be wrong about.
func TestTheRealShellRunsWhereTheSessionIs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("here\n"), 0o600))

	mgr := watch.New(watch.Options{Cwd: func() string { return dir }})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	_, err := mgr.Start(watch.Spec{Label: "list the directory", Command: "ls", On: watch.OnExit})
	require.NoError(t, err)

	var report string
	for ev := range events {
		if ev.Final {
			break
		}
		report = ev.Text
	}
	assert.Contains(t, report, "succeeded", "the exit code comes back with the report")
	assert.Contains(t, report, "marker.txt", "and the command ran in the session's directory")
}

// TestTheRealShellReportsAFailingCommand pins the other half of on=exit: a
// non-zero exit is the event worth waking for, and it arrives with its output.
func TestTheRealShellReportsAFailingCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("needs a POSIX shell")
	}
	mgr := watch.New(watch.Options{Cwd: func() string { return t.TempDir() }})
	t.Cleanup(mgr.Close)
	events, cancel := mgr.Subscribe()
	t.Cleanup(cancel)

	_, err := mgr.Start(watch.Spec{
		Label:   "a build that fails",
		Command: "echo 'undefined reference to main' >&2; exit 3",
		On:      watch.OnExit,
	})
	require.NoError(t, err)

	var report string
	for ev := range events {
		if ev.Final {
			break
		}
		report = ev.Text
	}
	assert.Contains(t, report, "failed with exit 3")
	assert.Contains(t, report, "undefined reference to main", "stderr is part of what happened")
}
