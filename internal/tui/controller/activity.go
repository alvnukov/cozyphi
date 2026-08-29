package controller

import (
	"fmt"

	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/session"
)

// Activity mirrors footer session status (driven by the stream pipeline).
type Activity int

// Activity values map to footer status messages shown while the pipeline runs.
const (
	ActivityIdle Activity = iota
	ActivitySubmitting
	ActivityWaiting
	ActivityStreaming
	ActivityTools
	ActivityCancelled
	ActivityCompacting
	ActivityAwaitingApproval
)

// ActivityHandler owns footer/stream activity state.
// It only mutates itself when Apply is called on the UI goroutine.
type ActivityHandler struct {
	Current Activity
	spin    *status.Spinner
}

// NewActivityHandler builds an ActivityHandler that owns the given spinner.
func NewActivityHandler(spin *status.Spinner) *ActivityHandler {
	return &ActivityHandler{spin: spin}
}

// Apply sets activity from a SetActivityMsg (or direct call on UI thread).
func (h *ActivityHandler) Apply(a Activity) {
	if h == nil {
		return
	}
	h.Current = a
	if h.spin != nil {
		h.spin.Frame = 0
	}
}

// ShowSpinner reports whether the current activity animates a spinner.
func (h *ActivityHandler) ShowSpinner() bool {
	if h == nil {
		return false
	}
	return h.Current.showSpinner()
}

// Label returns the footer text for the current activity and session snapshot.
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

// ShowFooterSpinner reports whether the footer paints its own spinner. It
// mirrors the shared ticker — including streaming, where the transcript's
// "model · thinking" wave carries the feed while the footer names who runs.
func (h *ActivityHandler) ShowFooterSpinner() bool {
	if h == nil {
		return false
	}
	return h.Current.showSpinner()
}

// FooterLabel returns the activity text the footer paints. While streaming
// it names the model producing the round — the transcript wave shows
// progress, the footer says who — falling back to the generic label until
// the provider names the model.
func (h *ActivityHandler) FooterLabel(snap session.Snapshot) string {
	if h == nil {
		return ""
	}
	if h.Current == ActivityStreaming {
		if m := session.StreamingModel(snap); m != "" {
			return m
		}
	}
	return h.Label(snap)
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
		return "Compacting…"
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
	case ActivitySubmitting, ActivityWaiting, ActivityStreaming, ActivityTools, ActivityCompacting:
		return true
	default:
		return false
	}
}
