package harnesssettings_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/harnesssettings"
	"github.com/alvnukov/cozyphi/internal/plangate"
)

// openManager opens a manager over a config written for the test.
func openManager(t *testing.T, body string) (*harnesssettings.Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	manager, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)
	return manager, path
}

// The exemptions key is the whole editable set, so "no exemptions at all" has
// to be a state the file can hold. Written as an omitted key it would read
// back as "not configured" on the next start and silently restore the shipped
// list — the user's unchecked boxes would check themselves again.
func TestManagerPersistsAnEmptyExemptionSet(t *testing.T) {
	manager, path := openManager(t, "plan:\n  defaults:\n    types:\n      - name: explore\n        tools: [read]\n")
	require.Equal(t, plangate.DefaultDefaults().Exemptions, manager.Snapshot().Plan.Exemptions,
		"a section with no exemptions key ships the defaults")

	draft := manager.Snapshot().Draft()
	draft.Plan.Exemptions = nil
	applied, err := manager.Apply(t.Context(), draft)
	require.NoError(t, err)
	assert.Empty(t, applied.Plan.Exemptions)

	written, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(written), "exemptions: []", "an empty set is written, not omitted")

	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)
	reopened, err := harnesssettings.Open(path, runtime, nil)
	require.NoError(t, err)
	assert.Empty(t, reopened.Snapshot().Plan.Exemptions, "an empty set survives a restart")
	assert.Equal(t, []string{"plan"}, runtime.Current().ExemptTools(), "the floor is all that is left")

	defaults, err := harnesssettings.LoadPlanDefaults(path)
	require.NoError(t, err)
	assert.Empty(t, defaults.Exemptions, "the process-start reader agrees")
}

// A merged or aliased plan.defaults decodes like any other mapping, so the
// presence check that guards the shipped defaults has to read it the same
// way. A literal key scan called the merged exemptions absent and replaced
// the user's list with the seven defaults.
func TestManagerReadsExemptionsThroughAliasesAndMergeKeys(t *testing.T) {
	const anchored = `shared: &shared
  types:
    - name: explore
      tools: [read]
  exemptions: [context]
`
	for name, body := range map[string]string{
		"alias":     anchored + "plan:\n  defaults: *shared\n",
		"merge key": anchored + "plan:\n  defaults:\n    <<: *shared\n",
	} {
		t.Run(name, func(t *testing.T) {
			manager, _ := openManager(t, body)
			assert.Equal(t, []string{"context"}, manager.Snapshot().Plan.Exemptions,
				"the configured list wins over the shipped defaults")
		})
	}
}
