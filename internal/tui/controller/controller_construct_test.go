package controller

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
)

func TestController_StartPromptQueuesWhileRunning(t *testing.T) {
	bus := NewBus(nil)
	ctrl := &Controller{bus: bus}

	ctrl.StartPrompt("first", nil, "")
	ctrl.StartPrompt("second", nil, "")

	deadline := time.After(time.Second)
	completed := 0
	for completed < 2 {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				event, ok := msg.(SessionEventMsg)
				if !ok {
					continue
				}
				update, ok := event.Event.(session.AssistantMessageUpdate)
				if ok && update.Message.State == session.StateError {
					completed++
				}
			}
		case <-deadline:
			t.Fatalf("completed prompts = %d, want 2", completed)
		}
	}
}

// TestController_DequeuePromotesQueuedUser: when the in-flight turn finishes
// and the controller dequeues the next prompt, it emits UserPromoted so the
// transcript can drop the "(queued)" hint on that row.
func TestController_DequeuePromotesQueuedUser(t *testing.T) {
	bus := NewBus(nil)
	ctrl := &Controller{bus: bus}

	ctrl.StartPrompt("first", nil, "")
	ctrl.StartPrompt("second", nil, "u2")

	deadline := time.After(time.Second)
	promoted := ""
	for promoted == "" {
		select {
		case <-bus.Chan():
			for _, msg := range bus.Drain() {
				event, ok := msg.(SessionEventMsg)
				if !ok {
					continue
				}
				if p, ok := event.Event.(session.UserPromoted); ok {
					promoted = p.ID
				}
			}
		case <-deadline:
			t.Fatal("queued prompt was never promoted")
		}
	}
	if promoted != "u2" {
		t.Fatalf("promoted id = %q, want u2", promoted)
	}
}

func TestNewController_RequiresCollaborators(t *testing.T) {
	bus := NewBus(nil)
	_, err := NewController(nil, &project.Project{}, t.TempDir(), "")
	assert.Error(t, err)

	_, err = NewController(bus, nil, t.TempDir(), "")
	assert.Error(t, err)
}

func TestNewController_ReadyEngine(t *testing.T) {
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

	bus := NewBus(nil)
	ctrl, err := NewController(bus, proj, cwd, "")
	require.NoError(t, err)
	require.NotNil(t, ctrl)
	require.NotNil(t, ctrl.engine)
	assert.Equal(t, cwd, ctrl.cwd)
	assert.NotEmpty(t, ctrl.sessionDir)
	assert.Same(t, proj, ctrl.proj)
}

// TestNewController_ResumesSessionFromFile covers `cozyphi --continue/--resume`:
// the controller boots with its engine on the resumed session, and the
// transcript replay is already available before the first frame.
func TestNewController_ResumesSessionFromFile(t *testing.T) {
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

	sessionDir := proj.SessionDir()
	require.NoError(t, os.MkdirAll(sessionDir, 0o755))
	path := filepath.Join(sessionDir, "sess_resumesess-0001.jsonl")
	content := `{"type":"EntrySession","id":"resumesess-0001","timestamp":"2026-08-23T12:00:00Z","cwd":"/tmp"}` + "\n" +
		`{"type":"EntryMessage","id":"m1","message":{"role":"user","content":"hello resumed"}}` + "\n" +
		`{"type":"EntryMessage","id":"m2","parentID":"m1","message":{"role":"assistant","content":"hi from history"},"usage":{"prompt_tokens":1200,"completion_tokens":80,"total_tokens":1280}}` + "\n" +
		`{"type":"EntryCompaction","id":"c1","parentID":"m2","timestamp":"2026-08-23T12:01:00Z","compaction":{"summary":"resumed summary","firstKeptEntryId":"m1","tokensBefore":1280,"tokensAfter":320,"messagesSummarized":4,"messagesKept":2}}` + "\n" +
		`{"type":"EntryPlan","id":"p1","timestamp":"2026-08-23T12:02:00Z","plan":{"revision":2,"updatedAt":"2026-08-23T12:02:00Z","items":[{"content":"verify resume","status":"in_progress"}]}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))

	bus := NewBus(nil)
	ctrl, err := NewController(bus, proj, cwd, path)
	require.NoError(t, err)

	assert.Equal(t, "resumesess-0001", ctrl.SessionID())

	snap := ctrl.ReplaySnapshot()
	require.Len(t, snap.Messages, 3)
	assert.Equal(t, "hello resumed", snap.Messages[0].Text)
	assert.Equal(t, "hi from history", snap.Messages[1].Text)
	assert.Equal(t, 1200, snap.Messages[1].Usage.PromptTokens, "resumed context usage must be visible immediately")
	assert.Equal(t, session.RoleCompaction, snap.Messages[2].Role)
	assert.Equal(t, 320, snap.Messages[2].Usage.PromptTokens)
	assert.True(t, snap.Messages[2].Usage.Estimated, "compacted context must supersede retained pre-compaction usage")
	require.Len(t, ctrl.Plan().Items, 1)
	assert.Equal(t, "verify resume", ctrl.Plan().Items[0].Content, "plan must be available before the first frame")
}

// TestNewController_BadResumePathFails keeps startup honest: a resume path
// that cannot be opened is a usage error, not a silently fresh session.
func TestNewController_BadResumePathFails(t *testing.T) {
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

	_, err = NewController(NewBus(nil), proj, cwd, filepath.Join(t.TempDir(), "missing.jsonl"))
	require.Error(t, err)
}

func TestRedrawRelay_BindAfterBus(t *testing.T) {
	relay := NewRedrawRelay()
	bus := NewBus(relay.Fire)
	var n int
	relay.Bind(func() { n++ })
	bus.Publish(SubmitMsg{Text: "y"})
	assert.GreaterOrEqual(t, n, 1)

	// Drain so the next Publish can re-arm wake + Fire.
	_ = bus.Drain()
	bus.Publish(SubmitMsg{Text: "z"})
	assert.GreaterOrEqual(t, n, 2)
}
