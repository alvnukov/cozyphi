package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

type boundSpawnResult struct {
	JobID string `json:"job_id"`
	Model string `json:"model"`
}

// Each user submission delegates once, then consumes the tool result.
func boundSpawnServer(t *testing.T, role job.Role, firstRequest func()) *httptest.Server {
	t.Helper()
	var once sync.Once
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct {
				Role string `json:"role"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode inference: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if firstRequest != nil {
			once.Do(firstRequest)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		if len(req.Messages) > 0 && req.Messages[len(req.Messages)-1].Role == "user" {
			args := fmt.Sprintf(
				`{"prompt":"delegated task","role":%q,"skills":[],"no_skill_reason":"snapshot probe"}`,
				role,
			)
			_, _ = fmt.Fprint(w, sseToolCallChunk("spawn", "agent_spawn", args))
		} else {
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv
}

func boundSpawnLoop(ctx context.Context, engine *Engine) (boundSpawnResult, error) {
	var result boundSpawnResult
	for ev, err := range engine.Loop(ctx, "delegate", LoopOpts{}) {
		if err != nil {
			return result, err
		}
		if td, ok := ev.(session.ToolData); ok && td.Run.Name == "agent_spawn" {
			if td.Run.Status == session.ToolError || td.Run.Status == session.ToolRejected {
				return result, fmt.Errorf("spawn failed: %s", td.Run.Error)
			}
			if td.Run.Status == session.ToolDone {
				if err := json.Unmarshal([]byte(td.Run.Output), &result); err != nil {
					return result, err
				}
			}
		}
	}
	if result.JobID == "" {
		return result, fmt.Errorf("loop returned no accepted spawn")
	}
	return result, nil
}

func TestLoopSharedJobsBindOriginatingSnapshot(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	inferenceStarted := make(chan struct{})
	releaseInference := make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(releaseInference) })
	serverA := boundSpawnServer(t, job.RoleExplore, func() {
		close(inferenceStarted)
		select {
		case <-releaseInference:
		case <-ctx.Done():
		}
	})
	serverB := boundSpawnServer(t, job.RoleExplore, nil)
	releaseJobs := make(chan struct{})
	var jobsOnce sync.Once
	defer jobsOnce.Do(func() { close(releaseJobs) })
	var legacyRuns atomic.Int32
	manager, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
			legacyRuns.Add(1)
			return "wrong manager runner", nil
		}),
		ModelNameForRole: func(job.Role) (string, bool) { return "wrong manager pin", true },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })

	newParent := func(name, url string) *Engine {
		hookManager := hooks.NewManager()
		query := tools.LSPQueryFunc(func(context.Context, lsp.Query) (lsp.Result, error) {
			return lsp.Result{}, fmt.Errorf("%s lsp", name)
		})
		engine, createErr := NewEngine(EngineOpts{
			Model:       llm.ModelConfig{Name: name, BaseURL: url, APIKey: "x"},
			SessionOpts: SessionOpts{Cwd: t.TempDir()},
			Tools:       []tools.Tool{},
			Gate:        permission.AllowAll{},
			Jobs:        manager,
			Hooks:       hookManager,
			LSP:         query,
			JobRunner: func(
				model llm.ModelConfig,
				inheritedHooks *hooks.Manager,
				inheritedLSP tools.LSPQueryFunc,
			) job.Runner {
				return job.RunnerFunc(func(ctx context.Context, _ job.RunEnv) (string, error) {
					select {
					case <-releaseJobs:
					case <-ctx.Done():
						return "", ctx.Err()
					}
					assert.Same(t, hookManager, inheritedHooks)
					if inheritedLSP == nil {
						return "", fmt.Errorf("missing inherited LSP")
					}
					_, queryErr := inheritedLSP(ctx, lsp.Query{})
					return fmt.Sprintf("%s / %v", model.Name, queryErr), nil
				})
			},
		})
		require.NoError(t, createErr)
		return engine
	}
	parentA := newParent("parent-a", serverA.URL)
	parentB := newParent("parent-b", serverB.URL)
	type loopResult struct {
		spawn boundSpawnResult
		err   error
	}
	finishedA := make(chan loopResult, 1)
	go func() {
		spawn, loopErr := boundSpawnLoop(ctx, parentA)
		finishedA <- loopResult{spawn, loopErr}
	}()
	select {
	case <-inferenceStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Change A before its old inference response even asks to spawn.
	require.NoError(t, parentA.SetModel(llm.ModelConfig{Name: "future-a", BaseURL: serverA.URL, APIKey: "x"}))
	parentA.SetHooks(hooks.NewManager())
	spawnB, err := boundSpawnLoop(ctx, parentB)
	require.NoError(t, err)
	releaseOnce.Do(func() { close(releaseInference) })
	var resultA loopResult
	select {
	case resultA = <-finishedA:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, resultA.err)
	// Change B after admission but before either delayed runner consumes its snapshot.
	require.NoError(t, parentB.SetModel(llm.ModelConfig{Name: "future-b", BaseURL: serverB.URL, APIKey: "x"}))
	parentB.SetHooks(hooks.NewManager())
	jobsOnce.Do(func() { close(releaseJobs) })
	for _, tc := range []struct {
		spawn boundSpawnResult
		want  string
	}{
		{resultA.spawn, "parent-a / parent-a lsp"},
		{spawnB, "parent-b / parent-b lsp"},
	} {
		result, waitErr := manager.Wait(ctx, tc.spawn.JobID)
		require.NoError(t, waitErr)
		assert.Equal(t, job.StatusCompleted, result.Info.Status)
		assert.Equal(t, tc.want, result.Summary)
		assert.Equal(t, "inherit", tc.spawn.Model, "scoped runner must not display the manager's pin")
	}
	assert.Zero(t, legacyRuns.Load())
}

// Preserve the production runner's optional model-name view while delaying Run.
type delayedBoundEngineRunner struct {
	EngineRunner
	release <-chan struct{}
}

func (r delayedBoundEngineRunner) Run(ctx context.Context, env job.RunEnv) (string, error) {
	select {
	case <-r.release:
		return r.EngineRunner.Run(ctx, env)
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func TestLoopSharedJobsRoleDisplayMatchesDelayedChild(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	childServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Model string `json:"model"`
			Tools []struct {
				Function struct {
					Name string `json:"name"`
				} `json:"function"`
			} `json:"tools"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode child inference: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var names []string
		for _, tool := range req.Tools {
			names = append(names, tool.Function.Name)
		}
		assert.Contains(t, names, "write", "worker retains its role tool ceiling")
		for _, forbidden := range []string{"agent_spawn", "memory", "task", "watch", "question"} {
			assert.NotContains(t, names, forbidden)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		// Echo the actual provider model rather than the configured display name.
		body, _ := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"delta": map[string]any{"role": "assistant", "content": req.Model}}},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
	}))
	t.Cleanup(childServer.Close)
	parentServer := boundSpawnServer(t, job.RoleWorker, nil)
	manager, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
			return "wrong legacy runner", nil
		}),
		ModelNameForRole: func(job.Role) (string, bool) { return "wrong legacy pin", true },
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = manager.Close() })
	release := make(chan struct{})
	var once sync.Once
	defer once.Do(func() { close(release) })
	var pin atomic.Pointer[llm.ModelConfig]
	pin.Store(&llm.ModelConfig{Name: "old-pin", APIName: "old-provider", BaseURL: childServer.URL, APIKey: "x"})
	parent, err := NewEngine(EngineOpts{
		Model:       llm.ModelConfig{Name: "parent", BaseURL: parentServer.URL, APIKey: "x"},
		SessionOpts: SessionOpts{Cwd: t.TempDir()},
		Tools:       []tools.Tool{},
		Gate:        permission.AllowAll{},
		Jobs:        manager,
		JobRunner: func(model llm.ModelConfig, hookManager *hooks.Manager, query tools.LSPQueryFunc) job.Runner {
			resolved := *pin.Load() // the factory resolves role configuration once, not at Run time
			return delayedBoundEngineRunner{
				EngineRunner: EngineRunner{
					Model: model,
					Hooks: hookManager,
					LSP:   query,
					ModelForRole: func(role job.Role) (llm.ModelConfig, bool) {
						return resolved, role == job.RoleWorker
					},
				},
				release: release,
			}
		},
	})
	require.NoError(t, err)
	oldSpawn, err := boundSpawnLoop(ctx, parent)
	require.NoError(t, err)
	pin.Store(&llm.ModelConfig{Name: "new-pin", APIName: "new-provider", BaseURL: childServer.URL, APIKey: "x"})
	parent.RefreshTools()
	newSpawn, err := boundSpawnLoop(ctx, parent)
	require.NoError(t, err)
	// Neither child has run yet, and the ambient role source has changed again.
	pin.Store(&llm.ModelConfig{Name: "future-pin", APIName: "future-provider", BaseURL: childServer.URL, APIKey: "x"})
	parent.RefreshTools()
	once.Do(func() { close(release) })
	for _, tc := range []struct {
		spawn    boundSpawnResult
		name     string
		provider string
	}{
		{oldSpawn, "old-pin", "old-provider"},
		{newSpawn, "new-pin", "new-provider"},
	} {
		assert.Equal(t, tc.name, tc.spawn.Model)
		result, waitErr := manager.Wait(ctx, tc.spawn.JobID)
		require.NoError(t, waitErr)
		assert.Equal(t, job.StatusCompleted, result.Info.Status)
		assert.Equal(t, tc.provider, result.Summary)
	}
}

