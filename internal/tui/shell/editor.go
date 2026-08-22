package shell

import (
	"github.com/pulseaiclub/xui"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/app"
	"github.com/pulseaiclub/phi/internal/tui/commands"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

// Editor is a deprecated alias for Shell. Prefer NewShell in new code.
type Editor = Shell

// NewEditor is a deprecated alias for NewShell.
func NewEditor(
	application *app.App,
	bus *controller.Bus,
	ctrl *controller.Controller,
	registry *commands.CommandRegistry,
	vx *xui.XUI,
	theme components.Theme,
	cwd, model, skillPath string,
	contextWindow int,
	modelNames []string,
) *Shell {
	return NewShell(
		application,
		bus,
		ctrl,
		registry,
		vx,
		theme,
		cwd,
		model,
		skillPath,
		contextWindow,
		modelNames,
	)
}
