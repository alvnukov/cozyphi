package agenttool_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// installSkill writes one SKILL.md the way engine_plan_actions_test.go does,
// so spawn validation resolves names against a real catalog shape.
func installSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: test skill\n---\n"+body),
		0o644,
	))
}

// skillsRegistry wires a spawn tool whose catalog is the temp dir and lets a
// test read what the spawned jobs carried. Jobs run on their own goroutine,
// so the captured metas sit behind a mutex.
func skillsRegistry(t *testing.T, skillPath string) (tools.Registry, func() []job.Meta) {
	t.Helper()
	var (
		mu      sync.Mutex
		spawned []job.Meta
	)
	mgr, err := job.New(job.Options{
		Root: t.TempDir(),
		Runner: job.RunnerFunc(func(_ context.Context, env job.RunEnv) (string, error) {
			mu.Lock()
			spawned = append(spawned, env.Job)
			mu.Unlock()
			return "ok", nil
		}),
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })
	reg := tools.NewRegistry(tools.AgentTools(tools.AgentDeps{
		Manager:   mgr,
		ParentID:  func() string { return "p" },
		WorkDir:   func() string { return t.TempDir() },
		SkillPath: func() string { return skillPath },
	}))
	return reg, func() []job.Meta {
		mu.Lock()
		defer mu.Unlock()
		return append([]job.Meta(nil), spawned...)
	}
}

// waitSpawn blocks for the spawned job so assertions read settled state.
func waitSpawn(t *testing.T, reg tools.Registry, res toolResult) {
	t.Helper()
	waitRes, err := reg["agent_wait"].Run(t.Context(), mustArgs(t, map[string]any{"job_id": res.JobID}))
	require.NoError(t, err)
	assert.Contains(t, waitRes.Content, `"status": "completed"`)
}

// toolResult is the subset of the spawn result the tests read.
type toolResult struct {
	JobID string `json:"job_id"`
}

func decodeResult(t *testing.T, res tools.Result) toolResult {
	t.Helper()
	var out toolResult
	require.NoError(t, json.Unmarshal([]byte(res.Content), &out))
	return out
}

// TestSpawnSkillsDecisionIsRequired pins the explicit-decision contract: a
// call with no skills at all — omitted or an empty array — fails unless the
// model states a non-blank reason, and the error says what to pass.
func TestSpawnSkillsDecisionIsRequired(t *testing.T) {
	reg, metas := skillsRegistry(t, "")

	for name, args := range map[string]json.RawMessage{
		"omitted":  mustArgs(t, map[string]any{"prompt": "p"}),
		"empty":    mustArgs(t, map[string]any{"prompt": "p", "skills": []string{}}),
		"blankish": mustArgs(t, map[string]any{"prompt": "p", "skills": []string{}, "no_skill_reason": "   "}),
	} {
		_, err := reg["agent_spawn"].Run(t.Context(), args)
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "skills is required", name)
		assert.Contains(t, err.Error(), "no_skill_reason", name)
	}
	assert.Empty(t, metas(), "no refused call may create a job")
}

// TestSpawnEmptySkillsWithReasonSpawns: skills: [] plus a reason passes, the
// transcript row carries the reason, and the result JSON echoes both halves
// of the decision.
func TestSpawnEmptySkillsWithReasonSpawns(t *testing.T) {
	reg, metas := skillsRegistry(t, t.TempDir())

	args := mustArgs(t, map[string]any{
		"prompt": "p", "description": "probe",
		"skills": []string{}, "no_skill_reason": "nothing installed fits",
	})
	res, err := reg["agent_spawn"].Run(t.Context(), args)
	require.NoError(t, err)
	assert.Contains(t, res.Content, `"skills": []`)
	assert.Contains(t, res.Content, `"no_skill_reason": "nothing installed fits"`)

	detail := reg["agent_spawn"].DetailFromArgs(args)
	require.Contains(t, detail, "no skills: nothing installed fits")
	require.NotContains(t, detail, "skills: []")

	waitSpawn(t, reg, decodeResult(t, res))
	jobs := metas()
	require.Len(t, jobs, 1)
	assert.Empty(t, jobs[0].Skills)
}

