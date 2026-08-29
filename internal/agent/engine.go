package agent

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/alvnukov/cozyphi/internal/agent/prompt"
	"github.com/alvnukov/cozyphi/internal/hooks"
	"github.com/alvnukov/cozyphi/internal/job"
	"github.com/alvnukov/cozyphi/internal/llm"
	llmclient "github.com/alvnukov/cozyphi/internal/llm/client"
	"github.com/alvnukov/cozyphi/internal/llm/skills"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/memory"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/plangate"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// ErrMaxRounds is returned (wrapped) by Loop when the model exceeds the
// configured tool-round budget and continuation is declined or unavailable.
// Callers can distinguish it from other runtime errors with errors.Is,
// e.g. for a dedicated exit code.
var ErrMaxRounds = errors.New("exceeded maximum tool rounds")

var errEventConsumerStopped = errors.New("event consumer stopped")

const defaultMaxToolRounds = 128

// ContinueFunc asks whether to grant another maxRounds budget after the
// current budget is exhausted. Nil means hard-fail with ErrMaxRounds
// (headless / sub-agent default). True continues the loop with a fresh budget.
type ContinueFunc func(ctx context.Context, maxRounds int) (bool, error)

// Engine drives the agent loop: stream → tools → stream…
// and yields session.Event for the TUI reducer. Context compaction is owned
// here so Session stays a thin message store.
type Engine struct {
	// mu guards every field below: setters (TUI goroutine) swap the client and
	// executor as a pair under the write lock while a running round works off
	// an immutable roundSnapshot, so changes land at round boundaries and a
	// round always finishes under the posture it started with.
	mu            sync.RWMutex
	client        *llmclient.Client
	executor      *Executor
	maxRounds     int
	stopOnLimit   bool
	mode          Mode
	skillPath     string
	contextWindow int
	modelCfg      llm.ModelConfig
	resolveModel  func(string) (llm.ModelConfig, bool)
	modelNames    func() []string
	gate          permission.Gate
	ask           permission.AskFunc
	continueAsk   ContinueFunc
	jobs          *job.Manager
	hooks         *hooks.Manager
	mcp           *mcp.Pool
	memory        *memory.Store
	watches       *watch.Manager
	// memoryPrompt is the memory block baked into the current client, so a
	// fact written mid-turn can be told from one the model already sees.
	memoryPrompt  string
	lsp           tools.LSPQueryFunc
	questionAsk   func(ctx context.Context, qs []tools.Question) ([]tools.QuestionAnswer, error)
	onPlanUpdated func(session.Plan)
	// sessionEvents receives live events the engine emits outside a streaming
	// round — plan action runs today. Nil drops them; records stay durable.
	sessionEvents       func(session.Event)
	planEnabled         bool
	planGate            *plangate.Checker // nil until planEnabled; Hint phase by default
	planRuntime         *plangate.Runtime // immutable live policy shared by replacement engines
	projectedPlanPolicy *plangate.Policy  // policy baked into the current prompt and schemas
	autoApprove         func() bool       // if set and true, updatePlan approves a revised plan synchronously
	// baseTools is the tool set from EngineOpts.Tools; nil means DefaultTools.
	// rebindTools rebuilds from it so setters never widen a readonly engine.
	baseTools []tools.Tool

	session *Session
	// pendingCompact records a model-requested compaction (context tool).
	// Loop applies it at the next tool-round boundary, then clears it.
	pendingCompact bool

	// planSkills parks skill names queued by inject_skill plan actions; the
	// next composed user prompt drains the queue into its instruction.
	planSkills []string

	// planModelSaved/planModelActive remember the session model while plan
	// step models are in play; closing the plan hands it back.
	planModelSaved  llm.ModelConfig
	planModelActive bool

	// telemetrySink is the live session's plan telemetry manager, held
	// atomically: the projection record fires inside systemPrompt, which
	// rebindClient runs under mu, and the mutex is not reentrant. Swapped in
	// lockstep with session (construction, ReplaceSession).
	telemetrySink atomic.Pointer[session.Manager]
	// projectionLast is the byte count of the last recorded projection;
	// systemPrompt dedupes byte-stable re-renders against it so internal
	// rebinds (publishPlan, memory sync) do not spend budget. Guarded by mu.
	projectionLast uint64
}

