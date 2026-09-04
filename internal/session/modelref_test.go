package session

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParseModelRef: the plan store and every picker share one
// "name:effort" reference convention. A suffix outside the reasoning
// ladder is not an effort — the whole string stays the model name,
// mirroring the legacy controller pick, so a typo never silently drops
// half a model reference.
func TestParseModelRef(t *testing.T) {
	cases := []struct {
		ref        string
		wantName   string
		wantEffort string
	}{
		{"openai/gpt-5.5", "openai/gpt-5.5", ""},
		{"openai/gpt-5.5:high", "openai/gpt-5.5", "high"},
		{"openai/gpt-5.5:turbo", "openai/gpt-5.5:turbo", ""},
		{"plan-b:medium", "plan-b", "medium"},
		{"glm-5.2:minimal", "glm-5.2", "minimal"},
		{"", "", ""},
	}
	for _, tc := range cases {
		name, effort := ParseModelRef(tc.ref)
		assert.Equal(t, tc.wantName, name, "ParseModelRef(%q) name", tc.ref)
		assert.Equal(t, tc.wantEffort, effort, "ParseModelRef(%q) effort", tc.ref)
	}
}

// TestFormatModelRef: formatting is the exact inverse of parsing, so a
// reference can round-trip through the plan store unchanged.
func TestFormatModelRef(t *testing.T) {
	assert.Equal(t, "openai/gpt-5.5", FormatModelRef("openai/gpt-5.5", ""))
	assert.Equal(t, "openai/gpt-5.5:high", FormatModelRef("openai/gpt-5.5", "high"))
}

// TestModelLabel: every info line renders a model the same way — the
// bare name, plus " · <effort>" only when one is selected.
func TestModelLabel(t *testing.T) {
	assert.Equal(t, "openai/gpt-5.5", ModelLabel("openai/gpt-5.5", ""))
	assert.Equal(t, "openai/gpt-5.5 · high", ModelLabel("openai/gpt-5.5", "high"))
	assert.Equal(t, "no model", ModelLabel("", ""))
}

// TestReplacePlanV2AcceptsEffortRefModels: a step or type model may name
// its effort through the shared reference convention; validation must
// not reject what every picker writes.
func TestReplacePlanV2AcceptsEffortRefModels(t *testing.T) {
	m := NewManager(t.TempDir())
	fixture := actionFixture()
	fixture.Items[0].Model = "plan-b:high"
	fixture.ModelsByType = map[StepType]string{StepEdit: "plan-b:high"}

	_, _, err := m.ReplacePlanV2(fixture, false)
	require.NoError(t, err, "an effort reference is a valid model reference")
}
