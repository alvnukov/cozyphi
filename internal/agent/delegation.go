package agent

import (
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/job"
)

// delegationRoles are the composer @agent mentions, in picker order.
var delegationRoles = []string{
	string(job.RoleExplore),
	string(job.RoleWorker),
	string(job.RoleReview),
}

// splitDelegationPrefix reports a leading "@role " mention addressing a
// sub-agent. rest is the task with the mention and surrounding whitespace
// stripped; a bare role with no task is not a delegation.
func splitDelegationPrefix(prompt string) (role, rest string, ok bool) {
	s := strings.TrimLeft(prompt, " \t\n\r")
	if !strings.HasPrefix(s, "@") {
		return "", "", false
	}
	token := s[1:]
	for _, r := range delegationRoles {
		if !strings.HasPrefix(token, r) {
			continue
		}
		after := token[len(r):]
		if after != "" && after[0] != ' ' && after[0] != '\t' && after[0] != '\n' {
			continue
		}
		if task := strings.TrimLeft(after, " \t\n\r"); task != "" {
			return r, task, true
		}
	}
	return "", "", false
}

// delegationInstruction rewrites an "@role task" prompt into an explicit
// spawn: the sub-agent pipeline (agent_spawn/agent_wait, nested transcript
// rows) renders it like any other delegation.
func delegationInstruction(role string) string {
	return fmt.Sprintf(
		"The user addressed this message to the %q sub-agent. Delegate it now: call agent_spawn with role %q, an explicit skills decision (the fitting installed skills, or skills: [] with a no_skill_reason), and the message below verbatim as the prompt, then agent_wait and relay the sub-agent's final summary as your answer. Do not answer it yourself.",
		role,
		role,
	)
}
