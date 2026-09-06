package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/runerror"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/usage"
	"github.com/alvnukov/cozyphi/internal/version"
)

// runOptions holds parsed `cozyphi run` flags.
type runOptions struct {
	prompt       string
	jsonl        bool
	yolo         bool
	maxRounds    int
	timeout      time.Duration
	session      string
	continueLast bool
	sessionDir   string
	// developerMode is read from args and from nowhere else: no config key,
	// no environment variable and no resumed session can set it, so the
	// read-only harness view exists for exactly the run the user asked for.
	developerMode bool
	help          bool
}

func runCmd(args []string) int {
	opts, err := parseRunArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi run:", err)
		printRunUsage(os.Stderr)
		return ExitUsage
	}
	if opts.help {
		printRunUsage(os.Stdout)
		return ExitOK
	}
	if strings.TrimSpace(opts.prompt) == "" {
		fmt.Fprintln(os.Stderr, "cozyphi run: prompt is required (-p \"...\")")
		printRunUsage(os.Stderr)
		return ExitUsage
	}
	if opts.continueLast && opts.session != "" {
		fmt.Fprintln(os.Stderr, "cozyphi run: --continue-last and --session are mutually exclusive")
		return ExitUsage
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	bs, err := loadRunBootstrap(ctx, project.GetDefaultProject(), opts.sessionDir, opts.yolo)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi run:", err)
		return ExitUsage
	}
	return runHeadless(ctx, bs, opts)
}

