package diag_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveLSP is a manager with the two cases worth telling apart: a profile
// that is on this machine and running for two roots, and one this build
// carries that nobody has installed. The gap between them is what the three
// layers are for.
func liveLSP() diag.LSPState {
	return diag.LSPState{
		Known:     true,
		Enabled:   true,
		Opened:    true,
		Workspace: "/w/project",
		Servers: []diag.LSPServerFacts{
			{
				Name:       "gopls",
				Language:   "go",
				Installed:  diag.LSPInstallPresent,
				Roots:      []string{".", "tools"},
				Operations: []string{"definition", "hover", "diagnostics"},
			},
			{
				Name:       "rust-analyzer",
				Language:   "rust",
				Installed:  diag.LSPInstallMissing,
				Operations: []string{"definition", "expand_macro"},
			},
		},
		LastStart: diag.LSPStartSucceeded,
		Freshness: []string{"fresh", "cached", "unconfirmed", "pending"},
		Revision:  "s2.i1.r2.succeeded",
	}
}

func lspFields(t *testing.T, state diag.LSPState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{LSP: func() diag.LSPState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

// "No language server" is six different situations with one symptom, fixed
// in six different places — and two of them are not problems at all.
func TestTheLifecycleSeparatesTheWaysThereCanBeNoLanguageServer(t *testing.T) {
	tests := []struct {
		name  string
		state func(diag.LSPState) diag.LSPState
		want  diag.LSPLifecycle
	}{
		{
			name:  "switched off in the configuration",
			state: func(s diag.LSPState) diag.LSPState { s.Enabled = false; return s },
			want:  diag.LSPDisabled,
		},
		{
			name:  "nobody built a manager",
			state: func(s diag.LSPState) diag.LSPState { s.Opened = false; return s },
			want:  diag.LSPNotOpened,
		},
		{
			name: "building one was refused",
			state: func(s diag.LSPState) diag.LSPState {
				s.Opened, s.OpenFailed = false, true
				return s
			},
			want: diag.LSPOpenFailed,
		},
		{
			name:  "the manager was shut down",
			state: func(s diag.LSPState) diag.LSPState { s.Closed = true; return s },
			want:  diag.LSPClosed,
		},
		{
			name: "nothing this build carries is on the machine",
			state: func(s diag.LSPState) diag.LSPState {
				s.Servers = []diag.LSPServerFacts{{
					Name: "gopls", Language: "go", Installed: diag.LSPInstallMissing,
				}}
				return s
			},
			want: diag.LSPNotInstalled,
		},
		{
			name: "installed and nothing has needed it yet",
			state: func(s diag.LSPState) diag.LSPState {
				s.Servers = []diag.LSPServerFacts{{
					Name: "gopls", Language: "go", Installed: diag.LSPInstallPresent,
				}}
				return s
			},
			want: diag.LSPIdle,
		},
		{
			name:  "a server is running",
			state: func(s diag.LSPState) diag.LSPState { return s },
			want:  diag.LSPRunning,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := lspFields(t, test.state(liveLSP()))
			state := fieldByKey(t, fields, diag.KeyLSPState)
			assert.Equal(t, string(test.want), state.Effective.Value.Str,
				"the acting layer names the situation, not just its symptom")
			assert.Equal(t, diag.ApplyRestart, state.Apply,
				"a manager is built once per workspace")
		})
	}
}

// Being off is a decision someone made, and the field says whose. The
// configured layer is the setting; the loaded layer is what came of it.
func TestTheLifecycleNamesWhoSwitchedLSPOff(t *testing.T) {
	on := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPState)
	assert.Equal(t, string(diag.LSPEnabled), on.Configured.Value.Str)
	assert.Equal(t, diag.SourceDefault, on.Configured.Source.Kind,
		"nothing has to ask for LSP; it is on unless something switches it off")
	assert.Equal(t, string(diag.LSPOpened), on.Loaded.Value.Str)

	off := liveLSP()
	off.Enabled, off.Opened = false, false
	field := fieldByKey(t, lspFields(t, off), diag.KeyLSPState)
	assert.Equal(t, string(diag.LSPDisabled), field.Configured.Value.Str)
	assert.Equal(t, diag.SourceConfigFile, field.Configured.Source.Kind,
		"a subsystem that is off says which layer switched it off")
	assert.Equal(t, string(diag.LSPNotOpened), field.Loaded.Value.Str,
		"no manager was built, and that is the consequence rather than the reason")
}

