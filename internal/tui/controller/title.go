package controller

import (
	"errors"
	"fmt"
)

// SessionTitle returns the durable session's safe display title, including its fallback.
func (c *Controller) SessionTitle() string {
	if c == nil || c.engine == nil {
		return ""
	}
	return c.engine.Session().DisplayTitle()
}

// SetSessionTitle pins a user title on this controller's session, not the selected view.
func (c *Controller) SetSessionTitle(title string) error {
	if c == nil || c.engine == nil {
		return errors.New("cannot rename: no session is open")
	}
	if err := c.engine.Session().SetTitle(title, "user"); err != nil {
		return fmt.Errorf("rename session: %w", err)
	}
	return nil
}
