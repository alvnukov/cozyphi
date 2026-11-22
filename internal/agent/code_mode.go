package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// CodeMode binds a program runner to a session. Only the executor supplies
// its host callback, so the guest can never obtain a native tool handler.
type CodeMode struct {
	Settings diag.CodeModeSettings
	Run      func(context.Context, string, string, bool, string, []llm.ToolDefinition, func(context.Context, string, json.RawMessage) (any, error)) (CodeResult, error)
	Tools    func(sessionID, cwd string, writable bool, definitions []llm.ToolDefinition) []tools.Tool
	Hints    func(context.Context, string, string, string, []llm.ToolDefinition) string
}

// CodeResult carries exact library resolutions outside guest-controlled output.
type CodeResult struct {
	Output  string
	Imports map[string]string
}

type (
	codeRunnerKey struct{}
	codeRequest   struct {
		Source    string `json:"source"`
		ID        string `json:"id"`
		YieldMS   int    `json:"yield_ms"`
		Terminate bool   `json:"terminate"`
	}
)
type runSource func(context.Context, codeRequest) (tools.Result, error)

func codeTool() tools.Tool {
	return tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "code",
			Description: "Run bounded TypeScript or JavaScript. Use await tools.call(name, args) or await tools.read(args). Return selected data. await tools.list({}) lists available native tools. await tools.describe({names:['read']}) returns their schemas. Import tested functions from 'cozy:lib/package'. Every operation uses the session's normal permissions. Completed effects remain after a failure.",
			Params: &llm.FunctionParameters{Type: "object", Properties: llm.Object{
				"source": llm.Object{
					"type":        "string",
					"description": "TypeScript program. Use return to select its result.",
				},
				"yield_ms": codeYieldParameter(),
			}, Required: []string{"source"}},
		},
		DetailFromArgs: func(json.RawMessage) string { return "TypeScript" },
		Run: func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
			run, ok := ctx.Value(codeRunnerKey{}).(runSource)
			if !ok {
				return tools.Result{}, errors.New("code requires a session executor")
			}
			var in codeRequest
			if err := json.Unmarshal(args, &in); err != nil {
				return tools.Result{}, fmt.Errorf("decode code: %w", err)
			}
			return run(ctx, in)
		},
	}
}

// operationResult captures only the post-hook representation allowed to leave
// the dispatcher. Its zero value also covers every refusal before execution.
type operationResult struct {
	success bool
	value   any
	notes   string
	// Only an explicitly read-only native call may release the group lock.
	// Hooks, permission prompts, result handling and trace events stay serial.
	dispatchLock *sync.Mutex
	admit        func()
}

func (r *operationResult) invoke(ctx context.Context, tool tools.Tool, args json.RawMessage) (tools.Result, error) {
	if r != nil && r.dispatchLock != nil && tool.ConcurrentRead != nil && tool.ConcurrentRead(args) {
		r.admit()
		r.dispatchLock.Unlock()
		defer r.dispatchLock.Lock()
	}
	return tool.Run(ctx, args)
}

type codeExecution struct {
	executor    *Executor
	parent      string
	state       *roundState
	emit        func(session.ToolData) bool
	definitions []llm.ToolDefinition
	hostMu      sync.Mutex
	mu          sync.Mutex
	running     bool
	count       int
	notes       []string
	deliveries  []string
	imports     map[string]string
	traceErr    error
	cancel      context.CancelFunc
}

func (e *Executor) runCode(
	ctx context.Context,
	call llm.ToolCall,
	emit func(session.ToolData) bool,
	state *roundState,
) llm.Message {
	cellCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cell := &codeExecution{executor: e, parent: call.ID, state: state, emit: emit, running: true, cancel: cancel}
	for name, tool := range e.registry {
		if name != "code" && name != "code_wait" {
			cell.definitions = append(cell.definitions, tool.Definition)
		}
	}
	slices.SortFunc(cell.definitions, func(a, b llm.ToolDefinition) int { return strings.Compare(a.Name, b.Name) })
	runCtx := context.WithValue(cellCtx, codeRunnerKey{}, runSource(cell.run))
	msg := e.runOne(runCtx, call, emit, state, nil)
	cell.mu.Lock()
	defer cell.mu.Unlock()
	cell.running = false
	// A caught JS exception or an early return must not swallow trusted
	// instructions, approvals, committed deliveries or partial execution.
	if cell.traceErr != nil {
		cell.notes = append(cell.notes, cell.traceErr.Error())
	}
	if cell.count > 0 && (ctx.Err() != nil || state.stop.stopped || state.skillRetryRequired || cell.traceErr != nil) {
		cell.notes = append(
			cell.notes,
			"The code execution stopped. Completed operations remain applied. Consult the recorded operations before retrying.",
		)
	}
	if len(cell.notes) > 0 {
		msg.Content = appendModelReminder(msg.Content, strings.Join(cell.notes, "\n\n"))
	}
	msg.DeliveryIDs = cell.deliveries
	msg.CodeImports = cell.imports
	return msg
}