// The three lists are the answer to three different questions that get asked
// as one: what can this build serve, what is on this machine, and what is
// actually running. The gaps between them are what a reader came for.
func TestTheLanguageAndServerListsShrinkFromConfiguredToRunning(t *testing.T) {
	fields := lspFields(t, liveLSP())

	languages := fieldByKey(t, fields, diag.KeyLSPLanguages)
	assert.Equal(t, []string{"go", "rust"}, languages.Configured.Value.List,
		"a language missing here is one cozyphi has no server for at all")
	assert.Equal(t, []string{"go"}, languages.Loaded.Value.List,
		"the rust server is not on this machine, so no install of it happened here")
	assert.Equal(t, []string{"go"}, languages.Effective.Value.List)

	servers := fieldByKey(t, fields, diag.KeyLSPServers)
	assert.Equal(t, []string{"gopls", "rust-analyzer"}, servers.Configured.Value.List)
	assert.Equal(t, []string{"gopls"}, servers.Loaded.Value.List)
	assert.Equal(t, []string{"gopls"}, servers.Effective.Value.List)

	// Installed and running are different answers, and a reader who cannot
	// tell them apart will go looking for a missing binary that is there.
	idle := liveLSP()
	idle.Servers[0].Roots = nil
	idle.Servers = append([]diag.LSPServerFacts(nil), idle.Servers...)
	quiet := fieldByKey(t, lspFields(t, idle), diag.KeyLSPServers)
	assert.Equal(t, []string{"gopls"}, quiet.Loaded.Value.List, "it is still installed")
	assert.Empty(t, quiet.Effective.Value.List, "it has simply not been needed yet")
}

// A source is told by what it names and by which layer it belongs to: the
// build fixes what exists, the machine decides what can run, and the session
// decides what does.
func TestEachLSPLayerNamesADifferentAuthority(t *testing.T) {
	servers := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPServers)

	assert.Equal(t, diag.SourceBuild, servers.Configured.Source.Kind)
	assert.Contains(t, servers.Configured.Source.Ref, "no server for",
		"the configured layer says an absent language is not a misconfiguration")

	assert.Equal(t, diag.SourceComputed, servers.Loaded.Source.Kind)
	assert.Contains(t, servers.Loaded.Source.Ref, "downloads nothing",
		"the lookup that answers this layer has no effect of its own")

	assert.Equal(t, diag.SourceSession, servers.Effective.Source.Kind)
	assert.Contains(t, servers.Effective.Source.Ref, "never by this",
		"reading what is running must not be what starts one")
}

// An operation exists because the build implements it, can run because the
// server is here, and is accepted because the running server advertised it —
// and only the first two are knowable without asking.
func TestAnOperationIsPromisedByTheBuildAndGatedByTheServer(t *testing.T) {
	operations := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPOperations)

	assert.Equal(t, []string{"definition", "hover", "diagnostics", "expand_macro"},
		operations.Configured.Value.List, "the build's whole set, each operation once")
	assert.Equal(t, diag.SourceBuild, operations.Configured.Source.Kind)

	assert.Equal(t, []string{"definition", "hover", "diagnostics"}, operations.Loaded.Value.List,
		"nothing can run for a server that is not on this machine")

	assert.Equal(t, diag.StateUnavailable, operations.Effective.State,
		"what a running server accepts is settled per call, against capabilities this view never asks for")
	assert.Contains(t, operations.Effective.Source.Ref, "asking is the one thing this view does not do")
	assert.Empty(t, operations.Effective.Value.List,
		"a refusal to guess must not look like a promise")
}

// Nothing configures a root and nothing loads one: the query picks it from
// the file it was asked about, so the only layer with an answer is the
// acting one.
func TestTheRootsAreOnlyKnownOnceAQueryHasChosenThem(t *testing.T) {
	roots := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPRoots)

	assert.Equal(t, diag.StateNotApplicable, roots.Configured.State)
	assert.Equal(t, diag.StateNotApplicable, roots.Loaded.State)
	assert.Contains(t, roots.Configured.Source.Ref, "go.work",
		"the not-applicable layers say how a root is chosen instead")

	assert.Equal(t, []string{".", "tools"}, roots.Effective.Value.List,
		"workspace-relative, one server per root")
	assert.Equal(t, diag.ApplyImmediate, roots.Apply,
		"the next query can add one without restarting anything")
}

// Why there is no server is worth more than the fact, and the manager's own
// record is the only thing that knows — as a category, never as its message.
func TestTheStartOutcomeIsACategoryAndNotAMessage(t *testing.T) {
	for _, outcome := range []diag.LSPStart{
		diag.LSPStartSucceeded,
		diag.LSPStartUnavailable,
		diag.LSPStartProtocol,
		diag.LSPStartClosed,
		diag.LSPStartRejected,
	} {
		state := liveLSP()
		state.LastStart = outcome
		field := fieldByKey(t, lspFields(t, state), diag.KeyLSPStart)
		assert.Equal(t, string(outcome), field.Effective.Value.Str)
		assert.Equal(t, diag.SourceSession, field.Effective.Source.Kind)
	}

	// A manager nobody has queried has recorded nothing, and that is an
	// answer of its own rather than a gap in the record.
	fresh := liveLSP()
	fresh.LastStart = ""
	field := fieldByKey(t, lspFields(t, fresh), diag.KeyLSPStart)
	assert.Equal(t, string(diag.LSPStartNotAttempted), field.Effective.Value.Str,
		"a server starts on the first query that needs one, and none has come")
	assert.Equal(t, diag.StateNotApplicable, field.Configured.State,
		"nothing configures whether a start succeeds")
	assert.Equal(t, diag.StateNotApplicable, field.Loaded.State)
}

