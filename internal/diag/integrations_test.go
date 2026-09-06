package diag_test

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// liveMCP is a pool with one of every case worth telling apart: a server two
// sources define, one that was called and failed, one the configuration
// switched off and that could never have connected anyway, one nobody has
// called, and one this session toggled off after all three sources named it.
func liveMCP() diag.MCPState {
	return diag.MCPState{
		Known:     true,
		Enabled:   true,
		Loaded:    true,
		Workspace: "/w/project",
		Servers: []diag.MCPServerFacts{
			{
				Name:       "docs",
				Origins:    []diag.MCPOrigin{diag.MCPOriginImported, diag.MCPOriginGlobal},
				Usable:     true,
				Enabled:    true,
				Connection: diag.MCPServerConnected,
			},
			{
				Name:       "index",
				Origins:    []diag.MCPOrigin{diag.MCPOriginGlobal},
				Usable:     true,
				Enabled:    true,
				Connection: diag.MCPServerFailed,
			},
			{
				Name:        "legacy",
				Origins:     []diag.MCPOrigin{diag.MCPOriginImported},
				Enabled:     false,
				OffInConfig: true,
				Connection:  diag.MCPServerDisabled,
			},
			{
				Name:       "search",
				Origins:    []diag.MCPOrigin{diag.MCPOriginProject},
				Usable:     true,
				Enabled:    true,
				Connection: diag.MCPServerNotConnected,
			},
			{
				Name: "shared",
				Origins: []diag.MCPOrigin{
					diag.MCPOriginImported,
					diag.MCPOriginGlobal,
					diag.MCPOriginProject,
				},
				Usable:     true,
				Enabled:    false,
				Connection: diag.MCPServerDisabled,
			},
		},
		Revision: "s5.r3.c1",
	}
}

func integrationFields(t *testing.T, state diag.MCPState) []diag.Field {
	t.Helper()
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{MCP: func() diag.MCPState { return state }}))
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	require.Equal(t, diag.AvailabilityAvailable, snapshot.Categories[0].Availability)
	require.False(t, snapshot.Truncated, "the category fits the response budget on its own")
	return snapshot.Categories[0].Fields
}

func TestEveryDeclaredIntegrationKeyIsAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{MCP: liveMCP}))
	fields := integrationFields(t, liveMCP())

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryIntegrations {
			entry = candidate
		}
	}
	require.NotEmpty(t, entry.Keys, "the catalog lists what can be asked for")

	for _, key := range entry.Keys {
		field := fieldByKey(t, fields, key)
		assert.NotEqual(t, diag.StateUnavailable, field.Effective.State,
			"a key the catalog advertises is answered by an owner that knows: %s", key)
		assert.Equal(t, "s5.r3.c1", field.Revision,
			"every field of one answer describes the same observation: %s", key)
	}
	assert.Len(t, fields, len(entry.Keys), "the answer carries exactly the declared keys")
}

// Listing what can be asked for must not reach the pool. The catalog is the
// one call a reader makes before it knows what it wants, and it may not be
// the call that starts a server.
func TestListingTheCatalogNeverReachesTheOwner(t *testing.T) {
	reads := 0
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{MCP: func() diag.MCPState {
			reads++
			return liveMCP()
		}}))

	for range 3 {
		require.NotEmpty(t, registry.Catalog().Categories)
	}
	assert.Zero(t, reads, "the catalog is answered from the declared key set alone")

	_, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	assert.Equal(t, 1, reads, "one snapshot reads the owner once, and every field comes from that read")
}

