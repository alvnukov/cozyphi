package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestUnavailableToolReturnsRecoveryWithoutExecuting(t *testing.T) {
	for _, scenario := range []struct {
		name     string
		mode     Mode
		approved bool
		reason   string
	}{
		{"unapproved", ModeUsePlan, false, "the plan is not approved"},
		{"wrong step type", ModeUsePlan, true, "not allowed on a explore step"},
		{"planning mode", ModePlan, false, "plan mode"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			engine, err := NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Gate:        permission.AllowAll{},
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, engine.Session().Close()) })
			engine.SetMode(scenario.mode)
			if scenario.approved {
				_, err = engine.updatePlan(t.Context(), []session.PlanItem{{
					Content: "inspect", Status: session.PlanInProgress, Type: session.StepExplore,
				}})
				require.NoError(t, err)
				_, err = engine.SetPlanApproved(true)
				require.NoError(t, err)
			}
			path := filepath.Join(engine.SessionCwd(), "must-not-exist.txt")
			args, err := json.Marshal(struct {
				Path    string `json:"file_path"`
				Content string `json:"content"`
				Step    int    `json:"plan_step"`
			}{Path: path, Content: "blocked", Step: 1})
			require.NoError(t, err)
			var rejected []session.ToolRun
			msgs, _, _ := engine.executor.run(t.Context(), []llm.ToolCall{{
				ID: "denied", Function: llm.Function{Name: "write", Arguments: string(args)},
			}}, func(td session.ToolData) bool {
				if td.Run.Status == session.ToolRejected {
					rejected = append(rejected, td.Run)
				}
				return true
			})
			require.Len(t, rejected, 1)
			assert.Contains(t, rejected[0].Error, scenario.reason)
			_, err = os.Stat(path)
			require.ErrorIs(t, err, os.ErrNotExist, "a denial must not reach the file handler")
			require.Len(t, msgs, 1)
			assert.Equal(t, "denied", msgs[0].ToolCallID)
			var body struct {
				Error struct {
					Code        string `json:"code"`
					Tool        string `json:"tool"`
					Phase       string `json:"current_phase"`
					Reason      string `json:"reason"`
					NextAction  string `json:"next_action"`
					RetryPolicy string `json:"retry_policy"`
				} `json:"tool_error"`
			}
			require.NoError(t, json.Unmarshal([]byte(msgs[0].Content), &body))
			assert.Equal(t, "TOOL_NOT_AVAILABLE_IN_CURRENT_PHASE", body.Error.Code)
			assert.Equal(t, "write", body.Error.Tool)
			assert.Equal(t, string(scenario.mode), body.Error.Phase)
			assert.Contains(t, body.Error.Reason, scenario.reason)
			assert.NotEmpty(t, body.Error.NextAction)
			assert.Contains(t, body.Error.RetryPolicy, "Do not substitute another tool")
			assert.NotContains(t, body.Error.NextAction, "use a tool that step allows")
		})
	}
}
