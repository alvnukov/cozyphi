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

// installFakeGopls puts one executable named gopls on PATH, in a directory
// whose name is a sentinel. The lookup has something real to find, and the
// resolved path — the one thing about it that could leak — is recognizable
// anywhere it appears.
func installFakeGopls(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "SENTINEL-bin")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gopls"), []byte("#!/bin/sh\nexit 1\n"), 0o700))
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	return dir
}

func TestTheHeadlessRunReportsTheLanguageServerItCouldRunAndNeverStartsIt(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return text("done")
	})
	sentinel := installFakeGopls(t)

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you look up", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs, "the harness call must have produced a tool result")
	snapshot := strings.Join(outputs, "\n")

	assert.Contains(t, snapshot, `"`+diag.KeyLSPState+`"`)
	assert.Contains(t, snapshot, `"string": "`+string(diag.LSPIdle)+`"`,
		"the server is on this machine and no query has needed it yet")
	assert.Contains(t, snapshot, `"string": "`+string(diag.LSPStartNotAttempted)+`"`,
		"asking about the manager is not what starts one")
	assert.Contains(t, snapshot, `"gopls"`, "the build's own name for the profile")
	assert.NotContains(t, snapshot, sentinel,
		"the resolved executable is a path the owner chose, and it stays with the owner")
	assert.NotContains(t, snapshot, "test-key",
		"and the provider credential never reaches an observation either")
}

// A run on a machine with no gopls must answer the question rather than fail
// it: a missing binary and a broken subsystem are fixed in different places,
// and the category has to say which one this is.
func TestAHeadlessRunWithNoInstalledServerStillAnswersTheWholeHarness(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return text("done")
	})

	// The fixture's PATH is a temp directory holding only fd and rg, so the
	// lookup runs and finds nothing — which is the answer, not a failure.
	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you look up", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	snapshot := strings.Join(fixture.toolOutputs(), "\n")
	assert.Contains(t, snapshot, `"category": "integrations"`)
	assert.Contains(t, snapshot, `"string": "`+string(diag.LSPNotInstalled)+`"`)
	assert.Contains(t, snapshot, `"string": "`+string(diag.LSPEnabled)+`"`,
		"nothing switched it off; there is simply nothing on this machine to run")
}

// The headless run and the TUI observe the same manager through the same
// projection, so the category declares the same keys in both — and declares
// them without opening a manager or looking anything up.
func TestTheIntegrationCatalogDeclaresTheLanguageServerKeysToo(t *testing.T) {
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

	for _, key := range []string{
		diag.KeyLSPState, diag.KeyLSPWorkspace, diag.KeyLSPLanguages, diag.KeyLSPServers,
		diag.KeyLSPOperations, diag.KeyLSPRoots, diag.KeyLSPStart, diag.KeyLSPDiagnostics,
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}

	// A per-server key is the tempting shape and deliberately not offered:
	// answering it would make listing the catalog read a manager.
	assert.NotContains(t, catalog, `"lsp.server.gopls"`)
}