// roundRuntime is the immutable view one inference/tool round runs against.
// The client and executor are swapped as a pair under mu together with the
// round-budget knobs, so a round started under one posture finishes under it
// and setter changes land at the next round boundary.
type roundRuntime struct {
	client        *llmclient.Client
	executor      *Executor
	maxRounds     int
	stopOnLimit   bool
	contextWindow int
	modelName     string
	continueAsk   ContinueFunc
}

// roundSnapshot copies the fields a running round reads, under the read lock.
func (engine *Engine) roundSnapshot() roundRuntime {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return roundRuntime{
		client:        engine.client,
		executor:      engine.executor,
		maxRounds:     engine.maxRounds,
		stopOnLimit:   engine.stopOnLimit,
		contextWindow: engine.contextWindow,
		modelName:     engine.modelCfg.Name,
		continueAsk:   engine.continueAsk,
	}
}

// sessionRef returns the current session store pointer under the read lock.
func (engine *Engine) sessionRef() *Session {
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.session
}

// EngineOpts configures NewEngine.
type EngineOpts struct {
	Model         llm.ModelConfig
	SessionOpts   SessionOpts
	Gate          permission.Gate                                                                // nil = allow all
	Ask           permission.AskFunc                                                             // nil = deny on Ask
	ContinueAsk   ContinueFunc                                                                   // nil = ErrMaxRounds on budget exhaust
	Tools         []tools.Tool                                                                   // nil = tools.DefaultTools(); sub-agents use ChildTools()
	MaxRounds     int                                                                            // 0 = package default
	Jobs          *job.Manager                                                                   // if set, register agent_* tools on this engine
	Hooks         *hooks.Manager                                                                 // nil = no hooks; child engines inherit parent Manager
	MCP           *mcp.Pool                                                                      // if set, register mcp_list/inspect/call meta-tools
	Memory        *memory.Store                                                                  // if set, carry memory in the system prompt and recall past-budget facts per turn
	Watches       *watch.Manager                                                                 // if set, register the watch tool; events are delivered by the session, not here
	LSP           tools.LSPQueryFunc                                                             // if set, register the lsp tool
	QuestionAsk   func(ctx context.Context, qs []tools.Question) ([]tools.QuestionAnswer, error) // if set, register the question tool
	PlanUpdated   func(session.Plan)                                                             // called after a durable primary-session plan update
	SessionEvents func(session.Event)                                                            // if set, receives events emitted outside a streaming round (plan action runs)
	AutoApprove   func() bool                                                                    // if set and true, updatePlan approves a revised plan before returning it
	PlanRuntime   *plangate.Runtime                                                              // nil = built-in defaults; read at each tool call
	ResolveModel  func(string) (llm.ModelConfig, bool)                                           // map a resumed session model name
	// ModelNames lists every model a plan pin may reference; nil means the
	// environment cannot enumerate them and the plan tool skips its
	// authoring check (step-start resolution still fails closed).
	ModelNames func() []string
}

// NewEngine wires an LLM client, tool executor, and session store.
func NewEngine(opts EngineOpts) (*Engine, error) {
	if opts.SessionOpts.Model == "" {
		opts.SessionOpts.Model = opts.Model.Name
	}
	sess, err := NewSession(opts.SessionOpts)
	if err != nil {
		return nil, err
	}
	cfg := opts.Model
	if (opts.SessionOpts.ResumePath != "" || opts.SessionOpts.ResumeID != "") && opts.ResolveModel != nil {
		if name := sess.Model(); name != "" && name != opts.Model.Name {
			if resolved, ok := opts.ResolveModel(name); ok {
				cfg = resolved
			}
		}
	}
	engine := &Engine{
		maxRounds:     defaultMaxToolRounds,
		stopOnLimit:   true,
		skillPath:     cfg.SkillPath,
		contextWindow: cfg.ContextWindow,
		modelCfg:      cfg,
		resolveModel:  opts.ResolveModel,
		modelNames:    opts.ModelNames,
		session:       sess,
		gate:          opts.Gate,
		ask:           opts.Ask,
		continueAsk:   opts.ContinueAsk,
		jobs:          opts.Jobs,
		hooks:         opts.Hooks,
		mcp:           opts.MCP,
		memory:        opts.Memory,
		watches:       opts.Watches,
		lsp:           opts.LSP,
		questionAsk:   opts.QuestionAsk,
		onPlanUpdated: opts.PlanUpdated,
		sessionEvents: opts.SessionEvents,
		autoApprove:   opts.AutoApprove,
		planEnabled:   opts.SessionOpts.ParentID == "" && opts.Tools == nil,
		baseTools:     opts.Tools,
		mode:          ModeUsePlan,
	}
	engine.telemetrySink.Store(sess.manager)
	if engine.planEnabled {
		engine.planRuntime = opts.PlanRuntime
		if engine.planRuntime == nil {
			engine.planRuntime, err = plangate.NewRuntime(plangate.DefaultDefaults())
			if err != nil {
				return nil, err
			}
		}
		engine.planGate = plangate.NewChecker(plangate.PhaseHint)
		engine.planGate.Runtime = engine.planRuntime
	}
	// The primary posture is useplan: keep the gate's enforcement phase in
	// lockstep from the first round.
	engine.applyPlanGatePhase()
	if opts.MaxRounds > 0 {
		engine.maxRounds = opts.MaxRounds
	}
	engine.rebindTools()
	return engine, nil
}

