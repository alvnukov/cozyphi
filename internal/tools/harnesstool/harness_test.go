package harnesstool_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/tools/harnesstool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// newTool builds the tool over a real registry with the runtime collector
// wired, and returns the one thing a test needs: the call the model makes.
func newTool(t *testing.T) func(string) (tooldef.Result, error) {
	t.Helper()
	registry := diag.NewRegistry(nil, diag.DefaultLimits(), diag.NewRuntimeCollector(diag.RuntimeDeps{
		Version:   "1.4.2",
		Mode:      "headless",
		Enabled:   true,
		Workspace: func() string { return "/srv/work" },
		SessionID: func() string { return "sess-1" },
	}))
	tool := harnesstool.Tool(harnesstool.Deps{Registry: registry})
	return func(args string) (tooldef.Result, error) {
		return tool.Run(t.Context(), json.RawMessage(args))
	}
}

func TestToolDeclaresOneReadOnlyEntryPoint(t *testing.T) {
	tool := harnesstool.Tool(harnesstool.Deps{})

	assert.Equal(t, "harness", tool.Definition.Name)
	require.NotNil(t, tool.Definition.Params)
	assert.Equal(t, []string{"action"}, tool.Definition.Params.Required)
	assert.Contains(t, tool.Definition.Params.Properties, "action")
	assert.Contains(t, tool.Definition.Params.Properties, "category")
	assert.Contains(t, tool.Definition.Params.Properties, "key")
	assert.NotNil(t, tool.DetailFromArgs)
	assert.Contains(t, tool.Definition.Description, "Read-only")
}

func TestCatalogListsEveryCategoryIncludingTheUnwiredOnes(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":"catalog"}`)
	require.NoError(t, err)
	assert.Equal(t, "catalog", result.Detail)
	for _, category := range diag.Categories() {
		assert.Contains(t, result.Content, `"category": "`+string(category)+`"`)
	}
	assert.Contains(t, result.Content, string(diag.AvailabilityNotImplemented))
}

func TestSnapshotWithoutCategoryIsTheOverview(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":"snapshot"}`)
	require.NoError(t, err)
	assert.Equal(t, "snapshot", result.Detail)
	assert.Contains(t, result.Content, `"mode": "`+diag.ModeOverview+`"`)
	assert.Contains(t, result.Content, `"headless"`)
}

func TestSnapshotWithCategoryCarriesEveryLayer(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":"snapshot","category":"runtime"}`)
	require.NoError(t, err)
	assert.Equal(t, "snapshot runtime", result.Detail)
	assert.Contains(t, result.Content, `"mode": "`+diag.ModeDetail+`"`)
	assert.Contains(t, result.Content, `"configured"`)
	assert.Contains(t, result.Content, `"loaded"`)
	assert.Contains(t, result.Content, `"effective"`)
}

func TestExplainAnswersOneField(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":"explain","category":"runtime","key":"developer_mode"}`)
	require.NoError(t, err)
	assert.Equal(t, "explain runtime developer_mode", result.Detail)
	assert.Contains(t, result.Content, `"key": "developer_mode"`)
	assert.Contains(t, result.Content, `"--developer-mode"`)
	assert.Contains(t, result.Content, `"bool": true`)
}

func TestActionAndCategoryAreCaseAndSpaceForgiving(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":" SNAPSHOT ","category":"Runtime"}`)
	require.NoError(t, err)
	assert.Contains(t, result.Content, `"category": "runtime"`)
}

func TestInvalidArgumentsSayWhatToDoInstead(t *testing.T) {
	call := newTool(t)

	cases := []struct {
		name string
		args string
		want string
	}{
		{"missing action", `{}`, "action is required"},
		{"unknown action", `{"action":"set"}`, `unknown action "set"`},
		{"catalog with category", `{"action":"catalog","category":"runtime"}`, "catalog takes no category"},
		{"catalog with key", `{"action":"catalog","key":"version"}`, "catalog takes no category"},
		{"snapshot with key", `{"action":"snapshot","category":"runtime","key":"version"}`, "snapshot takes no key"},
		{"explain without category", `{"action":"explain","key":"version"}`, "explain needs both"},
		{"explain without key", `{"action":"explain","category":"runtime"}`, "explain needs both"},
		{"unknown category", `{"action":"snapshot","category":"wallet"}`, `unknown category "wallet"`},
		{"unknown key", `{"action":"explain","category":"runtime","key":"nope"}`, `no field "nope"`},
		{"unwired category", `{"action":"explain","category":"storage","key":"path"}`, "not implemented"},
		{"unknown field", `{"action":"catalog","depth":3}`, "invalid arguments"},
		{"not an object", `[]`, "invalid arguments"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, err := call(tc.args)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)
			assert.Empty(t, result.Content, "a rejected call answers nothing")
		})
	}
}

func TestUnknownActionIsEchoedBounded(t *testing.T) {
	call := newTool(t)

	_, err := call(`{"action":"` + strings.Repeat("z", 200) + `"}`)
	require.Error(t, err)
	assert.Less(t, len(err.Error()), 200, "a hostile argument is not echoed whole")
}

func TestPlanStepIsAcceptedBecauseTheGateInjectsIt(t *testing.T) {
	call := newTool(t)

	result, err := call(`{"action":"catalog","plan_step":{"id":"s1","title":"look"}}`)
	require.NoError(t, err)
	assert.NotEmpty(t, result.Content)
}

func TestWithoutTheCapabilityEveryCallIsRefusedSafely(t *testing.T) {
	tool := harnesstool.Tool(harnesstool.Deps{})

	for _, args := range []string{
		`{"action":"catalog"}`,
		`{"action":"snapshot","category":"runtime"}`,
		`{"action":"explain","category":"runtime","key":"version"}`,
		`{"action":"set"}`,
		`not json`,
	} {
		result, err := tool.Run(t.Context(), json.RawMessage(args))
		require.Error(t, err, args)
		assert.Contains(t, err.Error(), "--developer-mode", args)
		assert.Empty(t, result.Content, args)
	}
}

func TestDetailFromArgsSurvivesArgumentsThatNeverParsed(t *testing.T) {
	tool := harnesstool.Tool(harnesstool.Deps{})

	assert.Equal(t, "explain runtime version",
		tool.DetailFromArgs(json.RawMessage(`{"action":"explain","category":"runtime","key":"version"}`)))
	assert.NotEmpty(t, tool.DetailFromArgs(json.RawMessage(`{`)), "a broken call still gets a label")
}
