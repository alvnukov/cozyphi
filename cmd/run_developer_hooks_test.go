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

// installHooks writes a hook into each directory the run consults, one name
// defined in both so precedence is observable. The scripts are real and would
// announce themselves; the marker is the proof that none of them ran, and the
// run path is the one thing about them that must not travel.
//
// Every hook here matches bash, which the run never calls. A post_tool hook
// matching every tool would legitimately fire after the harness call itself
// (asynchronously, so whether the marker lands before the assertion is a
// question of process spawn latency — it did on CI and did not on a Mac), and
// that would prove nothing about observation.
func installHooks(t *testing.T, fixture *developerFixture) string {
	t.Helper()
	marker := filepath.Join(t.TempDir(), "SENTINEL-hook-ran")
	write := func(dir, manifest string) {
		require.NoError(t, os.MkdirAll(dir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(manifest), 0o644))
		script := "#!/bin/sh\necho ran >> " + marker + "\nexit 0\n"
		require.NoError(t, os.WriteFile(filepath.Join(dir, "run.sh"), []byte(script), 0o700))
	}
	write(fixture.bs.Proj.Global().HooksDir(), `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","fail_closed":true},
	  {"name":"audit","event":"post_tool","match":"bash","run":"./run.sh","async":true}
	]}`)
	write(fixture.bs.Proj.HooksDir(), `{"hooks":[
	  {"name":"guard-bash","event":"pre_tool","match":"bash","run":"./run.sh","timeout":"12s"}
	]}`)
	return marker
}

func TestTheHeadlessRunReportsTheHooksItLoadedAndRunsNoneOfThem(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return doneText()
	})
	marker := installHooks(t, fixture)

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what stands in front of your tools", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs, "the harness call must have produced a tool result")
	snapshot := strings.Join(outputs, "\n")

	assert.Contains(t, snapshot, `"`+diag.KeyHooksState+`"`)
	assert.Contains(t, snapshot, `"string":"`+string(diag.HooksActive)+`"`,
		"hooks are loaded and nothing has triggered one")
	assert.Contains(t, snapshot, `"string":"`+string(diag.HookLoadClean)+`"`)
	assert.Contains(t, snapshot, `"guard-bash"`, "the name the manifest chose")
	assert.Contains(t, snapshot, `"audit"`)

	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "asking about a hook is not what runs one")
	assert.NotContains(t, snapshot, marker, "a hook's script can write anything, and its path stays with the owner")
	assert.NotContains(t, snapshot, "run.sh", "and so does the run path the load resolved")
	assert.NotContains(t, snapshot, "test-key", "the provider credential never reaches an observation either")
}

// Switched off and "nothing is configured" produce the same empty answer and
// mean two unrelated things, fixed in two different places.
func TestAHeadlessRunWithHooksSwitchedOffSaysSoRatherThanReportingNone(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"integrations"}`)
		}
		return doneText()
	})
	marker := installHooks(t, fixture)
	t.Setenv("COZYPHI_HOOKS", "off")

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what stands in front of your tools", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	snapshot := strings.Join(fixture.toolOutputs(), "\n")
	assert.Contains(t, snapshot, `"string":"`+string(diag.HooksDisabled)+`"`)
	assert.Contains(t, snapshot, "COZYPHI_HOOKS", "and the answer names what to change")
	assert.NotContains(t, snapshot, `"guard-bash"`, "no directory was read, so nothing was found to report")

	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err))
}

// The headless run and the TUI observe the same manager through the same
// projection, so the category declares the same keys in both — and declares
// them without loading a hook or reading a directory.
func TestTheIntegrationCatalogDeclaresTheHookKeysToo(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return doneText()
	})
	marker := installHooks(t, fixture)

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you observe", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	catalog := outputs[0]

	for _, key := range []string{
		diag.KeyHooksState, diag.KeyHooksRegistered, diag.KeyHooksEvents, diag.KeyHooksTools,
		diag.KeyHooksUser, diag.KeyHooksProject, diag.KeyHooksBlocking, diag.KeyHooksAsync,
		diag.KeyHooksTimeout, diag.KeyHooksLoad,
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}

	// A per-hook key is the tempting shape and deliberately not offered:
	// answering it would make listing the catalog read a manager.
	assert.NotContains(t, catalog, `"hooks.hook.guard-bash"`)

	_, err := os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "listing what can be asked for runs nothing at all")
}
