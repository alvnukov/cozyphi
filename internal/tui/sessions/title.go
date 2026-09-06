package sessions

import (
	"time"

	"github.com/alvnukov/cozyphi/internal/components/toast"
)

// DisplayTitle resolves the current durable session on every draw. A retained
// View can resume or clear without changing its registry membership or pointer.
func (e *View) DisplayTitle() string {
	if e == nil || e.ctrl == nil {
		return ""
	}
	return e.ctrl.SessionTitle()
}

// RenameSession belongs to the command's originating View, even when hidden.
func (e *View) RenameSession(title string) error {
	if err := e.ctrl.SetSessionTitle(title); err != nil {
		return err
	}
	e.Toast("Session renamed: "+e.DisplayTitle(), toast.ToastSuccess, 2*time.Second)
	e.RequestRedraw()
	return nil
}

// DisplayName uses durable titles for conversations and preserves job labels.
func (e Entry) DisplayName() string {
	if e.View != nil && e.View.ctrl != nil && e.View.ctrl.Assignment().JobID != "" {
		return e.Name
	}
	if title := e.View.DisplayTitle(); title != "" {
		return title
	}
	return e.Name
}