// runHeadless assembles one headless session from an explicitly bootstrapped workspace.
func runHeadless(ctx context.Context, bs *runBootstrap, opts runOptions) (exitCode int) {
	if opts.yolo {
		fmt.Fprintln(os.Stderr, "warning: --yolo skips all permission checks for this run")
	}

	// running is assigned once NewEngine returns and is read only through the
	// session-id accessor below, from the tool call the model makes inside
	// the loop this function later starts.
	var running *agent.Engine

	var owned *agent.Session
	defer func() {
		if err := owned.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "cozyphi run: close session:", err)
		}
	}()

	model, err := bs.requireModel()
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi run:", err)
		return ExitUsage
	}
	// The hook manager and the record of the load that built it are taken
	// together: the manager keeps only entries, so which directory defined
	// one and what the load had to skip survive nowhere else.
	hooksMgr, hooksLoad := loadRunHooks(bs)
	engineOpts := agent.EngineOpts{
		Model: model,
		SessionOpts: agent.SessionOpts{
			Cwd:          bs.Cwd,
			SessionDir:   bs.SessionDir,
			Persist:      true,
			ResumeID:     opts.session,
			ContinueLast: opts.continueLast,
		},
		Gate: bs.Gate,
		// Ask is nil: in headless mode any Ask decision is denied, so no
		// approval UI is ever reachable (Ask≡Deny even if the config mode
		// does not fold Ask).
		Ask:          nil,
		Hooks:        hooksMgr,
		ResolveModel: bs.findModel,
		ModelNames:   bs.modelNames,
	}

	// The MCP pool and the language-server manager are opened further down,
	// once the engine options are assembled. The collector reaches them
	// through these rather than through a copy, so a harness question asked
	// at turn time describes what this run ended up with instead of the
	// nothing it had here.
	var (
		mcpPool *mcp.Pool
		mcpLoad mcp.LoadFacts
		lspMgr  *lsp.Manager
		lspOpen lsp.OpenFacts
	)

	// Developer mode is a capability of this run, granted on the command line
	// and nowhere else. The registry is what carries it into the engine: with
	// no registry there is no harness tool, and nothing downstream can create
	// one. Owners keep their state — the collector reaches the session id
	// through an accessor, so it reports unavailable until the engine below
	// exists rather than a value invented here.
	if opts.developerMode {
		engineOpts.Diagnostics = diag.NewRegistry(nil, diag.DefaultLimits(),
			diag.NewRuntimeCollector(diag.RuntimeDeps{
				Version:   version.Version,
				Mode:      "headless",
				Enabled:   true,
				Workspace: func() string { return bs.Cwd },
				SessionID: func() string {
					if running == nil {
						return ""
					}
					return running.SessionID()
				},
			}),
			// The same two owners the TUI reports from: the loader for what
			// was configured, the engine for what is loaded and acting. A
			// headless run reaches the engine through the same accessor as
			// the session id, so the model category is unavailable until the
			// engine exists rather than answering from the model this
			// function resolved above.
			diag.NewModelCollector(diag.ModelDeps{
				Configured: func() diag.ModelFacts { return agent.ModelFacts(bs.Config.Model()) },
				ConfiguredSource: func() diag.Source {
					return diag.ModelSelectionSource(bs.Config.ModelEnvOverride(), bs.Config.DefaultModel != "")
				},
				State:     func() diag.ModelState { return running.ModelObservation() },
				Providers: bs.Providers.Observation,
				Import:    func() diag.ImportFacts { return bs.ImportState },
			}),
			// The boundary is observed where it stands, not recomputed from
			// the configuration: --yolo replaces it outright and the headless
			// default narrows it, so the permissions block on disk is only
			// ever the configured layer here.
			diag.NewPermissionCollector(diag.PermissionDeps{
				Configured: func() diag.PermissionFacts {
					return permission.PolicyObservation(bs.Config.Permissions)
				},
				Defaults: permission.DefaultObservation,
				Gate:     func() diag.GateFacts { return permission.Observe(bs.Gate) },
				Overlay:  func() diag.Source { return headlessPermissionOverlay(bs.Config.Permissions, opts.yolo) },
			}),
			// Which tools this run carries is the engine's own answer and
			// nobody else's: the same accessor as the model layer, so the
			// category reports unavailable until the engine exists rather
			// than listing the tools this function is about to ask for.
			diag.NewToolCollector(diag.ToolDeps{
				State: func() diag.ToolState { return running.ToolObservation() },
			}),
			// The context layer likewise: the window, the usage and the
			// record of what the last prompt render loaded all live on the
			// engine, and observing them re-reads none of it.
			diag.NewContextCollector(diag.ContextDeps{
				State: func() diag.ContextState { return running.ContextObservation() },
			}),
			diag.NewPlanCollector(diag.PlanDeps{
				State: func() diag.PlanState { return running.PlanObservation() },
			}),
			// MCP and LSP are observed where they live — the pool and the
			// manager — and never re-derived from the configuration this
			// function just read: a server counts as connected because a
			// call reached it, and as running because a query started it,
			// not because a file names it.
			diag.NewIntegrationCollector(diag.IntegrationDeps{
				MCP:   func() diag.MCPState { return mcp.Observe(mcpPool, mcpLoad) },
				LSP:   func() diag.LSPState { return lsp.Observe(lspMgr, lspOpen) },
				Hooks: func() diag.HooksState { return hooks.Observe(hooksMgr, hooksLoad) },
			}),
		)
	}

	history, _ := usage.Open(bs.Proj.Global().UsageFile())
	if store, err := memory.Open(bs.Proj.MemoryDir(), usage.Memory{
		Store: history,
		Dir:   bs.Proj.MemoryDir(),
	}); err != nil {
		fmt.Fprintln(os.Stderr, "warning: memory:", err)
	} else {
		engineOpts.Memory = store
	}

	if reg, err := tasks.Discover(bs.Proj.RepoRoot()); err != nil {
		fmt.Fprintln(os.Stderr, "warning: tasks:", err)
	} else if reg != nil {
		engineOpts.Tasks = reg
		engineOpts.TasksAccess = bs.Proj.Config().Permissions.Tasks
	}

	var lspQuery lsp.QueryFunc
	lspConfig := lsp.DefaultConfig()
	mgr, lspErr := lsp.Open(ctx, bs.Cwd, lspConfig)
	lspMgr, lspOpen = mgr, lsp.ObserveOpen(lspConfig, lspErr)
	if lspErr != nil {
		fmt.Fprintln(os.Stderr, "warning: lsp:", lspErr)
	} else if lspMgr != nil {
		lspQuery = lspMgr.Query
		engineOpts.LSP = lspQuery
		defer func() { _ = lspMgr.Close(context.Background()) }()
	}

	pool, mcpErr := mcp.LoadPoolInDir(bs.Proj.MCPConfigFile(), bs.Cwd, bs.OpenCode.MCPServers())
	mcpPool, mcpLoad = pool, mcp.ObserveLoad(mcpErr)
	if mcpErr != nil {
		fmt.Fprintln(os.Stderr, "warning: mcp:", mcpErr)
	} else if pool != nil {
		engineOpts.MCP = pool
		defer func() { _ = pool.Close() }()
	}
	if bs.Config.Agents.Enabled {
		engineOpts.JobRunner = runJobRunnerFactory(bs)
		jobs, jobErr := job.New(job.Options{
			Root:   bs.Proj.JobsDir(),
			Runner: engineOpts.JobRunner(model, engineOpts.Hooks, engineOpts.LSP),
		})
		if jobErr != nil {
			fmt.Fprintln(os.Stderr, "cozyphi run:", jobErr)
			return ExitUsage
		}
		// Close joins every runner before the earlier MCP/LSP defers release
		// services borrowed by the session and its children.
		defer func() {
			if err := jobs.Close(); err != nil {
				fmt.Fprintln(os.Stderr, "cozyphi run: close child assignments:", err)
				if exitCode == ExitOK {
					exitCode = ExitError
				}
			}
		}()
		engineOpts.Jobs = jobs
	}

	engine, err := agent.NewEngine(engineOpts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "cozyphi run:", err)
		return ExitUsage
	}
	owned = engine.Session()
	running = engine
	if opts.maxRounds > 0 {
		if err := engine.SetMaxRounds(opts.maxRounds); err != nil {
			fmt.Fprintln(os.Stderr, "cozyphi run:", err)
			return ExitUsage
		}
	}

	fmt.Fprintf(os.Stderr, "session: %s\n", engine.SessionID())
	if f := engine.SessionFile(); f != "" {
		fmt.Fprintf(os.Stderr, "file: %s\n", f)
	}

	runCtx := ctx
	if opts.timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, opts.timeout)
		defer cancel()
	}

	return runLoop(runCtx, engine, opts)
}

