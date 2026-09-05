package mcp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// tempHome isolates config reads/writes from the real ~/.cozyphi.
func tempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// trackingClient observes that a disable closes the live connection.
type trackingClient struct {
	statusClient
	closed *int
}

func (c trackingClient) Close() error {
	*c.closed++
	return c.statusClient.Close()
}

func TestSetDisabledRoundTripPreservesServers(t *testing.T) {
	home := tempHome(t)
	require.NoError(t, SaveUser(map[string]ServerConfig{
		"alpha": {Command: []string{"true"}},
		"beta":  {Command: []string{"true"}},
	}))

	// Duplicates collapse; the list lands sorted for a stable file.
	require.NoError(t, SetDisabled([]string{"beta", "beta", "alpha"}))

	var doc fileShape
	data, err := os.ReadFile(filepath.Join(home, ".cozyphi", "mcp.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Len(t, doc.Servers, 2, "servers must survive a disabled-list write")
	assert.Equal(t, []string{"alpha", "beta"}, doc.Disabled)

	// Server writes must not drop the disabled list.
	require.NoError(t, SaveUser(map[string]ServerConfig{
		"alpha": {Command: []string{"true"}},
		"beta":  {Command: []string{"true"}},
	}))
	require.NoError(t, AddServer("gamma", ServerConfig{Command: []string{"true"}}))
	removed, err := RemoveServer("gamma")
	require.NoError(t, err)
	require.True(t, removed)

	data, err = os.ReadFile(filepath.Join(home, ".cozyphi", "mcp.json"))
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, []string{"alpha", "beta"}, doc.Disabled)

	got, err := LoadDisabled(filepath.Join(t.TempDir(), "absent.json"))
	require.NoError(t, err)
	assert.True(t, got["alpha"])
	assert.True(t, got["beta"])
	assert.Len(t, got, 2)
}

func TestLoadDisabledUnionsUserAndProject(t *testing.T) {
	tempHome(t)
	userPath, err := UserConfigPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0o755))
	require.NoError(t, os.WriteFile(userPath, []byte(`{"disabled":["user-off"]}`), 0o600))

	projectPath := filepath.Join(t.TempDir(), "mcp.json")
	require.NoError(t, os.WriteFile(projectPath, []byte(`{"disabled":["proj-off"]}`), 0o600))

	got, err := LoadDisabled(projectPath)
	require.NoError(t, err)
	assert.True(t, got["user-off"])
	assert.True(t, got["proj-off"])
}

func TestPoolSetEnabledHidesServerFromModel(t *testing.T) {
	p := NewPool(map[string]ServerConfig{"alpha": {}, "beta": {}})
	closed := 0
	p.clients["beta"] = trackingClient{statusClient: statusClient{}, closed: &closed}

	require.NoError(t, p.SetEnabled("beta", false))

	assert.Equal(t, []string{"alpha"}, p.ServerNames(), "disabled server leaves the prompt catalog")
	assert.Equal(t, []string{"beta"}, p.DisabledNames())
	assert.Equal(t, 1, closed, "disabling closes the live client")

	statuses := p.ServerStatuses()
	require.Len(t, statuses, 2)
	assert.Equal(t, "alpha", statuses[0].Name)
	assert.Equal(t, StateConfigured, statuses[0].State)
	assert.Equal(t, "beta", statuses[1].Name)
	assert.Equal(t, StateDisabled, statuses[1].State)

	_, err := p.ListTools(t.Context(), "beta")
	require.ErrorContains(t, err, `"beta" is disabled`)

	require.ErrorContains(t, p.SetEnabled("ghost", false), `unknown mcp server "ghost"`)

	require.NoError(t, p.SetEnabled("beta", true))
	assert.Equal(t, []string{"alpha", "beta"}, p.ServerNames())
	assert.Empty(t, p.DisabledNames())
}

func TestLoadPoolStartsDisabledServersOff(t *testing.T) {
	tempHome(t)
	userPath, err := UserConfigPath()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(userPath), 0o755))
	require.NoError(t, os.WriteFile(userPath, []byte(`{
		"servers": {"alpha": {"command": ["true"]}, "beta": {"command": ["true"]}},
		"disabled": ["beta", "gone"]
	}`), 0o600))

	pool, err := LoadPool(filepath.Join(t.TempDir(), "absent.json"))
	require.NoError(t, err)
	require.NotNil(t, pool)

	assert.Equal(t, []string{"alpha"}, pool.ServerNames(), "stale and live disabled names apply at startup")
	assert.Equal(t, []string{"beta"}, pool.DisabledNames(), "unknown names are ignored, not kept")

	rows := pool.Doctor(t.Context())
	byName := map[string]DoctorResult{}
	for _, row := range rows {
		byName[row.Name] = row
	}
	assert.False(t, byName["beta"].OK)
	assert.Contains(t, byName["beta"].Detail, "disabled")
}
