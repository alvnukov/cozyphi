package agenttool_test

import (
	"testing"

	"github.com/alvnukov/cozyphi/internal/tools"
)

func TestParseAgentResultSummary(t *testing.T) {
	out := `{
  "job_id": "job_1",
  "status": "completed",
  "error": "",
  "summary": "## Findings\n\n- ok\n"
}`
	r := tools.ParseAgentResult(out)
	if !r.OK || r.JobID != "job_1" || r.Status != "completed" {
		t.Fatalf("%+v", r)
	}
	if got := r.RenderableSummary(); got != "## Findings\n\n- ok" {
		t.Fatalf("summary %q", got)
	}
}

func TestParseAgentResultRunningNoSummary(t *testing.T) {
	r := tools.ParseAgentResult(`{"job_id":"j","status":"running"}`)
	if !r.OK || r.RenderableSummary() != "" {
		t.Fatalf("%+v", r)
	}
}

func TestParseAgentResultRejectsPlain(t *testing.T) {
	if tools.ParseAgentResult("hello").OK {
		t.Fatal("expected reject")
	}
}
