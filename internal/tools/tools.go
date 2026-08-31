package tools

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/alvnukov/cozyphi/internal/tools/agenttool"
	"github.com/alvnukov/cozyphi/internal/tools/bashtool"
	"github.com/alvnukov/cozyphi/internal/tools/contexttool"
	"github.com/alvnukov/cozyphi/internal/tools/editledger"
	"github.com/alvnukov/cozyphi/internal/tools/findtool"
	"github.com/alvnukov/cozyphi/internal/tools/greptool"
	"github.com/alvnukov/cozyphi/internal/tools/lsptool"
	"github.com/alvnukov/cozyphi/internal/tools/lstool"
	"github.com/alvnukov/cozyphi/internal/tools/mcptool"
	"github.com/alvnukov/cozyphi/internal/tools/memorytool"
	"github.com/alvnukov/cozyphi/internal/tools/plantool"
	"github.com/alvnukov/cozyphi/internal/tools/questiontool"
	"github.com/alvnukov/cozyphi/internal/tools/readtool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/tools/watchtool"
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
	// QuestionDeps re-exports questiontool.Deps.
	QuestionDeps = questiontool.Deps
	// Question re-exports questiontool.Question.
	Question = questiontool.Question
	// QuestionAnswer re-exports questiontool.Answer.
	QuestionAnswer = questiontool.Answer
	// LSPQueryFunc re-exports lsp.QueryFunc.
	LSPQueryFunc = lsp.QueryFunc
	// WatchDeps re-exports watchtool.Deps.
	WatchDeps = watchtool.Deps
)

// AgentTools, ParseAgentResult, and the inherit sentinel are re-exported
// from agenttool.
var (
	AgentTools       = agenttool.AgentTools
	ParseAgentResult = agenttool.ParseAgentResult
	InheritModel     = agenttool.InheritModel
	ContextTools     = contexttool.Tools
	MCPTools         = mcptool.Tools
	PlanTool         = plantool.Tool
	PlanHint         = plantool.Hint
	QuestionTool     = questiontool.Tool
	LSPTool          = lsptool.Tool
	MemoryTool       = memorytool.Tool
	WatchTool        = watchtool.Tool
)

// DefaultTools returns the built-in agent tool set.
func DefaultTools() []Tool {
	ledger := editledger.New()
	return []Tool{
		bashtool.BashTool(),
		readtool.ReadTool(ledger),
		writetool.WriteTool(),
		grepTool(ledger),
		lstool.LsTool(),
		writetool.EditTool(ledger),
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

func grepTool(ledger *editledger.Ledger) Tool {
	tool := greptool.GrepTool()
	run := tool.Run
	tool.Run = func(ctx context.Context, input json.RawMessage) (Result, error) {
		result, err := run(ctx, input)
		if err == nil {
			authorizeGrepOutput(ctx, ledger, result.Content)
		}
		return result, err
	}
	return tool
}

func authorizeGrepOutput(ctx context.Context, ledger *editledger.Ledger, output string) {
	var path, display, tag string
	var anchors []string
	authorizeBlock := func() {
		if path != "" {
			ledger.Authorize(path, tag, anchors)
		}
		anchors = nil
	}
	for line := range strings.SplitSeq(output, "\n") {
		if strings.HasPrefix(line, "@file ") {
			authorizeBlock()
			header := strings.TrimPrefix(line, "@file ")
			i := strings.LastIndex(header, "#")
			if i < 1 || i == len(header)-1 {
				path, display, tag = "", "", ""
				continue
			}
			display, tag = header[:i], header[i+1:]
			resolved, err := tooldef.ResolveToCwd(ctx, display)
			if err != nil {
				path, display, tag = "", "", ""
				continue
			}
			path = resolved
			continue
		}
		if path == "" || !strings.HasPrefix(line, display+":") {
			continue
		}
		ref := strings.TrimLeft(strings.TrimPrefix(line, display+":"), "> ")
		if before, _, ok := strings.Cut(ref, "|"); ok {
			anchors = append(anchors, before)
		}
	}
	authorizeBlock()
}
