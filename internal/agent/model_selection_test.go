package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestSelectModelDuringInferenceAndTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	requests := make(chan string, 2)
	releaseInference := make(chan struct{})
	toolStarted := make(chan struct{})
	releaseTool := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- string(body)
		w.Header().Set("Content-Type", "text/event-stream")
		select {
		case <-releaseInference:
		case <-ctx.Done():
			return
		}
		var req struct {
			Model string `json:"model"`
		}
		_ = json.Unmarshal(body, &req)
		if req.Model == "before" {
			_, _ = fmt.Fprint(w, sseToolCallChunk("one", "hold", `{}`))
		} else {
			_, _ = fmt.Fprint(w, sseTextChunk())
		}
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	cfg := llm.ModelConfig{
		Name:             "before",
		BaseURL:          server.URL,
		APIKey:           "x",
		ReasoningEfforts: []llm.ReasoningEffort{llm.ReasoningEffortLow, llm.ReasoningEffortHigh},
	}
	engine, err := NewEngine(
		EngineOpts{
			Model:       cfg,
			SessionOpts: SessionOpts{Cwd: t.TempDir()},
			Gate:        permission.AllowAll{},
			Tools: []tools.Tool{{
				Definition: llm.ToolDefinition{Name: "hold"},
				Run: func(context.Context, json.RawMessage) (tools.Result, error) {
					close(toolStarted)
					select {
					case <-releaseTool:
					case <-ctx.Done():
						return tools.Result{}, ctx.Err()
					}
					return tools.Result{Content: "done"}, nil
				},
			}},
		},
	)
	require.NoError(t, err)
	defer engine.Session().Close()
	done := make(chan error, 1)
	go func() {
		for _, err := range engine.Loop(ctx, "go", LoopOpts{}) {
			if err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	select {
	case body := <-requests:
		require.Contains(t, body, `"model":"before"`)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	cfg.Name = "after"
	require.NoError(t, engine.SelectModel(cfg, llm.ReasoningEffortHigh))
	require.True(t, engine.ModelStatus().Pending)
	require.Equal(t, "before", engine.ModelStatus().Effective.Name)
	before := engine.ModelStatus()
	require.Error(t, engine.SelectModel(cfg, llm.ReasoningEffort("ultra")))
	require.Equal(t, before, engine.ModelStatus())
	close(releaseInference)
	select {
	case <-toolStarted:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.Equal(t, "before", engine.ModelStatus().Effective.Name, "tool round retains its inference snapshot")
	require.NoError(t, engine.SelectModel(cfg, llm.ReasoningEffortLow))
	close(releaseTool)
	select {
	case body := <-requests:
		require.Contains(t, body, `"model":"after"`)
		require.Contains(t, body, `"reasoning_effort":"low"`)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	require.NoError(t, <-done)
	require.False(t, engine.ModelStatus().Pending)
	require.Equal(t, llm.ReasoningEffortLow, engine.ModelStatus().Effective.Effort)
	require.False(t, engine.HasTool("write"))
}
