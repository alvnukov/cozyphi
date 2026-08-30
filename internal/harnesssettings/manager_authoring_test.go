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

// Config load is where a bogus selector dies: the settings file is the only
// free-form surface, and it must never smuggle prompt text into the policy.
func TestOpenRejectsUnknownAuthoringPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(
		"plan:\n  defaults:\n    authoring_policy: chatty\n    types:\n      - name: inspect\n        tools: [read]\n",
	), 0o600))

	runtime, err := plangate.NewRuntime(plangate.DefaultDefaults())
	require.NoError(t, err)

	_, err = harnesssettings.Open(path, runtime, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authoring_policy")
}

// The draft validates on write: a bad value stops in the pane, never at
// Apply, and leaves the draft untouched.
func TestDraftSetAuthoringPolicyValidates(t *testing.T) {
	draft := &harnesssettings.Draft{Plan: plangate.DefaultDefaults()}

	require.NoError(t, draft.SetAuthoringPolicy(plangate.AuthoringLegacy))
	assert.Equal(t, plangate.AuthoringLegacy, draft.Plan.AuthoringPolicy)

	require.Error(t, draft.SetAuthoringPolicy("chatty"))
	assert.Equal(t, plangate.AuthoringLegacy, draft.Plan.AuthoringPolicy,
		"a refused value must not leak into the draft")
}