// buildToolList assembles the tool set for the current posture.
func (engine *Engine) buildToolList() []tools.Tool {
	return engine.buildToolListFor(engine.mode)
}

// buildToolListFor assembles the tool set for one posture: the configured
// base (DefaultTools when EngineOpts.Tools was nil) plus MCP and agent_*
// tools for whichever managers are attached. Plan mode narrows only the
// built-in set; explicitly configured tools stay (their reach is bounded by
// the permission policy, not the mode).
func (engine *Engine) buildToolListFor(mode Mode) []tools.Tool {
	base := engine.baseTools
	if base == nil {
		base = tools.DefaultTools()
	}
	if mode == ModePlan && engine.baseTools == nil {
		base = tools.ReadonlyTools()
	}
	out := append([]tools.Tool(nil), base...)
	if engine.planEnabled {
		out = append(out, tools.PlanTool(tools.PlanDeps{
			Update:     engine.updatePlan,
			Create:     engine.createPlan,
			Get:        engine.getPlan,
			Patch:      engine.PatchPlan,
			Transition: engine.transitionPlan,
			Telemetry:  engine.planTelemetry,
			StepTypes:  engine.planRuntime.Current().StepTypes(),
		}))
	}
	if engine.questionAsk != nil {
		out = append(out, tools.QuestionTool(tools.QuestionDeps{Ask: engine.questionAsk}))
	}
	// The context tool rides on every engine (main and sub-agents): it only
	// reports usage numbers and compacts the engine's own context view.
	out = append(out, tools.ContextTools(tools.ContextDeps{
		Stats:          engine.contextStats,
		RequestCompact: engine.requestCompact,
	}))
	if engine.mcp != nil {
		if mcpTools := tools.MCPTools(engine.mcp); len(mcpTools) > 0 {
			out = append(out, mcpTools...)
		}
	}
	// The lsp tool rides on every engine with a borrowed query function and
	// stays before plan-step injection so the primary gate still sees it.
	if engine.lsp != nil {
		out = append(out, tools.LSPTool(engine.lsp))
	}
	// The memory tool reaches what the system prompt had no room for: the full
	// catalog, and any fact by name. Read-only — memories are written with
	// `write`, through the gate, like any other file.
	if engine.memory != nil {
		out = append(out, tools.MemoryTool(engine.memory))
	}
	// The watch tool starts background work that outlives the turn. Delivery
	// is the session's job: an engine that had to poll its own watches would
	// be the loop a watch exists to remove.
	if engine.watches != nil {
		out = append(out, tools.WatchTool(tools.WatchDeps{Manager: engine.watches}))
	}
	if engine.jobs != nil {
		out = append(out, tools.AgentTools(tools.AgentDeps{
			Manager:  engine.jobs,
			ParentID: engine.SessionID,
			WorkDir:  engine.SessionCwd,
		})...)
	}
	// Inject plan_step last: every gateable tool must carry it, including the
	// MCP meta-tools and agent_* tools that are appended after the base set.
	if engine.planEnabled {
		out = engine.planRuntime.Current().InjectPlanStep(out)
	}
	return out
}

// SetModel replaces the LLM client and model-related settings without
// discarding the session tree. Agent tools remain registered when Jobs is set.
func (engine *Engine) SetModel(cfg llm.ModelConfig) error {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	// A user switch supersedes the plan's saved default: closing the plan
	// must not restore a model the user already replaced by hand.
	engine.planModelActive = false
	engine.setModelLocked(cfg)
	return nil
}

