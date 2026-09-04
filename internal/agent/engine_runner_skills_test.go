package agent_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// installRunnerSkill writes one SKILL.md so buildChild resolves job skills
// against a real catalog.
func installRunnerSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "SKILL.md"),
		[]byte("---\nname: "+name+"\ndescription: test skill\n---\n"+body),
		0o644,
	))
}

// capturingSSEServer records every request body and streams one text reply,
// so a test can read exactly what reached the child's model.
func capturingSSEServer(t *testing.T, reply string) (*httptest.Server, func() []string) {
	t.Helper()
	var (
		mu     sync.Mutex
		bodies []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(raw))
		mu.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprintf(w, "data: %s\n\n", jsonMarshalDelta(reply))
		_, _ = fmt.Fprint(
			w,
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":1,\"completion_tokens\":1,\"total_tokens\":2}}\n\n",
		)
		_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), bodies...)
	}
}

// TestEngineRunnerPromptCarriesJobSkills pins the child prompt: the parent's
// skill decision arrives as one intro line plus each body in order, as plain
// text the model never spends a read call on.
func TestEngineRunnerPromptCarriesJobSkills(t *testing.T) {
	skillDir := t.TempDir()
	installRunnerSkill(t, skillDir, "first", "FIRST-BODY rule one")
	installRunnerSkill(t, skillDir, "second", "SECOND-BODY rule two")

	srv, bodies := capturingSSEServer(t, "done")
	runner := agent.EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: srv.URL, APIKey: "x", SkillPath: skillDir},
	}
	mgr, err := job.New(job.Options{Root: t.TempDir(), Runner: runner})
	require.NoError(t, err)
	t.Cleanup(func() { _ = mgr.Close() })

	info, err := mgr.Spawn(t.Context(), job.SpawnRequest{
		Prompt:  "go",
		WorkDir: t.TempDir(),
		Skills:  []string{"first", "second"},
	})
	require.NoError(t, err)
	res, err := mgr.Wait(t.Context(), info.ID)
	require.NoError(t, err)
	require.Equal(t, job.StatusCompleted, res.Info.Status)

	all := bodies()
	require.NotEmpty(t, all)
	prompt := all[0]
	const intro = "The parent equipped this job with these skills. Follow them; their SKILL.md files need no read call."
	require.Contains(t, prompt, intro)

	// Order matters: intro, then each heading above its own body.
	positions := []int{
		strings.Index(prompt, intro),
		strings.Index(prompt, "## Skill: first"),
		strings.Index(prompt, "FIRST-BODY rule one"),
		strings.Index(prompt, "## Skill: second"),
		strings.Index(prompt, "SECOND-BODY rule two"),
	}
	for i, pos := range positions {
		require.GreaterOrEqual(t, pos, 0, "part %d missing from child prompt:\n%s", i, prompt)
		if i > 0 {
			require.Greater(t, pos, positions[i-1], "skill parts out of order at %d", i)
		}
	}
}

// TestEngineRunnerFailsOnUnresolvedJobSkill: a persisted skill name the
// catalog can no longer supply fails the job naming the skill — never a
// silent skip, and never a child started with shrunken guidance.
func TestEngineRunnerFailsOnUnresolvedJobSkill(t *testing.T) {
	skillDir := t.TempDir()
	installRunnerSkill(t, skillDir, "unrelated", "body")

	srv, bodies := capturingSSEServer(t, "unused")
	runner := agent.EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: srv.URL, APIKey: "x", SkillPath: skillDir},
	}
	_, err := runner.Run(t.Context(), job.RunEnv{
		Job: job.Meta{
			Dir:     t.TempDir(),
			Prompt:  "x",
			WorkDir: t.TempDir(),
			Skills:  []string{"gone"},
		},
		Log: func(string) {},
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), `"gone"`)
	assert.Empty(t, bodies(), "no engine loop may start for an unresolvable job skill")
}
