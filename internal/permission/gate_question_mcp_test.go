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
