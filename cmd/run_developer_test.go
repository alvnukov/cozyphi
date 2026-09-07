package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
)

func TestDeveloperModeComesFromTheCommandLine(t *testing.T) {
	opts, err := parseRunArgs([]string{"-p", "x", "--developer-mode"})
	require.NoError(t, err)
	assert.True(t, opts.developerMode)

	opts, err = parseRunArgs([]string{"-p", "x"})
	require.NoError(t, err)
	assert.False(t, opts.developerMode)
}

func TestDeveloperModeIsNotSpelledLoosely(t *testing.T) {
	// The flag is a bare switch like --yolo. Anything else is an unknown
	// argument, so a typo fails the run instead of quietly not granting
	// what the user asked for.
	for _, args := range [][]string{
		{"--developer-mode=true"},
		{"--developer_mode"},
		{"--developer"},
		{"--developer-modes"},
		{"-developer-mode"},
	} {
		_, err := parseRunArgs(args)
		assert.Error(t, err, "args %v", args)
	}
}

func TestDeveloperModeIgnoresTheEnvironment(t *testing.T) {
	for _, name := range []string{
		"COZYPHI_DEVELOPER_MODE", "COZYPHI_DEVELOPER", "COZYPHI_DEBUG", "DEVELOPER_MODE",
	} {
		t.Setenv(name, "1")
	}

	opts, err := parseRunArgs([]string{"-p", "x"})
	require.NoError(t, err)
	assert.False(t, opts.developerMode, "no environment variable grants the capability")
}

func TestRunUsageAnnouncesDeveloperMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "usage.txt")
	file, err := os.Create(path)
	require.NoError(t, err)
	printRunUsage(file)
	require.NoError(t, file.Close())

	usage, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(usage), "--developer-mode")
	assert.Contains(t, string(usage), "read-only")
}

// developerFixture stands up a project with a local fake provider and returns
// the bootstrap plus a recorder of what each request offered the model.
type developerFixture struct {
	bs    *runBootstrap
	mu    sync.Mutex
	tools []string
	calls []string
}

