package editor

import (
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/session"
)

// syncModelControls follows the actual execution configuration, including
// resumed sessions and plan-step overrides, rather than the startup label.
func (e *Editor) syncModelControls() {
	if e.composer == nil || e.ctrl == nil {
		return
	}
	if e.ctrl.SyncQuotaSelection() && e.usagepane != nil {
		e.usagepane.InvalidateReset()
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
	e.composer.Chat.EffortLabel = ""
	if len(e.ModelEfforts(name)) > 0 {
		if effort == "" {
			effort = "default"
		}
		e.composer.Chat.EffortLabel = effort
	}
}

// pickModelEffort surfaces rejection (for example, an active run) on the
// mouse/keyboard picker path, whose command callback cannot return an error.
func (e *Editor) pickModelEffort(name, effort string) error {
	err := e.SetModelEffort(name, effort)
	if err != nil {
		e.Toast(err.Error(), toast.ToastError, 5*time.Second)
	}
	return err
}

func (e *Editor) openCurrentEffortPicker() {
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