func TestLoopJobRunnerLegacyAndNilAdmission(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(fmt.Sprintf("scoped=%t", scoped), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			server := boundSpawnServer(t, job.RoleExplore, nil)
			var runs atomic.Int32
			manager, err := job.New(job.Options{
				Root: t.TempDir(),
				Runner: job.RunnerFunc(func(context.Context, job.RunEnv) (string, error) {
					runs.Add(1)
					return "legacy child", nil
				}),
				ModelNameForRole: func(job.Role) (string, bool) { return "legacy pin", true },
			})
			require.NoError(t, err)
			t.Cleanup(func() { _ = manager.Close() })
			var factory JobRunnerFactory
			if scoped {
				factory = func(llm.ModelConfig, *hooks.Manager, tools.LSPQueryFunc) job.Runner { return nil }
			}
			parent, err := NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "parent", BaseURL: server.URL, APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Tools:       []tools.Tool{},
				Gate:        permission.AllowAll{},
				Jobs:        manager,
				JobRunner:   factory,
			})
			require.NoError(t, err)
			spawn, err := boundSpawnLoop(ctx, parent)
			if scoped {
				require.ErrorContains(t, err, "Runner is required")
				assert.Zero(t, runs.Load(), "nil scoped runner must not fall back to the manager")
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "legacy pin", spawn.Model)
			result, err := manager.Wait(ctx, spawn.JobID)
			require.NoError(t, err)
			assert.Equal(t, "legacy child", result.Summary)
			assert.Equal(t, int32(1), runs.Load())
		})
	}
}
