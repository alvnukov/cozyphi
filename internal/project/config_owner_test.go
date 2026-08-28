package project

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
)

// The two in-process config.yaml writers — the settings manager (plan.defaults)
// and the allow-all dialog (permissions.dangerously_allow_all) — commit through
// one owner, so a save from one must never revert the other.
func TestConfigYamlWritersDoNotClobberEachOther(t *testing.T) {
	p := discoverInTempHome(t)
	path := p.Global().ConfigFile()
	writeTestConfigBody(t, p, `models:
  - name: m
    api_key: k
permissions:
  mode: ask
plan:
  defaults:
    types:
      - name: inspect
        tools: [read]
`)
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)

	draft := manager.Snapshot().Draft()
	draft.Plan.Types = []plangate.TypeDefaults{{Name: "review", Tools: []string{"read"}}}
	_, err = manager.Apply(t.Context(), draft)
	require.NoError(t, err)

	require.NoError(t, SetDangerouslyAllowAll(p.Global(), true))

	require.NoError(t, p.LoadConfig())
	assert.True(t, p.Config().Permissions.DangerouslyAllowAll, "the allow-all write survives the settings apply")
	assert.Equal(t, "ask", string(p.Config().Permissions.Mode))
	defaults, err := harnesssettings.LoadPlanDefaults(path)
	require.NoError(t, err)
	require.Len(t, defaults.Types, 1)
	assert.Equal(t, session.StepType("review"), defaults.Types[0].Name,
		"the applied plan defaults survive the allow-all write")
}
