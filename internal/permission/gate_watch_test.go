package permission

import (
	"strings"
	"testing"
)

// TestWatchIsJudgedByTheBashPolicy pins the decision that matters: a watch is
// a shell command, so nothing a user forbade in bash becomes reachable by
// wrapping it in a watch.
func TestWatchIsJudgedByTheBashPolicy(t *testing.T) {
	policy := DefaultPolicy()
	policy.BashDefault = Ask
	policy.BashDeny = []string{`rm\s+-rf`}
	policy.BashAllow = []string{`^git status\b`}
	// The allowlist clears a command to run once, not to run forever: a watch
	// asks anyway. Only an allow-everything default starts one unattended.
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name    string
		command string
		want    Decision
	}{
		{"denied stays denied", "rm -rf /", Deny},
		{"allowlisted still asks", "git status", Ask},
		{"anything else asks", "tail -f app.log", Ask},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := ExtractAt("watch", []byte(`{"action":"start","command":`+quote(tc.command)+`}`), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			if req.Action != ActionWatch {
				t.Fatalf("want action %q, got %q", ActionWatch, req.Action)
			}
			if dec, reason := g.Check(t.Context(), req); dec != tc.want {
				t.Fatalf("want %v, got %v (%s)", tc.want, dec, reason)
			}
		})
	}
}

// TestWatchApprovalSaysItKeepsRunning pins the wording: the user is approving
// a command that outlives the tool call, and the ask must not read like a
// one-shot bash run.
func TestWatchApprovalSaysItKeepsRunning(t *testing.T) {
	policy := DefaultPolicy()
	policy.BashDefault = Ask
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dec, reason := g.Check(t.Context(), Request{Action: ActionWatch, Tool: "watch", Command: "tail -f app.log"})
	if dec != Ask {
		t.Fatalf("want Ask, got %v (%s)", dec, reason)
	}
	if !strings.Contains(reason, "background") || !strings.Contains(reason, "tail -f app.log") {
		t.Fatalf("reason should say what keeps running: %q", reason)
	}
}

// TestWatchBookkeepingNeedsNoApproval pins that reading and stopping watches
// is free: those actions address a watch by id and start nothing.
func TestWatchBookkeepingNeedsNoApproval(t *testing.T) {
	policy := DefaultPolicy()
	policy.BashDefault = Deny
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range []string{`{"action":"list"}`, `{"action":"stop","id":"w1"}`, `{"action":"log","id":"w1"}`} {
		req, err := ExtractAt("watch", []byte(args), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		if dec, reason := g.Check(t.Context(), req); dec != Allow {
			t.Fatalf("%s: want Allow, got %v (%s)", args, dec, reason)
		}
	}
}

// TestAllowEverythingStartsWatchesUnattended pins the one policy that lets a
// watch start with no approval — the flag the user set on purpose.
func TestAllowEverythingStartsWatchesUnattended(t *testing.T) {
	policy := DefaultPolicy()
	policy.BashDefault = Allow
	g, err := NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if dec, reason := g.Check(t.Context(), Request{
		Action: ActionWatch, Tool: "watch", Command: "tail -f app.log",
	}); dec != Allow {
		t.Fatalf("want Allow, got %v (%s)", dec, reason)
	}
	// A deny pattern still outranks the default.
	if dec, _ := g.Check(t.Context(), Request{
		Action: ActionWatch, Tool: "watch", Command: "sudo tail -f /var/log/secure",
	}); dec != Deny {
		t.Fatalf("want Deny for a denied command, got %v", dec)
	}
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `\"`) + `"` }
