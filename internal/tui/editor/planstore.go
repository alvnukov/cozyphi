package editor

import (
	"context"
	"errors"

	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// planStore adapts the controller to the plan editor's persistence seam:
// reads return the durable snapshot, writes go through the same
// revision-guarded patch path the model tool uses.
type planStore struct {
	ctrl *controller.Controller
}

var _ planedit.Store = planStore{}

func (s planStore) Snapshot() session.Plan {
	if s.ctrl == nil {
		return session.Plan{}
	}
	return s.ctrl.Plan()
}

func (s planStore) Apply(
	ctx context.Context,
	expectedRevision uint64,
	ops []session.PlanPatchOp,
) (session.Plan, error) {
	if s.ctrl == nil {
		return session.Plan{}, errors.New("editor: controller unavailable")
	}
	return s.ctrl.PatchPlan(ctx, expectedRevision, ops)
}