// runJobRunnerFactory resolves role pins at binding, not when a queued child starts.
// Only the factory consults the catalog; each runner retains a fixed tool snapshot.
func runJobRunnerFactory(bs *runBootstrap) agent.JobRunnerFactory {
	return func(model llm.ModelConfig, hooksManager *hooks.Manager, query tools.LSPQueryFunc) job.Runner {
		models := bs.Config.AgentModels(bs.findModel)
		resolved := make(map[job.Role]llm.ModelConfig)
		for _, role := range job.Roles() {
			if cfg, ok := models.For(role); ok {
				resolved[role] = cfg
			}
		}
		return agent.EngineRunner{
			Model: model, Hooks: hooksManager, LSP: query,
			ModelForRole: func(role job.Role) (llm.ModelConfig, bool) {
				cfg, ok := resolved[role]
				return cfg, ok
			},
		}
	}
}

// loadRunHooks discovers user + project hooks for headless `cozyphi run`.
// Failures are non-fatal (fail-open). Warnings go to debuglog and a one-line stderr hint.
//
// The load's own record comes back with the manager. A failed load still
// returns one — it says the load was attempted and failed, which is what
// tells a broken hook directory from a run that simply has no hooks.
func loadRunHooks(bs *runBootstrap) (*hooks.Manager, hooks.LoadFacts) {
	if bs == nil || bs.Proj == nil {
		return nil, hooks.LoadFacts{}
	}
	mgr, facts, warns, err := hooks.LoadObserved(bs.Proj.Global().HooksDir(), bs.Proj.HooksDir())
	if err != nil {
		fmt.Fprintln(os.Stderr, "warning: hooks:", err)
		return nil, facts
	}
	hooks.LogWarnings(warns)
	if summary := hooks.FormatWarningsSummary(warns); summary != "" {
		fmt.Fprintln(os.Stderr, summary)
	}
	return mgr, facts
}