func (c *codeExecution) run(ctx context.Context, req codeRequest) (tools.Result, error) {
	if req.YieldMS < 0 || req.YieldMS > 30000 {
		return tools.Result{}, errors.New("yield_ms must be between 0 and 30000")
	}
	if req.ID != "" || req.YieldMS > 0 {
		hub, ok := ctx.Value(codeHubKey{}).(*codeHub)
		if !ok {
			return tools.Result{}, errors.New("yield requires a live engine turn")
		}
		return hub.advance(ctx, req, c)
	}
	source := req.Source
	e := c.executor
	out, err := e.codeMode.Run(ctx, e.sessionID, e.cwd, e.codeWritable, source, c.definitions, c.call)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
	c.imports = out.Imports
	if err != nil || c.traceErr != nil {
		return tools.Result{}, errors.Join(err, c.traceErr)
	}
	return tools.Result{Content: out.Output, Detail: fmt.Sprintf("TypeScript · %d operations", c.count)}, nil
}

func (c *codeExecution) call(ctx context.Context, name string, args json.RawMessage) (any, error) {
	c.hostMu.Lock()
	defer c.hostMu.Unlock()
	if name == "cozy:parallel_read" {
		return c.readGroup(ctx, args)
	}
	return c.single(ctx, name, args, nil)
}

func (c *codeExecution) single(ctx context.Context, name string, args json.RawMessage, admit func()) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running || c.state.stop.stopped || c.state.skillRetryRequired || ctx.Err() != nil || c.traceErr != nil {
		return nil, errors.New("code execution stopped")
	}
	if name == "code" || name == "code_wait" {
		return nil, errors.New("recursive code execution is unavailable")
	}
	c.count++
	call := llm.ToolCall{
		ID:       fmt.Sprintf("%s/%d", c.parent, c.count),
		Type:     "function",
		Function: llm.Function{Name: name, Arguments: string(args)},
	}
	var observed operationResult
	if admit != nil {
		observed.dispatchLock = &c.mu
		observed.admit = admit
	}
	emit := func(event session.ToolData) bool {
		event.Run.ParentUseID = c.parent
		// Generic runners use raw JSON as their UI fallback. An operation
		// journal keeps identity and outcome without copying the request.
		if event.Run.Detail == string(args) {
			event.Run.Detail = name
		}
		if !c.record(event.Run) {
			return false
		}
		if !c.emit(event) {
			c.running = false
			c.cancel()
			return false
		}
		return true
	}
	message := c.executor.runOne(ctx, call, emit, c.state, &observed)
	if c.state.stop.stopped || c.state.skillRetryRequired {
		c.cancel()
	}
	if message.DeliveryID != "" {
		c.deliveries = append(c.deliveries, message.DeliveryID)
		// The receipt acknowledges actual delivery to the parent. Always
		// retain the child outcome even when the program drops its value.
		c.notes = append(c.notes, message.Content)
	} else if observed.notes != "" {
		c.notes = append(c.notes, observed.notes)
	}
	if !observed.success {
		c.notes = append(c.notes, fmt.Sprintf("%s: %s", name, message.Content))
		return nil, errors.New(message.Content)
	}
	return observed.value, nil
}

func (c *codeExecution) record(run session.ToolRun) bool {
	if c.executor.saveCodeTrace == nil {
		return true
	}
	if err := c.executor.saveCodeTrace(run); err != nil {
		c.traceErr = fmt.Errorf("record %s operation %s: %w", run.Name, run.ToolUseID, err)
		c.cancel()
		return false
	}
	return true
}
