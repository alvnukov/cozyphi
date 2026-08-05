package tui

import (
	"fmt"
	"time"

	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/session"
)

// Activity mirrors footer session status (driven by the stream pipeline).
type Activity int

const (
	ActivityIdle Activity = iota
	ActivitySubmitting
	ActivityWaiting
	ActivityStreaming
	ActivityTools
	ActivityRetrying
	ActivityCancelled
	ActivityCompacting
	ActivityAwaitingApproval
)

// ActivityHandler owns footer/stream activity state.
// It only mutates itself when Apply / SyncFromSnap are called on the UI goroutine.
type ActivityHandler struct {
	Current Activity
	At      time.Time
	spin    *status.Spinner
}

func NewActivityHandler(spin *status.Spinner) *ActivityHandler {
	return &ActivityHandler{spin: spin}
}

// Apply sets activity from a SetActivityMsg (or direct call on UI thread).
func (h *ActivityHandler) Apply(a Activity) {
	if h == nil {
		return
	}
	h.Current = a
	h.At = time.Now()
	if h.spin != nil {
		h.spin.Frame = 0
	}
}

// SyncFromSnap derives activity from the session snapshot after model updates.
func (h *ActivityHandler) SyncFromSnap(snap session.Snapshot) {
	if h == nil {
		return
	}
	// Don't clobber the approval footer while the confirmation UI is up.
	if h.Current == ActivityAwaitingApproval {
		return
	}
	if snap.Compacting {
		if h.Current != ActivityCompacting {
			h.Apply(ActivityCompacting)
		}
		return
	}
	if session.HasRunningTools(snap) {
		if h.Current != ActivityTools {
			h.Apply(ActivityTools)
		}
		return
	}
	if session.IsStreaming(snap) {
		if h.Current != ActivityStreaming && h.Current != ActivityWaiting &&
			h.Current != ActivitySubmitting && h.Current != ActivityCompacting {
			h.Apply(ActivityStreaming)
		}
		return
	}
	switch h.Current {
	case ActivityStreaming, ActivityWaiting, ActivitySubmitting, ActivityTools, ActivityCompacting:
		h.Current = ActivityIdle
	}
}

func (h *ActivityHandler) ShowSpinner() bool {
	if h == nil {
		return false
	}
	return h.Current.showSpinner()
}

func (h *ActivityHandler) Label(snap session.Snapshot) string {
	if h == nil {
		return ""
	}
	if h.Current == ActivityTools {
		n := session.RunningToolCount(snap)
		if n > 1 {
			return fmt.Sprintf("Calling %d tools…", n)
		}
	}
	return activityMessage(h.Current)
}

func activityMessage(a Activity) string {
	switch a {
	case ActivitySubmitting:
		return "Sending…"
	case ActivityWaiting:
		return "Awaiting reply…"
	case ActivityStreaming:
		return "Generating…"
	case ActivityTools:
		return "Calling tools…"
	case ActivityCompacting:
		return "Auto-compacting…"
	case ActivityRetrying:
		return "Retrying after disconnect…"
	case ActivityCancelled:
		return "Stopped"
	case ActivityAwaitingApproval:
		return "Waiting for approval..."
	default:
		return ""
	}
}

func (a Activity) showSpinner() bool {
	switch a {
	case ActivitySubmitting, ActivityWaiting, ActivityStreaming, ActivityTools, ActivityRetrying, ActivityCompacting:
		return true
	default:
		return false
	}
}
