package lsp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// The whole point of this view is that asking is free. A manager nobody has
// queried must stay a manager nobody has queried: no process is spawned, no
// message reaches a server, and the answer says so rather than inventing a
// server to describe.
func TestObservingAManagerStartsNothingAndAsksNothing(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	hist := filepath.Join(t.TempDir(), "history")
	config := fakeConfig("LSP_TEST_HISTORY="+hist, "LSP_TEST_DEF_RESULT="+defFixture(uriFromPath(otherFile)))
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	open := ObserveOpen(config, err)
	for range 3 {
		state := Observe(mgr, open)
		assert.True(t, state.Known)
		assert.True(t, state.Enabled)
		assert.True(t, state.Opened)
		assert.False(t, state.Closed)
		assert.Equal(t, dir, state.Workspace)
		require.Len(t, state.Servers, 1)
		assert.Equal(t, "gopls", state.Servers[0].Name)
		assert.Equal(t, "go", state.Servers[0].Language)
		assert.Empty(t, state.Servers[0].Roots, "no query has needed a server yet")
		assert.Equal(t, diag.LSPStartNotAttempted, state.LastStart)
	}
	assert.Empty(t, history(t, hist), "no server was started, so nothing recorded a method")

	// Once a real query has started one, the observation reports it — and
	// still puts nothing on the wire of its own.
	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)
	before := history(t, hist)
	require.NotEmpty(t, before, "the query itself did talk to the server")

	state := Observe(mgr, open)
	assert.Equal(t, []string{"."}, state.Servers[0].Roots, "workspace-relative, one server per root")
	assert.Equal(t, diag.LSPStartSucceeded, state.LastStart)
	assert.Equal(t, before, history(t, hist),
		"observing after a query must not synchronize a file, dispatch a query or ask for diagnostics")
}

// A nil manager means two unrelated things, and only the caller of Open knows
// which: LSP was switched off, or building one was refused. Reporting both as
// "there is no manager" would hide the one answer that is actionable.
func TestOpenFactsSeparateSwitchedOffFromRefused(t *testing.T) {
	off := Config{Enabled: false}
	mgr, err := Open(t.Context(), t.TempDir(), off)
	require.NoError(t, err)
	require.Nil(t, mgr, "a disabled configuration builds no manager")

	state := Observe(mgr, ObserveOpen(off, err))
	assert.True(t, state.Known)
	assert.False(t, state.Enabled, "the configuration switched it off, and that is the answer")
	assert.False(t, state.OpenFailed)
	assert.False(t, state.Opened)

	missing := filepath.Join(t.TempDir(), "nowhere")
	mgr, err = Open(t.Context(), missing, DefaultConfig())
	require.Error(t, err)
	require.Nil(t, mgr)

	state = Observe(mgr, ObserveOpen(DefaultConfig(), err))
	assert.True(t, state.Enabled, "nothing switched it off; opening it failed")
	assert.True(t, state.OpenFailed)
	assert.False(t, state.Opened)

	// The rejection names the path it could not stat, and none of that text
	// is carried across the seam.
	require.Contains(t, err.Error(), "nowhere")
	assert.NotContains(t, marshal(t, state), "nowhere")

	// A caller that never opened LSP at all is neither of those: the layer is
	// unwired, and the view refuses to report it as "no server is configured".
	unwired := Observe(nil, OpenFacts{})
	assert.False(t, unwired.Known)
	assert.Empty(t, unwired.Servers)
}

// A shut-down manager is not an idle one: no query will start a server again
// without a new manager, and the answer has to say which of the two it is.
func TestObservingAClosedManagerSaysItIsClosed(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	config := fakeConfig("LSP_TEST_DEF_RESULT=" + defFixture(uriFromPath(otherFile)))
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)

	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)
	require.NoError(t, mgr.Close(t.Context()))

	state := Observe(mgr, ObserveOpen(config, nil))
	assert.True(t, state.Closed)
	assert.True(t, state.Opened, "it did exist, and that is why there is something to report")
	assert.Empty(t, state.Servers[0].Roots, "a shut-down client is not a live one")
	assert.Equal(t, diag.LSPStartSucceeded, state.LastStart,
		"the last attempt still succeeded; what became of the manager afterwards is a separate answer")
}

