package permission_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/permission"
)

func TestEveryToolInTheCatalogIsClassified(t *testing.T) {
	for _, name := range diag.ToolCatalog() {
		assert.NotEqual(t, diag.CheckUnknown, permission.ToolCheckKind(name),
			"%s is in the harness catalog, so the view can be asked about it", name)
	}
}

func TestAToolThisPackageDoesNotJudgeByNameIsReportedUnknown(t *testing.T) {
	assert.Equal(t, diag.CheckUnknown, permission.ToolCheckKind("some_caller_supplied_tool"))
	assert.Equal(t, diag.CheckUnknown, permission.ToolCheckKind(""),
		"guessing on an unnamed tool's behalf would be a guess in the permissive direction")
}

func TestToolsWhoseDecisionReadsTheirArgumentsSaySo(t *testing.T) {
	// Each of these reaches something outside the process — a shell, a path,
	// an MCP server, the task ledger — and which one is in the call.
	for _, name := range []string{
		"bash", "read", "write", "edit", "grep", "find", "ls", "lsp",
		"watch", "mcp_call", "task",
	} {
		assert.Equal(t, diag.CheckArguments, permission.ToolCheckKind(name),
			"%s must never be described as decided by its name alone", name)
	}
}

func TestToolsDecidedByTheirNameAloneCarryNothingToJudge(t *testing.T) {
	for _, name := range []string{
		"session", "plan", "question", "context", "harness",
		"mcp_list", "mcp_inspect",
		"agent_spawn", "agent_list", "agent_wait", "agent_cancel",
	} {
		assert.Equal(t, diag.CheckToolName, permission.ToolCheckKind(name), name)
	}
}

func TestTheMemoryToolIsDecidedByThePolicyAlone(t *testing.T) {
	assert.Equal(t, diag.CheckPolicy, permission.ToolCheckKind("memory"),
		"the memory tool takes a name, not a path: the bound directory is the whole decision")
}

func TestClassifyingAToolDecidesNothing(t *testing.T) {
	// A gate that would record any decision asked of it. Classification must
	// leave it untouched: observing what a decision would cost is not a
	// decision.
	gate := &countingGate{}
	before := gate.calls

	for _, name := range diag.ToolCatalog() {
		permission.ToolCheckKind(name)
	}

	assert.Equal(t, before, gate.calls, "no request is built and no gate is consulted")
}

// countingGate is a real boundary, so a classification that consulted one
// would have to go through it.
var _ permission.Gate = (*countingGate)(nil)

type countingGate struct{ calls int }

func (g *countingGate) Check(context.Context, permission.Request) (permission.Decision, string) {
	g.calls++
	return permission.Allow, ""
}
