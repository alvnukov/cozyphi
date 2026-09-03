package permission

import (
	"strings"
	"testing"

	"github.com/alvnukov/cozyphi/internal/tasks"
)

// TestTaskAccessDecidesInEveryMode pins what the task tool relies on: the
// registry level is the user's setting, and it holds the same in every
// mode. A read is free unless the registry is off; a write follows the
// level — and under ask it keeps asking even in readonly, because a note is
// bookkeeping about the work rather than a change to it, and the user who
// chose to be asked is there to answer. Only the modes with nobody to ask
// fold the question into a refusal.
func TestTaskAccessDecidesInEveryMode(t *testing.T) {
	read, err := ExtractAt("task", []byte(`{"action":"current"}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if read.Action != ActionTaskRead {
		t.Fatalf("want %q, got %q", ActionTaskRead, read.Action)
	}
	write, err := ExtractAt("task", []byte(`{"action":"done","id":"fix-login","note":"merged"}`), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if write.Action != ActionTaskWrite || write.Target != "fix-login" {
		t.Fatalf("want %q on fix-login, got %+v", ActionTaskWrite, write)
	}
	if bare, _ := ExtractAt("task", []byte(`{}`), t.TempDir()); bare.Action != ActionTaskRead {
		t.Fatalf("an empty call is current, a read; got %q", bare.Action)
	}

	for _, tc := range []struct {
		level tasks.Access
		mode  Mode
		read  Decision
		write Decision
	}{
		{tasks.AccessWrite, ModeInteractive, Allow, Allow},
		{tasks.AccessWrite, ModeReadonly, Allow, Allow},
		{tasks.AccessWrite, ModeAutopilot, Allow, Allow},
		{"", ModeReadonly, Allow, Allow}, // the empty level is the default: write
		{tasks.AccessAsk, ModeInteractive, Allow, Ask},
		{tasks.AccessAsk, ModeReadonly, Allow, Ask},
		{tasks.AccessAsk, ModeAutopilot, Allow, Deny},
		{tasks.AccessAsk, ModeHeadlessStrict, Allow, Deny},
		{tasks.AccessRead, ModeInteractive, Allow, Deny},
		{tasks.AccessRead, ModeReadonly, Allow, Deny},
		{tasks.AccessOff, ModeInteractive, Deny, Deny},
		{tasks.AccessOff, ModeReadonly, Deny, Deny},
	} {
		t.Run(string(tc.level)+"/"+string(tc.mode), func(t *testing.T) {
			policy := DefaultPolicy()
			policy.Mode = tc.mode
			policy.Tasks = tc.level
			g, err := NewGate(policy, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if dec, reason := g.Check(t.Context(), read); dec != tc.read {
				t.Fatalf("read: want %v, got %v (%s)", tc.read, dec, reason)
			}
			if dec, reason := g.Check(t.Context(), write); dec != tc.write {
				t.Fatalf("write: want %v, got %v (%s)", tc.write, dec, reason)
			}
		})
	}

	// A refusal under read tells the model what to do instead, and names
	// the setting so the user can find it.
	policy := DefaultPolicy()
	policy.Tasks = tasks.AccessRead
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, reason := g.Check(t.Context(), write); !strings.Contains(reason, "permissions.tasks: read") ||
		!strings.Contains(reason, "describe the change") {
		t.Fatalf("the reason must name the setting and the way out: %q", reason)
	}
}