// newDeveloperFixture stands up one headless developer-mode run. extraConfig
// is appended to the configuration the run is loaded with, for the tests
// whose subject is a setting rather than a default.
func newDeveloperFixture(
	t *testing.T,
	reply func(round int) map[string]any,
	extraConfig ...string,
) *developerFixture {
	t.Helper()
	_, pathDir := testProject(t)
	t.Setenv("COZYPHI_MODEL", "")
	t.Setenv("COZYPHI_API_KEY", "")
	for _, name := range []string{"fd", "rg"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		require.NoError(t, os.WriteFile(filepath.Join(pathDir, name), []byte("x"), 0o755))
	}
	p, err := project.Discover(t.TempDir())
	require.NoError(t, err)

	fixture := &developerFixture{}
	round := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Tools []struct {
				Function struct{ Name string } `json:"function"`
			} `json:"tools"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); !assert.NoError(t, err) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fixture.mu.Lock()
		fixture.tools = nil
		for _, tool := range request.Tools {
			fixture.tools = append(fixture.tools, tool.Function.Name)
		}
		for _, message := range request.Messages {
			if message.Role == "tool" {
				fixture.calls = append(fixture.calls, message.Content)
			}
		}
		fixture.mu.Unlock()
		round++
		w.Header().Set("Content-Type", "text/event-stream")
		payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": reply(round)}}})
		assert.NoError(t, err)
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
	}))
	t.Cleanup(server.Close)

	cache := fmt.Sprintf(`{"version":1,"providers":[{"id":"fixture","name":"Fixture",
		"base_url":%q,"protocol":"openai","models":[{"id":"only","name":"Only"}]}]}`, server.URL)
	credentials := fmt.Sprintf(`{"version":1,"providers":{"fixture":{
		"type":"api","key":"test-key","base_url":%q,"protocol":"openai"}}}`, server.URL)
	require.NoError(t, os.WriteFile(p.Global().ProviderCatalogFile(), []byte(cache), 0o600))
	require.NoError(t, os.WriteFile(p.Global().CredentialsFile(), []byte(credentials), 0o600))

	// A config that asks for developer mode in every spelling anyone might
	// guess. The flag is the only grant, so all of this must be inert.
	config := "developerMode: true\ndeveloper_mode: true\ndeveloper: true\n" +
		strings.Join(extraConfig, "\n")
	require.NoError(t, os.WriteFile(p.Global().ConfigFile(), []byte(config), 0o600))

	bs, err := loadRunBootstrap(t.Context(), p, "", false)
	require.NoError(t, err)
	fixture.bs = bs
	return fixture
}

func (f *developerFixture) offered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.tools...)
}

func (f *developerFixture) toolOutputs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.calls...)
}

func doneText() map[string]any {
	return map[string]any{"role": "assistant", "content": "done"}
}

func TestRunHeadlessWithoutDeveloperModeOffersNoHarness(t *testing.T) {
	fixture := newDeveloperFixture(t, func(int) map[string]any { return doneText() })

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "look at yourself", maxRounds: 2, timeout: 10 * time.Second,
	})

	assert.Equal(t, ExitOK, exit)
	assert.NotContains(t, fixture.offered(), "harness",
		"config keys asking for developer mode must not register the tool")
	assert.Contains(t, fixture.offered(), "plan", "the ordinary toolset is unaffected")
}

func TestRunHeadlessWithDeveloperModeAnswersFromTheRealRuntime(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"runtime"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "look at yourself", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	// The run has no approved plan, so the model sees only the plan gate's
	// exempt tools: harness among them, and the rest of the loop unchanged.
	assert.Contains(t, fixture.offered(), "harness")
	assert.Contains(t, fixture.offered(), "plan")

	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs, "the harness call must have produced a tool result")
	snapshot := strings.Join(outputs, "\n")
	assert.Contains(t, snapshot, `"category": "runtime"`)
	assert.Contains(t, snapshot, `"headless"`)
	assert.Contains(t, snapshot, `"--developer-mode"`)
	assert.Contains(t, snapshot, `"bool": true`)
	assert.NotContains(t, snapshot, "test-key", "the provider credential never reaches an observation")
}

func TestRunHeadlessDeveloperModeReportsTheLiveSessionID(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness",
				`{"action":"explain","category":"runtime","key":"session.id"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "which session", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	assert.Contains(t, outputs[0], `"key": "session.id"`)
	assert.Contains(t, outputs[0], `"state": "present"`,
		"the accessor reads the engine that already exists when the model calls")
}

func TestRunHeadlessDeveloperModeSeparatesTheConfiguredModelFromTheRunningOne(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"model"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "which model", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	snapshot := outputs[0]

	assert.Contains(t, snapshot, `"category": "model"`)
	assert.Contains(t, snapshot, `"availability": "available"`)
	// The fixture's config declares no models: the model this run answers on
	// came from a connected provider, so the configured layer has nothing to
	// report and says so instead of naming the model that is running.
	assert.Contains(t, snapshot, `"state": "unset"`)
	assert.Contains(t, snapshot, `"kind": "default"`)
	assert.Contains(t, snapshot, `"ref": "the first models[] entry"`)
	assert.Contains(t, snapshot, `"kind": "session"`,
		"a model that arrived after the load is attributed to the session, not to a config file")

	// The model runs on a credentialed endpoint; the observation of it carries
	// neither the key nor the URL.
	assert.NotContains(t, snapshot, "test-key")
	assert.NotContains(t, snapshot, "127.0.0.1")
	assert.NotContains(t, snapshot, "http://",
		"a source may name the config key an endpoint is set in; it never carries the endpoint")
}

func TestRunHeadlessDeveloperModeReportsTheCatalogAndTheImportItStartedWith(t *testing.T) {
	// The import's locations are pinned here, so a real opencode installation
	// on the machine running the tests contributes nothing to the answer.
	opencodeDir := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", opencodeDir)
	t.Setenv("XDG_DATA_HOME", dataHome)
	require.NoError(t, os.WriteFile(filepath.Join(opencodeDir, "opencode.json"),
		[]byte(`{"provider": {}}`), 0o600))
	authPath := filepath.Join(dataHome, "opencode", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(authPath), 0o755))
	require.NoError(t, os.WriteFile(authPath,
		[]byte(`{"fixture":{"type":"api","key":"import-key-sentinel"}}`), 0o600))

	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"model"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what are you connected to", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	snapshot := outputs[0]

	assert.Contains(t, snapshot, `"catalog.connected"`)
	assert.Contains(t, snapshot, `"fixture"`, "the provider a credential is stored for is named")
	assert.Contains(t, snapshot, `"import.opencode"`)
	assert.Contains(t, snapshot, `"loaded"`, "the import ran in the headless entry point too")

	// Both files the run read hold a key, and neither key is in the answer.
	assert.NotContains(t, snapshot, "test-key")
	assert.NotContains(t, snapshot, "import-key-sentinel")
}