// TestSpawnSkillsResolveDedupeAndEcho: names resolve case-insensitively to
// their canonical form, duplicates fold keeping the first order, the job
// carries the canonical set, and the result JSON names it.
func TestSpawnSkillsResolveDedupeAndEcho(t *testing.T) {
	skillDir := t.TempDir()
	installSkill(t, skillDir, "tdd", "write the failing test first")
	installSkill(t, skillDir, "grill", "ask until it hurts")
	reg, metas := skillsRegistry(t, skillDir)

	args := mustArgs(t, map[string]any{
		"prompt": "p", "description": "drill",
		"skills": []string{"TDD", "tdd", "grill"},
	})
	res, err := reg["agent_spawn"].Run(t.Context(), args)
	require.NoError(t, err)
	// Echo check is decoded, not substring: the exact canonical set rides
	// the JSON, and no_skill_reason stays absent for a skill-bearing spawn.
	var out struct {
		Skills        []string `json:"skills"`
		NoSkillReason string   `json:"no_skill_reason"`
	}
	require.NoError(t, json.Unmarshal([]byte(res.Content), &out))
	assert.Equal(t, []string{"tdd", "grill"}, out.Skills)
	assert.Empty(t, out.NoSkillReason)

	assert.Contains(t, reg["agent_spawn"].DetailFromArgs(args), "skills: TDD, tdd, grill")

	waitSpawn(t, reg, decodeResult(t, res))
	jobs := metas()
	require.Len(t, jobs, 1)
	assert.Equal(t, []string{"tdd", "grill"}, jobs[0].Skills)
}

// TestSpawnUnknownSkillListsCatalog: a name the catalog does not know fails
// closed and lists what is installed, so one retry can fix the call.
func TestSpawnUnknownSkillListsCatalog(t *testing.T) {
	skillDir := t.TempDir()
	installSkill(t, skillDir, "tdd", "write the failing test first")
	installSkill(t, skillDir, "grill", "ask until it hurts")
	reg, metas := skillsRegistry(t, skillDir)

	_, err := reg["agent_spawn"].Run(t.Context(), mustArgs(t, map[string]any{
		"prompt": "p", "skills": []string{"nope"},
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"nope"`)
	assert.Contains(t, err.Error(), "tdd")
	assert.Contains(t, err.Error(), "grill")
	assert.Empty(t, metas())
}

// TestSpawnSkillsCountLimit: nine distinct installed names is over the cap of
// eight; the error says the cap instead of silently truncating the decision.
func TestSpawnSkillsCountLimit(t *testing.T) {
	skillDir := t.TempDir()
	names := make([]string, 0, 9)
	for i := range 9 {
		name := "skill-" + string(rune('a'+i))
		installSkill(t, skillDir, name, "body")
		names = append(names, name)
	}
	reg, metas := skillsRegistry(t, skillDir)

	_, err := reg["agent_spawn"].Run(t.Context(), mustArgs(t, map[string]any{
		"prompt": "p", "skills": names,
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at most 8 skills")
	assert.Empty(t, metas())
}

// TestSpawnSkillsBodySizeLimit: eight bodies over 32 KiB in total refuse the
// spawn — the child prompt would carry them verbatim.
func TestSpawnSkillsBodySizeLimit(t *testing.T) {
	skillDir := t.TempDir()
	names := make([]string, 0, 8)
	for i := range 8 {
		name := "big-" + string(rune('a'+i))
		installSkill(t, skillDir, name, strings.Repeat("x", 5*1024))
		names = append(names, name)
	}
	reg, metas := skillsRegistry(t, skillDir)

	_, err := reg["agent_spawn"].Run(t.Context(), mustArgs(t, map[string]any{
		"prompt": "p", "skills": names,
	}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "over the 32768-byte limit")
	assert.Empty(t, metas())
}
