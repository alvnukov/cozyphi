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
	Providers  *provider.Manager
	OpenCode   *opencode.Source
	Cwd        string
	SessionDir string
	Gate       permission.Gate
}

// printConfigWarnings reports the load-time guesses and deprecations that did
// not fail the start (a sniffed protocol is the first one). Every entry point
// prints them the same way and before anything else takes the terminal: the
// TUI is about to own the screen, where a later stderr line would corrupt the
// draw, and a headless run has already begun its output by then.
func printConfigWarnings(cfg *project.Config) {
	for _, w := range cfg.Warnings() {
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
}

// loadRunBootstrap wires the shared startup path used by `cozyphi run` (and any
// future headless subcommand). It must stay in sync with the TUI controller's
// initialization; search-tool install failures are non-fatal warnings.
// When yolo is true, permission checks are skipped for this run only.
// proj is the discovered workspace — a parameter, not GetDefaultProject, so
// tests boot against their own HOME instead of the process-wide singleton.
func loadRunBootstrap(
	ctx context.Context,
	proj *project.Project,
	sessionDirOverride string,
	yolo bool,
) (*runBootstrap, error) {
	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	providers, openCodeSource, err := loadRuntimeSources(proj, proj.Config().OpenCode.Enabled)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: providers/opencode:", err)
	}
	printConfigWarnings(proj.Config())
	bs := &runBootstrap{
		Proj:      proj,
		Config:    proj.Config(),
		Providers: providers,
		OpenCode:  openCodeSource,
	}
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

// loadRuntimeSources opens the model sources beyond config.yaml: the
// connected-provider manager and, when enabled, the read-only opencode view
// over its catalog. Both come from one place because opencode resolves its
// models against the provider catalog.
func loadRuntimeSources(proj *project.Project, enabled bool) (*provider.Manager, *opencode.Source, error) {
	providers, err := provider.Open(provider.Options{
		CachePath:       proj.Global().ProviderCatalogFile(),
		CredentialsPath: proj.Global().CredentialsFile(),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("initialize provider catalog: %w", err)
	}
	if !enabled {
		return providers, nil, nil
	}
	source, err := opencode.Load(opencode.Options{Catalog: providers.Providers()})
	if err != nil {
		return providers, nil, fmt.Errorf("load opencode source: %w", err)
	}
	return providers, source, nil
}

// models is the runtime catalog a headless run can resolve against — the
// same three sources, in the same order, as the TUI controller's catalog.
func (b *runBootstrap) models() []llm.ModelConfig {
	models := b.Config.AllModels()
	if b.Providers != nil {
		models = append(models, b.Providers.Models()...)
	}
	return append(models, b.OpenCode.Models()...)
}

// requireModel returns the engine model for a headless run, or an error
// naming every way to configure one. Headless has no screen to refuse on,
// so an unresolvable model stops the run before anything connects — unlike
// the TUI, which starts and guides the user to /connect or /model.
// The resolution order is the TUI's startup order: the config default
// (where the COZYPHI_* environment lands) → the last model the user picked
// → the first runtime-catalog model, connected providers before opencode.
func (b *runBootstrap) requireModel() (llm.ModelConfig, error) {
	if m := b.Config.Model(); m.Name != "" {
		return m, nil
	}
	if state, err := project.LoadUIState(b.Proj.Global()); err == nil && state.LastModel != "" {
		if m, ok := b.findModel(state.LastModel); ok {
			return m, nil
		}
	}
	for _, m := range b.models() {
		if m.Name == "" {
			continue
		}
		if m.SkillPath == "" {
			m.SkillPath = b.Config.SkillPath
		}
		return m, nil
	}
	return llm.ModelConfig{}, fmt.Errorf(
		"no model configured — pick one:\n"+
			"  - edit %s (cozyphi config)\n"+
			"  - /connect in the TUI\n"+
			"  - set COZYPHI_MODEL and COZYPHI_API_KEY",
		b.Proj.Global().ConfigFile())
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
