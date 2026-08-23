package prompt

import (
	"strings"
	"testing"
)

// TestBuildPlanAppendix pins the plan-mode posture: read-only exploration,
// no file mutations, and a concrete numbered plan as the deliverable.
func TestBuildPlanAppendix(t *testing.T) {
	plan := Build("", false, nil, true)
	build := Build("", false, nil, false)

	if !strings.Contains(plan, "plan mode") {
		t.Fatal("expected plan mode marker in plan prompt")
	}
	if !strings.Contains(plan, "read-only") {
		t.Fatal("expected plan prompt to state exploration is read-only")
	}
	if !strings.Contains(plan, "do not call `write` or `edit`") {
		t.Fatal("expected plan prompt to forbid write/edit explicitly")
	}
	if !strings.Contains(plan, "numbered plan") {
		t.Fatal("expected plan prompt to require a numbered plan")
	}

	for _, leak := range []string{"plan mode", "numbered plan", "do not call `write` or `edit`"} {
		if strings.Contains(build, leak) {
			t.Fatalf("build prompt must not contain plan-only phrase %q", leak)
		}
	}
}
