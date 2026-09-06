package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// mcpSentinel stands in for everything an MCP definition can carry that must
// never reach a transcript: the binary path, an argument, an environment
// entry, a URL and a header.
const mcpSentinel = "SENTINEL"

// writeHeadlessMCPConfig defines two servers the run will never call, one of
// them switched off, with a sentinel in every member that could leak.
func writeHeadlessMCPConfig(t *testing.T, path string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	config := `{
	  "servers": {
	    "notes": {
	      "command": ["/opt/` + mcpSentinel + `-binary"],
	      "args": ["--token=` + mcpSentinel + `"],
	      "env": {"` + mcpSentinel + `_KEY": "` + mcpSentinel + `"}
	    },
	    "remote": {
	      "transport": "http",
	      "url": "https://` + mcpSentinel + `.example.test/mcp",
	      "headers": {"Authorization": "Bearer ` + mcpSentinel + `"}
	    }
	  },
	  "disabled": ["remote"]
	}`
	require.NoError(t, os.WriteFile(path, []byte(config), 0o600))
}

func TestTheHeadlessRunReportsThePoolItLoadedAndNothingItIsBuiltFrom(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return text("done")
	})
	writeHeadlessMCPConfig(t, fixture.bs.Proj.MCPConfigFile())

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what do you talk to", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs, "the harness call must have produced a tool result")
	snapshot := strings.Join(outputs, "\n")

	assert.Contains(t, snapshot, `"category": "integrations"`)
	assert.Contains(t, snapshot, `"notes"`, "a server the user configured is named")
	assert.Contains(t, snapshot, `"remote"`)
	assert.Contains(t, snapshot, `"string": "`+string(diag.MCPReady)+`"`,
		"the run loaded a pool with servers in it")
	assert.NotContains(t, snapshot, mcpSentinel,
		"no command, argument, environment entry, URL or header may reach the transcript")
	assert.NotContains(t, snapshot, "Bearer")
	assert.NotContains(t, snapshot, "test-key", "and the provider credential never reaches an observation either")
}

// The headless run and the TUI observe the same pool through the same
// projection, so the category answers the same questions in both. The catalog
// is where that contract is declared, and it is declared without reading a
// pool at all.
func TestTheIntegrationCategoryIsTheSameContractInBothEntryPoints(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return text("done")
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you observe", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	catalog := outputs[0]

	assert.Contains(t, catalog, `"integrations"`)
	for _, key := range []string{
		diag.KeyMCPState, diag.KeyMCPWorkspace, diag.KeyMCPServers,
		diag.KeyMCPImported, diag.KeyMCPGlobal, diag.KeyMCPProject,
		diag.KeyMCPDisabled, diag.KeyMCPFailed,
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}
}

// A run with no MCP at all must answer the question rather than fail it: the
// category says the subsystem is off, and the rest of the harness is intact.
func TestAHeadlessRunWithNoPoolStillAnswersTheWholeHarness(t *testing.T) {
	t.Setenv("COZYPHI_MCP", "off")
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return text("done")
	})
	writeHeadlessMCPConfig(t, fixture.bs.Proj.MCPConfigFile())

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what do you talk to", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	snapshot := strings.Join(fixture.toolOutputs(), "\n")
	assert.Contains(t, snapshot, `"category": "integrations"`)
	assert.Contains(t, snapshot, `"string": "`+string(diag.MCPDisabled)+`"`,
		"the subsystem itself is off, which is a different answer from a server being off")
	assert.NotContains(t, snapshot, `"notes"`,
		"a configured file nobody loaded is not reported as servers this session has")
	assert.NotContains(t, snapshot, mcpSentinel)
}
