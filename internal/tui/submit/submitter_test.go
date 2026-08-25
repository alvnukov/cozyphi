package submit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/components"
	"github.com/alvnukov/cozyphi/internal/components/status"
	"github.com/alvnukov/cozyphi/internal/components/toast"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tui/commands"
	"github.com/alvnukov/cozyphi/internal/tui/controller"
	"github.com/alvnukov/cozyphi/internal/tui/transcript"
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

func TestSubmitter_CanSubmit(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")

	sub := NewSubmitter(nil, nil, tp, nil, stubComposer{}, nil, nil, nil, nil, nil, nil, nil)
	if !sub.CanSubmit() {
		t.Fatal("idle submitter must accept prompts")
	}

	bash := NewBashRunner(tp, stubComposer{}, nil, nil)
	bash.running.Store(true)
	sub = NewSubmitter(nil, nil, tp, nil, stubComposer{}, bash, nil, nil, nil, nil, nil, nil)
	if sub.CanSubmit() {
		t.Fatal("local shell run must block submit")
	}

	sub = NewSubmitter(nil, nil, tp, nil, stubComposer{}, nil, nil, nil,
		func() bool { return true }, nil, nil, nil)
	if sub.CanSubmit() {
		t.Fatal("permission overlay must block submit")
	}

	sub = NewSubmitter(nil, nil, tp, nil, stubComposer{}, nil, nil, nil, nil,
		func() bool { return true }, nil, nil)
	if sub.CanSubmit() {
		t.Fatal("continue overlay must block submit")
	}
}

// The gate must consult the controller synchronously: StartPrompt flips the
// run active before the first stream event, and that window is exactly where
// double submits used to slip through the activity ladder.
func TestSubmitter_CanSubmitRunActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	ctrl, err := controller.NewController(controller.NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	sub := NewSubmitter(ctrl, nil, transcript.NewTranscriptPane(th, spin, "CozyPhi test"),
		nil, stubComposer{}, nil, nil, nil, nil, nil, nil, nil)

	if !sub.CanSubmit() {
		t.Fatal("fresh controller must accept prompts")
	}
	ctrl.StartPrompt("run", nil)
	if sub.CanSubmit() {
		t.Fatal("in-flight run must block submit")
	}
	ctrl.Cancel()
}

func TestSubmitter_Submit_unknownSlashFallsThroughToAgent(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")
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
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")
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
	activity := controller.NewActivityHandler(spin)
	activity.Apply(controller.ActivityStreaming)
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")
	tp.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateStreaming,
	}})
	composer := &recordingComposer{}
	sub := NewSubmitter(nil, nil, tp, activity, composer, nil, nil, nil, nil, nil, nil, nil)

	sub.Submit("follow up")

	require.Equal(t, 1, composer.clearInputCalls)
	require.Len(t, tp.Snapshot().Messages, 2)
	assert.Equal(t, session.RoleUser, tp.Snapshot().Messages[1].Role)
	assert.Equal(t, "follow up", tp.Snapshot().Messages[1].Text)
	// No controller is wired here, so the submitter cannot see a run and
	// stamps its own waiting label; the streaming-label path is covered by
	// TestSubmitter_CanSubmitRunActive.
	assert.Equal(t, controller.ActivityWaiting, activity.Current)
}

// TestSubmitter_SubmitMarksQueuedWhileRunActive: a submit accepted while the
// controller reports an in-flight run must carry the queued flag into the
// transcript, so the UI can render it as waiting rather than as sent.
func TestSubmitter_SubmitMarksQueuedWhileRunActive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("COZYPHI_MODEL", "test-model")
	t.Setenv("COZYPHI_API_KEY", "test-key")
	t.Setenv("COZYPHI_BASE_URL", "http://127.0.0.1:9")

	cwd := t.TempDir()
	proj, err := project.Discover(cwd)
	require.NoError(t, err)
	require.NoError(t, proj.LoadConfig())
	ctrl, err := controller.NewController(controller.NewBus(nil), proj, cwd, "")
	require.NoError(t, err)

	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	activity := controller.NewActivityHandler(spin)
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")
	tp.ApplySession(session.AssistantMessageUpdate{Message: session.Message{
		ID:    "a1",
		State: session.StateStreaming,
	}})
	sub := NewSubmitter(ctrl, nil, tp, activity, stubComposer{}, nil, nil, nil, nil, nil, nil, nil)

	ctrl.StartPrompt("first", nil) // makes RunActive true
	sub.Submit("follow up")

	msgs := tp.Snapshot().Messages
	require.Len(t, msgs, 2)
	require.Equal(t, session.RoleUser, msgs[1].Role)
	require.Equal(t, "follow up", msgs[1].Text)
	require.True(t, msgs[1].Queued, "submit while a run is active must mark the message queued")

	ctrl.Cancel()
}

func TestSubmitter_SubmitWhileBashRunsShowsReasonAndPreservesInput(t *testing.T) {
	th := components.DefaultTheme()
	spin := status.NewSpinner(th.ToolName)
	tp := transcript.NewTranscriptPane(th, spin, "CozyPhi test")
	composer := &recordingComposer{}
	var notice string
	bash := NewBashRunner(tp, composer, func(msg string, _ toast.ToastKind, _ time.Duration) {
		notice = msg
	}, nil)
	bash.running.Store(true)
	sub := NewSubmitter(nil, nil, tp, nil, composer, bash, nil, nil, nil, nil, nil, nil)

	sub.Submit("run after shell")

	assert.Contains(t, notice, "shell command is running")
	assert.Zero(t, composer.clearInputCalls)
	assert.Empty(t, tp.Snapshot().Messages)
}
