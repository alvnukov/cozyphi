package agent

import (
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// ChildSpec is the capability profile for a sub-agent role.
type ChildSpec struct {
	Role  job.Role
	Tools []tools.Tool
	Mode  permission.Mode // used when EngineRunner.Gate is nil
	Hint  string          // appended to the child prompt
}

// ChildTools returns the default (explore) tool set.
func ChildTools() []tools.Tool {
	return SpecForRole(job.RoleExplore).Tools
}

// SpecForRole returns tools, permission mode, and closing hint for a role.
func SpecForRole(role job.Role) ChildSpec {
	role = job.NormalizeRole(string(role))
	switch role {
	case job.RoleWorker:
		return ChildSpec{
			Role:  job.RoleWorker,
			Tools: tools.DefaultTools(), // no agent_*; writable
			Mode:  permission.ModeHeadlessStrict,
			Hint:  workerSummaryHint,
		}
	case job.RoleReview:
		return ChildSpec{
			Role:  job.RoleReview,
			Tools: tools.ReadonlyTools(),
			Mode:  permission.ModeReadonly,
			Hint:  reviewSummaryHint,
		}
	default:
		return ChildSpec{
			Role:  job.RoleExplore,
			Tools: tools.ReadonlyTools(),
			Mode:  permission.ModeReadonly,
			Hint:  exploreSummaryHint,
		}
	}
}

const exploreSummaryHint = `You are an explore sub-agent (read-only). Use your tools to search and understand the codebase, then finish with one concise final reply.

Notes:
1. Be direct. Prefer cwd-relative file paths in the final reply.
2. Include key findings, relevant paths, and short snippets only when they help the parent act.
3. The parent sees only this final reply — not your tool transcript.
4. You cannot modify files; if edits are needed, report what should change and where.`

const reviewSummaryHint = `You are a review sub-agent (read-only + allowlisted bash). Inspect diffs, run checks, and report findings — do not implement fixes.

Notes:
1. Prefer cwd-relative paths. Cite evidence (commands run, failing tests, suspicious hunks).
2. Separate must-fix issues from nits. Do not edit files; recommend concrete changes for the parent.
3. The parent sees only this final reply — not your tool transcript.`

const workerSummaryHint = `You are a worker sub-agent. Implement the planned task with the tools available, verify when possible, then finish with one concise final reply.

Notes:
1. Stay within the assigned scope; do not expand into unrelated refactors.
2. Prefer cwd-relative paths. Summarize what you changed and how you verified.
3. The parent sees only this final reply — not your tool transcript.
4. You cannot spawn further agents.`
