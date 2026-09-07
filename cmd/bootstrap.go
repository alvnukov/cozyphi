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

	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/notify"
	"github.com/alvnukov/cozyphi/internal/opencode"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/provider"
	"github.com/alvnukov/cozyphi/internal/toolmanager"
	"github.com/alvnukov/cozyphi/internal/tui/keys"
	"github.com/alvnukov/cozyphi/internal/voice"
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
// The optional root selects an explicit workspace; omission retains the CLI default.
func HeadlessGate(policy permission.Policy, root ...string) (permission.Gate, error) {
	if policy.Mode == "" {
		policy.Mode = permission.ModeHeadlessStrict
	}
	if policy.DangerouslyAllowAll {
		return permission.AllowAll{}, nil
	}
	workspace := ""
	if len(root) > 0 {
		workspace = root[0]
	}
	return permission.NewGate(policy, workspace)
}

// runBootstrap is the shared startup state for headless entrypoints:
// Discover → config → search tools → gate → session dir.
type runBootstrap struct {
	Proj      *project.Project
	Config    *project.Config
	Providers *provider.Manager
	OpenCode  *opencode.Source
	// ImportState is what became of the opencode import, recorded where it
	// happened. A headless run reports it to the developer-mode harness view;
	// the load error itself is not kept, because it names the files it read.
	ImportState diag.ImportFacts
	Cwd         string
	SessionDir  string
	Gate        permission.Gate
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
	// Resolve identity before assembling the gate and session storage: aliases
	// of one workspace must not create separate headless sessions.
	cwd, err := filepath.EvalSymlinks(proj.Root())
	if err != nil {
		return nil, fmt.Errorf("resolve headless workspace %q: %w", proj.Root(), err)
	}
	if err := proj.LoadConfig(); err != nil {
		return nil, err
	}
	providers, openCodeSource, importState, err := loadRuntimeSources(proj, proj.Config().OpenCode.Enabled)
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: providers/opencode:", err)
	}
	printConfigWarnings(proj.Config())
	bs := &runBootstrap{
		Proj:        proj,
		Config:      proj.Config(),
		Providers:   providers,
		OpenCode:    openCodeSource,
		ImportState: importState,
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
	gate, err := HeadlessGate(policy, cwd)
	if err != nil {
		return nil, fmt.Errorf("permissions: %w", err)
	}
	sessionDir := sessionDirOverride
	if sessionDir == "" {
		sessionDir = project.ProjectSessionDir(proj.Global().SessionBase(), cwd)
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
func loadRuntimeSources(
	proj *project.Project, enabled bool,
) (*provider.Manager, *opencode.Source, diag.ImportFacts, error) {
	providers, err := provider.Open(provider.Options{
		CachePath:       proj.Global().ProviderCatalogFile(),
		CredentialsPath: proj.Global().CredentialsFile(),
	})
	if err != nil {
		// The import resolves against the catalog, so a catalog that would
		// not open leaves it not loaded rather than failed: it never ran.
		return nil, nil, opencode.ImportObservation(enabled, nil, nil),
			fmt.Errorf("initialize provider catalog: %w", err)
	}
	if !enabled {
		return providers, nil, opencode.ImportObservation(false, nil, nil), nil
	}
	source, err := opencode.Load(opencode.Options{Catalog: providers.Providers()})
	if err != nil {
		return providers, nil, opencode.ImportObservation(true, nil, err),
			fmt.Errorf("load opencode source: %w", err)
	}
	return providers, source, opencode.ImportObservation(true, source, nil), nil
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

// headlessUIFacts is what a source asked a surface to be in a run that has no
// surface. It is read once, here, where the configuration is already in hand:
// a question about the keymap must never be what re-reads the preferences off
// disk, and a headless run reading them at all is only so that a setting
// somebody made does not vanish from the answer because nothing is painting.
//
// Each section is projected by its own owner, so what may be reported is
// decided where the secrets are: no chord, no command line, no device, no
// endpoint and no key ever reaches this function.
func headlessUIFacts(bs *runBootstrap) diag.UIConfigFacts {
	if bs == nil || bs.Config == nil {
		return diag.UIConfigFacts{}
	}
	facts := diag.UIConfigFacts{
		Known:         true,
		Keybinds:      keys.ObserveConfig(bs.Config.Keybinds),
		Notifications: notify.ObserveConfig(bs.Config.Notifications.Mode, bs.Config.Notifications.Sound),
		Voice:         voice.ObserveConfig(bs.Config.Voice),
	}
	// The stored dialect is taken raw rather than parsed: an empty string is
	// "nothing was persisted", which parses into the same mode an explicit
	// "standard" does, and the difference is the whole point of the layer.
	if bs.Proj != nil {
		if state, err := project.LoadUIState(bs.Proj.Global()); err == nil {
			facts.KeymapRead = true
			facts.Keymap = state.EditingMode
		}
	}
	return facts
}

// headlessPermissionOverlay names what a headless run puts between the
// configured permissions block and the boundary it assembles. --yolo replaces
// the whole boundary with one that judges nothing, and a policy that names no
// mode falls to headless-strict, where an approval nobody is there to answer
// becomes a refusal. With neither in force the configured policy is the one
// that was compiled, and the overlay is empty rather than inventing a reason
// for a difference that does not exist.
func headlessPermissionOverlay(policy permission.Policy, yolo bool) diag.Source {
	switch {
	case yolo || policy.DangerouslyAllowAll:
		return diag.Source{Kind: diag.SourceCLIFlag, Ref: "--yolo, or permissions.dangerously_allow_all"}
	case policy.Mode == "":
		return diag.Source{
			Kind: diag.SourceComputed,
			Ref:  "a headless run defaults to headless-strict: nobody is there to answer an approval",
		}
	default:
		return diag.Source{}
	}
}
