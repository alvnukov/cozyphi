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
	ActivityListening
	ActivityVoicePaused
	ActivityTranscribing
	ActivityDownloadingModel
)

// isVoice reports whether the activity comes from voice input rather than
// from the run pipeline.
func (a Activity) isVoice() bool {
	return a == ActivityListening || a == ActivityVoicePaused || a == ActivityTranscribing ||
		a == ActivityDownloadingModel
}

// isRun reports whether the activity describes a run in progress. The run
// owns the footer while it lasts.
func (a Activity) isRun() bool {
	switch a {
	case ActivitySubmitting, ActivityWaiting, ActivityStreaming, ActivityTools,
		ActivityCompacting, ActivityAwaitingApproval:
		return true
	default:
		return false
	}
}

// ActivityHandler owns footer/stream activity state.
// It only mutates itself when Apply is called on the UI goroutine.
type ActivityHandler struct {
	Current Activity
	// detail is the optional suffix of the current activity, such as the
	// percentage of a model download. It dies with the activity.
	detail string
	spin   *status.Spinner
}

// NewActivityHandler builds an ActivityHandler that owns the given spinner.
func NewActivityHandler(spin *status.Spinner) *ActivityHandler {
	return &ActivityHandler{spin: spin}
}

// Apply sets activity from a SetActivityMsg (or direct call on UI thread).
func (h *ActivityHandler) Apply(a Activity) {
	h.ApplyDetail(a, "")
}

// ApplyDetail sets activity together with the suffix the footer appends to
// its label. Re-applying the same activity keeps the label and only refreshes
// the suffix, so a download percentage does not restart the spinner.
func (h *ActivityHandler) ApplyDetail(a Activity, detail string) {
	if h == nil {
		return
	}
	// Voice is a side channel: a microphone must never hide what the run is
	// doing, so a running pipeline keeps the footer.
	if a.isVoice() && h.Current.isRun() {
		return
	}
	same := h.Current == a
	h.Current = a
	h.detail = detail
	if h.spin != nil && !same {
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
	label := activityMessage(h.Current)
	if label != "" && h.detail != "" {
		label += " " + h.detail
	}
	return label
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
	case ActivityListening:
		return "Listening…"
	case ActivityVoicePaused:
		return "Voice paused"
	case ActivityTranscribing:
		return "Transcribing…"
	case ActivityDownloadingModel:
		return "Downloading model…"
	default:
		return ""
	}
}

func (a Activity) showSpinner() bool {
	switch a {
	case ActivitySubmitting, ActivityWaiting, ActivityStreaming, ActivityTools, ActivityCompacting,
		ActivityListening, ActivityTranscribing, ActivityDownloadingModel:
		return true
	default:
		return false
	}
}
