package agent

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// NewJobManager creates a process-level job manager whose runner drives child Engines.
// modelFn may be nil; then model is used as a fixed snapshot.
// hooksFn supplies hooks for child engines (may return nil); prefer a live
// getter so TUI reload updates sub-agents too.
func NewJobManager(
	root string,
	model llm.ModelConfig,
	modelFn func() llm.ModelConfig,
	hooksFn func() *hooks.Manager,
	lspQuery tools.LSPQueryFunc,
) (*job.Manager, error) {
	if root == "" {
		return nil, errors.New("agent: jobs root is required")
	}
	return job.New(job.Options{
		Root: root,
		Runner: EngineRunner{
			Model:   model,
			ModelFn: modelFn,
			HooksFn: hooksFn,
			LSP:     lspQuery,
		},
	})
}
