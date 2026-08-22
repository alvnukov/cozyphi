package submit

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/tui/controller"
	"github.com/pulseaiclub/phi/internal/tui/transcript"
)

type stubComposer struct {
	skills []string
}

func (stubComposer) HideCompleters()           {}
func (stubComposer) ClearInput()               {}
func (s stubComposer) PendingSkills() []string { return s.skills }
func (stubComposer) ClearPendingSkills()       {}
func (stubComposer) SyncBashBorder(string)     {}
func (stubComposer) CloseMentionSlash()        {}
func (stubComposer) SetBashBorderActive(bool)  {}

func TestSubmitter_IsBusy(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	transcript := transcript.NewTranscriptPane(th, spin, "Phi test")
	bash := NewBashRunner(transcript, stubComposer{}, nil, nil)

	sub := NewSubmitter(nil, nil, transcript, nil, stubComposer{}, bash, nil, nil, nil, nil, nil, nil)

	if sub.IsBusy() {
		t.Fatal("expected idle submitter")
	}
}

func TestSubmitter_StreamActive_activity(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	activity := controller.NewActivityHandler(spin)
	sub := NewSubmitter(
		nil,
		nil,
		transcript.NewTranscriptPane(th, spin, "Phi test"),
		activity,
		stubComposer{},
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	activity.Apply(controller.ActivityWaiting)
	if !sub.StreamActive() {
		t.Fatal("expected stream active while waiting")
	}
}
