package tui

import (
	"testing"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/tui/controller"
)

type stubComposer struct {
	skills []string
}

func (s stubComposer) HideCompleters()          {}
func (s stubComposer) ClearInput()              {}
func (s stubComposer) PendingSkills() []string  { return s.skills }
func (s stubComposer) ClearPendingSkills()      {}
func (s stubComposer) SyncBashBorder(string)    {}
func (s stubComposer) CloseMentionSlash()       {}
func (s stubComposer) SetBashBorderActive(bool) {}

func TestSubmitter_IsBusy(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	transcript := NewTranscriptPane(th, spin, "Phi test")
	bash := NewBashRunner(BashRunnerDeps{Transcript: transcript, Composer: stubComposer{}})

	sub := NewSubmitter(SubmitterDeps{
		Transcript: transcript,
		Composer:   stubComposer{},
		Bash:       bash,
	})

	if sub.IsBusy() {
		t.Fatal("expected idle submitter")
	}
}

func TestSubmitter_StreamActive_activity(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	activity := controller.NewActivityHandler(spin)
	sub := NewSubmitter(SubmitterDeps{
		Transcript: NewTranscriptPane(th, spin, "Phi test"),
		Activity:   activity,
		Composer:   stubComposer{},
	})

	activity.Apply(controller.ActivityWaiting)
	if !sub.StreamActive() {
		t.Fatal("expected stream active while waiting")
	}
}
