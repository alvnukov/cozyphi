package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
)

// This opt-in evaluation drives the real Engine and provider adapters. Normal
// tests neither load credentials nor connect to a provider. The same file can
// be overlaid onto an older checkout for a paired harness comparison.
func TestLiveEditReliability(t *testing.T) {
	selected := os.Getenv("COZYPHI_EDIT_EVAL_MODELS")
	if selected == "" {
		t.Skip("set COZYPHI_EDIT_EVAL_MODELS to explicitly run paid model evaluations")
	}
	models := editEvalModels(t)
	if selected == "list" {
		for _, model := range models {
			t.Logf("model=%s protocol=%s efforts=%v", model.Name, model.Protocol, model.ReasoningEfforts)
		}
		return
	}
	output := os.Getenv("COZYPHI_EDIT_EVAL_OUTPUT")
	revision := os.Getenv("COZYPHI_EDIT_EVAL_REVISION")
	require.NotEmpty(t, output, "an explicit artifact directory keeps live results reviewable")
	require.NotEmpty(t, revision, "record the actual harness commit or tree identifier")
	repeats := 2
	if value := os.Getenv("COZYPHI_EDIT_EVAL_REPEATS"); value != "" {
		var err error
		repeats, err = strconv.Atoi(value)
		require.NoError(t, err)
		require.Positive(t, repeats)
		require.LessOrEqual(t, repeats, 10)
	}
	for name := range strings.SplitSeq(selected, ",") {
		index := slices.IndexFunc(models, func(model llm.ModelConfig) bool { return model.Name == name })
		require.NotEqual(t, -1, index, "unknown configured model %q", name)
		model := models[index]
		t.Run(strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			t.Parallel()
			for _, scenario := range editEvalScenarios() {
				for run := 1; run <= repeats; run++ {
					t.Run(fmt.Sprintf("%s/%d", scenario.name, run), func(t *testing.T) {
						runEditEval(t, model, scenario, output, revision, run)
					})
				}
			}
		})
	}
}

func editEvalModels(t *testing.T) []llm.ModelConfig {
	t.Helper()
	home, err := os.UserHomeDir()
	require.NoError(t, err)
	manager, err := provider.Open(provider.Options{
		CachePath:       filepath.Join(home, ".cozyphi", "providers.json"),
		CredentialsPath: filepath.Join(home, ".cozyphi", "credentials.json"),
	})
	require.NoError(t, err)
	source, err := opencode.Load(opencode.Options{Catalog: manager.Providers()})
	require.NoError(t, err)
	models := manager.Models()
	for _, imported := range source.Models() {
		if !slices.ContainsFunc(models, func(model llm.ModelConfig) bool { return model.Name == imported.Name }) {
			models = append(models, imported)
		}
	}
	return models
}

type editEvalScenario struct {
	name     string
	initial  map[string]string
	prompts  []string
	expected map[string]string
}

func editEvalScenarios() []editEvalScenario {
	lines := make([]string, 140)
	for i := range lines {
		lines[i] = fmt.Sprintf("setting_%03d=keep", i+1)
	}
	initial := strings.Join(lines, "\n") + "\n"
	lines[99] = "setting_100=enabled"
	lines[119] = "setting_120=enabled"
	lines[129] = "setting_130=enabled"
	return []editEvalScenario{
		{
			name:    "incremental",
			initial: map[string]string{"settings.txt": "alpha=1\nbeta=2\ngamma=3\n"},
			prompts: []string{
				"In settings.txt change beta=2 to beta=20. Preserve every other byte. Make the change now.",
				"Now change gamma=3 to gamma=30 in settings.txt. Preserve every other byte. Make the change now.",
			},
			expected: map[string]string{"settings.txt": "alpha=1\nbeta=20\ngamma=30\n"},
		},
		{
			name:    "multiple_ranges",
			initial: map[string]string{"settings.txt": initial, "other.txt": "untouched\n"},
			prompts: []string{
				"In settings.txt enable settings 100 and 120: replace their '=keep' with '=enabled'. Preserve all other bytes and files. Make both changes now.",
				"Now also enable setting 130 in settings.txt, preserving every other byte and file.",
			},
			expected: map[string]string{"settings.txt": strings.Join(lines, "\n") + "\n", "other.txt": "untouched\n"},
		},
		{
			name:    "write_followup",
			initial: map[string]string{"other.txt": "untouched\n"},
			prompts: []string{
				"Create settings.txt with exactly these three lines and a final newline: alpha=1, beta=2, gamma=3. Each comma-separated item must be on its own line. Preserve other.txt.",
				"Now change beta=2 to beta=20 in settings.txt. Preserve every other byte and file.",
			},
			expected: map[string]string{"settings.txt": "alpha=1\nbeta=20\ngamma=3\n", "other.txt": "untouched\n"},
		},
	}
}

type editEvalManifest struct {
	Model        string         `json:"model"`
	ModelVersion string         `json:"model_version"`
	Effort       string         `json:"effort"`
	Harness      string         `json:"harness"`
	Revision     string         `json:"harness_revision"`
	Scenario     string         `json:"scenario"`
	Run          int            `json:"run"`
	Success      bool           `json:"task_success"`
	Elapsed      int64          `json:"elapsed_ms"`
	Failure      string         `json:"failure,omitempty"`
	Transcript   string         `json:"transcript"`
	Usage        *editEvalUsage `json:"usage,omitempty"`
	Unexpected   []string       `json:"unexpected_files,omitempty"`
}