// "No MCP servers" is five different situations that call for five different
// fixes — and one of them is not a problem at all.
func TestTheLifecycleSeparatesTheWaysThereCanBeNoServers(t *testing.T) {
	closed := liveMCP()
	closed.Closed = true

	for _, tc := range []struct {
		name       string
		state      diag.MCPState
		configured diag.MCPLifecycle
		loaded     diag.MCPLifecycle
		effective  diag.MCPLifecycle
	}{
		{
			name:       "switched off in the environment",
			state:      diag.MCPState{Known: true},
			configured: diag.MCPDisabled, loaded: diag.MCPNotLoaded, effective: diag.MCPDisabled,
		},
		{
			name:       "on, but nothing loaded a pool",
			state:      diag.MCPState{Known: true, Enabled: true},
			configured: diag.MCPEnabled, loaded: diag.MCPNotLoaded, effective: diag.MCPNotLoaded,
		},
		{
			name:       "the configuration would not parse",
			state:      diag.MCPState{Known: true, Enabled: true, LoadFailed: true},
			configured: diag.MCPEnabled, loaded: diag.MCPLoadFailed, effective: diag.MCPLoadFailed,
		},
		{
			name:       "loaded, and no source named a server",
			state:      diag.MCPState{Known: true, Enabled: true, Loaded: true},
			configured: diag.MCPEnabled, loaded: diag.MCPLoaded, effective: diag.MCPNotConfigured,
		},
		{
			name:       "shut down",
			state:      closed,
			configured: diag.MCPEnabled, loaded: diag.MCPLoaded, effective: diag.MCPClosed,
		},
		{
			name:       "servers the model can ask for",
			state:      liveMCP(),
			configured: diag.MCPEnabled, loaded: diag.MCPLoaded, effective: diag.MCPReady,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := fieldByKey(t, integrationFields(t, tc.state), diag.KeyMCPState)
			assert.Equal(t, string(tc.configured), state.Configured.Value.Str)
			assert.Equal(t, string(tc.loaded), state.Loaded.Value.Str)
			assert.Equal(t, string(tc.effective), state.Effective.Value.Str)
			assert.Equal(t, diag.ApplyRestart, state.Apply,
				"nothing here changes without the subsystem being built again")
		})
	}
}

// Whether MCP is on is read from the environment or left at its default, and
// which of the two it was is the difference between "someone turned this off"
// and "nobody has ever touched it".
func TestTheLifecycleNamesWhoSwitchedMCPOff(t *testing.T) {
	on := fieldByKey(t, integrationFields(t, liveMCP()), diag.KeyMCPState)
	assert.Equal(t, diag.SourceDefault, on.Configured.Source.Kind)

	off := fieldByKey(t, integrationFields(t, diag.MCPState{Known: true}), diag.KeyMCPState)
	assert.Equal(t, diag.SourceEnv, off.Configured.Source.Kind)
	assert.Equal(t, "COZYPHI_MCP", off.Configured.Source.Ref)
}

// The three lists shrink in one direction, and every gap between them is an
// answer: configured but not reachable is switched off, reachable but not
// connected has simply not been called.
func TestTheThreeServerListsShrinkFromConfiguredToConnected(t *testing.T) {
	servers := fieldByKey(t, integrationFields(t, liveMCP()), diag.KeyMCPServers)

	assert.Equal(t, []string{"docs", "index", "legacy", "search", "shared"}, servers.Configured.Value.List)
	assert.Equal(t, []string{"docs", "index", "search"}, servers.Loaded.Value.List,
		"the model may reach every configured server that is not switched off")
	assert.Equal(t, []string{"docs"}, servers.Effective.Value.List,
		"only a call opens a connection, so only a server already called is connected")

	assert.Equal(t, diag.SourceConfigFile, servers.Configured.Source.Kind)
	assert.Equal(t, diag.SourceSession, servers.Loaded.Source.Kind)
	assert.Equal(t, diag.SourceSession, servers.Effective.Source.Kind)
	assert.Equal(t, diag.ScopeWorkspace, servers.Scope, "a pool belongs to the workspace that opened it")
}

