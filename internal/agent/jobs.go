package agent

import (
	"fmt"

	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
)

// NewJobManager creates a process-level job manager whose runner drives child Engines.
// modelFn may be nil; then model is used as a fixed snapshot.
// hooksFn supplies hooks for child engines (may return nil); prefer a live
// getter so TUI reload updates sub-agents too.
func NewJobManager(root string, model llm.ModelConfig, modelFn func() llm.ModelConfig, hooksFn func() *hooks.Manager) (*job.Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("agent: jobs root is required")
	}
	return job.New(job.Options{
		Root: root,
		Runner: EngineRunner{
			Model:   model,
			ModelFn: modelFn,
			HooksFn: hooksFn,
		},
	})
}
