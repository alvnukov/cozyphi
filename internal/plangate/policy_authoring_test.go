package plangate_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/plangate"
)

// The authoring_policy selector is closed: a config carrying anything else
// fails at load, not silently at prompt assembly.
func TestCompileRejectsUnknownAuthoringPolicy(t *testing.T) {
	defaults := plangate.DefaultDefaults()
	defaults.AuthoringPolicy = "helpful"

	_, err := plangate.Compile(defaults)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "authoring_policy")
}

func TestCompileAcceptsClosedAuthoringPolicyValues(t *testing.T) {
	for _, value := range []string{"", plangate.AuthoringAdaptiveMinimal, plangate.AuthoringLegacy} {
		defaults := plangate.DefaultDefaults()
		defaults.AuthoringPolicy = value
		_, err := plangate.Compile(defaults)
		require.NoError(t, err, "authoring_policy %q", value)
	}
}

// The selector rides the compiled policy and every detached copy, so the
// settings draft and the prompt projection read one value.
func TestAuthoringPolicySurvivesCompileAndCopy(t *testing.T) {
	defaults := plangate.DefaultDefaults()
	defaults.AuthoringPolicy = plangate.AuthoringLegacy

	policy, err := plangate.Compile(defaults)
	require.NoError(t, err)

	assert.Equal(t, plangate.AuthoringLegacy, policy.AuthoringPolicy())
	assert.Equal(t, plangate.AuthoringLegacy, policy.Defaults().AuthoringPolicy)
}
