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

// ShowFooterSpinner reports whether the footer should animate. Streaming owns
// its live status in the transcript, so only the phases without a transcript
// row (plus tools and compaction) animate here.
func (h *ActivityHandler) ShowFooterSpinner() bool {
	if h == nil || h.Current == ActivityStreaming {
		return false
	}
	return h.Current.showSpinner()
}

// FooterLabel returns footer-only activity text. The transcript renders the
// live model and thinking state once streaming starts; duplicating
// "Generating…" below it creates two competing status indicators.
func (h *ActivityHandler) FooterLabel(snap session.Snapshot) string {
	if h == nil || h.Current == ActivityStreaming {
		return ""
	}
	return h.Label(snap)
}

// Label returns the general activity text used outside the footer, including
// the sidebar where streaming remains a useful runtime state.
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
