package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// loadedPool builds a pool the way the product does — through the loader,
// from real files — so what the observation says about provenance is what
// the merge actually did rather than what a fixture asserted.
func loadedPool(t *testing.T, global, project string, imported map[string]ServerConfig) *Pool {
	t.Helper()
	tempHome(t)
	t.Setenv(envDisable, "on")
	if global != "" {
		userPath, err := UserConfigPath()
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0o755))
		require.NoError(t, os.WriteFile(userPath, []byte(global), 0o600))
	}
	projectPath := filepath.Join(t.TempDir(), "mcp.json")
	if project != "" {
		require.NoError(t, os.WriteFile(projectPath, []byte(project), 0o600))
	}
	var sources []map[string]ServerConfig
	if imported != nil {
		sources = append(sources, imported)
	}
	pool, err := LoadPoolInDir(projectPath, t.TempDir(), sources...)
	require.NoError(t, err)
	require.NotNil(t, pool)
	return pool
}

// observed finds one server in an observation. A missing name is a failure
// rather than a zero value, so a test never asserts against a server the
// observation quietly left out.
func observed(t *testing.T, state diag.MCPState, name string) diag.MCPServerFacts {
	t.Helper()
	for _, facts := range state.Servers {
		if facts.Name == name {
			return facts
		}
	}
	t.Fatalf("server %q is missing from the observation", name)
	return diag.MCPServerFacts{}
}

func TestTheObservationNamesEverySourceThatDefinedAServerAndTheOneThatWon(t *testing.T) {
	pool := loadedPool(t,
		`{"servers":{"shared":{"command":["global"]},"only-global":{"command":["g"]}}}`,
		`{"servers":{"shared":{"command":["project"]},"only-project":{"command":["p"]}}}`,
		map[string]ServerConfig{
			"shared":        {Command: []string{"imported"}},
			"only-imported": {Command: []string{"i"}},
		})

	state := Observe(pool, ObserveLoad(nil))
	require.Len(t, state.Servers, 4)

	assert.Equal(t,
		[]diag.MCPOrigin{diag.MCPOriginImported, diag.MCPOriginGlobal, diag.MCPOriginProject},
		observed(t, state, "shared").Origins,
		"a name three sources define belongs to all three, in precedence order")
	assert.Equal(t, []diag.MCPOrigin{diag.MCPOriginImported}, observed(t, state, "only-imported").Origins)
	assert.Equal(t, []diag.MCPOrigin{diag.MCPOriginGlobal}, observed(t, state, "only-global").Origins)
	assert.Equal(t, []diag.MCPOrigin{diag.MCPOriginProject}, observed(t, state, "only-project").Origins)

	// The observation reports the precedence the loader applied; it does not
	// decide it. What the pool will actually run is the project's definition.
	servers := pool.servers
	assert.Equal(t, []string{"project"}, servers["shared"].Command,
		"the last source in precedence order is the one whose definition the pool holds")
}

func TestTheObservationTellsAServerNobodyCalledFromOneThatFailed(t *testing.T) {
	pool := NewPool(map[string]ServerConfig{
		"answering": {Command: []string{"true"}},
		"refusing":  {Command: []string{"true"}},
		"unasked":   {Command: []string{"true"}},
	})
	pool.clients["answering"] = statusClient{}
	pool.clients["refusing"] = statusClient{err: errors.New("connection refused")}

	before := Observe(pool, ObserveLoad(nil))
	for _, name := range []string{"answering", "refusing", "unasked"} {
		assert.Equal(t, diag.MCPServerNotConnected, observed(t, before, name).Connection,
			"before any call every server is one nobody has called")
	}

	_, err := pool.ListTools(t.Context(), "answering")
	require.NoError(t, err)
	_, err = pool.ListTools(t.Context(), "refusing")
	require.Error(t, err)

	after := Observe(pool, ObserveLoad(nil))
	assert.Equal(t, diag.MCPServerConnected, observed(t, after, "answering").Connection)
	assert.Equal(t, diag.MCPServerFailed, observed(t, after, "refusing").Connection)
	assert.Equal(t, diag.MCPServerNotConnected, observed(t, after, "unasked").Connection,
		"a server the session never called is not failed, and not connected either")
	assert.NotEqual(t, before.Revision, after.Revision,
		"a connection that opened is a different state, and the revision says so")
}

// countingClient records every way a client can be exercised, so the test
// can state that observing did none of them.
type countingClient struct {
	calls *int
}

func (c countingClient) Initialize(context.Context) error { *c.calls++; return nil }

func (c countingClient) ListTools(context.Context) ([]ToolDef, error) {
	*c.calls++
	return []ToolDef{{Name: "read"}}, nil
}

func (c countingClient) FindTool(context.Context, string) (*ToolDef, error) {
	*c.calls++
	return &ToolDef{Name: "read"}, nil
}

func (c countingClient) CallTool(context.Context, string, map[string]any) (string, error) {
	*c.calls++
	return "ok", nil
}
func (c countingClient) Close() error { *c.calls++; return nil }

func TestObservingThePoolConnectsToNothingAndAsksNobodyAnything(t *testing.T) {
	calls := 0
	pool := NewPool(map[string]ServerConfig{
		"live":     {Command: []string{"true"}},
		"unopened": {Command: []string{"true"}},
	})
	pool.clients["live"] = countingClient{calls: &calls}

	for range 3 {
		state := Observe(pool, ObserveLoad(nil))
		require.Len(t, state.Servers, 2)
	}

	assert.Zero(t, calls,
		"an observation may not initialize, list, inspect, call or close a client")
	assert.Len(t, pool.clients, 1,
		"a server with no client keeps none: reading the pool must not connect lazily")
	assert.Equal(t, StateConfigured, pool.status["live"].State,
		"observing leaves the states the pool recorded exactly where they stood")
}