// Installed is answered by the owner's own lookup, which reads the filesystem
// and does nothing else: it never executes the candidate, never downloads one
// and never installs one.
func TestTheInstallLookupRunsNothingAndDownloadsNothing(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "ran")
	server := filepath.Join(dir, "gopls-lookalike")
	script := "#!/bin/sh\ntouch " + marker + "\n"
	require.NoError(t, os.WriteFile(server, []byte(script), 0o700))

	assert.Equal(t, diag.LSPInstallPresent, observedInstall([]string{server}))
	assert.NoFileExists(t, marker, "resolving a command must not be what runs it")

	assert.Equal(t, diag.LSPInstallMissing, observedInstall([]string{filepath.Join(dir, "absent")}))

	// No command is not a missing binary: there is nothing to look for, and
	// this view does not read the configuration file to find one — that file
	// has trust rules of its own and a second reader would skip them.
	assert.Equal(t, diag.LSPInstallUnknown, observedInstall(nil))
}

// Why there is no server is the answer worth having, and the manager records
// it as a category so the message — which can name the resolved executable or
// carry the server's own words — never has to leave.
func TestTheStartRecordCrossesAsACategoryAndNotAsAMessage(t *testing.T) {
	tests := []struct {
		err  error
		want diag.LSPStart
	}{
		{nil, diag.LSPStartSucceeded},
		{newError(ErrUnavailable, "gopls not found at /home/secret/bin/gopls"), diag.LSPStartUnavailable},
		{newError(ErrProtocol, "initialize failed: token=%s", "s3cr3t"), diag.LSPStartProtocol},
		{newError(ErrClosed, "manager is closed"), diag.LSPStartClosed},
		{newError(ErrInvalid, "bad root"), diag.LSPStartRejected},
		{newError(ErrAmbiguous, "two roots"), diag.LSPStartRejected},
		{newError(ErrUnsupported, "not implemented"), diag.LSPStartRejected},
		{os.ErrPermission, diag.LSPStartProtocol},
	}

	dir, _, _ := setupWorkspace(t)
	for _, test := range tests {
		mgr, err := Open(t.Context(), dir, DefaultConfig())
		require.NoError(t, err)
		mgr.mu.Lock()
		mgr.recordStart(test.err)
		mgr.mu.Unlock()

		state := Observe(mgr, ObserveOpen(DefaultConfig(), nil))
		assert.Equal(t, test.want, state.LastStart)
		if test.err != nil {
			// The owner still keeps the message for the languages operation
			// it renders itself; what crosses this seam is the category.
			assert.NotEmpty(t, mgr.lastStartErr)
			assert.NotContains(t, marshal(t, state), "secret")
			assert.NotContains(t, marshal(t, state), "s3cr3t")
		}
		require.NoError(t, mgr.Close(t.Context()))
	}
}

// The seam carries no configuration. A command that is an absolute path, an
// environment entry, an initialization option and a settings value are all
// things a configuration file can put secrets in, and none of them has
// anywhere to land on the far side.
func TestNoConfigurationCrossesTheObservationSeam(t *testing.T) {
	dir, _, _ := setupWorkspace(t)
	secret := "s3cr3t-token"
	config := Config{
		Enabled: true,
		Gopls: GoplsConfig{
			Command:               []string{filepath.Join(t.TempDir(), "opt", secret, "gopls")},
			Env:                   []string{"GOPLS_TOKEN=" + secret},
			InitializationOptions: map[string]any{"apiKey": secret},
			Settings:              map[string]any{"gopls": map[string]any{"token": secret}},
		},
	}
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	encoded := marshal(t, Observe(mgr, ObserveOpen(config, nil)))
	assert.NotContains(t, encoded, secret,
		"nothing the configuration carries may reach a transcript through this view")
	assert.NotContains(t, encoded, config.Gopls.Command[0],
		"not the configured command, and not the directory it was found in")
	assert.Contains(t, encoded, `"Name":"gopls"`,
		"the build's own name for the profile is what is reported instead")
}

