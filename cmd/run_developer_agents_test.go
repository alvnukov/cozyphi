package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// harnessAnswer runs a headless developer session that makes exactly one
// harness call and returns what the model was handed back.
func harnessAnswer(t *testing.T, arguments string) string {
	t.Helper()
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", arguments)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "look at yourself", maxRounds: 3, timeout: 20 * time.Second, developerMode: true,
	})
	require.Equal(t, ExitOK, exit)

	outputs := fixture.toolOutputs()
	require.Len(t, outputs, 1, "exactly one harness call, so exactly one answer")
	return outputs[0]
}

// A headless run builds no watch manager, so nothing there can start a watch
// and no event could reach a turn even if one did. Reporting that as an empty
// list would read as a session that simply has not started one — which is the
// opposite of the truth and would send someone looking for the watch that
// "stopped working".
func TestHeadlessReportsNoWatchManagerRatherThanAnEmptySession(t *testing.T) {
	fields := harnessFields(t, harnessAnswer(t, `{"action":"snapshot","category":"diagnostics"}`))

	state := fields["watches.state"]
	assert.Equal(t, "supported", state.Configured.Value.String, "the build carries watches wherever it runs")
	assert.Equal(t, "no_manager", state.Loaded.Value.String)
	assert.Equal(t, "no_manager", state.Effective.Value.String)
	assert.Contains(t, state.Loaded.Source.Ref, "headless run holds none")

	assert.NotEmpty(t, fields["watches.limits"].Configured.Value.List,
		"the budgets still hold wherever a manager would be built")
	assert.Equal(t, "not_applicable", fields["watches.limits"].Effective.State,
		"and there is no slot to be free of")

	for _, key := range []string{
		"watches.count", "watches.shapes", "watches.cadence", "watches.events", "watches.outcomes",
	} {
		assert.Equal(t, "not_applicable", fields[key].Effective.State, key)
		assert.Contains(t, fields[key].Effective.Source.Ref, "a headless run has none", key)
	}
}

// A headless run does spawn sub-agents, and the ceilings that refuse one are
// the run's own. What it has none of is a session identity to scope by, which
// is the legacy unscoped owner the agent tools themselves use there.
func TestHeadlessAnswersAboutSubAgentsFromTheRunsOwnManager(t *testing.T) {
	fields := harnessFields(t, harnessAnswer(t, `{"action":"snapshot","category":"agents"}`))

	assert.Equal(t, "enabled", fields["agents.state"].Configured.Value.String)
	assert.Equal(t, "managed", fields["agents.state"].Loaded.Value.String)
	assert.Equal(t, "idle", fields["agents.state"].Effective.Value.String,
		"a manager is in force and nothing is out")

	assert.Equal(t, []string{"explore", "worker", "review"}, fields["agents.roles"].Configured.Value.List)
	assert.Equal(t, "unset", fields["agents.roles"].Effective.State,
		"the run itself is nobody's sub-agent")

	assert.Equal(t, int64(1), fields["agents.depth"].Configured.Value.Int)
	assert.True(t, fields["agents.depth"].Effective.Value.Bool, "there is room below the top")
	assert.Positive(t, fields["agents.concurrency"].Configured.Value.Int)

	assert.Equal(t, []string{"explore=inherit", "worker=inherit", "review=inherit"},
		fields["agents.models"].Effective.Value.List,
		"nothing is pinned, so every role runs what the run itself runs")

	assert.False(t, fields["agents.developer"].Effective.Value.Bool,
		"the capability came from this run's command line and no spawn passes it on")
}

// A key is a stable address, so the catalog must name every one of them
// without observing anything: listing what can be asked reaches no manager
// and reads no watch.
func TestTheCatalogDeclaresEveryAgentAndWatchKey(t *testing.T) {
	answer := harnessAnswer(t, `{"action":"catalog"}`)

	var catalog struct {
		Categories []struct {
			Category string   `json:"category"`
			Keys     []string `json:"keys"`
		} `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(answer), &catalog))

	keys := make(map[string][]string, len(catalog.Categories))
	for _, entry := range catalog.Categories {
		keys[entry.Category] = entry.Keys
	}

	assert.Equal(t, []string{
		"agents.state", "agents.roles", "agents.models", "agents.depth",
		"agents.concurrency", "agents.assignments", "agents.developer",
	}, keys["agents"])
	assert.Equal(t, []string{
		"watches.state", "watches.limits", "watches.count", "watches.shapes",
		"watches.cadence", "watches.events", "watches.outcomes",
	}, keys["diagnostics"])
}

// The two categories this ticket opened answer with counts and vocabularies,
// and a headless run has the credential that would prove otherwise sitting
// right there in its own configuration.
func TestNeitherCategoryCarriesACredentialOrACommand(t *testing.T) {
	for _, category := range []string{"agents", "diagnostics"} {
		t.Run(category, func(t *testing.T) {
			answer := harnessAnswer(t, `{"action":"snapshot","category":"`+category+`"}`)
			assert.NotContains(t, answer, "test-key", "the provider credential never reaches an observation")
			assert.NotContains(t, strings.ToLower(answer), "authorization")
		})
	}
}