// runLoop consumes the same engine.Loop the TUI uses — no second loop is
// implemented here — and maps events to stdout/stderr + exit codes.
func runLoop(ctx context.Context, engine *agent.Engine, opts runOptions) int {
	enc := &jsonlEncoder{out: os.Stdout, enabled: opts.jsonl}

	exit := ExitOK
	finalText := ""

	for ev, err := range engine.Loop(ctx, opts.prompt, agent.LoopOpts{}) {
		if err != nil {
			exit = exitCodeForRunError(err)
			enc.errorEvent(err.Error())
			fmt.Fprintln(os.Stderr, "error:", err)
			// The cause and its wording come from the shared classifier, the
			// same one the TUI prints; only the remedy is this surface's,
			// because a headless run has no slash commands. The hint says
			// what to do where the exit code alone cannot.
			if hint := runerror.Hint(err, headlessRemedies); hint != "" {
				fmt.Fprintln(os.Stderr, hint)
			}
			break
		}
		if ev == nil {
			continue
		}
		enc.event(ev)
		if !opts.jsonl {
			switch e := ev.(type) {
			case session.AssistantMessageUpdate:
				if e.Message.State == session.StateComplete {
					finalText = e.Message.FlatText()
				}
			case session.ToolData:
				r := e.Run
				fmt.Fprintf(os.Stderr, "tool: %s [%s] %s\n", r.Name, r.Status, truncate(r.Detail, 100))
				if r.Error != "" {
					fmt.Fprintln(os.Stderr, "  ", truncate(r.Error, 200))
				}
			}
		}
	}

	if ctxErr := ctx.Err(); ctxErr != nil && exit == ExitOK {
		exit = ExitError // context cancellation is not success
		if errors.Is(ctxErr, context.DeadlineExceeded) {
			enc.errorEvent(ctxErr.Error())
			fmt.Fprintln(os.Stderr, "error:", ctxErr)
		}
	}

	if !opts.jsonl && exit == ExitOK && strings.TrimSpace(finalText) != "" {
		fmt.Fprintln(os.Stdout, finalText)
	}

	enc.doneEvent(engine.SessionID(), engine.SessionFile(), exit)
	return exit
}

// --- flag parsing ---------------------------------------------------------