func TestAPoolThatIsGoneIsSaidToBeGoneRatherThanReady(t *testing.T) {
	absent := Observe(nil, ObserveLoad(nil))
	assert.True(t, absent.Known, "a load was attempted, so the layer can answer")
	assert.True(t, absent.Enabled)
	assert.False(t, absent.Loaded)
	assert.Empty(t, absent.Servers)

	unwired := Observe(nil, LoadFacts{})
	assert.False(t, unwired.Known, "nobody loaded MCP, so nothing here is an observation")

	failed := Observe(nil, ObserveLoad(errors.New("parse mcp config /home/u/.cozyphi/mcp.json: bad json")))
	assert.True(t, failed.LoadFailed)
	assert.False(t, failed.Loaded)

	pool := loadedPool(t, `{"servers":{"alpha":{"command":["true"]}}}`, "", nil)
	open := Observe(pool, ObserveLoad(nil))
	assert.False(t, open.Closed)
	require.NoError(t, pool.Close())
	assert.True(t, Observe(pool, ObserveLoad(nil)).Closed,
		"a closed pool still lists what it was configured with, and says it is closed")
	assert.Len(t, Observe(pool, ObserveLoad(nil)).Servers, 1)
}

func TestTheEnvironmentSwitchIsRecordedWhereItIsRead(t *testing.T) {
	tempHome(t)
	t.Setenv(envDisable, "off")

	pool, err := LoadPool(filepath.Join(t.TempDir(), "mcp.json"))
	require.NoError(t, err)
	require.Nil(t, pool, "the switch is honored by the loader, not by the view")

	state := Observe(pool, ObserveLoad(err))
	assert.True(t, state.Known)
	assert.False(t, state.Enabled, "the observation reports the switch as it read at load")
	assert.False(t, state.Loaded)
	assert.False(t, state.LoadFailed)
}

func TestASwitchedOffServerSaysWhereTheChoiceCameFrom(t *testing.T) {
	pool := loadedPool(t,
		`{"servers":{"persisted":{"command":["true"]},"toggled":{"command":["true"]}},"disabled":["persisted"]}`,
		"", nil)

	state := Observe(pool, ObserveLoad(nil))
	persisted := observed(t, state, "persisted")
	assert.False(t, persisted.Enabled)
	assert.True(t, persisted.OffInConfig, "the file switched it off before the session began")
	assert.Equal(t, diag.MCPServerDisabled, persisted.Connection)

	require.NoError(t, pool.SetEnabled("toggled", false))
	toggled := observed(t, Observe(pool, ObserveLoad(nil)), "toggled")
	assert.False(t, toggled.Enabled)
	assert.False(t, toggled.OffInConfig,
		"a session toggle nobody has written down yet is not what the file says")
}

func TestADefinitionThatCouldNeverConnectIsKnownWithoutCallingAnything(t *testing.T) {
	pool := NewPool(map[string]ServerConfig{
		"nothing-to-run":   {},
		"nowhere-to-reach": {Transport: "http"},
		"unspoken":         {Transport: "carrier-pigeon", Command: []string{"true"}},
		"fine":             {Command: []string{"true"}},
		"reachable":        {Transport: "http", URL: "https://example.test/mcp"},
	})

	state := Observe(pool, ObserveLoad(nil))
	assert.False(t, observed(t, state, "nothing-to-run").Usable)
	assert.False(t, observed(t, state, "nowhere-to-reach").Usable)
	assert.False(t, observed(t, state, "unspoken").Usable)
	assert.True(t, observed(t, state, "fine").Usable)
	assert.True(t, observed(t, state, "reachable").Usable)

	for _, facts := range state.Servers {
		assert.Equal(t, diag.MCPServerNotConnected, facts.Connection,
			"deciding a definition is unusable must not count as having tried it")
	}
}

func TestNothingAServerConfigCarriesCrossesTheSeam(t *testing.T) {
	const sentinel = "SENTINEL"
	pool := NewPool(map[string]ServerConfig{
		"visible": {
			Command: []string{"/opt/" + sentinel + "-binary"},
			Args:    []string{"--token=" + sentinel + "-argument"},
			Env:     map[string]string{sentinel + "_KEY": sentinel + "-value"},
			Timeout: "300s",
		},
		"remote": {
			Transport: "http",
			URL:       "https://" + sentinel + ".example.test/mcp",
			Headers:   map[string]string{"Authorization": "Bearer " + sentinel},
		},
	})
	pool.clients["visible"] = statusClient{err: errors.New("spawn /opt/" + sentinel + "-binary: no such file")}
	_, err := pool.ListTools(t.Context(), "visible")
	require.Error(t, err)

	rendered, err := json.Marshal(Observe(pool, ObserveLoad(errors.New(sentinel+" in the load error"))))
	require.NoError(t, err)
	assert.NotContains(t, string(rendered), sentinel,
		"no command, argument, environment entry, header, URL or error text may reach the view")
	assert.Contains(t, string(rendered), "visible", "the name the user chose is the whole of what is named")
	assert.Contains(t, string(rendered), string(diag.MCPServerFailed),
		"that the call failed is reported; what it said is not")
	assert.NotContains(t, strings.ToLower(string(rendered)), "bearer")
}
