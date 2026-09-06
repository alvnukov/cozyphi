package agent

import (
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/sessiontool"
)

// The closure captures the owning store, not the foreground tab or a later
// ReplaceSession target. An in-flight executor never renames another session.
func (engine *Engine) titleTool() tools.Tool {
	owner := engine.session
	return sessiontool.Tool(func(title string) (string, error) {
		if err := owner.SetTitle(title, "model"); err != nil {
			return "", err
		}
		saved, _ := owner.Title()
		return saved, nil
	})
}

func (engine *Engine) titleInstruction() string {
	if !engine.sessionNaming {
		return ""
	}
	return "\n\n# Session naming\n\n" + sessiontool.Instruction
}
