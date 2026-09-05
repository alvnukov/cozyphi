package sessions

import (
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
)

// syncModelControls follows the actual execution configuration, including
// resumed sessions and plan-step overrides, rather than the startup label.
func (e *View) syncModelControls() {
	if e.composer == nil || e.ctrl == nil {
		return
	}
	if e.ctrl.SyncQuotaSelection() {
		// The selection moved to another provider: nothing fetched for the
		// old one still describes this session.
		if e.usagepane != nil {
			e.usagepane.InvalidateReset()
		}
		if e.sidebar != nil {
			e.sidebar.ClearQuota()
		}
		e.refreshQuota()
	}
	if e.status.Visible() {
		cfg := e.ctrl.ModelConfig()
		e.status.SetModel(cfg.Name, cfg.ProviderID)
	}
	name, effort := session.ParseModelRef(e.ctrl.ModelRef())
	e.composer.SetModelLabel(e.ctrl.ModelLabel())
	if name == "" {
		name = session.NoModelLabel
	}
	e.composer.Chat.ModelName = name
	e.composer.Chat.ModelStateLabel = modelStateLabel(e.ctrl.ModelSelectionStatus())
	e.composer.Chat.EffortLabel = ""
	if len(e.ModelEfforts(name)) > 0 {
		if effort == "" {
			effort = "default"
		}
		e.composer.Chat.EffortLabel = effort
	}
}

func modelStateLabel(status controller.ModelSelectionStatus) string {
	label := func(model agent.ModelSelection) string {
		if model.Effort != "" {
			return model.Name + "[" + string(model.Effort) + "]"
		}
		return model.Name
	}
	var parts []string
	if status.Pending {
		parts = append(parts, "next; running "+label(status.Effective))
	}
	if status.Selected != status.Next {
		parts = append(parts, "selected "+label(status.Selected))
	}
	return strings.Join(parts, "; ")
}

// pickModelEffort surfaces invalid selections on the
// mouse/keyboard picker path, whose command callback cannot return an error.
func (e *View) pickModelEffort(name, effort string) error {
	err := e.SetModelEffort(name, effort)
	if err != nil {
		e.Toast(err.Error(), toast.ToastError, 5*time.Second)
	}
	return err
}

func (e *View) openCurrentEffortPicker() {
	name, _ := session.ParseModelRef(e.ctrl.ModelRef())
	if name == "" {
		e.OpenModelPicker()
		return
	}
	if len(e.ModelEfforts(name)) == 0 {
		e.Toast("This model has no selectable reasoning effort", toast.ToastWarning, 3*time.Second)
		return
	}
	e.OpenModelEffortPicker(name)
}