// The manager never selects a root outside its workspace, and a path that
// would still come out as an escape is dropped rather than reported: this
// view carries no path the workspace does not contain.
func TestRootsAreReportedRelativeToTheWorkspaceOrNotAtAll(t *testing.T) {
	roots := relativeRoots("/w/project", []string{
		"/w/project",
		"/w/project/tools",
		"/w/project/a/b",
		"/w/elsewhere",
		"/etc",
	})
	assert.Equal(t, []string{".", "tools", filepath.Join("a", "b")}, roots)
}

// The vocabulary a diagnostics answer draws from is reported from the owner's
// own constants, so a new provenance cannot appear in one place and not the
// other.
func TestTheFreshnessVocabularyIsTheOwnersOwn(t *testing.T) {
	assert.Equal(t,
		[]string{StatusFresh, StatusCached, StatusUnconfirmed, StatusPending},
		diagnosticProvenance())
}

// The snapshot must be detached: a reader that sorts the operations it was
// handed must not reorder the build's own frozen set.
func TestTheObservationIsDetachedFromThePackagesState(t *testing.T) {
	dir, _, _ := setupWorkspace(t)
	config := DefaultConfig()
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close(t.Context()) })

	state := Observe(mgr, ObserveOpen(config, nil))
	require.Len(t, state.Servers, 1)
	require.NotEmpty(t, state.Servers[0].Operations)
	state.Servers[0].Operations[0] = "mutated"

	assert.Equal(t, "definition", knownOperations[0])
}

// The revision exists so two snapshots taken across a first query or a
// shutdown are visibly of two different states, and it counts nothing the
// manager does not already know.
func TestTheRevisionChangesWhenTheStateDoes(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	config := fakeConfig("LSP_TEST_DEF_RESULT=" + defFixture(uriFromPath(otherFile)))
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)
	open := ObserveOpen(config, nil)

	idle := Observe(mgr, open).Revision
	assert.Equal(t, idle, Observe(mgr, open).Revision, "nothing changed, so neither does the answer")

	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)
	running := Observe(mgr, open).Revision
	assert.NotEqual(t, idle, running, "a first query started a server, and the answer says so")

	require.NoError(t, mgr.Close(t.Context()))
	assert.NotEqual(t, running, Observe(mgr, open).Revision, "so does a shutdown")
}

// Observation is a read that runs concurrently with real work, including the
// shutdown that tears the clients down underneath it. Run with -race.
func TestObservingIsSafeWhileTheManagerIsClosing(t *testing.T) {
	dir, mainFile, otherFile := setupWorkspace(t)
	config := fakeConfig("LSP_TEST_DEF_RESULT=" + defFixture(uriFromPath(otherFile)))
	mgr, err := Open(t.Context(), dir, config)
	require.NoError(t, err)
	open := ObserveOpen(config, nil)

	_, err = mgr.Query(t.Context(), Query{Op: OpDefinition, File: mainFile, Line: 5, Character: 2})
	require.NoError(t, err)

	stop := make(chan struct{})
	var (
		wg    sync.WaitGroup
		reads atomic.Int64
	)
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				reads.Add(int64(len(Observe(mgr, open).Servers)))
			}
		})
	}
	wg.Go(func() {
		_, _ = mgr.Query(t.Context(), Query{Op: OpLanguages})
	})

	require.NoError(t, mgr.Close(t.Context()))
	close(stop)
	wg.Wait()

	assert.Positive(t, reads.Load(), "the readers did run alongside the shutdown")
	assert.True(t, Observe(mgr, open).Closed)
}

// marshal renders an observation the way the harness would hand it on, which
// is the shape a secret would have to survive to reach a transcript.
func marshal(t *testing.T, state diag.LSPState) string {
	t.Helper()
	raw, err := json.Marshal(state)
	require.NoError(t, err)
	return string(raw)
}