// opencodeHome pins the import's config and data directories at fresh
// temporary ones and returns them, so nothing on the machine running the
// tests is read and nothing written here escapes the test.
func opencodeHome(t *testing.T) (string, string) {
	t.Helper()
	configDir := t.TempDir()
	dataHome := t.TempDir()
	t.Setenv("OPENCODE_CONFIG", "")
	t.Setenv("OPENCODE_CONFIG_DIR", configDir)
	t.Setenv("XDG_DATA_HOME", dataHome)
	authPath := filepath.Join(dataHome, "opencode", "auth.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(authPath), 0o755))
	return configDir, authPath
}

func TestTheHeadlessBootstrapRecordsWhatTheImportDid(t *testing.T) {
	t.Run("switched off", func(t *testing.T) {
		p, _ := testProject(t)
		opencodeHome(t)

		_, source, facts, err := loadRuntimeSources(p, false)

		require.NoError(t, err)
		assert.Nil(t, source)
		assert.Equal(t, diag.ImportFacts{Known: true, State: diag.ImportDisabled}, facts)
	})

	t.Run("ran", func(t *testing.T) {
		p, _ := testProject(t)
		configDir, authPath := opencodeHome(t)
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "opencode.json"),
			[]byte(`{"provider": {}}`), 0o600))
		require.NoError(t, os.WriteFile(authPath,
			[]byte(`{"zai-coding-plan":{"type":"api","key":"import-key-sentinel"}}`), 0o600))

		_, source, facts, err := loadRuntimeSources(p, true)

		require.NoError(t, err)
		require.NotNil(t, source)
		assert.Equal(t, diag.ImportLoaded, facts.State)
		assert.Equal(t, len(source.Models()), facts.Models)
		assert.Positive(t, facts.Models, "a built-in provider with a key behind it contributes models")
	})

	t.Run("ran and failed", func(t *testing.T) {
		p, _ := testProject(t)
		configDir, _ := opencodeHome(t)
		require.NoError(t, os.WriteFile(filepath.Join(configDir, "opencode.json"),
			[]byte(`{"provider": "config-body-sentinel"`), 0o600))

		_, source, facts, err := loadRuntimeSources(p, true)

		require.Error(t, err)
		assert.Nil(t, source)
		assert.Equal(t, diag.ImportFacts{Known: true, State: diag.ImportFailed}, facts)
		assert.NotContains(t, fmt.Sprintf("%#v", facts), "config-body-sentinel",
			"the state says that it failed; the error's text stays with the error")
	})

	t.Run("never reached", func(t *testing.T) {
		p, _ := testProject(t)
		opencodeHome(t)
		// The import resolves against the provider catalog, so a catalog that
		// will not open stops it before it runs.
		require.NoError(t, os.WriteFile(p.Global().ProviderCatalogFile(),
			[]byte(`{"version":1,"providers":[{"id":"!!"}]}`), 0o600))

		_, source, facts, err := loadRuntimeSources(p, true)

		require.Error(t, err)
		assert.Nil(t, source)
		assert.Equal(t, diag.ImportFacts{Known: true, State: diag.ImportNotLoaded}, facts,
			"an import that never ran is not an import that failed")
	})
}

func TestTheModelCategoryIsTheSameContractInBothEntryPoints(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you observe", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	catalog := outputs[0]

	for _, key := range []string{
		"name", "request_name", "provider", "protocol", "credential", "credential.kind",
		"effort", "effort.request", "effort.levels", "context_window", "max_output_tokens",
		"variants", "options", "thinking", "pinned_by_plan",
		"catalog.providers", "catalog.connected", "import.opencode", "import.opencode.models",
		"source_order",
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares the whole category")
	}
	assert.Contains(t, catalog, "api keys", "the exclusions are published before a call is spent finding them")
}

// harnessObservation is one layer of a harness field as the model receives
// it. The tests below decode the tool result rather than matching substrings
// in it, because what this category has to get right is which layer an answer
// lands in.
type harnessObservation struct {
	State string `json:"state"`
	Value struct {
		String string   `json:"string"`
		Int    int64    `json:"int"`
		Bool   bool     `json:"bool"`
		List   []string `json:"list"`
	} `json:"value"`
	Source struct {
		Kind string `json:"kind"`
		Ref  string `json:"ref"`
	} `json:"source"`
}

