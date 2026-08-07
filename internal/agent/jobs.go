package agent

import (
	"fmt"

	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
)

// NewJobManager creates a process-level job manager whose runner drives child Engines.
// modelFn may be nil; then model is used as a fixed snapshot.
func NewJobManager(root string, model llm.ModelConfig, modelFn func() llm.ModelConfig) (*job.Manager, error) {
	if root == "" {
		return nil, fmt.Errorf("agent: jobs root is required")
	}
	return job.New(job.Options{
		Root: root,
		Runner: EngineRunner{
			Model:   model,
			ModelFn: modelFn,
		},
	})
}
