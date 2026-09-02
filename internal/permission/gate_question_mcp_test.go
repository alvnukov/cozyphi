package permission

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExtractQuestionAndMCP(t *testing.T) {
	req, err := Extract("question", json.RawMessage(`{"questions":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionQuestion {
		t.Fatalf("question: want %q, got %q", ActionQuestion, req.Action)
	}

	req, err = Extract("mcp_list", json.RawMessage(`{"server":"github"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionMCPList {
		t.Fatalf("mcp_list: want %q, got %q", ActionMCPList, req.Action)
	}

	req, err = Extract("mcp_inspect", json.RawMessage(`{"server":"github","tool":"create_issue"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionMCPInspect {
		t.Fatalf("mcp_inspect: want %q, got %q", ActionMCPInspect, req.Action)
	}

	req, err = Extract("mcp_call", json.RawMessage(`{"server":"github","tool":"create_issue"}`))
	if err != nil {
		t.Fatal(err)
	}
	if req.Action != ActionMCPCall {
		t.Fatalf("mcp_call: want %q, got %q", ActionMCPCall, req.Action)
	}
	if req.Target != "github/create_issue" {
		t.Fatalf("mcp_call target: want github/create_issue, got %q", req.Target)
	}
	if got := Summarize(req); got != "github/create_issue" {
		t.Fatalf("summarize: want github/create_issue, got %q", got)
	}
}

func TestExtractMalformedArgsFailClosed(t *testing.T) {
	for _, tool := range []string{"bash", "grep", "find", "mcp_call"} {
		if _, err := Extract(tool, json.RawMessage(`{nope`)); err == nil {
			t.Fatalf("%s: malformed args must fail closed, got nil", tool)
		}
	}
}

func TestQuestionAndMCPGateDecisions(t *testing.T) {
	g, err := NewGate(DefaultPolicy(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	// Interactive: the ask channel and read-only introspection pass without an
	// approval; mcp_call asks naming the server and tool, not "unknown action".
	dec, reason := g.Check(t.Context(), Request{Action: ActionQuestion, Tool: "question"})
	if dec != Allow {
		t.Fatalf("question: want Allow, got %v (%s)", dec, reason)
	}
	for _, a := range []Action{ActionMCPList, ActionMCPInspect} {
		dec, reason = g.Check(t.Context(), Request{Action: a, Tool: string(a)})
		if dec != Allow {
			t.Fatalf("%s: want Allow, got %v (%s)", a, dec, reason)
		}
	}
	dec, reason = g.Check(t.Context(), Request{Action: ActionMCPCall, Tool: "mcp_call", Target: "github/create_issue"})
	if dec != Ask {
		t.Fatalf("mcp_call: want Ask, got %v (%s)", dec, reason)
	}
	if !strings.Contains(reason, "github/create_issue") {
		t.Fatalf("mcp_call reason should name the target, got %q", reason)
	}

	// Unattended modes keep the ask channel itself available: the tool renders
	// the question or reports that no UI is attached; a gate denial would only
	// mask that with a permission error.
	p := DefaultPolicy()
	p.Mode = ModeAutopilot
	ag, err := NewGate(p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	dec, reason = ag.Check(t.Context(), Request{Action: ActionQuestion, Tool: "question"})
	if dec != Allow {
		t.Fatalf("question in autopilot: want Allow, got %v (%s)", dec, reason)
	}
}

func TestMCPCallFoldsInUnattendedModes(t *testing.T) {
	for _, mode := range []Mode{ModeAutopilot, ModeHeadlessStrict, ModeReadonly} {
		p := DefaultPolicy()
		p.Mode = mode
		g, err := NewGate(p, t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		dec, reason := g.Check(
			t.Context(),
			Request{Action: ActionMCPCall, Tool: "mcp_call", Target: "github/create_issue"},
		)
		if dec != Deny {
			t.Fatalf("%s: mcp_call want Deny, got %v (%s)", mode, dec, reason)
		}
	}
}

// An explicit permissions.mcp.allow entry pre-approves a server's tools: that
// is the only way mcp_call runs where nobody can answer an ask — headless
// runs and sub-agents — so a configured server keeps working under `phi run`
// instead of being denied on every call while its pool stays loaded.
func TestMCPCallAllowlist(t *testing.T) {
	p := DefaultPolicy()
	p.Mode = ModeHeadlessStrict
	p.MCPAllow = []string{`^github/`, `^fetch/get$`}
	g, err := NewGate(p, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"github/create_issue", "fetch/get"} {
		dec, reason := g.Check(t.Context(), Request{Action: ActionMCPCall, Tool: "mcp_call", Target: target})
		if dec != Allow {
			t.Fatalf("allowlisted %s in headless: want Allow, got %v (%s)", target, dec, reason)
		}
	}
	dec, reason := g.Check(t.Context(), Request{Action: ActionMCPCall, Tool: "mcp_call", Target: "githubx/anything"})
	if dec != Deny {
		t.Fatalf("unlisted server must still fold to Deny headless, got %v (%s)", dec, reason)
	}
	if !strings.Contains(reason, "permissions.mcp.allow") {
		t.Fatalf("folded reason must say how to pre-approve, got %q", reason)
	}

	// Interactive sessions ask once per unlisted call; the list only removes
	// the asks the user has already answered in config.
	ip := DefaultPolicy()
	ip.MCPAllow = []string{`^github/`}
	ig, err := NewGate(ip, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if dec, _ := ig.Check(
		t.Context(),
		Request{Action: ActionMCPCall, Tool: "mcp_call", Target: "github/create_issue"},
	); dec != Allow {
		t.Fatalf("allowlisted github interactive: want Allow, got %v", dec)
	}
	if dec, _ := ig.Check(
		t.Context(),
		Request{Action: ActionMCPCall, Tool: "mcp_call", Target: "gitlab/merge"},
	); dec != Ask {
		t.Fatalf("unlisted gitlab interactive: want Ask, got %v", dec)
	}

	// A call without a target never matches the list: pre-approval is by
	// named server and tool, not "any tool that failed to say its name".
	if dec, _ := g.Check(t.Context(), Request{Action: ActionMCPCall, Tool: "mcp_call"}); dec != Deny {
		t.Fatalf("targetless mcp_call must not match the allowlist, got %v", dec)
	}
}

func TestMCPCallAllowlistRejectsBadPattern(t *testing.T) {
	p := DefaultPolicy()
	p.MCPAllow = []string{"["}
	if _, err := NewGate(p, t.TempDir()); err == nil {
		t.Fatal("invalid mcp allow pattern must fail gate construction")
	}
}