type harnessField struct {
	Key        string             `json:"key"`
	Configured harnessObservation `json:"configured"`
	Loaded     harnessObservation `json:"loaded"`
	Effective  harnessObservation `json:"effective"`
}

func harnessFields(t *testing.T, snapshot string) map[string]harnessField {
	t.Helper()
	var decoded struct {
		Categories []struct {
			Availability string         `json:"availability"`
			Fields       []harnessField `json:"fields"`
		} `json:"categories"`
	}
	require.NoError(t, json.Unmarshal([]byte(snapshot), &decoded))
	require.Len(t, decoded.Categories, 1)
	require.Equal(t, "available", decoded.Categories[0].Availability)
	fields := make(map[string]harnessField, len(decoded.Categories[0].Fields))
	for _, field := range decoded.Categories[0].Fields {
		fields[field.Key] = field
	}
	return fields
}

// headlessPermissionSnapshot runs one developer-mode round that asks for the
// permission category and returns the decoded answer plus its raw text.
func headlessPermissionSnapshot(t *testing.T, opts runOptions, prepare func(*developerFixture)) (
	*developerFixture, map[string]harnessField, string,
) {
	t.Helper()
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"permissions"}`)
		}
		return doneText()
	})
	if prepare != nil {
		prepare(fixture)
	}
	opts.prompt = "what may you do"
	opts.maxRounds = 3
	opts.timeout = 10 * time.Second
	opts.developerMode = true

	require.Equal(t, ExitOK, runHeadless(t.Context(), fixture.bs, opts))
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	return fixture, harnessFields(t, outputs[0]), outputs[0]
}

func TestRunHeadlessDeveloperModeObservesTheBoundaryItRunsUnder(t *testing.T) {
	fixture, fields, snapshot := headlessPermissionSnapshot(t, runOptions{}, nil)

	assert.Equal(t, "static", fields["gate"].Effective.Value.String,
		"a headless run judges on a compiled ruleset")

	// The config declares no permissions block, so every rule is the built-in
	// one and says so rather than naming a file that set nothing.
	mode := fields["mode"]
	assert.Equal(t, "interactive", mode.Configured.Value.String)
	assert.Equal(t, "default", mode.Configured.Source.Kind)
	assert.Contains(t, mode.Configured.Source.Ref, "permissions.mode")
	assert.Equal(t, "interactive", mode.Effective.Value.String)

	allow := fields["bash.allow"]
	assert.Positive(t, allow.Effective.Value.Int, "the built-in allowlist is in force")
	assert.Equal(t, "default", allow.Effective.Source.Kind)
	assert.Positive(t, fields["paths.sensitive"].Effective.Value.Int)

	assert.False(t, fields["bypass"].Loaded.Value.Bool,
		"nothing stands in front of a headless boundary to hold it open")
	assert.False(t, fields["bypass"].Effective.Value.Bool)
	assert.True(t, fields["memory"].Effective.Value.Bool,
		"the run binds its own memory directory to the gate")

	// Counts, never the rules themselves: no built-in pattern, no sensitive
	// path prefix and no path of this machine is in the answer.
	for _, leak := range []string{"sudo", "rm -rf", ".ssh", ".aws", fixture.bs.Cwd, "test-key"} {
		assert.NotContains(t, snapshot, leak, "a rule is counted and attributed, never quoted")
	}
}

func TestRunHeadlessWithYoloObservesABoundaryWithNoRules(t *testing.T) {
	_, fields, _ := headlessPermissionSnapshot(t, runOptions{yolo: true}, func(f *developerFixture) {
		// What --yolo produces: HeadlessGate returns the unjudged boundary
		// outright rather than a ruleset with a switch in front of it.
		f.bs.Gate = permission.AllowAll{}
	})

	assert.Equal(t, "allow_all", fields["gate"].Effective.Value.String,
		"the shape of the boundary is the honest answer")

	mode := fields["mode"]
	assert.Equal(t, "interactive", mode.Configured.Value.String, "the configuration still says what it says")
	assert.Equal(t, "unavailable", mode.Loaded.State,
		"a boundary with no rules holds no mode, so none is reported")
	assert.Equal(t, "unavailable", mode.Effective.State,
		"and the configured one is not substituted for it")
	assert.Equal(t, "unavailable", fields["bash.deny"].Effective.State)
	assert.Equal(t, "unavailable", fields["paths.sensitive"].Effective.State)

	bypass := fields["bypass"]
	assert.True(t, bypass.Effective.Value.Bool, "every request is allowed without being judged")
	assert.Equal(t, "cli_flag", bypass.Effective.Source.Kind, "and the flag that did it is named")
}

func TestTheHeadlessRunNamesWhatStandsBetweenTheConfigAndItsBoundary(t *testing.T) {
	strict := permission.Policy{}
	configured := permission.DefaultPolicy()

	yolo := headlessPermissionOverlay(configured, true)
	assert.Equal(t, diag.SourceCLIFlag, yolo.Kind)
	assert.Contains(t, yolo.Ref, "--yolo")

	flagged := configured
	flagged.DangerouslyAllowAll = true
	assert.Equal(t, diag.SourceCLIFlag, headlessPermissionOverlay(flagged, false).Kind,
		"the config flag opts into the same unjudged boundary the flag does")

	assert.Equal(t, diag.SourceComputed, headlessPermissionOverlay(strict, false).Kind,
		"a policy with no mode is assembled headless-strict, which the configuration did not ask for")
	assert.Contains(t, headlessPermissionOverlay(strict, false).Ref, "headless-strict")

	assert.Equal(t, diag.Source{}, headlessPermissionOverlay(configured, false),
		"an ordinary run narrows nothing, and a difference that remains is not this function's to explain")
}

func TestThePermissionCategoryIsTheSameContractInBothEntryPoints(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you observe", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	catalog := outputs[0]

	assert.Contains(t, catalog, `"permissions"`)
	for _, key := range []string{
		"gate", "mode", "bypass", "bash.default", "bash.allow", "bash.deny",
		"workspace.only_writes", "workspace.only_reads", "paths.sensitive", "mcp.allow",
		"tasks", "memory", "ask_timeout_sec",
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}
}

func TestRunHeadlessDeveloperModeReportsItsOwnToolLayer(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"tools"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "which tools", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	snapshot := outputs[0]

	assert.Contains(t, snapshot, `"category": "tools"`)
	assert.Contains(t, snapshot, `"tool.harness"`)
	assert.Contains(t, snapshot, `"registered"`)
	assert.Contains(t, snapshot, `"useplan"`, "the posture the run stands in, not the process shape")
	// The run is a primary session in useplan with nothing approved, so the
	// gate is denying and the tools that change things are out of reach.
	assert.Contains(t, snapshot, `"deny"`)
	assert.Contains(t, snapshot, `"restricted"`)
}

func TestTheHeadlessToolAnswerCarriesNoSchemaAndNoArgument(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"snapshot","category":"tools"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "which tools", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	snapshot := outputs[0]

	for _, leak := range []string{
		"test-key",            // the provider credential
		`"properties"`,        // a tool's parameter schema
		`"input_schema"`,      // the same, in the other protocol's spelling
		"Run a shell command", // a tool description
	} {
		assert.NotContains(t, snapshot, leak,
			"the tool category names tools; it carries nothing they were built from")
	}
	assert.NotContains(t, snapshot, fixture.bs.Cwd,
		"where the run happens belongs to the runtime category, not to a tool answer")
}

func TestTheToolCategoryIsTheSameContractInBothEntryPoints(t *testing.T) {
	fixture := newDeveloperFixture(t, func(round int) map[string]any {
		if round == 1 {
			return headlessToolDelta("h1", "harness", `{"action":"catalog"}`)
		}
		return doneText()
	})

	exit := runHeadless(t.Context(), fixture.bs, runOptions{
		prompt: "what can you observe", maxRounds: 3, timeout: 10 * time.Second, developerMode: true,
	})

	assert.Equal(t, ExitOK, exit)
	outputs := fixture.toolOutputs()
	require.NotEmpty(t, outputs)
	catalog := outputs[0]

	assert.Contains(t, catalog, `"tools"`)
	for _, key := range []string{
		diag.KeyToolsRegistered, diag.KeyToolsMode, diag.KeyToolsPlanGate,
	} {
		assert.Contains(t, catalog, `"`+key+`"`, "the headless catalog declares %s too", key)
	}
	for _, name := range diag.ToolCatalog() {
		assert.Contains(t, catalog, `"`+diag.ToolKey(name)+`"`,
			"every tool the catalog knows can be asked about in a headless run")
	}
}