func parseRunArgs(args []string) (runOptions, error) {
	var o runOptions
	i := 0
	next := func(name string) (string, error) {
		i++
		if i >= len(args) {
			return "", fmt.Errorf("%s requires a value", name)
		}
		return args[i], nil
	}
	for ; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "-h" || arg == "--help":
			o.help = true
		case arg == "--jsonl":
			o.jsonl = true
		case arg == "--yolo":
			o.yolo = true
		case arg == "--developer-mode":
			o.developerMode = true
		case arg == "--continue-last":
			o.continueLast = true
		case arg == "-p" || arg == "--prompt":
			v, err := next(arg)
			if err != nil {
				return o, err
			}
			o.prompt = v
		case strings.HasPrefix(arg, "--prompt="):
			o.prompt = strings.TrimPrefix(arg, "--prompt=")
		case strings.HasPrefix(arg, "-p="):
			o.prompt = strings.TrimPrefix(arg, "-p=")
		case arg == "--max-rounds":
			v, err := next(arg)
			if err != nil {
				return o, err
			}
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return o, fmt.Errorf("--max-rounds must be a positive integer, got %q", v)
			}
			o.maxRounds = n
		case strings.HasPrefix(arg, "--max-rounds="):
			v := strings.TrimPrefix(arg, "--max-rounds=")
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 {
				return o, fmt.Errorf("--max-rounds must be a positive integer, got %q", v)
			}
			o.maxRounds = n
		case arg == "--timeout":
			v, err := next(arg)
			if err != nil {
				return o, err
			}
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				return o, fmt.Errorf("--timeout must be a positive duration, got %q", v)
			}
			o.timeout = d
		case strings.HasPrefix(arg, "--timeout="):
			v := strings.TrimPrefix(arg, "--timeout=")
			d, err := time.ParseDuration(v)
			if err != nil || d <= 0 {
				return o, fmt.Errorf("--timeout must be a positive duration, got %q", v)
			}
			o.timeout = d
		case arg == "--session":
			v, err := next(arg)
			if err != nil {
				return o, err
			}
			o.session = v
		case strings.HasPrefix(arg, "--session="):
			o.session = strings.TrimPrefix(arg, "--session=")
		case arg == "--session-dir":
			v, err := next(arg)
			if err != nil {
				return o, err
			}
			o.sessionDir = v
		case strings.HasPrefix(arg, "--session-dir="):
			o.sessionDir = strings.TrimPrefix(arg, "--session-dir=")
		default:
			return o, fmt.Errorf("unknown flag %q", arg)
		}
	}
	return o, nil
}

func printRunUsage(w *os.File) {
	fmt.Fprintf(w, `usage: cozyphi run -p "PROMPT" [flags]

Run one agent loop headlessly and exit. Human logs go to stderr; with
--jsonl, machine-readable events go to stdout (one JSON object per line).

flags:
  -p, --prompt STRING   prompt to run (required)
      --jsonl           emit JSONL events to stdout
      --yolo            skip all permission checks for this run (benchmarks / CI only)
      --developer-mode  let the model read cozyphi's own configuration (read-only harness tool)
      --max-rounds N    cap tool rounds (default 64)
      --timeout DURATION stop after a wall-clock duration (e.g. 10m; default unlimited)
      --session ID      resume a persisted session by id or unique prefix
      --continue-last   resume the newest persisted session for this directory
      --session-dir DIR override the session storage directory
  -h, --help            show this help

exit codes:
  0 success   1 runtime/LLM error   2 max rounds   3 config/usage
`)
}

// --- JSONL event schema ---------------------------------------------------

// jsonlEncoder writes the pinned event schema to a writer. Fields are
// explicit so the wire format never depends on Go struct tags of internal
// session types and never carries API keys or other config secrets.
type jsonlEncoder struct {
	out     io.Writer
	enabled bool
}

func (enc *jsonlEncoder) emit(v any) {
	if enc == nil || !enc.enabled {
		return
	}
	data, err := json.Marshal(v)
	if err != nil {
		fmt.Fprintf(enc.out, `{"type":"error","message":%q}`+"\n", "encode event: "+err.Error())
		return
	}
	_, _ = enc.out.Write(data)
	_, _ = enc.out.Write([]byte("\n"))
}

type jsonlAssistant struct {
	Type     string      `json:"type"` // "assistant"
	ID       string      `json:"id"`
	State    string      `json:"state"` // streaming | complete | cancelled | error
	Reason   string      `json:"reason,omitempty"`
	Text     string      `json:"text"`
	Thinking string      `json:"thinking,omitempty"`
	Usage    *jsonlUsage `json:"usage,omitempty"`
}

type jsonlUsage struct {
	Prompt     int `json:"prompt"`
	Completion int `json:"completion"`
	Total      int `json:"total"`
}

type jsonlTool struct {
	Type      string `json:"type"` // "tool"
	ToolUseID string `json:"toolUseId"`
	ToolName  string `json:"toolName,omitempty"`
	Status    string `json:"status"` // in-progress | done | error | cancelled | rejected
	Detail    string `json:"detail,omitempty"`
	Output    string `json:"output,omitempty"`
}

