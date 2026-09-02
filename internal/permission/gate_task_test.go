package permission

import "testing"

// TestTaskReadsAreFreeAndWritesAreMutations pins the split the task tool
// relies on: looking at the registry costs nothing, changing a note is a
// mutation, and so is refused wherever the session may not change files.
func TestTaskReadsAreFreeAndWritesAreMutations(t *testing.T) {
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
		name string
		mode Mode
		read Decision
		want Decision
	}{
		{"interactive", ModeInteractive, Allow, Allow},
		{"autopilot", ModeAutopilot, Allow, Allow},
		{"readonly", ModeReadonly, Allow, Deny},
	} {
		t.Run(tc.name, func(t *testing.T) {
			policy := DefaultPolicy()
			policy.Mode = tc.mode
			g, err := NewGate(policy, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if dec, reason := g.Check(t.Context(), read); dec != tc.read {
				t.Fatalf("read: want %v, got %v (%s)", tc.read, dec, reason)
			}
			if dec, reason := g.Check(t.Context(), write); dec != tc.want {
				t.Fatalf("write: want %v, got %v (%s)", tc.want, dec, reason)
			}
		})
	}
}
