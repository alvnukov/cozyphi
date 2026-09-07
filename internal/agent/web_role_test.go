package agent

import (
	"strings"
	"testing"

	cozyconfig "github.com/alvnukov/cozy-tools/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
)

func testWebOptions(t *testing.T) WebOptions {
	t.Helper()
	on := true
	return WebOptions{
		Policy:     cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()},
		Quarantine: true,
	}
}

// TestChildRolesCarryTheWebTool: a sub-agent researching a library needs the
// same bounded access the parent has, and every call still crosses the
// parent's gate.
func TestChildRolesCarryTheWebTool(t *testing.T) {
	runner := EngineRunner{
		Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"},
		Web:   testWebOptions(t),
	}
	for _, role := range []job.Role{job.RoleExplore, job.RoleWorker, job.RoleReview} {
		t.Run(string(role), func(t *testing.T) {
			eng, _, err := runner.buildChild(job.Meta{
				Dir: t.TempDir(), Prompt: "p", WorkDir: t.TempDir(), ParentID: "parent", Role: role,
			})
			require.NoError(t, err)
			assert.True(t, eng.HasTool("web"), "the web tool belongs in this role's ceiling")
		})
	}
}

// TestChildrenLoseWebWhenTheParentHasNone: the ceiling can only narrow.
func TestChildrenLoseWebWhenTheParentHasNone(t *testing.T) {
	runner := EngineRunner{Model: llm.ModelConfig{Name: "fake", BaseURL: "http://127.0.0.1:9", APIKey: "x"}}
	eng, _, err := runner.buildChild(job.Meta{
		Dir: t.TempDir(), Prompt: "p", WorkDir: t.TempDir(), ParentID: "parent", Role: job.RoleWorker,
	})
	require.NoError(t, err)
	assert.False(t, eng.HasTool("web"))
}

// TestChildWebBlanksTheReader: the reader is the thing that reads untrusted
// pages, so handing it a fetch would be the exfiltration channel the
// quarantine exists to close.
func TestChildWebBlanksTheReader(t *testing.T) {
	parent := testWebOptions(t)
	assert.Equal(t, parent, childWeb(parent, job.RoleWorker))

	narrowed := childWeb(parent, job.RoleWebReader)
	assert.Equal(t, WebOptions{}, narrowed)
	assert.False(t, narrowed.enabled(), "a blanked option set must register no tool")
	assert.False(t, RoleAllowsWeb(job.RoleWebReader))
	assert.True(t, RoleAllowsWeb(job.RoleExplore))
}

// TestWebReaderSpecHasNoTools pins the quarantine role's whole capability
// surface: no real tools and no writes. The decoys it is shown at call time
// are not part of the spec.
func TestWebReaderSpecHasNoTools(t *testing.T) {
	spec := SpecForRole(job.RoleWebReader)
	assert.Equal(t, job.RoleWebReader, spec.Role, "the reader must not fold into explore")
	assert.Empty(t, spec.Tools)
	assert.Equal(t, permission.ModeReadonly, spec.Mode)
	assert.NotEmpty(t, spec.Hint)
}

// TestWebReaderIsNotSpawnable: only the web tool creates a reader, so neither
// the model nor a config can name the role.
func TestWebReaderIsNotSpawnable(t *testing.T) {
	_, err := job.ParseRole(string(job.RoleWebReader))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be spawned")

	for _, r := range job.Roles() {
		assert.NotEqual(t, job.RoleWebReader, r, "the reader must stay out of the spawnable list")
	}
}

// TestWebOptionsFromConfig covers the translation of the `web:` section,
// including the quarantine default and the off switch.
func TestWebOptionsFromConfig(t *testing.T) {
	on := true
	cfg := project.WebConfig{
		Policy:     cozyconfig.WebPolicy{Enabled: &on, CacheDir: t.TempDir()},
		Quarantine: project.WebQuarantineReader,
	}
	opts := WebOptionsFrom(cfg)
	assert.True(t, opts.Quarantine)
	assert.True(t, opts.enabled())

	cfg.Quarantine = project.WebQuarantineOff
	assert.False(t, WebOptionsFrom(cfg).Quarantine)

	off := false
	assert.False(t, WebOptionsFrom(project.WebConfig{
		Policy: cozyconfig.WebPolicy{Enabled: &off, CacheDir: t.TempDir()},
	}).enabled())

	assert.False(t, WebOptionsFrom(project.WebConfig{Policy: cozyconfig.WebPolicy{Enabled: &on}}).enabled(),
		"no cache directory means no place to keep a fetched page")
}

// TestReaderHintNamesItsOwnRules: the hint is the reader's whole defense
// against a page that talks to it.
func TestReaderHintNamesItsOwnRules(t *testing.T) {
	hint := SpecForRole(job.RoleWebReader).Hint
	for _, want := range []string{"untrusted", "question"} {
		assert.Contains(t, strings.ToLower(hint), want, "the hint must mention %q", want)
	}
}