// A diagnostic's freshness belongs to the query that fetched it. Repeating
// the last one as though it were current is the mistake this field exists to
// refuse, and it says so rather than going blank.
func TestFreshnessIsRefusedRatherThanRepeated(t *testing.T) {
	diagnostics := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPDiagnostics)

	assert.Equal(t, []string{"fresh", "cached", "unconfirmed", "pending"},
		diagnostics.Configured.Value.List, "the vocabulary comes from the owner, so the two cannot drift")
	assert.Equal(t, diag.SourceBuild, diagnostics.Configured.Source.Kind)

	assert.Equal(t, diag.StateNotApplicable, diagnostics.Loaded.State,
		"there is no standing freshness to have loaded")

	assert.Equal(t, diag.StateUnavailable, diagnostics.Effective.State)
	assert.Contains(t, diagnostics.Effective.Source.Ref, "a past answer is not repeated")
	assert.Empty(t, diagnostics.Effective.Value.List)
}

// The workspace is decided by the session that opened it. No configuration
// file has an opinion, so the configured layer does not exist rather than
// reporting a default that nothing would honor.
func TestTheLSPWorkspaceBelongsToTheSessionAndNotToAFile(t *testing.T) {
	workspace := fieldByKey(t, lspFields(t, liveLSP()), diag.KeyLSPWorkspace)

	assert.Equal(t, diag.StateNotApplicable, workspace.Configured.State)
	assert.Contains(t, workspace.Configured.Source.Ref, "no configuration file does")
	assert.Equal(t, "/w/project", workspace.Loaded.Value.Str)
	assert.Equal(t, "/w/project", workspace.Effective.Value.Str)
	assert.Equal(t, diag.SourceSession, workspace.Effective.Source.Kind)
	assert.Equal(t, diag.ScopeWorkspace, workspace.Scope)
}

// Switched off, never wired and never opened produce the same empty answer
// from a list, and mean three unrelated things. Every per-server field has
// to keep them apart, not just the lifecycle.
func TestSwitchedOffAndNeverOpenedAreNotAnEmptyServerList(t *testing.T) {
	perServer := []string{
		diag.KeyLSPWorkspace,
		diag.KeyLSPLanguages,
		diag.KeyLSPServers,
		diag.KeyLSPOperations,
		diag.KeyLSPRoots,
		diag.KeyLSPStart,
		diag.KeyLSPDiagnostics,
	}

	off := liveLSP()
	off.Enabled, off.Opened = false, false
	for _, key := range perServer {
		field := fieldByKey(t, lspFields(t, off), key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "switched LSP off", key)
	}

	// Off is reported ahead of the missing manager: switching LSP off is
	// what stops one from being built, so naming the consequence would hide
	// an answer that is known behind one that is not.
	unopened := liveLSP()
	unopened.Opened = false
	for _, key := range perServer {
		field := fieldByKey(t, lspFields(t, unopened), key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
	}

	unwired := lspFields(t, diag.LSPState{})
	for _, key := range perServer {
		field := fieldByKey(t, unwired, key)
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, key)
		assert.Empty(t, field.Revision, "an unobserved manager has no state to fingerprint")
	}
}

// The snapshot must not alias the manager's slices: a reader that sorts a
// list it was handed must not reorder what the owner holds.
func TestTheLSPSnapshotIsDetachedFromTheManagersState(t *testing.T) {
	state := liveLSP()
	fields := lspFields(t, state)

	fieldByKey(t, fields, diag.KeyLSPServers).Configured.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyLSPRoots).Effective.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyLSPOperations).Configured.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyLSPDiagnostics).Configured.Value.List[0] = "mutated"

	assert.Equal(t, "gopls", state.Servers[0].Name)
	assert.Equal(t, ".", state.Servers[0].Roots[0])
	assert.Equal(t, "definition", state.Servers[0].Operations[0])
	assert.Equal(t, "fresh", state.Freshness[0])
}

// The seam is an allowlist by construction. A configured command, an
// argument, an environment entry, an initialization option, a settings value
// or the text of a start error cannot be leaked by a view that has no member
// to put one in — so the shape of the struct is the guarantee, and this is
// the test that notices when someone widens it.
func TestNoLSPServerFactHasSomewhereForASecretToLand(t *testing.T) {
	allowed := map[string]string{
		"Name":       "string",
		"Language":   "string",
		"Installed":  "diag.LSPInstall",
		"Roots":      "[]string",
		"Operations": "[]string",
	}

	facts := reflect.TypeFor[diag.LSPServerFacts]()
	assert.Len(t, allowed, facts.NumField(),
		"a new member of this struct is a new way for a server's configuration to reach a transcript")
	for field := range facts.Fields() {
		want, ok := allowed[field.Name]
		require.True(t, ok, "undeclared member %q: state why it cannot carry a secret before adding it", field.Name)
		assert.Equal(t, want, field.Type.String(), field.Name)
	}
}
