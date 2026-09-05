package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestRunHeadlessChildInheritsResolvedModel(t *testing.T) {
	for _, lastModel := range []bool{false, true} {
		t.Run(fmt.Sprintf("last_model=%t", lastModel), func(t *testing.T) {
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
			childModels := make(chan string, 1)
			var parentRequests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var request struct {
					Model string `json:"model"`
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
				parent := false
				for _, tool := range request.Tools {
					parent = parent || tool.Function.Name == "agent_spawn"
				}
				delta := map[string]any{"role": "assistant", "content": "child complete"}
				if !parent {
					select {
					case childModels <- request.Model:
					default:
					}
				} else {
					switch parentRequests.Add(1) {
					case 1:
						delta = headlessToolDelta(
							"spawn",
							"agent_spawn",
							`{"prompt":"report child complete","skills":[],"no_skill_reason":"test fixture","plan_step":1}`,
						)
					case 2:
						var spawned struct {
							JobID string `json:"job_id"`
						}
						for _, message := range request.Messages {
							if message.Role == "tool" {
								assert.NoError(t, json.NewDecoder(strings.NewReader(message.Content)).Decode(&spawned))
							}
						}
						assert.NotEmpty(t, spawned.JobID)
						args, _ := json.Marshal(map[string]any{"job_id": spawned.JobID, "plan_step": 1})
						delta = headlessToolDelta("wait", "agent_wait", string(args))
					default:
						last := request.Messages[len(request.Messages)-1]
						assert.Contains(t, last.Content, "child complete")
					}
				}
				w.Header().Set("Content-Type", "text/event-stream")
				payload, err := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": delta}}})
				assert.NoError(t, err)
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", payload)
			}))
			defer server.Close()
			// A connected, cached provider keeps all inference on the local server.
			cache := fmt.Sprintf(`{"version":1,"providers":[{"id":"fixture","name":"Fixture",
				"base_url":%q,"protocol":"openai","models":[
				{"id":"first","name":"First"},{"id":"last","name":"Last"}]}]}`, server.URL)
			credentials := fmt.Sprintf(`{"version":1,"providers":{"fixture":{
				"type":"api","key":"test-key","base_url":%q,"protocol":"openai"}}}`, server.URL)
			require.NoError(t, os.WriteFile(p.Global().ProviderCatalogFile(), []byte(cache), 0o600))
			require.NoError(t, os.WriteFile(p.Global().CredentialsFile(), []byte(credentials), 0o600))
			want := "first"
			if lastModel {
				want = "last"
				require.NoError(t, project.MutateUIState(p.Global(), func(s *project.UIState) {
					s.LastModel = "fixture/last"
				}))
			}
			bs, err := loadRunBootstrap(t.Context(), p, "", false)
			require.NoError(t, err)
			require.Empty(t, bs.Config.Model().Name, "exercise resolution beyond config.yaml")
			require.True(t, bs.Config.Agents.Enabled)
			// Headless has no approval UI. Resume user-approved delegation rather
			// than bypassing the independent plan gate in this assembly test.
			stored, err := session.NewSessionManager(
				bs.Cwd,
				session.WithSessionDir(bs.SessionDir),
				session.WithShouldFlush(true),
			)
			require.NoError(t, err)
			_, err = stored.ReplacePlanWithAutoApprove([]session.PlanItem{{
				Content: "delegate", Type: session.StepDelegate, Status: session.PlanInProgress,
			}}, true)
			require.NoError(t, err)

			require.NoError(t, stored.Close())
			exit := runHeadless(t.Context(), bs, runOptions{
				prompt: "delegate", maxRounds: 4, timeout: 10 * time.Second, continueLast: true,
			})

			assert.Equal(t, ExitOK, exit)
			assert.EqualValues(t, 3, parentRequests.Load())
			select {
			case got := <-childModels:
				assert.Equal(t, want, got)
			default:
				t.Error("child never reached the selected provider endpoint")
			}
		})
	}
}

func TestRunHeadlessJobRunnerRebindsModelSnapshot(t *testing.T) {
	for _, pinned := range []bool{false, true} {
		t.Run(fmt.Sprintf("pinned=%t", pinned), func(t *testing.T) {
			initial := llm.ModelConfig{Name: "initial", APIKey: "test-key"}
			selected := llm.ModelConfig{Name: "selected", APIKey: "test-key"}
			roleModel := llm.ModelConfig{Name: "role-model", APIKey: "role-key"}
			bs := &runBootstrap{Config: &project.Config{
				Models: []llm.ModelConfig{initial, selected, roleModel},
				Agents: project.AgentsConfig{Models: map[string]string{}},
			}}
			if pinned {
				bs.Config.Agents.Models[string(job.RoleExplore)] = roleModel.Name
			}
			factory := runJobRunnerFactory(bs)
			hookManager := hooks.NewManager()
			query := tools.LSPQueryFunc(func(context.Context, lsp.Query) (lsp.Result, error) {
				return lsp.Result{}, fmt.Errorf("snapshot query")
			})
			manager, err := job.New(job.Options{
				Root: t.TempDir(), Runner: factory(initial, hookManager, query),
			})
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, manager.Close()) })
			var bound agent.EngineRunner
			engine, err := agent.NewEngine(agent.EngineOpts{
				Model: initial, SessionOpts: agent.SessionOpts{Cwd: t.TempDir()},
				Tools: []tools.Tool{}, Jobs: manager, Hooks: hookManager, LSP: query,
				JobRunner: func(model llm.ModelConfig, h *hooks.Manager, q tools.LSPQueryFunc) job.Runner {
					runner := factory(model, h, q)
					bound = runner.(agent.EngineRunner)
					return runner
				},
			})
			require.NoError(t, err)
			require.Equal(t, initial, bound.Model)
			first := bound
			require.NoError(t, engine.SetModel(selected))
			require.Equal(t, selected, bound.Model, "rebinding must not use the startup manager model")
			assert.Equal(t, initial, first.Model, "an already-bound runner keeps its model")
			assert.Nil(t, bound.ModelFn)
			assert.Nil(t, bound.HooksFn)
			assert.Same(t, hookManager, bound.Hooks)
			require.NotNil(t, bound.LSP)
			_, err = bound.LSP(t.Context(), lsp.Query{})
			require.EqualError(t, err, "snapshot query")

			// A delayed child must not resolve its role against a changed catalog or pin.
			bs.Config.Models[2] = selected
			bs.Config.Agents.Models[string(job.RoleExplore)] = selected.Name
			got, ok := bound.ModelForRole(job.RoleExplore)
			require.Equal(t, pinned, ok)
			if pinned {
				assert.Equal(t, roleModel, got)
				name, resolved := bound.ModelNameForRole(job.RoleExplore)
				assert.True(t, resolved)
				assert.Equal(t, roleModel.Name, name)
			}
		})
	}
}

func headlessToolDelta(id, name, args string) map[string]any {
	return map[string]any{
		"role": "assistant",
		"tool_calls": []any{map[string]any{
			"index": 0, "id": id, "type": "function",
			"function": map[string]string{"name": name, "arguments": args},
		}},
	}
}
