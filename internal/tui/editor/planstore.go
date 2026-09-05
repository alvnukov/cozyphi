package editor

import (
	"context"
	"errors"

	"github.com/alvnukov/cozyphi/internal/components/palette"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/planedit"
)

// planStore adapts the controller to the plan editor's persistence seam:
// reads return the durable snapshot, writes go through the same
// revision-guarded patch path the model tool uses.
type planStore struct {
	ctrl     *controller.Controller
	commands *commands.CommandRegistry
}

var _ planedit.Store = planStore{}

func (s planStore) Snapshot() session.Plan {
	if s.ctrl == nil {
		return session.Plan{}
	}
	return s.ctrl.Plan()
}

func (s planStore) StepTypes() []session.StepType {
	if s.ctrl == nil || s.ctrl.PlanRuntime() == nil {
		return nil
	}
	names := s.ctrl.PlanRuntime().Current().StepTypes()
	types := make([]session.StepType, len(names))
	for i, name := range names {
		types[i] = session.StepType(name)
	}
	return types
}

// ModelPicker binds plan draft choices to the same history and catalog as /model.
func (s planStore) ModelPicker(onPick func(string, string) error) palette.PaletteCommand {
	var names []string
	var efforts func(string) []string
	if s.ctrl != nil {
		names = s.ctrl.ModelNames()
		efforts = s.ctrl.ModelEfforts
	}
	return s.commands.ModelPickerPage(onPick, names, efforts)
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

// Create stores the first plan of a planless session: a patch has nothing
// to diff against until a contract exists.
func (s planStore) Create(ctx context.Context, contract session.PlanV2) (session.Plan, error) {
	if s.ctrl == nil {
		return session.Plan{}, errors.New("editor: controller unavailable")
	}
	return s.ctrl.CreatePlan(ctx, contract)
}
