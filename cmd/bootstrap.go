package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/toolmanager"
)

const bootstrapDownloadTimeout = 5 * time.Minute

// cozyphi run exit codes.
const (
	ExitOK        = 0 // loop finished without errors
	ExitError     = 1 // runtime / LLM / session error
	ExitMaxRounds = 2 // model exceeded --max-rounds
	ExitUsage     = 3 // config or CLI usage error
)

// HeadlessGate builds the permission gate for non-interactive entrypoints.
// An empty policy mode defaults to headless-strict so Ask decisions fold to
// Deny (Ask≡Deny); dangerously_allow_all is honored exactly like the TUI.
func HeadlessGate(policy permission.Policy) (permission.Gate, error) {
	if policy.Mode == "" {
		policy.Mode = permission.ModeHeadlessStrict
	}
	if policy.DangerouslyAllowAll {
		return permission.AllowAll{}, nil
	}
	return permission.NewGate(policy, permission.WorkspaceRoot())
}

// runBootstrap is the shared startup state for headless entrypoints:
// Discover → config → search tools → gate → session dir.
type runBootstrap struct {
	Proj       *project.Project
	Config     *project.Config
	OpenCode   *opencode.Source
	Cwd        string
	SessionDir string
	Gate       permission.Gate
}

// loadRunBootstrap wires the shared startup path used by `cozyphi run` (and any
// future headless subcommand). It must stay in sync with the TUI controller's
// initialization; search-tool install failures are non-fatal warnings.
// When yolo is true, permission checks are skipped for this run only.
func loadRunBootstrap(ctx context.Context, sessionDirOverride string, yolo bool) (*runBootstrap, error) {
	proj := project.GetDefaultProject()
	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	openCodeSource, err := loadOpenCodeSource(proj, proj.Config().OpenCode.Enabled)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: opencode:", err)
	}
	// Load-time guesses (a sniffed protocol) still work; say so once here
	// instead of failing the run.
	for _, w := range proj.Config().Warnings() {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	bs := &runBootstrap{Proj: proj, Config: proj.Config(), OpenCode: openCodeSource}
	// A stale agents.models pin degrades to inheritance at spawn time; say
	// so once here instead of failing the run.
	for _, w := range proj.Config().AgentModels(bs.findModel).Stale() {
		fmt.Fprintln(os.Stderr, "warning: unknown model in agents.models (inherit):", w)
	}
	if err := EnsureSearchTools(ctx, proj); err != nil {
		fmt.Fprintln(os.Stderr, "warning: could not install search tools:", err)
	}
	policy := proj.Config().Permissions
	// The agent's memory is the one write target outside the workspace it may
	// use; the gate has to know where it is.
	policy.MemoryDir = proj.MemoryDir()
	if yolo {
		policy.DangerouslyAllowAll = true
	}
	gate, err := HeadlessGate(policy)
	if err != nil {
		return nil, fmt.Errorf("permissions: %w", err)
	}
	cwd, _ := os.Getwd()
	sessionDir := sessionDirOverride
	if sessionDir == "" {
		sessionDir = proj.SessionDir()
	}
	bs.Cwd = cwd
	bs.SessionDir = sessionDir
	bs.Gate = gate
	return bs, nil
}

func loadOpenCodeSource(proj *project.Project, enabled bool) (*opencode.Source, error) {
	if !enabled {
		return nil, nil
	}
	providers, err := provider.Open(provider.Options{
		CachePath:       proj.Global().ProviderCatalogFile(),
		CredentialsPath: proj.Global().CredentialsFile(),
	})
	if err != nil {
		return nil, fmt.Errorf("initialize provider catalog: %w", err)
	}
	return opencode.Load(opencode.Options{Catalog: providers.Providers()})
}

func (b *runBootstrap) models() []llm.ModelConfig {
	models := b.Config.AllModels()
	return append(models, b.OpenCode.Models()...)
}

func (b *runBootstrap) findModel(name string) (llm.ModelConfig, bool) {
	for _, cfg := range b.models() {
		if cfg.Name == name {
			if cfg.SkillPath == "" {
				cfg.SkillPath = b.Config.SkillPath
			}
			return cfg, true
		}
	}
	return llm.ModelConfig{}, false
}

func (b *runBootstrap) modelNames() []string {
	models := b.models()
	names := make([]string, 0, len(models))
	for _, model := range models {
		names = append(names, model.Name)
	}
	return names
}

// EnsureSearchTools installs fd and ripgrep into the cozyphi bin dir
// (~/.cozyphi/bin) when they are missing from both the bin dir and PATH.
// Failures are non-fatal: the search tools fall back to PATH at runtime
// and report a clear error if truly unavailable.
func EnsureSearchTools(ctx context.Context, proj *project.Project) error {
	return ensureSearchTools(ctx, proj, toolmanager.DownloadTool)
}

type searchToolDownloader func(context.Context, string) (string, error)

func ensureSearchTools(ctx context.Context, proj *project.Project, download searchToolDownloader) error {
	type downloadResult struct {
		index int
		err   error
	}

	tools := []string{"fd", "rg"}
	results := make(chan downloadResult, len(tools))
	scheduled := 0
	installErrors := make([]error, len(tools))
	for index, tool := range tools {
		if !shouldBootstrap(proj, tool) {
			continue
		}
		scheduled++
		go func(index int, tool string) {
			dlCtx, cancel := context.WithTimeout(ctx, bootstrapDownloadTimeout)
			defer cancel()
			_, err := download(dlCtx, tool)
			if err != nil {
				err = fmt.Errorf("%s: %w", tool, err)
			}
			results <- downloadResult{index: index, err: err}
		}(index, tool)
	}

	for range scheduled {
		result := <-results
		installErrors[result.index] = result.err
	}

	joinedErrors := installErrors[:0]
	for _, err := range installErrors {
		if err != nil {
			joinedErrors = append(joinedErrors, err)
		}
	}
	return errors.Join(joinedErrors...)
}

// shouldBootstrap is true when the tool binary is missing from the cozyphi bin
// dir and from PATH, i.e. it needs a download. This mirrors panda's
// fileutil.ShouldBootstrapSearchTool.
func shouldBootstrap(proj *project.Project, name string) bool {
	binName := name
	if runtime.GOOS == "windows" {
		binName += ".exe"
	}
	if _, err := os.Stat(filepath.Join(proj.Global().BinDir(), binName)); err == nil {
		return false
	}
	if _, err := exec.LookPath(binName); err == nil {
		return false
	}
	return true
}