type jsonlCompaction struct {
	Type   string `json:"type"`  // "compaction"
	Phase  string `json:"phase"` // started | complete
	Failed bool   `json:"failed,omitempty"`
}

type jsonlError struct {
	Type    string `json:"type"` // "error"
	Message string `json:"message"`
}

type jsonlDone struct {
	Type      string `json:"type"` // "done"
	SessionID string `json:"sessionId,omitempty"`
	File      string `json:"file,omitempty"`
	ExitCode  int    `json:"exitCode"`
}

func (enc *jsonlEncoder) event(ev session.Event) {
	switch e := ev.(type) {
	case session.AssistantMessageUpdate:
		m := e.Message
		var usage *jsonlUsage
		if m.Usage.Reported() {
			usage = &jsonlUsage{
				Prompt:     m.Usage.PromptTokens,
				Completion: m.Usage.CompletionTokens,
				Total:      m.Usage.TotalTokens,
			}
		}
		enc.emit(jsonlAssistant{
			Type:     "assistant",
			ID:       m.ID,
			State:    m.State.String(),
			Reason:   reasonString(m.StopReason),
			Text:     assistantText(m),
			Thinking: thinkingText(m.Content),
			Usage:    usage,
		})
	case session.ToolData:
		r := e.Run
		enc.emit(jsonlTool{
			Type:      "tool",
			ToolUseID: r.ToolUseID,
			ToolName:  r.Name,
			Status:    r.Status.String(),
			Detail:    r.Detail,
			Output:    r.Output,
		})
	case session.CompactionStarted:
		enc.emit(jsonlCompaction{Type: "compaction", Phase: "started"})
	case session.CompactionComplete:
		enc.emit(jsonlCompaction{Type: "compaction", Phase: "complete", Failed: e.Failed})
	}
}

func (enc *jsonlEncoder) errorEvent(message string) {
	enc.emit(jsonlError{Type: "error", Message: message})
}

func (enc *jsonlEncoder) doneEvent(sessionID, file string, exit int) {
	enc.emit(jsonlDone{Type: "done", SessionID: sessionID, File: file, ExitCode: exit})
}

func reasonString(r session.StopReason) string {
	switch r {
	case session.StopEndTurn:
		return "end_turn"
	case session.StopToolUse:
		return "tool_use"
	case session.StopMaxTokens:
		return "max_tokens"
	default:
		return ""
	}
}

func thinkingText(blocks []session.ContentBlock) string {
	var out strings.Builder
	for _, b := range blocks {
		if b.Type == session.BlockThinking {
			out.WriteString(b.Text)
		}
	}
	return out.String()
}

// assistantText joins the text blocks of an assistant event. It does not use
// Message.FlatText because raw events may carry the zero Role (RoleUser),
// which would make FlatText fall back to m.Text.
func assistantText(m session.Message) string {
	var out strings.Builder
	for _, b := range m.Content {
		if b.Type == session.BlockText {
			out.WriteString(b.Text)
		}
	}
	if out.Len() == 0 {
		return m.Text
	}
	return out.String()
}

// headlessRemedies are the fixes this surface can offer. /connect and
// /compact belong to the TUI, so a headless run names the file and the
// prompt instead.
var headlessRemedies = runerror.Remedies{
	Auth:            "Set a valid API key in the config and retry.",
	ContextOverflow: "Shorten the prompt or raise the model's context_window, then retry.",
}

// exitCodeForRunError maps a loop error to the exit-code contract:
// max rounds → 2, anything else → 1. The cause a person reads is
// runerror.Classify; this is only the number the shell sees.
func exitCodeForRunError(err error) int {
	if errors.Is(err, agent.ErrMaxRounds) {
		return ExitMaxRounds
	}
	return ExitError
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