// A name two sources define belongs to both. Which sources name it and which
// one supplied the definition in force are different facts, and the field
// says both rather than leaving the merge to be inferred.
func TestASourceIsToldByWhatItNamesAndByWhatItSupplied(t *testing.T) {
	fields := integrationFields(t, liveMCP())

	imported := fieldByKey(t, fields, diag.KeyMCPImported)
	assert.Equal(t, []string{"docs", "legacy", "shared"}, imported.Configured.Value.List,
		"an imported source names all three, whichever definition ends up running")
	assert.Equal(t, []string{"legacy"}, imported.Loaded.Value.List,
		"it only supplied the one no higher-precedence source redefined")

	global := fieldByKey(t, fields, diag.KeyMCPGlobal)
	assert.Equal(t, []string{"docs", "index", "shared"}, global.Configured.Value.List)
	assert.Equal(t, []string{"docs", "index"}, global.Loaded.Value.List)

	project := fieldByKey(t, fields, diag.KeyMCPProject)
	assert.Equal(t, []string{"search", "shared"}, project.Configured.Value.List)
	assert.Equal(t, []string{"search", "shared"}, project.Loaded.Value.List,
		"the highest-precedence source keeps every name it defines")

	for _, field := range []diag.Field{imported, global, project} {
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State,
			"where a definition came from is not something that acts: %s", field.Key)
		assert.NotEmpty(t, field.Effective.Source.Ref, "and the field says why: %s", field.Key)
	}

	// Every configured name is supplied by exactly one source: the lists
	// partition the pool rather than overlapping it.
	var supplied []string
	for _, field := range []diag.Field{imported, global, project} {
		supplied = append(supplied, field.Loaded.Value.List...)
	}
	assert.ElementsMatch(t, fieldByKey(t, fields, diag.KeyMCPServers).Configured.Value.List, supplied)
}

// A server switched off by the file and one switched off by /mcp read the
// same to the model and mean different things to whoever has to keep them
// that way — the second choice has not been written down yet.
func TestASwitchedOffServerSaysWhetherTheConfigurationAskedForIt(t *testing.T) {
	disabled := fieldByKey(t, integrationFields(t, liveMCP()), diag.KeyMCPDisabled)

	assert.Equal(t, []string{"legacy"}, disabled.Configured.Value.List,
		"the files started one server switched off")
	assert.Equal(t, []string{"legacy", "shared"}, disabled.Effective.Value.List,
		"this session switched off a second one")
	assert.Equal(t, disabled.Loaded.Value.List, disabled.Effective.Value.List,
		"there is no step between the toggle and its effect for the two layers to differ over")
	assert.Equal(t, diag.ApplyImmediate, disabled.Apply, "a toggle takes hold without a restart")
	assert.Equal(t, diag.SourceConfigFile, disabled.Configured.Source.Kind)
	assert.Equal(t, diag.SourceSession, disabled.Effective.Source.Kind)
}

// The two ways a server can be broken are fixed in different places, so they
// are reported on different layers: a definition that could never connect is
// a configuration fault, while a failed exchange happened at a call.
func TestABrokenDefinitionAndAFailedCallAreDifferentLayers(t *testing.T) {
	failed := fieldByKey(t, integrationFields(t, liveMCP()), diag.KeyMCPFailed)

	assert.Equal(t, []string{"legacy"}, failed.Configured.Value.List,
		"nothing to run, nothing to reach, or a transport this client does not speak")
	assert.Equal(t, diag.StateNotApplicable, failed.Loaded.State,
		"a failure is met by a call, not produced by a load")
	assert.Equal(t, []string{"index"}, failed.Effective.Value.List,
		"a server reachable enough to try and refused is a different problem")
	assert.NotContains(t, failed.Effective.Source.Ref, "index")
}

// Where a local server runs is decided by the workspace this session opened.
// No configuration file has a say, so that layer does not exist rather than
// reporting a default nobody set.
func TestTheWorkspaceLayerBelongsToTheSessionAndNotToAFile(t *testing.T) {
	bound := fieldByKey(t, integrationFields(t, liveMCP()), diag.KeyMCPWorkspace)
	assert.Equal(t, diag.StateNotApplicable, bound.Configured.State)
	assert.NotEmpty(t, bound.Configured.Source.Ref)
	assert.Equal(t, diag.StatePresent, bound.Effective.State)
	assert.Equal(t, "/w/project", bound.Effective.Value.Str)
	assert.Equal(t, bound.Loaded, bound.Effective)

	inherited := liveMCP()
	inherited.Workspace = ""
	loose := fieldByKey(t, integrationFields(t, inherited), diag.KeyMCPWorkspace)
	assert.Equal(t, diag.StateUnset, loose.Effective.State,
		"no directory was pinned, so a local process inherits cozyphi's own")
	assert.Equal(t, diag.SourceDefault, loose.Effective.Source.Kind)
}

