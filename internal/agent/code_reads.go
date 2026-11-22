package agent

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/alvnukov/cozyphi/internal/plangate"
)

const (
	maxJoinedReads      = 8
	concurrentReadLimit = 4
)

type joinedRead struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type joinedResult struct {
	Value any    `json:"value"`
	Error string `json:"error,omitempty"`
}

// readGroup receives only a runtime-created group of ready promises. Every
// member still goes through the ordinary executor, including its actual args.
func (c *codeExecution) readGroup(ctx context.Context, args json.RawMessage) ([]joinedResult, error) {
	var group struct {
		Calls []joinedRead `json:"calls"`
	}
	if err := json.Unmarshal(args, &group); err != nil || len(group.Calls) == 0 || len(group.Calls) > maxJoinedReads {
		return nil, errors.New("invalid joined read group")
	}
	concurrent := !c.executor.activeHooks().HasToolHooks() &&
		(c.executor.plan == nil || !c.executor.plan().Approved)
	for _, call := range group.Calls {
		switch call.Name {
		case "read", "find", "ls":
		default:
			return nil, errors.New("only read, find and ls may participate in a joined read group")
		}
		tool := c.executor.registry[call.Name]
		envelope, _, err := plangate.SplitEnvelope(call.Args)
		if tool.ConcurrentRead == nil || !tool.ConcurrentRead(call.Args) ||
			plangate.StepFromArgs(call.Args) != (plangate.StepRef{}) ||
			!envelope.Empty() ||
			err != nil {
			concurrent = false
		}
	}
	results := make([]joinedResult, len(group.Calls))
	run := func(i int, admit func()) {
		call := group.Calls[i]
		value, err := c.single(ctx, call.Name, call.Args, admit)
		results[i].Value = value
		if err != nil {
			results[i].Error = err.Error()
		}
	}
	if !concurrent {
		for i := range group.Calls {
			run(i, nil)
		}
		return results, nil
	}
	// Gates run in submission order. A worker admits the next call only
	// after its own gates finish. Mutations cannot interleave this group.
	for first := 0; first < len(group.Calls); first += concurrentReadLimit {
		var workers sync.WaitGroup
		for i := first; i < min(first+concurrentReadLimit, len(group.Calls)); i++ {
			ready := make(chan struct{})
			admit := sync.OnceFunc(func() { close(ready) })
			workers.Go(func() {
				defer admit() // a refusal before native dispatch also advances the queue
				run(i, admit)
			})
			<-ready
		}
		workers.Wait()
	}
	return results, nil
}
