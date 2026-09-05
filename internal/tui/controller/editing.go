package controller

import (
	"errors"

	"github.com/alvnukov/cozyphi/internal/editmode"
	"github.com/alvnukov/cozyphi/internal/project"
)

func (c *Controller) EditingMode() (editmode.Mode, error) {
	if c == nil || c.proj == nil {
		return editmode.Standard, errors.New("controller not initialized")
	}
	state, err := project.LoadUIState(c.proj.Global())
	if err != nil {
		return editmode.Standard, err
	}
	return editmode.Parse(state.EditingMode)
}

func (c *Controller) SaveEditingMode(mode editmode.Mode) error {
	if c == nil || c.proj == nil {
		return errors.New("controller not initialized")
	}
	if _, err := editmode.Parse(mode.String()); err != nil {
		return err
	}
	return project.MutateUIState(c.proj.Global(), func(s *project.UIState) {
		s.EditingMode = mode.String()
	})
}
