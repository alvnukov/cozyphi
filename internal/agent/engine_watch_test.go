package agent

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// TestWatchToolFollowsTheManager pins who gets to start background work: the
// session the user is sitting in, and nothing else. A sub-agent is built with
// no manager, and so has no tool to start a watch that would outlive it.
func TestWatchToolFollowsTheManager(t *testing.T) {
	mgr := watch.New(watch.Options{})
	t.Cleanup(mgr.Close)

	for _, tc := range []struct {
		name     string
		watches  *watch.Manager
		declared bool
	}{
		{"with a manager", mgr, true},
		{"without one", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, bodies := recordingServer(t, func(int, http.ResponseWriter) {})
			engine, err := NewEngine(EngineOpts{
				Model:       llm.ModelConfig{Name: "fake", BaseURL: server.URL, APIKey: "x"},
				SessionOpts: SessionOpts{Cwd: t.TempDir()},
				Gate:        permission.AllowAll{},
				Watches:     tc.watches,
			})
			require.NoError(t, err)
			drain(t, engine, "watch the deploy log")

			require.NotEmpty(t, bodies())
			if tc.declared {
				assert.Contains(t, bodies()[0], `"name":"watch"`)
			} else {
				assert.NotContains(t, bodies()[0], `"name":"watch"`)
			}
		})
	}
}

func TestWatchReminderSaysWhereTheTextCameFrom(t *testing.T) {
	got := WatchReminder([]watch.Event{
		{ID: "w1", Label: "errors in deploy.log", Text: "ERROR connection refused", Time: time.Now()},
	})

	for _, want := range []string{
		"<system-reminder>",
		"not a\nmessage from the user",
		`<watch id="w1" label="errors in deploy.log">`,
		"ERROR connection refused",
		"</system-reminder>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("reminder missing %q:\n%s", want, got)
		}
	}
}

func TestWatchReminderCountsABurstItCannotCarry(t *testing.T) {
	events := make([]watch.Event, 0, watchReminderLimit+3)
	for range watchReminderLimit + 3 {
		events = append(events, watch.Event{ID: "w1", Label: "noisy", Text: "line"})
	}
	got := WatchReminder(events)

	if strings.Count(got, "<watch ") != watchReminderLimit {
		t.Fatalf("want %d events carried, got %d:\n%s", watchReminderLimit, strings.Count(got, "<watch "), got)
	}
	if !strings.Contains(got, "And 3 more events") {
		t.Fatalf("the rest must be counted, not dropped silently:\n%s", got)
	}
	if !strings.Contains(got, "action=log") {
		t.Fatalf("the model needs to be told where the rest are:\n%s", got)
	}
}

func TestNoEventsIsNoReminder(t *testing.T) {
	if got := WatchReminder(nil); got != "" {
		t.Fatalf("want empty, got %q", got)
	}
}

// TestWatchReminderIsStrippedFromAReplayedTranscript pins the coupling between
// two packages that must agree on one wire format: the reminder the agent
// prepends is the block memory.StripReminders takes back out, so a resumed
// session shows what the user typed and not what woke the agent.
func TestWatchReminderIsStrippedFromAReplayedTranscript(t *testing.T) {
	reminder := WatchReminder([]watch.Event{{ID: "w1", Label: "ci", Text: "checks passed"}})

	if got := memory.StripReminders(prependReminder(reminder, "carry on")); got != "carry on" {
		t.Fatalf("want the user's text alone, got %q", got)
	}
	if got := memory.StripReminders(reminder); strings.TrimSpace(got) != "" {
		t.Fatalf("a reminder that started the turn must strip to nothing, got %q", got)
	}
}