// editEvalUsage is present only when the provider reported usage. Zero-valued
// counters are omitted so an incomplete provider response is not presented as
// a measured zero.
type editEvalUsage struct {
	Responses        int `json:"responses"`
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	CachedTokens     int `json:"cached_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func runEditEval(t *testing.T, model llm.ModelConfig, scenario editEvalScenario, output, revision string, run int) {
	t.Helper()
	work := t.TempDir()
	for name, content := range scenario.initial {
		require.NoError(t, os.WriteFile(filepath.Join(work, name), []byte(content), 0o600))
	}
	gate, err := permission.NewGate(permission.Policy{
		WorkspaceOnlyWrites: true, WorkspaceOnlyReads: true, BashDefault: permission.Deny,
	}, work)
	require.NoError(t, err)
	// No shell, jobs, memory, project instructions, or MCP can access user data.
	var available []tools.Tool
	for _, tool := range tools.DefaultTools() {
		if slices.Contains([]string{"read", "edit", "write", "grep"}, tool.Definition.Name) {
			available = append(available, tool)
		}
	}
	model.SkillPath = filepath.Join(work, "absent-skills")
	if effort := os.Getenv("COZYPHI_EDIT_EVAL_EFFORT"); effort != "" {
		level := llm.ReasoningEffort(effort)
		require.Contains(t, model.ReasoningEfforts, level, "effort must be supported explicitly")
		model.ReasoningEffort = level
	}
	runDir := filepath.Join(
		output,
		"session",
		strings.ReplaceAll(model.Name, "/", "_"),
		scenario.name,
		strconv.Itoa(run),
	)
	require.NoError(t, os.MkdirAll(runDir, 0o700))
	entries, err := os.ReadDir(runDir)
	require.NoError(t, err)
	require.Empty(t, entries, "refuse to mix or overwrite previous run evidence")
	engine, err := agent.NewEngine(agent.EngineOpts{
		Model: model, Tools: available, Gate: gate, MaxRounds: 12,
		SessionOpts: agent.SessionOpts{Cwd: work, SessionDir: runDir, Persist: true},
	})
	require.NoError(t, err)
	engine.SetMode(agent.ModeBuild)
	started := time.Now()
	manifest := editEvalManifest{
		Model: model.Name, ModelVersion: "unknown", Effort: string(model.ReasoningEffort),
		Harness: "cozyphi", Revision: revision, Scenario: scenario.name, Run: run,
		Success: true, Transcript: engine.SessionFile(),
	}
	var usage editEvalUsage
	usageReported := false
	for _, prompt := range scenario.prompts {
		ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
		for event, loopErr := range engine.Loop(ctx, prompt, agent.LoopOpts{}) {
			if update, ok := event.(session.AssistantMessageUpdate); ok &&
				update.Message.State == session.StateComplete && update.Message.Usage.Reported() {
				usageReported = true
				usage.Responses++
				usage.PromptTokens += update.Message.Usage.PromptTokens
				usage.CompletionTokens += update.Message.Usage.CompletionTokens
				usage.CachedTokens += update.Message.Usage.CachedTokens
				usage.TotalTokens += update.Message.Usage.TotalTokens
			}
			if loopErr != nil {
				manifest.Success = false
				// The transcript retains provider errors; the manifest avoids
				// accidentally copying endpoint/auth diagnostics into reports.
				manifest.Failure = "engine_error"
			}
		}
		cancel()
		if !manifest.Success {
			break
		}
	}
	if usageReported {
		manifest.Usage = &usage
	}
	for name, want := range scenario.expected {
		got, readErr := os.ReadFile(filepath.Join(work, name))
		if readErr != nil || string(got) != want {
			manifest.Success = false
			if manifest.Failure == "" {
				manifest.Failure = "final_content_mismatch"
			}
		}
	}
	unexpected, inspectErr := unexpectedEditEvalFiles(work, scenario.expected)
	if inspectErr != nil {
		// The fixture and its permitted tools cannot create unreadable paths;
		// this is a harness failure, not a measured model outcome.
		t.Errorf("inspect final workspace: %v", inspectErr)
	} else if len(unexpected) > 0 {
		manifest.Success = false
		manifest.Unexpected = unexpected
		if manifest.Failure == "" {
			manifest.Failure = "unexpected_files"
		}
	}
	manifest.Elapsed = time.Since(started).Milliseconds()
	data, err := json.MarshalIndent(manifest, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(runDir, "manifest.json"), append(data, '\n'), 0o600))
	t.Logf("success=%t failure=%s elapsed_ms=%d manifest=%s", manifest.Success, manifest.Failure, manifest.Elapsed,
		filepath.Join(runDir, "manifest.json"))
}

func unexpectedEditEvalFiles(root string, expected map[string]string) ([]string, error) {
	unexpected := make([]string, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if _, ok := expected[rel]; !ok {
			unexpected = append(unexpected, rel)
		}
		return nil
	})
	return unexpected, err
}

func TestUnexpectedEditEvalFiles(t *testing.T) {
	work := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(work, "expected.txt"), []byte("expected"), 0o600))
	require.NoError(t, os.Mkdir(filepath.Join(work, "nested"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(work, "nested", "extra.txt"), []byte("extra"), 0o600))

	unexpected, err := unexpectedEditEvalFiles(work, map[string]string{"expected.txt": "expected"})
	require.NoError(t, err)
	require.Equal(t, []string{"nested/extra.txt"}, unexpected)
}
