package agent

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// NewJobManager creates a process-level job manager whose runner drives child Engines.
// modelFn may be nil; then model is used as a fixed snapshot. modelForRole
// may be nil; when a role resolves (agents.models) it overrides model/modelFn
// for that role's children, so unset roles keep inheriting the session model.
func NewJobManager(
	root string,
	model llm.ModelConfig,
	modelFn func() llm.ModelConfig,
	modelForRole func(job.Role) (llm.ModelConfig, bool),
	hooksFn func() *hooks.Manager,
	lspQuery tools.LSPQueryFunc,
) (*job.Manager, error) {
	if root == "" {
		return nil, errors.New("agent: jobs root is required")
	}
	// The spawn surface shows the same name the runner will resolve; a
	// display-name view of the pin keeps job free of llm types.
	var modelNameForRole func(job.Role) (string, bool)
	if modelForRole != nil {
		modelNameForRole = func(role job.Role) (string, bool) {
			m, ok := modelForRole(role)
			if !ok {
				return "", false
			}
			return m.Name, true
		}
	}
	return job.New(job.Options{
		Root:             root,
		ModelNameForRole: modelNameForRole,
		Runner: EngineRunner{
			Model:        model,
			ModelFn:      modelFn,
			ModelForRole: modelForRole,
			HooksFn:      hooksFn,
			LSP:          lspQuery,
		},
	})
}
