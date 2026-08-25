package tools

import (
	"github.com/alvnukov/cozyphi/internal/tools/agenttool"
	"github.com/alvnukov/cozyphi/internal/tools/bashtool"
	"github.com/alvnukov/cozyphi/internal/tools/contexttool"
	"github.com/alvnukov/cozyphi/internal/tools/findtool"
	"github.com/alvnukov/cozyphi/internal/tools/greptool"
	"github.com/alvnukov/cozyphi/internal/tools/lsptool"
	"github.com/alvnukov/cozyphi/internal/tools/lstool"
	"github.com/alvnukov/cozyphi/internal/tools/mcptool"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
	"github.com/alvnukov/cozyphi/internal/tools/readtool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"

	"github.com/alvnukov/cozyphi/internal/lsp"
)

type (
	// Result re-exports tooldef.Result.
	Result = tooldef.Result
	// Handler re-exports tooldef.Handler.
	Handler = tooldef.Handler
	// Tool re-exports tooldef.Tool.
	Tool = tooldef.Tool
	// Registry re-exports tooldef.Registry.
	Registry = tooldef.Registry
)

// Definitions and the registry helpers are re-exported from tooldef.
var (
	Definitions    = tooldef.Definitions
	NewRegistry    = tooldef.NewRegistry
	WithToolCallID = tooldef.WithToolCallID
	ToolCallID     = tooldef.ToolCallID
	WithCwd        = tooldef.WithCwd
)

type (
	// ShellExecResult re-exports bashtool.ShellExecResult.
	ShellExecResult = bashtool.ShellExecResult
	// ShellExecOptions re-exports bashtool.ShellExecOptions.
	ShellExecOptions = bashtool.ShellExecOptions
	// BashOutputTail re-exports bashtool.BashOutputTail.
	BashOutputTail = bashtool.BashOutputTail
)

// Bash output limits are re-exported from bashtool.
const (
	BashMaxOutputLines = bashtool.BashMaxOutputLines
	BashMaxOutputBytes = bashtool.BashMaxOutputBytes
)

// ExecShell and NewBashOutputTail are re-exported from bashtool.
var (
	ExecShell         = bashtool.ExecShell
	NewBashOutputTail = bashtool.NewBashOutputTail
)

type (
	// AgentDeps re-exports agenttool.AgentDeps.
	AgentDeps = agenttool.AgentDeps
	// AgentResult re-exports agenttool.AgentResult.
	AgentResult = agenttool.AgentResult
	// ContextDeps re-exports contexttool.Deps.
	ContextDeps = contexttool.Deps
	// ContextStats re-exports contexttool.Stats.
	ContextStats = contexttool.Stats
	// PlanDeps re-exports plantool.Deps.
	PlanDeps = plantool.Deps
	// LSPQueryFunc re-exports lsp.QueryFunc.
	LSPQueryFunc = lsp.QueryFunc
)

// AgentTools, ParseAgentResult, ContextTools, and MCPTools are re-exported
// tool helpers.
var (
	AgentTools       = agenttool.AgentTools
	ParseAgentResult = agenttool.ParseAgentResult
	ContextTools     = contexttool.Tools
	MCPTools         = mcptool.Tools
	PlanTool         = plantool.Tool
	PlanHint         = plantool.Hint
	LSPTool          = lsptool.Tool
)

// DefaultTools returns the built-in agent tool set.
func DefaultTools() []Tool {
	return []Tool{
		bashtool.BashTool(),
		readtool.ReadTool(),
		writetool.WriteTool(),
		greptool.GrepTool(),
		lstool.LsTool(),
		writetool.EditTool(),
		findtool.FindTool(),
	}
}

// ReadonlyTools returns exploration tools without write/edit.
// Bash remains available but should be paired with ModeReadonly so only
// allowlisted commands run (no file mutations via the shell).
func ReadonlyTools() []Tool {
	return []Tool{
		bashtool.BashTool(),
		readtool.ReadTool(),
		greptool.GrepTool(),
		lstool.LsTool(),
		findtool.FindTool(),
	}
}
