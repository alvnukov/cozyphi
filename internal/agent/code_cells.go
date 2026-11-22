package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

const maxCodeCells = 4

type codeHubKey struct{}

// The engine goroutine owns the map and executes host requests. Only guest
// computations run independently, so yielding can never start another tool
// after its permission context or the event consumer has gone away.
type codeHub struct {
	ctx   context.Context
	cells map[string]*pendingCode
}
type pendingCode struct {
	cancel   context.CancelFunc
	requests chan nativeRequest
	done     chan guestOutcome
}
type guestOutcome struct {
	result CodeResult
	err    error
}
type nativeRequest struct {
	ctx   context.Context
	name  string
	args  json.RawMessage
	reply chan nativeResponse
}
type nativeResponse struct {
	value any
	err   error
}

func newCodeHub(ctx context.Context) *codeHub {
	return &codeHub{ctx: ctx, cells: make(map[string]*pendingCode)}
}

func (h *codeHub) close() {
	for id := range h.cells {
		h.remove(id)
	}
}

func (h *codeHub) remove(id string) {
	if cell := h.cells[id]; cell != nil {
		cell.cancel()
		delete(h.cells, id)
	}
}

func codeYieldParameter() llm.Object {
	return llm.Object{
		"type":        "integer",
		"description": "Yield the cell after this many milliseconds at a safe host-operation boundary (1–30000). Omit to wait until complete.",
	}
}

func codeWaitTool() tools.Tool {
	t := codeTool()
	t.Definition.Name = "code_wait"
	t.Definition.Description = "Resume a yielded code cell in this turn, or terminate it. Completed effects remain applied. A native operation runs to its next safe boundary before yielding."
	t.Definition.Params = &llm.FunctionParameters{Type: "object", Properties: llm.Object{
		"id":        llm.Object{"type": "string", "description": "Cell id returned by code."},
		"yield_ms":  codeYieldParameter(),
		"terminate": llm.Object{"type": "boolean", "description": "Stop the cell and discard future requests."},
	}, Required: []string{"id"}}
	base := t.Run
	t.Run = func(ctx context.Context, args json.RawMessage) (tools.Result, error) {
		var in codeRequest
		if err := json.Unmarshal(args, &in); err != nil || in.ID == "" {
			return tools.Result{}, errors.New("code_wait requires a cell id")
		}
		return base(ctx, args)
	}
	return t
}

func (h *codeHub) start(source string, c *codeExecution) (string, error) {
	if len(h.cells) >= maxCodeCells {
		return "", fmt.Errorf("at most %d cells may be active in a turn", maxCodeCells)
	}
	id := session.NewEntryID()
	ctx, cancel := context.WithCancel(h.ctx)
	cell := &pendingCode{cancel: cancel, requests: make(chan nativeRequest), done: make(chan guestOutcome, 1)}
	h.cells[id] = cell
	e := c.executor
	go func() {
		defer cancel()
		out, err := e.codeMode.Run(ctx, e.sessionID, e.cwd, e.codeWritable, source, c.definitions, cell.hostCall)
		cell.done <- guestOutcome{out, err}
	}()
	return id, nil
}

// The request's own context also carries the guest's deadline. It may expire
// while a program is parked between observe calls.
func (cell *pendingCode) hostCall(ctx context.Context, name string, args json.RawMessage) (any, error) {
	ch := make(chan nativeResponse, 1)
	select {
	case cell.requests <- nativeRequest{ctx: ctx, name: name, args: args, reply: ch}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	select {
	case result := <-ch:
		return result.value, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (h *codeHub) advance(ctx context.Context, req codeRequest, c *codeExecution) (tools.Result, error) {
	id := req.ID
	if id == "" {
		var err error
		id, err = h.start(req.Source, c)
		if err != nil {
			return tools.Result{}, err
		}
	}
	cell := h.cells[id]
	if cell == nil {
		return tools.Result{}, errors.New(
			"cell is unavailable in this turn; inspect completed operations before retrying",
		)
	}
	if req.Terminate {
		h.remove(id)
		return tools.Result{Content: "Cell terminated. Previously completed operations remain applied."}, nil
	}
	var tick <-chan time.Time
	if req.YieldMS > 0 {
		timer := time.NewTimer(time.Duration(req.YieldMS) * time.Millisecond)
		defer timer.Stop()
		tick = timer.C
	}
	for {
		select {
		case <-ctx.Done():
			h.remove(id)
			return tools.Result{}, ctx.Err()
		case <-tick:
			return tools.Result{Content: fmt.Sprintf(`{"status":"yielded","id":%q}`, id)}, nil
		case op := <-cell.requests:
			opCtx, release := context.WithCancel(ctx)
			stop := context.AfterFunc(op.ctx, release)
			if op.ctx.Err() != nil {
				release()
			}
			value, err := c.call(opCtx, op.name, op.args)
			stop()
			release()
			op.reply <- nativeResponse{value, err}
		case outcome := <-cell.done:
			h.remove(id)
			c.imports = outcome.result.Imports
			if outcome.err != nil || c.traceErr != nil {
				return tools.Result{}, errors.Join(outcome.err, c.traceErr)
			}
			return tools.Result{
				Content: outcome.result.Output,
				Detail:  fmt.Sprintf("TypeScript · %d operations", c.count),
			}, nil
		}
	}
}
