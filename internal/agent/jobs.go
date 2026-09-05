package agent

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// JobRunnerFactory binds child execution to an engine's current tool snapshot.
// It runs during tool binding/inspection under engine.mu (or construction), so
// it must be cheap, side-effect-free, and must not call locking Engine methods.
// The returned runner must retain the supplied model/hooks/LSP and any resolved
// role models, not callbacks into mutable session configuration. ModelConfig's
// reference fields and borrowed hooks/LSP must be treated as immutable snapshots.
// For pinned-model display, the runner may implement
// ModelNameForRole(job.Role) (string, bool); absent that method, tools report
// "inherit", never the shared manager's default pin. EngineRunner implements it.
// A nil result fails spawn admission; a nil factory keeps legacy Jobs behavior.
// The runner is captured with the executor and remains valid for its whole round.
// No manager ownership or lifecycle authority is transferred to the runner.
//
// A native factory returns EngineRunner with fixed Model, Hooks and LSP, and an
// immutable ModelForRole lookup; leave ModelFn and HooksFn nil.
type JobRunnerFactory func(llm.ModelConfig, *hooks.Manager, tools.LSPQueryFunc) job.Runner

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