// A subsystem switched off has no servers to describe; one that never loaded
// cannot say what it would have had. Both would otherwise render as an empty
// list, which reads as "you have no servers configured" and is a lie.
func TestSwitchedOffAndNeverLoadedAreNotAnEmptyServerList(t *testing.T) {
	perServer := []string{
		diag.KeyMCPWorkspace,
		diag.KeyMCPServers,
		diag.KeyMCPImported,
		diag.KeyMCPGlobal,
		diag.KeyMCPProject,
		diag.KeyMCPDisabled,
		diag.KeyMCPFailed,
	}

	off := integrationFields(t, diag.MCPState{Known: true})
	for _, key := range perServer {
		field := fieldByKey(t, off, key)
		assert.Equal(t, diag.StateNotApplicable, field.Effective.State, key)
		assert.Contains(t, field.Effective.Source.Ref, "COZYPHI_MCP", key)
		assert.Empty(t, field.Effective.Value.List, key)
	}

	broken := integrationFields(t, diag.MCPState{Known: true, Enabled: true, LoadFailed: true})
	for _, key := range perServer {
		assert.Equal(t, diag.StateUnavailable, fieldByKey(t, broken, key).Effective.State,
			"a configuration nobody could read defines no known servers, and none are guessed: %s", key)
	}
	assert.Equal(t, diag.StatePresent, fieldByKey(t, broken, diag.KeyMCPState).Effective.State,
		"the one thing still knowable is that the load failed, and it is still said")
}

// A wiring gap is one honest category, never an invented pool.
func TestAnIntegrationCollectorWithNoAccessorReportsUnavailable(t *testing.T) {
	unwired := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{}))
	snapshot, err := unwired.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)

	for _, field := range snapshot.Categories[0].Fields {
		assert.Equal(t, diag.StateUnavailable, field.Effective.State, field.Key)
		assert.Empty(t, field.Revision, "an unobserved owner has no state to fingerprint")
	}
}

func TestAnUnknownIntegrationKeyIsRefusedRatherThanAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{MCP: liveMCP}))

	// A per-server key is the tempting shape and deliberately not offered:
	// answering it would make the catalog read the pool.
	_, err := registry.Explain(t.Context(), diag.CategoryIntegrations, "mcp.server.docs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyMCPServers, "the refusal names what can be asked for instead")

	field, err := registry.Explain(t.Context(), diag.CategoryIntegrations, diag.KeyMCPServers)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, field.Field.Effective.Value.List)
}

// The snapshot must not alias the owner's slices: a reader that sorts a list
// it was handed must not reorder the pool behind it.
func TestTheIntegrationSnapshotIsDetachedFromTheOwnersState(t *testing.T) {
	state := liveMCP()
	fields := integrationFields(t, state)

	fieldByKey(t, fields, diag.KeyMCPServers).Configured.Value.List[0] = "mutated"
	fieldByKey(t, fields, diag.KeyMCPImported).Configured.Value.List[0] = "mutated"

	assert.Equal(t, "docs", state.Servers[0].Name)
	assert.Equal(t, diag.MCPOriginImported, state.Servers[0].Origins[0])
}

// The seam is an allowlist by construction. A command, an argument, an
// environment entry, a header, a URL, an error message or a tool schema
// cannot be leaked by a view that has no member to put one in — so the shape
// of the struct is the guarantee, and this is the test that notices when
// someone widens it.
func TestNoMCPServerFactHasSomewhereForASecretToLand(t *testing.T) {
	allowed := map[string]string{
		"Name":        "string",
		"Origins":     "[]diag.MCPOrigin",
		"Usable":      "bool",
		"Enabled":     "bool",
		"OffInConfig": "bool",
		"Connection":  "diag.MCPConnection",
	}

	facts := reflect.TypeFor[diag.MCPServerFacts]()
	assert.Len(t, allowed, facts.NumField(),
		"a new member of this struct is a new way for a server's configuration to reach a transcript")
	for field := range facts.Fields() {
		want, ok := allowed[field.Name]
		require.True(t, ok, "undeclared member %q: state why it cannot carry a secret before adding it", field.Name)
		assert.Equal(t, want, field.Type.String(), field.Name)
	}
}
