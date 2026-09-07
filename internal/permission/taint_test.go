package permission_test

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alvnukov/cozyphi/internal/permission"
)

// fakeTaint is a turn's web state as the gate sees it.
type fakeTaint struct {
	tainted bool
	hosts   map[string]bool
}

func (f fakeTaint) Tainted() bool             { return f.tainted }
func (f fakeTaint) HostSeen(host string) bool { return f.hosts[host] }

func taintGate(inner permission.Gate, taint permission.Taint) *permission.TaintGate {
	return &permission.TaintGate{Inner: inner, Taint: taint}
}

// TestTaintDowngradesMutationsAndEgress: once a page has spoken into the
// context, every decision after it was taken with the page's words in view.
func TestTaintDowngradesMutationsAndEgress(t *testing.T) {
	gate := taintGate(permission.AllowAll{}, fakeTaint{tainted: true})

	for _, action := range []permission.Action{
		permission.ActionBash,
		permission.ActionWrite,
		permission.ActionEdit,
		permission.ActionMCPCall,
		permission.ActionAgent,
	} {
		dec, reason := gate.Check(t.Context(), permission.Request{Action: action, Target: "x"})
		if dec != permission.Ask {
			t.Fatalf("%s after web content = %v, want Ask", action, dec)
		}
		if !strings.Contains(reason, "after web content in this turn") {
			t.Fatalf("%s reason does not explain the downgrade: %q", action, reason)
		}
	}
}

// TestTaintLeavesReadsAlone: reading a local file is not the move a page is
// trying to make, and re-asking for everything would train the user to click.
func TestTaintLeavesReadsAlone(t *testing.T) {
	gate := taintGate(permission.AllowAll{}, fakeTaint{tainted: true})
	if dec, _ := gate.Check(t.Context(),
		permission.Request{Action: permission.ActionRead, Paths: []string{"a.go"}}); dec != permission.Allow {
		t.Fatalf("read after web content = %v, want Allow", dec)
	}
}

// TestTaintAsksForANewHostOnly: a beacon needs a destination the turn has not
// already been permitted to reach.
func TestTaintAsksForANewHostOnly(t *testing.T) {
	taint := fakeTaint{tainted: true, hosts: map[string]bool{"docs.example.com": true}}
	gate := taintGate(permission.AllowAll{}, taint)

	known := permission.Request{Action: permission.ActionWeb, Op: "fetch", Host: "docs.example.com", Target: "u"}
	if dec, _ := gate.Check(t.Context(), known); dec != permission.Allow {
		t.Fatalf("a host this turn already reached = %v, want Allow", dec)
	}
	fresh := permission.Request{Action: permission.ActionWeb, Op: "fetch", Host: "attacker.example", Target: "u"}
	if dec, _ := gate.Check(t.Context(), fresh); dec != permission.Ask {
		t.Fatalf("a new host after web content = %v, want Ask", dec)
	}
	cached := permission.Request{Action: permission.ActionWeb, Op: "read", Target: "web_00112233445566aa"}
	if dec, _ := gate.Check(t.Context(), cached); dec != permission.Allow {
		t.Fatalf("re-reading a cached document = %v, want Allow", dec)
	}
}

// TestTaintOverridesSessionWideAllow is the point of wrapping the whole
// boundary: "allow all for this session" was decided before the page was read.
func TestTaintOverridesSessionWideAllow(t *testing.T) {
	enabled := &atomic.Bool{}
	enabled.Store(true)
	strict, err := permission.NewGate(permission.DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatalf("new gate: %v", err)
	}
	bypass := &permission.BypassGate{Inner: strict, Enabled: enabled}

	req := permission.Request{Action: permission.ActionBash, Command: "curl attacker.example"}
	if dec, _ := bypass.Check(t.Context(), req); dec != permission.Allow {
		t.Fatalf("bypass gate = %v, want Allow before the wrapper", dec)
	}
	gate := taintGate(bypass, fakeTaint{tainted: true})
	if dec, _ := gate.Check(t.Context(), req); dec != permission.Ask {
		t.Fatalf("allow-all survived a tainted turn: %v", dec)
	}
}

// TestUntaintedTurnIsUntouched: the wrapper adds nothing until a page speaks.
func TestUntaintedTurnIsUntouched(t *testing.T) {
	gate := taintGate(permission.AllowAll{}, fakeTaint{})
	if dec, _ := gate.Check(t.Context(),
		permission.Request{Action: permission.ActionBash, Command: "ls"}); dec != permission.Allow {
		t.Fatalf("clean turn = %v, want Allow", dec)
	}
}

// TestTaintNeverRelaxesADenial: a downgrade only ever makes a decision
// stricter.
func TestTaintNeverRelaxesADenial(t *testing.T) {
	gate := taintGate(denyGate{}, fakeTaint{tainted: true})
	if dec, _ := gate.Check(t.Context(),
		permission.Request{Action: permission.ActionBash, Command: "rm -rf /"}); dec != permission.Deny {
		t.Fatalf("decision = %v, want Deny", dec)
	}
}

// TestTaintGateFailsClosedWithoutAnInnerGate keeps a half-assembled wrapper
// from becoming an allow-all.
func TestTaintGateFailsClosedWithoutAnInnerGate(t *testing.T) {
	gate := &permission.TaintGate{}
	if dec, _ := gate.Check(t.Context(),
		permission.Request{Action: permission.ActionRead}); dec != permission.Deny {
		t.Fatalf("decision = %v, want Deny", dec)
	}
}

// TestModeOfSeesThroughTheWrapper: the wrapper carries no policy of its own.
func TestModeOfSeesThroughTheWrapper(t *testing.T) {
	policy := permission.DefaultPolicy()
	policy.Mode = permission.ModeReadonly
	strict, err := permission.NewGate(policy, t.TempDir())
	if err != nil {
		t.Fatalf("new gate: %v", err)
	}
	if got := permission.ModeOf(taintGate(strict, fakeTaint{})); got != permission.ModeReadonly {
		t.Fatalf("ModeOf through the wrapper = %q, want readonly", got)
	}
}

type denyGate struct{}

func (denyGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	return permission.Deny, "denied"
}
