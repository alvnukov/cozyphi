package agenttool_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/agenttool"
)

// TestSpawnTitleFromInput: every role names its child the same way —
// role(description) — including explore, which used to render bare.
func TestSpawnTitleFromInput(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"explore is named too", `{"role":"explore","description":"map the parser"}`, "explore(map the parser)"},
		{"worker", `{"role":"worker","description":"fix the lexer"}`, "worker(fix the lexer)"},
		{"review", `{"role":"review","description":"read the diff"}`, "review(read the diff)"},
		{"missing role falls back to explore", `{"description":"map the parser"}`, "explore(map the parser)"},
		{
			"no description falls back to the prompt",
			`{"role":"worker","prompt":"fix the lexer"}`,
			"worker(fix the lexer)",
		},
		{"nothing to describe by leaves the role alone", `{"role":"worker"}`, "worker"},
		{"unparseable input still names something", `not json`, "explore"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, agenttool.SpawnTitleFromInput(json.RawMessage(tc.input)))
			assert.Equal(t, tc.want, tools.SpawnTitleFromInput(json.RawMessage(tc.input)))
		})
	}
}

// TestSpawnDetailKeepsSkillsSuffix: the row title is the child's name plus
// the skills decision, in that order.
func TestSpawnDetailKeepsSkillsSuffix(t *testing.T) {
	reg, _ := skillsRegistry(t, t.TempDir())
	args := mustArgs(t, map[string]any{
		"prompt": "p", "description": "probe", "role": "worker",
		"skills": []string{}, "no_skill_reason": "nothing installed fits",
	})
	assert.Equal(
		t,
		"worker(probe) · no skills: nothing installed fits",
		reg["agent_spawn"].DetailFromArgs(args),
	)
}
