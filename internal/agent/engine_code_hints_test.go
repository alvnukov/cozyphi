package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/codemode"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/util"
)

type adviceTransport func(*http.Request) (*http.Response, error)

func (f adviceTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCodeHintsPermitFirstResponseToDisregardAdvisor(t *testing.T) {
	for _, advice := range []string{"", `{"id":"tool/failedPackages","purpose":"unique package names; repeated failures are collapsed"}`} {
		name := "with_advice"
		if advice == "" {
			name = "without_advice"
		}
		t.Run(name, func(t *testing.T) {
			client := util.DefaultHTTPClient()
			old := client.Transport
			t.Cleanup(func() { client.Transport = old })
			const alternate = "countFailuresByPackage"
			var rounds, runs int
			client.Transport = adviceTransport(func(r *http.Request) (*http.Response, error) {
				var req struct {
					Messages []llm.Message `json:"messages"`
				}
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					return nil, err
				}
				chunk := sseTextChunk()
				if rounds == 0 {
					msg := req.Messages[len(req.Messages)-1]
					require.Equal(t, llm.RoleUser, msg.Role)
					require.Equal(t, advice != "", strings.Contains(msg.Content, "repeated failures are collapsed"))
					source, _ := json.Marshal(
						map[string]string{
							"source": `return await tools.countFailuresByPackage({packages:["cli","cli"]});`,
						},
					)
					chunk = sseToolCallChunk("first-use", "code", string(source))
				} else {
					require.Len(t, req.Messages, 4)
					last := req.Messages[len(req.Messages)-1]
					require.Equal(t, llm.RoleTool, last.Role)
					require.Contains(t, last.Content, `"cli":2`)
				}
				rounds++
				return &http.Response{
					StatusCode: http.StatusOK, Header: make(http.Header), Request: r,
					Body: io.NopCloser(strings.NewReader(chunk + "data: [DONE]\n\n")),
				}, nil
			})
			gate := new(recordingGate)
			cm := &CodeMode{
				Run: func(ctx context.Context, _, _ string, _ bool, source string, defs []llm.ToolDefinition,
					call func(context.Context, string, json.RawMessage) (any, error),
				) (CodeResult, error) {
					raw, err := codemode.Run(ctx, codemode.Program{Source: source, Tools: defs},
						func(ctx context.Context, name string, input json.RawMessage) (json.RawMessage, error) {
							value, callErr := call(ctx, name, input)
							if callErr != nil {
								return nil, callErr
							}
							return json.Marshal(value)
						})
					return CodeResult{Output: string(raw)}, err
				},
			}
			if advice != "" {
				cm.Hints = func(context.Context, string, string, string, []llm.ToolDefinition) string { return advice }
			}
			count := tools.Tool{Definition: llm.ToolDefinition{Name: alternate, Params: &llm.FunctionParameters{
				Type:       "object",
				Properties: llm.Object{"packages": llm.Object{"type": "array", "items": llm.Object{"type": "string"}}},
				Required:   []string{"packages"},
			}}, Run: func(_ context.Context, in json.RawMessage) (tools.Result, error) {
				var query struct {
					Packages []string `json:"packages"`
				}
				if err := json.Unmarshal(in, &query); err != nil {
					return tools.Result{}, err
				}
				counts := map[string]int{}
				for _, p := range query.Packages {
					counts[p]++
				}
				runs++
				return tools.Result{Structured: counts, Content: "counted failure records"}, nil
			}}
			engine, err := NewEngine(EngineOpts{
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Gate:        gate,
				Model:       llm.ModelConfig{Name: "main", BaseURL: "http://main.invalid"},
				CodeMode:    cm,
				Tools: []tools.Tool{
					count,
					{
						Definition: llm.ToolDefinition{
							Name:   "failedPackages",
							Params: &llm.FunctionParameters{Type: "object"},
						},
					},
				},
			})
			require.NoError(t, err)
			for _, runErr := range engine.Loop(t.Context(), "Count failures for cli, cli.", LoopOpts{Origin: TurnUserInput}) {
				require.NoError(t, runErr)
			}
			require.Equal(t, 2, rounds)
			require.Equal(t, 1, runs)
			require.Equal(t, permission.Action(alternate), gate.last.Action)
		})
	}
}

func TestCodeHintsOverlapPreparationAndDiscardLateResult(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	started, done := make(chan struct{}), make(chan struct{})
	engine := &Engine{executor: &Executor{}, codeMode: &CodeMode{
		Hints: func(ctx context.Context, _, _, _ string, _ []llm.ToolDefinition) string {
			close(started)
			<-ctx.Done()
			close(done)
			return `{"id":"late"}`
		},
	}}
	pending := engine.startCodeHints(ctx, "which function?", true)
	<-started
	require.NoError(t, ctx.Err()) // Prompt preparation can run before the advisor returns.
	seen := make(map[string]bool)
	cancel()
	require.Empty(t, pending.reminder(ctx, seen))
	<-done
	require.Empty(t, seen)
}