// setModelLocked is the swap core for callers already holding engine.mu.
func (engine *Engine) setModelLocked(cfg llm.ModelConfig) {
	engine.modelCfg = cfg
	engine.skillPath = cfg.SkillPath
	engine.contextWindow = cfg.ContextWindow
	engine.rebindTools()
}

// ModelConfig returns the model the engine is currently configured with.
func (engine *Engine) ModelConfig() llm.ModelConfig {
	if engine == nil {
		return llm.ModelConfig{}
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.modelCfg
}

// SetJobs attaches or detaches the job manager and rebuilds the tool list.
// Pass nil to unregister agent_* tools (sub-agents disabled).
func (engine *Engine) SetJobs(jobs *job.Manager) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.jobs = jobs
	engine.rebindTools()
}

// rebindTools rebuilds the client and the executor from the current fields.
// The caller must hold engine.mu: the swap of the pair is what keeps a
// running round (working off its snapshot) coherent.
func (engine *Engine) rebindTools() {
	for {
		var policy *plangate.Policy
		if engine.planEnabled {
			policy = engine.planRuntime.Current()
		}
		toolList := engine.buildToolList()
		if engine.planEnabled && engine.planRuntime.Current() != policy {
			continue
		}
		engine.rebindClient(toolList)
		engine.bindExecutor(tools.NewRegistry(toolList))
		engine.projectedPlanPolicy = policy
		return
	}
}

