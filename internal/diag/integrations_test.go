package diag_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// wholeCategory wires every service the category speaks for, which is the
// shape a real session has: one collector reading owners that know nothing
// about each other.
func wholeCategory() diag.Collector {
	return diag.NewIntegrationCollector(diag.IntegrationDeps{
		MCP:   liveMCP,
		LSP:   liveLSP,
		Hooks: liveHooks,
		Web:   liveWeb,
	})
}

func TestEveryDeclaredIntegrationKeyIsAnswered(t *testing.T) {
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), wholeCategory())
	snapshot, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	require.Len(t, snapshot.Categories, 1)
	fields := snapshot.Categories[0].Fields

	var entry diag.CatalogEntry
	for _, candidate := range registry.Catalog().Categories {
		if candidate.Category == diag.CategoryIntegrations {
			entry = candidate
		}
	}
	require.NotEmpty(t, entry.Keys, "the catalog lists what can be asked for")

	// Each service fingerprints its own observation: the category reads two
	// owners one after the other, so its fields are not one instant.
	revisions := map[string]string{
		"mcp.":   "s5.r3.c1",
		"lsp.":   "s2.i1.r2.succeeded",
		"hooks.": "d4.r3.p1.w1",
		"web.":   "ready.t1.a2.d1",
	}
	for _, key := range entry.Keys {
		field := fieldByKey(t, fields, key)
		if field.Effective.State == diag.StateUnavailable {
			// A layer this view refuses to guess at is still an answer, and
			// it says so; a blank one would be a wiring gap wearing the same
			// word.
			assert.NotEmpty(t, field.Effective.Source.Ref,
				"a key answered with unavailable says why it cannot be known: %s", key)
		}
		service, _, _ := strings.Cut(key, ".")
		assert.Equal(t, revisions[service+"."], field.Revision,
			"every field of one service describes that service's observation: %s", key)
	}
	assert.Len(t, fields, len(entry.Keys), "the answer carries exactly the declared keys")
	assert.False(t, snapshot.Truncated,
		"narrowing to one category is the escape hatch the note points at, so this category has to fit whole; "+
			"the response budget is what gives first when a fourth service is added here")
}

// Listing what can be asked for must not reach either owner. The catalog is
// the one call a reader makes before it knows what it wants, and it may not
// be the call that starts a server.
func TestListingTheCatalogNeverReachesTheOwner(t *testing.T) {
	pool, manager, hooks, web := 0, 0, 0, 0
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(),
		diag.NewIntegrationCollector(diag.IntegrationDeps{
			MCP:   func() diag.MCPState { pool++; return liveMCP() },
			LSP:   func() diag.LSPState { manager++; return liveLSP() },
			Hooks: func() diag.HooksState { hooks++; return liveHooks() },
			Web:   func() diag.WebState { web++; return liveWeb() },
		}))

	for range 3 {
		require.NotEmpty(t, registry.Catalog().Categories)
	}
	assert.Zero(t, pool, "the catalog is answered from the declared key set alone")
	assert.Zero(t, manager, "the catalog is answered from the declared key set alone")
	assert.Zero(t, hooks, "and listing what can be asked for is not what reads a hook manager either")
	assert.Zero(t, web, "and it is emphatically not what reads a web policy or the key it names")

	_, err := registry.Snapshot(t.Context(), diag.CategoryIntegrations)
	require.NoError(t, err)
	assert.Equal(t, 1, pool, "one snapshot reads the pool once, and every MCP field comes from that read")
	assert.Equal(t, 1, manager, "one snapshot reads the manager once, and every LSP field comes from that read")
	assert.Equal(t, 1, hooks, "one snapshot reads the hook manager once, and every hooks field comes from that read")
	assert.Equal(t, 1, web, "one snapshot reads the web layer once, and every web field comes from that read")
}

// A wiring gap is one honest category, never an invented pool or manager.
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
	registry := diag.NewRegistry(fixedClock(), diag.DefaultLimits(), wholeCategory())

	// A per-server key is the tempting shape and deliberately not offered:
	// answering it would make the catalog read the pool.
	_, err := registry.Explain(t.Context(), diag.CategoryIntegrations, "mcp.server.docs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyMCPServers, "the refusal names what can be asked for instead")

	// And the same shape on the other side of the category, for the same
	// reason: Status must answer from the key set alone, so a key that
	// exists only once a manager has been read cannot be declared.
	_, err = registry.Explain(t.Context(), diag.CategoryIntegrations, "lsp.server.gopls")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyLSPServers, "the refusal names what can be asked for instead")

	// A per-hook key is the same shape a third time, and refused for the same
	// reason: a key that exists only once a manager has been read cannot be
	// declared without making the catalog read one.
	_, err = registry.Explain(t.Context(), diag.CategoryIntegrations, "hooks.hook.guard-bash")
	require.Error(t, err)
	assert.Contains(t, err.Error(), diag.KeyHooksRegistered, "the refusal names what can be asked for instead")

	field, err := registry.Explain(t.Context(), diag.CategoryIntegrations, diag.KeyMCPServers)
	require.NoError(t, err)
	assert.Equal(t, []string{"docs"}, field.Field.Effective.Value.List)
}
