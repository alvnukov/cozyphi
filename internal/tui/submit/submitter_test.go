package submit

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulseaiclub/phi/internal/components"
	"github.com/pulseaiclub/phi/internal/components/status"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/tui/commands"
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

type recordingComposer struct {
	stubComposer
	clearInputCalls int
}

func (c *recordingComposer) ClearInput() { c.clearInputCalls++ }

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

func TestSubmitter_Submit_unknownSlashFallsThroughToAgent(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "Phi test")
	sub := NewSubmitter(
		nil,
		commands.NewBuiltinRegistry(),
		tp,
		nil,
		stubComposer{},
		nil,
		func() commands.CommandContext { return commands.CommandContext{} },
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	sub.Submit("/not-a-real-command")
	require.Len(t, tp.Snapshot().Messages, 1)
	assert.Equal(t, "/not-a-real-command", tp.Snapshot().Messages[0].Text)
	assert.Equal(t, session.RoleUser, tp.Snapshot().Messages[0].Role)
}

func TestSubmitter_Submit_bareBangFallsThroughToAgent(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "Phi test")
	sub := NewSubmitter(
		nil,
		nil,
		tp,
		nil,
		stubComposer{},
		NewBashRunner(tp, stubComposer{}, nil, nil),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
	)

	sub.Submit("!")
	require.Len(t, tp.Snapshot().Messages, 1)
	assert.Equal(t, "!", tp.Snapshot().Messages[0].Text)
}

func TestSubmitter_SubmitQueuesPromptWhileStreaming(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "Phi test")
	tp.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateStreaming,
	}})
	composer := &recordingComposer{}
	sub := NewSubmitter(nil, nil, tp, nil, composer, nil, nil, nil, nil, nil, nil, nil)

	sub.Submit("follow up")

	require.Equal(t, 1, composer.clearInputCalls)
	require.Len(t, tp.Snapshot().Messages, 2)
	assert.Equal(t, session.RoleUser, tp.Snapshot().Messages[1].Role)
	assert.Equal(t, "follow up", tp.Snapshot().Messages[1].Text)
}