// ToolNames returns the tools present in the current engine runtime.
func (engine *Engine) ToolNames() []string {
	if engine == nil {
		return nil
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	toolList := engine.buildToolListFor(ModeUsePlan)
	names := make([]string, 0, len(toolList))
	for _, tool := range toolList {
		names = append(names, tool.Definition.Name)
	}
	return names
}

// rebindClient rebuilds the provider client; the caller must hold engine.mu.
func (engine *Engine) rebindClient(toolList []tools.Tool) {
	engine.client = llmclient.NewClient(
		engine.modelCfg,
		tools.Definitions(engine.visibleToProvider(toolList)),
		engine.systemPrompt(),
	)
}

// visibleToProvider narrows the tool list to what the current plan state
// permits the model to see: in deny phase (useplan) the provider schemas match
// the gate — exempt tools plus whatever the in_progress steps allow. The
// executor registry keeps the full list, so a call to a hidden tool,
// hallucinated from an earlier round, still resolves and gets the plan gate's
// actionable reason instead of an unknown-tool error. The caller must hold
// engine.mu.
func (engine *Engine) visibleToProvider(toolList []tools.Tool) []tools.Tool {
	if !engine.planEnabled || engine.planGate == nil || engine.planGate.Phase != plangate.PhaseDeny {
		return toolList
	}
	var plan session.Plan
	if engine.session != nil {
		plan = engine.session.Plan()
	}
	visible := engine.planRuntime.Current().VisibleTools(plan)
	kept := make([]tools.Tool, 0, len(toolList))
	for _, tool := range toolList {
		if _, ok := visible[tool.Definition.Name]; ok {
			kept = append(kept, tool)
		}
	}
	return kept
}

func (engine *Engine) systemPrompt() string {
	var mcpServers []string
	if engine.mcp != nil {
		mcpServers = engine.mcp.ServerNames()
	}
	system := prompt.Build(prompt.Options{
		SkillPath:  engine.skillPath,
		Agents:     engine.jobs != nil,
		LSP:        engine.lsp != nil,
		Watches:    engine.watches != nil,
		MCPServers: mcpServers,
		Plan:       engine.mode == ModePlan,
	})
	// Recorded, not just appended: syncMemory compares against this to see
	// whether a turn changed what memory contributes to the prompt.
	engine.memoryPrompt = engine.memory.PromptBlock()
	if engine.memoryPrompt != "" {
		system += "\n\n" + engine.memoryPrompt
	}
	if engine.planEnabled {
		injected := 0
		if engine.planGate != nil {
			block := "\n\n" + engine.planRuntime.Current().PromptBlock(engine.planGate.Phase)
			system += block
			injected += len(block)
		}
		if hint := tools.PlanHint(engine.planLocked()); hint != "" {
			block := "\n\n" + hint
			system += block
			injected += len(block)
		}
		// Account the prompt budget the plan feature just spent; the number
		// is the byte length actually injected, nothing recomputed. A
		// byte-stable re-render hands the client the prompt it already holds,
		// so only a changed projection spends budget.
		if u := uint64(injected); u != engine.projectionLast {
			engine.projectionLast = u
			engine.recordPlanProjection(injected)
		}
	}
	return system
}

// bindExecutor installs a freshly built executor; the caller must hold
// engine.mu so the swap pairs with the client swap.
func (engine *Engine) bindExecutor(registry tools.Registry) {
	engine.executor = NewExecutor(registry, engine.gate, engine.ask, engine.hooks)
	if engine.session != nil {
		engine.executor.SetMeta(engine.session.ID(), engine.session.Cwd())
	}
	if engine.planEnabled && engine.planGate != nil {
		engine.executor.SetPlanGate(
			engine.planGate, engine.Plan, engine.autoStartStep, engine.settlePlanFromCall,
			engine.recordStepAttempt, engine.approveStepJIT,
		)
		engine.executor.SetPlanTelemetry(engine.recordPlanMiss, engine.recordPlanOnlyRound)
	}
}

// HasTool reports whether a tool is currently registered on the executor.
func (engine *Engine) HasTool(name string) bool {
	if engine == nil {
		return false
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	if engine.executor == nil {
		return false
	}
	_, ok := engine.executor.registry[name]
	return ok
}

// Jobs returns the process-level job manager, if any.
func (engine *Engine) Jobs() *job.Manager {
	if engine == nil {
		return nil
	}
	return engine.jobs
}

// SetMaxRounds bounds the number of tool rounds per Loop call.
// Non-positive values are rejected.
func (engine *Engine) SetMaxRounds(n int) error {
	if engine == nil {
		return nil
	}
	if n <= 0 {
		return fmt.Errorf("agent: max rounds must be positive (got %d)", n)
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.maxRounds = n
	return nil
}

// SetStopOnLimit toggles whether the tool-round budget stops the loop.
// When disabled, the loop runs without a tool-round cap until the turn ends.
func (engine *Engine) SetStopOnLimit(enabled bool) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.stopOnLimit = enabled
}

// StopOnLimit reports whether the tool-round budget stops the loop.
func (engine *Engine) StopOnLimit() bool {
	if engine == nil {
		return false
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return engine.stopOnLimit
}

// Mode returns the current turn posture (useplan by default).
func (engine *Engine) Mode() Mode {
	if engine == nil {
		return ModeUsePlan
	}
	engine.mu.RLock()
	defer engine.mu.RUnlock()
	return normalizeMode(engine.mode)
}

// SetMode switches the turn posture. Unknown modes fall back to useplan.
// Switching rebinds the client: plan adds the plan appendix to the system
// prompt and narrows the built-in tool set to the read-only tools.
func (engine *Engine) SetMode(m Mode) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.mode = normalizeMode(m)
	engine.applyPlanGatePhase()
	engine.rebindTools()
}

// SetPermission updates the gate and ask handler. The running executor is
// never mutated in place: the rebuild hands the next tool round the new gate
// while a mid-flight round finishes under the one it started with.
func (engine *Engine) SetPermission(gate permission.Gate, ask permission.AskFunc) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.gate = gate
	engine.ask = ask
	engine.rebindTools()
}

// SetContinueAsk sets the handler invoked when the tool-round budget is exhausted.
// Pass nil to hard-fail with ErrMaxRounds (default for headless runs).
func (engine *Engine) SetContinueAsk(fn ContinueFunc) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.continueAsk = fn
}

