package agent

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/session"
)

// activePlanModel also covers the legacy snapshot door and reapproval of an
// already-started step. Neither goes through a pending-to-start transition.
func (engine *Engine) activePlanModel(plan session.Plan) (llm.ModelConfig, bool, error) {
	for _, item := range plan.Items {
		if item.Status == session.PlanInProgress {
			// Legacy items need not have IDs; resolve this item, not the first empty ID.
			plan.Items = []session.PlanItem{item}
			return engine.resolveStepModel(plan, item.ID)
		}
	}
	return llm.ModelConfig{}, false, nil
}

func (engine *Engine) syncApprovedPlanModel(plan session.Plan) error {
	if !plan.Approved {
		engine.restoreSessionModelOnClose()
		return nil
	}
	target, pinned, err := engine.activePlanModel(plan)
	if err != nil {
		engine.restoreSessionModelOnClose()
		// Auto-approval is stamped by the durable writer. Fail closed before any
		// actions: an invalid active override must not leave a usable approval.
		if _, revokeErr := engine.sessionRef().SetPlanApproved(false); revokeErr != nil {
			return fmt.Errorf("%w; could not revoke plan approval: %w", err, revokeErr)
		}
		return err
	}
	return engine.switchStepModel(target, pinned)
}