// SetHooks replaces the hooks manager. Pass nil to disable hooks.
// Does not drop Gate/Ask; rebindTools also preserves hooks.
func (engine *Engine) SetHooks(mgr *hooks.Manager) {
	if engine == nil {
		return
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.hooks = mgr
	engine.rebindTools()
}

// SessionID returns the durable session id.
func (engine *Engine) SessionID() string {
	sess := engine.sessionRef()
	if sess == nil {
		return ""
	}
	return sess.ID()
}

// SessionFile returns the JSONL path (empty in memory mode).
func (engine *Engine) SessionFile() string {
	sess := engine.sessionRef()
	if sess == nil {
		return ""
	}
	return sess.File()
}

// SessionCwd returns the cwd recorded on the session header.
func (engine *Engine) SessionCwd() string {
	sess := engine.sessionRef()
	if sess == nil {
		return ""
	}
	return sess.Cwd()
}

// ReplaceSession swaps the session store (used by /resume, between turns).
func (engine *Engine) ReplaceSession(opts SessionOpts) error {
	sess, err := NewSession(opts)
	if err != nil {
		return err
	}
	engine.mu.Lock()
	defer engine.mu.Unlock()
	engine.session = sess
	engine.telemetrySink.Store(sess.manager)
	if engine.executor != nil {
		engine.executor.SetMeta(sess.ID(), sess.Cwd())
	}
	return nil
}

// Session returns the underlying session wrapper (for UI transcript replay).
func (engine *Engine) Session() *Session {
	if engine == nil {
		return nil
	}
	return engine.sessionRef()
}

// LoopOpts configures a single agent loop turn.
type LoopOpts struct {
	// PendingSkills are skill names the user selected in the composer.
	// When set, the model is instructed to read those SKILL.md files first.
	PendingSkills []string

	// Media is inline image content attached to the user's prompt.
	Media []llm.Media

	// Inject, when set, is polled at every tool-round boundary. Each returned
	// prompt is appended to the session as a user message mid-turn, so the
	// model answers queued user input inside the SAME turn instead of after
	// it ends; session.UserPromoted tells the UI to drop the queued hint.
	Inject func() []InjectedPrompt
}

// InjectedPrompt is one queued user message pulled into a running turn.
type InjectedPrompt struct {
	Text   string
	Skills []string
	Media  []llm.Media
	UserID string // transcript row id; empty when the caller has no row
}

// Loop appends the user prompt and runs inference + tool rounds until the
// model stops calling tools or the context is cancelled.
//
// Compaction: persist the turn first, then check usage after
// the agent turn ends (final assistant with no tool_calls) — never mid-tool-loop.
func (engine *Engine) Loop(ctx context.Context, prompt string, opts LoopOpts) iter.Seq2[session.Event, error] {
	return func(yield func(session.Event, error) bool) {
		// The turn runs against one session store even if /resume swaps the
		// engine's underneath a stopped loop.
		sess := engine.sessionRef()
		// A compaction request from a cancelled turn never leaks into the next.
		engine.pendingCompact = false
		if _, err := sess.RepairPendingToolCalls(); err != nil {
			yield(nil, fmt.Errorf("agent: repair interrupted tool round: %w", err))
			return
		}
		// One recall pass per turn, and only past the prompt budget: a memory
		// surfaced for the opening prompt is not repeated for the prompts
		// queued after it.
		recall := engine.memory.Turn()
		// A memory written during this turn becomes visible to the next one
		// here — on every exit, so a cancelled turn does not lose it.
		defer engine.syncMemory()

		content := prompt
		if engine.jobs != nil {
			if role, task, ok := splitDelegationPrefix(prompt); ok {
				content = delegationInstruction(role) + "\n\n" + task
			}
		}
		content = engine.composeUserPrompt(recall, opts.PendingSkills, prompt, content)
		if err := sess.Append(llm.Message{
			Role:    llm.RoleUser,
			Content: content,
			Media:   opts.Media,
		}); err != nil {
			yield(nil, err)
			return
		}

		toolRounds := 0
		overflowRetried := false
		for {
			if ctx.Err() != nil {
				return
			}

			// One immutable runtime per round: setters (mode, permission,
			// hooks, model) swap the client/executor pair under mu, and the
			// change lands at this boundary — never mid-round. The plan
			// projection syncs first, so a live-policy change lands in this
			// round's snapshot, not the next one's.
			engine.syncPlanProjection()
			rt := engine.roundSnapshot()

			msgs := engine.inferenceContext(sess)

			msg, completeEvent, ok, streamErr := streamTurn(ctx, yield, msgs, rt)
			if !ok {
				if streamErr == nil {
					return
				}
				// A provider context-overflow rejection is recoverable: compact
				// once, then retry the same turn with the shrunken context.
				if llm.IsContextOverflow(streamErr) && !overflowRetried {
					overflowRetried = true
					if compacted, err := engine.compactForOverflow(ctx, yield, rt); err != nil {
						if errors.Is(err, errEventConsumerStopped) {
							return
						}
						yield(nil, err)
						return
					} else if compacted {
						continue
					}
				}
				yield(nil, streamErr)
				return
			}

			// Defer publishing and persisting the terminal assistant update until
			// the tool budget is checked. An over-budget tool request must not
			// leave an unexecuted tool call in the session or UI.
			if rt.stopOnLimit && len(msg.ToolCalls) > 0 && toolRounds >= rt.maxRounds {
				if rt.continueAsk == nil {
					yield(nil, fmt.Errorf("agent: %w (%d)", ErrMaxRounds, rt.maxRounds))
					return
				}
				ok, err := rt.continueAsk(ctx, rt.maxRounds)
				if err != nil {
					yield(nil, err)
					return
				}
				if !ok {
					yield(nil, fmt.Errorf("agent: %w (%d)", ErrMaxRounds, rt.maxRounds))
					return
				}
				// Granted: reset the budget; the current and following tool
				// rounds run under the fresh budget.
				toolRounds = 0
			}
			if !yield(completeEvent, nil) {
				return
			}

			if err := sess.AppendAssistant(msg, rt.modelName); err != nil {
				yield(nil, err)
				return
			}

			if len(msg.ToolCalls) == 0 {
				// Turn finished — compact using this assistant's usage (pi agent_end).
				if err := engine.maybeCompact(ctx, yield, msg.Usage.TotalTokens, rt); err != nil {
					if errors.Is(err, errEventConsumerStopped) {
						return
					}
					yield(nil, err)
				}
				return
			}

			toolRounds++
			toolMsgs, active := rt.executor.run(ctx, msg.ToolCalls, func(td session.ToolData) bool {
				return yield(td, nil)
			})
			if err := sess.Append(toolMsgs...); err != nil {
				yield(nil, err)
				return
			}
			if !active {
				return
			}

			if ctx.Err() != nil {
				return
			}

			// Tool-round boundary: a model-requested compaction applies here,
			// never mid-round, so assistant tool-call/result pairing survives.
			if engine.pendingCompact {
				engine.pendingCompact = false
				if err := engine.runCompaction(ctx, yield, true, rt.client); err != nil {
					if errors.Is(err, errEventConsumerStopped) {
						return
					}
					yield(nil, err)
					return
				}
			}

			// Same boundary: queued user input joins the context here, so the
			// model answers it mid-turn instead of the user waiting out the whole
			// agentic turn. UserPromoted clears the transcript's queued hint.
			if opts.Inject != nil {
				for _, item := range opts.Inject() {
					content := engine.composeUserPrompt(recall, item.Skills, item.Text, item.Text)
					if err := sess.Append(
						llm.Message{Role: llm.RoleUser, Content: content, Media: item.Media},
					); err != nil {
						yield(nil, err)
						return
					}
					if item.UserID != "" {
						if !yield(session.UserPromoted{ID: item.UserID}, nil) {
							return
						}
					}
				}
			}
		}
	}
}

// composeUserPrompt assembles a user message the way both entry points into a
// turn do — the opening prompt and a queued item injected mid-turn: skill
// instructions first, the memory reminder in front of the request it applies
// to, and the user's text last. query is what recall is keyed on: the user's
// own words, which on a delegated opening turn differ from text after the
// delegation rewrite has replaced them.
func (engine *Engine) composeUserPrompt(recall *memory.Recall, skillNames []string, query, text string) string {
	skillNames = engine.mergePlanSkills(skillNames)
	content := text
	if instr := pendingSkillsInstruction(engine.skillPath, skillNames); instr != "" {
		if content == "" {
			content = instr
		} else {
			content = instr + "\n\n" + content
		}
	}
	return prependReminder(recall.Reminder(engine.memoryQuery(query)), content)
}

// pendingSkillsInstruction tells the model to read SKILL.md files for the
// selected skills (panda-style: reuse the read tool, no dedicated skill tool).
func pendingSkillsInstruction(skillPath string, names []string) string {
	if len(names) == 0 {
		return ""
	}
	list, err := skills.LoadSkills(skillPath)
	targets := make([]string, 0, len(names))
	if err == nil {
		for _, name := range names {
			if s := skills.Find(list, name); s != nil && s.SkillFilePath != "" {
				targets = append(targets, s.SkillFilePath)
				continue
			}
			targets = append(targets, name)
		}
	} else {
		targets = append(targets, names...)
	}
	return fmt.Sprintf(
		"You MUST read these skill files first with the read tool and follow them: %s. Do this immediately before responding.",
		strings.Join(targets, ", "),
	)
}
