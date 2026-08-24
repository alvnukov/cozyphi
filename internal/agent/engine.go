package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"
	"strings"
	"time"

	"github.com/pulseaiclub/phi/internal/agent/prompt"
	"github.com/pulseaiclub/phi/internal/hooks"
	"github.com/pulseaiclub/phi/internal/job"
	"github.com/pulseaiclub/phi/internal/llm"
	llmclient "github.com/pulseaiclub/phi/internal/llm/client"
	"github.com/pulseaiclub/phi/internal/llm/skills"
	"github.com/pulseaiclub/phi/internal/mcp"
	"github.com/pulseaiclub/phi/internal/permission"
	"github.com/pulseaiclub/phi/internal/session"
	"github.com/pulseaiclub/phi/internal/session/compaction"
	"github.com/pulseaiclub/phi/internal/tools"
)

// ErrMaxRounds is returned (wrapped) by Loop when the model exceeds the
// configured tool-round budget and continuation is declined or unavailable.
// Callers can distinguish it from other runtime errors with errors.Is,
// e.g. for a dedicated exit code.
var ErrMaxRounds = errors.New("exceeded maximum tool rounds")

var errEventConsumerStopped = errors.New("event consumer stopped")

const defaultMaxToolRounds = 64

// ContinueFunc asks whether to grant another maxRounds budget after the
// current budget is exhausted. Nil means hard-fail with ErrMaxRounds
// (headless / sub-agent default). True continues the loop with a fresh budget.
type ContinueFunc func(ctx context.Context, maxRounds int) (bool, error)

// Engine drives the agent loop: stream → tools → stream…
// and yields session.Event for the TUI reducer. Context compaction is owned
// here so Session stays a thin message store.
type Engine struct {
	client        *llmclient.Client
	executor      *Executor
	maxRounds     int
	mode          Mode
	skillPath     string
	contextWindow int
	modelCfg      llm.ModelConfig
	gate          permission.Gate
	ask           permission.AskFunc
	continueAsk   ContinueFunc
	jobs          *job.Manager
	hooks         *hooks.Manager
	mcp           *mcp.Pool
	onPlanUpdated func(session.Plan)
	planEnabled   bool
	// baseTools is the tool set from EngineOpts.Tools; nil means DefaultTools.
	// rebindTools rebuilds from it so setters never widen a readonly engine.
	baseTools []tools.Tool

	session *Session
	// pendingCompact records a model-requested compaction (context tool).
	// Loop applies it at the next tool-round boundary, then clears it.
	pendingCompact bool
}

// EngineOpts configures NewEngine.
type EngineOpts struct {
	Model       llm.ModelConfig
	SessionOpts SessionOpts
	Gate        permission.Gate    // nil = allow all
	Ask         permission.AskFunc // nil = deny on Ask
	ContinueAsk ContinueFunc       // nil = ErrMaxRounds on budget exhaust
	Tools       []tools.Tool       // nil = tools.DefaultTools(); sub-agents use ChildTools()
	MaxRounds   int                // 0 = package default
	Jobs        *job.Manager       // if set, register agent_* tools on this engine
	Hooks       *hooks.Manager     // nil = no hooks; child engines inherit parent Manager
	MCP         *mcp.Pool          // if set, register mcp_list/inspect/call meta-tools
	PlanUpdated func(session.Plan) // called after a durable primary-session plan update
}

// NewEngine wires an LLM client, tool executor, and session store.
func NewEngine(opts EngineOpts) (*Engine, error) {
	sess, err := NewSession(opts.SessionOpts)
	if err != nil {
		return nil, err
	}
	cfg := opts.Model
	engine := &Engine{
		maxRounds:     defaultMaxToolRounds,
		skillPath:     cfg.SkillPath,
		contextWindow: cfg.ContextWindow,
		modelCfg:      cfg,
		session:       sess,
		gate:          opts.Gate,
		ask:           opts.Ask,
		continueAsk:   opts.ContinueAsk,
		jobs:          opts.Jobs,
		hooks:         opts.Hooks,
		mcp:           opts.MCP,
		onPlanUpdated: opts.PlanUpdated,
		planEnabled:   opts.SessionOpts.ParentID == "" && opts.Tools == nil,
		baseTools:     opts.Tools,
	}
	if opts.MaxRounds > 0 {
		engine.maxRounds = opts.MaxRounds
	}
	toolList := engine.buildToolList()
	engine.client = llmclient.NewClient(cfg, tools.Definitions(toolList), engine.systemPrompt())
	engine.bindExecutor(tools.NewRegistry(toolList))
	return engine, nil
}

// buildToolList assembles the current tool set: the configured base
// (DefaultTools when EngineOpts.Tools was nil) plus MCP and agent_* tools
// for whichever managers are attached.
func (engine *Engine) buildToolList() []tools.Tool {
	base := engine.baseTools
	if base == nil {
		base = tools.DefaultTools()
	}
	// Plan mode narrows only the built-in set; explicitly configured tools
	// stay (their reach is bounded by the permission policy, not the mode).
	if engine.mode == ModePlan && engine.baseTools == nil {
		base = tools.ReadonlyTools()
	}
	out := append([]tools.Tool(nil), base...)
	if engine.planEnabled {
		out = append(out, tools.PlanTool(tools.PlanDeps{Update: engine.updatePlan}))
	}
	// The context tool rides on every engine (main and sub-agents): it only
	// reports usage numbers and compacts the engine's own context view.
	ctxTool := tools.ContextTools(tools.ContextDeps{
		Stats:          engine.contextStats,
		RequestCompact: engine.requestCompact,
	})
	ctxMerged := make([]tools.Tool, 0, len(out)+1)
	ctxMerged = append(ctxMerged, out...)
	ctxMerged = append(ctxMerged, ctxTool)
	out = ctxMerged
	if engine.mcp != nil {
		mcpTools := tools.MCPTools(engine.mcp)
		if len(mcpTools) > 0 {
			merged := make([]tools.Tool, 0, len(out)+len(mcpTools))
			merged = append(merged, out...)
			merged = append(merged, mcpTools...)
			out = merged
		}
	}
	if engine.jobs == nil {
		return out
	}
	agentTools := tools.AgentTools(tools.AgentDeps{
		Manager:  engine.jobs,
		ParentID: engine.SessionID,
		WorkDir:  engine.SessionCwd,
	})
	merged := make([]tools.Tool, 0, len(out)+len(agentTools))
	merged = append(merged, out...)
	merged = append(merged, agentTools...)
	return merged
}

func (engine *Engine) updatePlan(ctx context.Context, items []session.PlanItem) (session.Plan, error) {
	if engine == nil || engine.session == nil {
		return session.Plan{}, errors.New("agent: session unavailable")
	}
	plan, err := engine.session.UpdatePlan(ctx, items)
	if err != nil {
		return session.Plan{}, fmt.Errorf("agent: update plan: %w", err)
	}
	if engine.onPlanUpdated != nil {
		engine.onPlanUpdated(plan.Clone())
	}
	return plan, nil
}

// Plan returns the latest durable model-managed plan snapshot.
func (engine *Engine) Plan() session.Plan {
	if engine == nil || engine.session == nil {
		return session.Plan{}
	}
	return engine.session.Plan()
}

// SetModel replaces the LLM client and model-related settings without
// discarding the session tree. Agent tools remain registered when Jobs is set.
func (engine *Engine) SetModel(cfg llm.ModelConfig) error {
	engine.modelCfg = cfg
	engine.skillPath = cfg.SkillPath
	engine.contextWindow = cfg.ContextWindow
	engine.rebindTools()
	return nil
}

// SetJobs attaches or detaches the job manager and rebuilds the tool list.
// Pass nil to unregister agent_* tools (sub-agents disabled).
func (engine *Engine) SetJobs(jobs *job.Manager) {
	if engine == nil {
		return
	}
	engine.jobs = jobs
	engine.rebindTools()
}

func (engine *Engine) rebindTools() {
	toolList := engine.buildToolList()
	engine.client = llmclient.NewClient(
		engine.modelCfg,
		tools.Definitions(toolList),
		engine.systemPrompt(),
	)
	engine.bindExecutor(tools.NewRegistry(toolList))
}

func (engine *Engine) systemPrompt() string {
	var mcpServers []string
	if engine.mcp != nil {
		mcpServers = engine.mcp.ServerNames()
	}
	return prompt.Build(engine.skillPath, engine.jobs != nil, mcpServers, engine.mode == ModePlan)
}

func (engine *Engine) bindExecutor(registry tools.Registry) {
	engine.executor = NewExecutor(registry, engine.gate, engine.ask, engine.hooks)
	engine.executor.SetMeta(engine.SessionID(), engine.SessionCwd())
}

// HasTool reports whether a tool is currently registered on the executor.
func (engine *Engine) HasTool(name string) bool {
	if engine == nil || engine.executor == nil {
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
	engine.maxRounds = n
	return nil
}

// Mode returns the current turn posture (build by default).
func (engine *Engine) Mode() Mode {
	if engine == nil {
		return ModeBuild
	}
	return normalizeMode(engine.mode)
}

// SetMode switches the turn posture. Unknown modes fall back to build.
// Switching rebinds the client: plan adds the plan appendix to the system
// prompt and narrows the built-in tool set to the read-only tools.
func (engine *Engine) SetMode(m Mode) {
	if engine == nil {
		return
	}
	engine.mode = normalizeMode(m)
	engine.rebindTools()
}

// SetPermission updates the gate and ask handler used by the tool executor.
func (engine *Engine) SetPermission(gate permission.Gate, ask permission.AskFunc) {
	if engine == nil {
		return
	}
	engine.gate = gate
	engine.ask = ask
	if engine.executor != nil {
		engine.executor.gate = gate
		engine.executor.ask = ask
		engine.executor.syncHookFilter()
	}
}

// SetContinueAsk sets the handler invoked when the tool-round budget is exhausted.
// Pass nil to hard-fail with ErrMaxRounds (default for headless runs).
func (engine *Engine) SetContinueAsk(fn ContinueFunc) {
	if engine == nil {
		return
	}
	engine.continueAsk = fn
}

// SetHooks replaces the hooks manager. Pass nil to disable hooks.
// Does not drop Gate/Ask; rebindTools also preserves hooks.
func (engine *Engine) SetHooks(mgr *hooks.Manager) {
	if engine == nil {
		return
	}
	engine.hooks = mgr
	if engine.executor != nil {
		engine.executor.hooks = mgr
	}
}

// SessionID returns the durable session id.
func (engine *Engine) SessionID() string {
	if engine == nil || engine.session == nil {
		return ""
	}
	return engine.session.ID()
}

// SessionFile returns the JSONL path (empty in memory mode).
func (engine *Engine) SessionFile() string {
	if engine == nil || engine.session == nil {
		return ""
	}
	return engine.session.File()
}

// SessionCwd returns the cwd recorded on the session header.
func (engine *Engine) SessionCwd() string {
	if engine == nil || engine.session == nil {
		return ""
	}
	return engine.session.Cwd()
}

// ReplaceSession swaps the session store (used by /resume).
func (engine *Engine) ReplaceSession(opts SessionOpts) error {
	sess, err := NewSession(opts)
	if err != nil {
		return err
	}
	engine.session = sess
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
	return engine.session
}

// LoopOpts configures a single agent loop turn.
type LoopOpts struct {
	// PendingSkills are skill names the user selected in the composer.
	// When set, the model is instructed to read those SKILL.md files first.
	PendingSkills []string
}

// Loop appends the user prompt and runs inference + tool rounds until the
// model stops calling tools or the context is cancelled.
//
// Compaction: persist the turn first, then check usage after
// the agent turn ends (final assistant with no tool_calls) — never mid-tool-loop.
func (engine *Engine) Loop(ctx context.Context, prompt string, opts LoopOpts) iter.Seq2[session.Event, error] {
	return func(yield func(session.Event, error) bool) {
		// A compaction request from a cancelled turn never leaks into the next.
		engine.pendingCompact = false
		if _, err := engine.session.RepairPendingToolCalls(); err != nil {
			yield(nil, fmt.Errorf("agent: repair interrupted tool round: %w", err))
			return
		}
		content := prompt
		if engine.jobs != nil {
			if role, task, ok := splitDelegationPrefix(prompt); ok {
				content = delegationInstruction(role) + "\n\n" + task
			}
		}
		if instr := pendingSkillsInstruction(engine.skillPath, opts.PendingSkills); instr != "" {
			if content == "" {
				content = instr
			} else {
				content = instr + "\n\n" + content
			}
		}
		if err := engine.session.Append(llm.Message{
			Role:    llm.RoleUser,
			Content: content,
		}); err != nil {
			yield(nil, err)
			return
		}

		toolRounds := 0
		for {
			if ctx.Err() != nil {
				return
			}

			msgs := engine.session.BuildContext()

			msg, completeEvent, ok := engine.streamTurn(ctx, yield, msgs)
			if !ok {
				return
			}

			// Defer publishing and persisting the terminal assistant update until
			// the tool budget is checked. An over-budget tool request must not
			// leave an unexecuted tool call in the session or UI.
			if len(msg.ToolCalls) > 0 && toolRounds >= engine.maxRounds {
				if engine.continueAsk == nil {
					yield(nil, fmt.Errorf("agent: %w (%d)", ErrMaxRounds, engine.maxRounds))
					return
				}
				ok, err := engine.continueAsk(ctx, engine.maxRounds)
				if err != nil {
					yield(nil, err)
					return
				}
				if !ok {
					yield(nil, fmt.Errorf("agent: %w (%d)", ErrMaxRounds, engine.maxRounds))
					return
				}
				// Granted: reset the budget; the current and following tool
				// rounds run under the fresh budget.
				toolRounds = 0
			}
			if !yield(completeEvent, nil) {
				return
			}

			if err := engine.session.Append(msg); err != nil {
				yield(nil, err)
				return
			}

			if len(msg.ToolCalls) == 0 {
				// Turn finished — compact using this assistant's usage (pi agent_end).
				if err := engine.maybeCompact(ctx, yield, msg.Usage.TotalTokens); err != nil {
					if errors.Is(err, errEventConsumerStopped) {
						return
					}
					yield(nil, err)
				}
				return
			}

			toolRounds++
			toolMsgs, active := engine.executor.run(ctx, msg.ToolCalls, func(td session.ToolData) bool {
				return yield(td, nil)
			})
			if err := engine.session.Append(toolMsgs...); err != nil {
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
				if err := engine.runCompaction(ctx, yield, true); err != nil {
					if errors.Is(err, errEventConsumerStopped) {
						return
					}
					yield(nil, err)
					return
				}
			}
		}
	}
}

// RunUntil is the reserved interface for task 007 (eval / until-goal): it
// will run Loop repeatedly against a verifier until a goal predicate passes,
// the budget is exhausted, or ctx is cancelled. Intentionally unimplemented
// here — the verifier contract does not exist until the eval suite lands.
//
// Suggested shape (final signature TBD in 007):
//
//	func (engine *Engine) RunUntil(
//		ctx context.Context,
//		goal func(snapshot) bool,
//		maxAttempts int,
//	) (bool, error)
func (engine *Engine) maybeCompact(
	ctx context.Context,
	yield func(session.Event, error) bool,
	usage int,
) error {
	settings := compaction.DefaultSettings()
	if engine.client == nil || !compaction.ShouldCompact(usage, engine.contextWindow, settings) {
		return nil
	}
	return engine.runCompaction(ctx, yield, false)
}

// runCompaction prepares and appends one compaction entry, emitting UI events.
// Called from turn end (auto threshold) and from the tool-round boundary
// (model request via the context tool). The PrepareCompact here is deliberate
// re-validation, not waste: entries appended since requestCompact checked
// change what can be compacted, and a silent no-op (already compacted) is
// correct at the boundary, where an error would fail the turn.
func (engine *Engine) runCompaction(
	ctx context.Context,
	yield func(session.Event, error) bool,
	manual bool,
) error {
	settings := compaction.DefaultSettings()
	prepare := compaction.PrepareCompact
	if manual {
		prepare = compaction.PrepareCompactManual
	}
	prep, err := prepare(engine.session.PathEntries(), settings)
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return nil
	}

	id := fmt.Sprintf("compaction-%d", time.Now().UnixNano())
	if !yield(session.CompactionStarted{}, nil) {
		return errEventConsumerStopped
	}

	result, err := compaction.Compact(ctx, *prep, engine.client)
	if err != nil {
		if !yield(session.CompactionComplete{ID: id, Failed: true}, nil) {
			return errEventConsumerStopped
		}
		return err
	}
	beforeTokens := result.TokensBefore
	if beforeTokens == 0 {
		beforeTokens = estimateContextTokens(engine.session.BuildContext())
	}
	compactedContext := make([]llm.Message, 0, len(prep.RecentMessages)+1)
	compactedContext = append(compactedContext, llm.Message{Role: llm.RoleUser, Content: result.Summary})
	compactedContext = append(compactedContext, prep.RecentMessages...)
	record := session.Compaction{
		Summary:            result.Summary,
		FirstKeptEntryID:   result.FirstKeptEntryID,
		TokensBefore:       beforeTokens,
		TokensAfter:        estimateContextTokens(compactedContext),
		MessagesSummarized: len(prep.MessagesToSummarize) + len(prep.TurnPrefixMessages),
		MessagesKept:       len(prep.RecentMessages),
		Details:            result.Details,
	}
	if err := engine.session.AppendCompaction(record); err != nil {
		if !yield(session.CompactionComplete{ID: id, Failed: true}, nil) {
			return errEventConsumerStopped
		}
		return err
	}
	if !yield(session.CompactionComplete{ID: id, Compaction: record}, nil) {
		return errEventConsumerStopped
	}
	return nil
}

// CompactNow runs a user-initiated compaction (/compact): summarize the
// history now, regardless of the auto-compaction threshold. yield receives
// the UI events (CompactionStarted/Complete); returning false cancels.
// An error means there was nothing to compact or the summary request
// failed — the caller surfaces it to the user.
func (engine *Engine) CompactNow(ctx context.Context, yield func(session.Event) bool) error {
	// Same guards as requestCompact: an immediate answer beats a background
	// round-trip for the "nothing to compact yet" case.
	prep, err := compaction.PrepareCompactManual(engine.session.PathEntries(), compaction.DefaultSettings())
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return errors.New("nothing to compact: no older turn is available to summarize yet")
	}
	err = engine.runCompaction(ctx, func(ev session.Event, _ error) bool {
		return yield(ev)
	}, true)
	if errors.Is(err, errEventConsumerStopped) {
		return context.Canceled
	}
	return err
}

// contextStats snapshots quantitative context usage for the context tool.
// Tokens come from the newest provider-reported usage after the latest
// compaction. Until then the durable post-compaction estimate is authoritative;
// provider usage on messages retained from before compaction describes the old
// context and must not leak back into the counter. Numbers only — conversation
// content never leaves the engine through here.
func (engine *Engine) contextStats() tools.ContextStats {
	msgs := engine.session.BuildContext()
	entries := engine.session.PathEntries()
	usedBytes := 0
	if raw, err := json.Marshal(msgs); err == nil {
		usedBytes = len(raw)
	}
	stats := tools.ContextStats{
		UsedBytes:     usedBytes,
		Messages:      len(msgs),
		ContextWindow: engine.contextWindow,
		TokenSource:   "estimate",
		ContextTokens: usedBytes / 4,
	}
	usage, compactedTokens, unchangedSinceCompaction := currentContextUsage(entries, msgs)
	if usage.PromptTokens > 0 || usage.TotalTokens > 0 {
		stats.TokenSource = "provider"
		stats.ContextTokens = max(usage.PromptTokens, 0)
		if stats.ContextTokens == 0 {
			stats.ContextTokens = usage.TotalTokens
		}
	} else if unchangedSinceCompaction && compactedTokens > 0 {
		stats.ContextTokens = compactedTokens
	}
	if window := engine.contextWindow; window > 0 {
		settings := compaction.DefaultSettings()
		stats.ThresholdTokens = settings.Threshold(window)
		stats.CompactionRecommended = compaction.ShouldCompact(stats.ContextTokens, window, settings)
	}
	return stats
}

// currentContextUsage ignores provider counters attached to messages retained
// from before the latest compaction. PathEntries projects the latest compaction
// first, followed by retained ancestors and then descendants. Timestamps mark
// the new usage epoch even when a branch node sits between the compaction and
// the first projected message; the direct parent check also covers synthetic
// and legacy entries without timestamps.
func currentContextUsage(entries []session.MessageEntry, msgs []llm.Message) (llm.Usage, int, bool) {
	var latestCompaction session.CompactionEntry
	hasCompaction := false
	compactedTokens := 0
	afterCompaction := false
	postCompactionMessages := 0
	usage := llm.Usage{}
	for _, entry := range entries {
		switch typed := entry.(type) {
		case session.CompactionEntry:
			latestCompaction = typed
			hasCompaction = true
			compactedTokens = typed.Compaction.TokensAfter
			afterCompaction = false
			postCompactionMessages = 0
			usage = llm.Usage{}
		case session.SessionMessageEntry:
			if !hasCompaction {
				continue
			}
			if session.MessageFollowsCompaction(latestCompaction, typed) {
				afterCompaction = true
			}
			if !afterCompaction {
				continue
			}
			postCompactionMessages++
			if typed.Message.Role == llm.RoleAssistant &&
				(typed.Message.Usage.PromptTokens > 0 || typed.Message.Usage.TotalTokens > 0) {
				usage = typed.Message.Usage
			}
		}
	}
	if !hasCompaction {
		return lastReportedUsage(msgs), 0, false
	}
	return usage, compactedTokens, postCompactionMessages == 0
}

func estimateContextTokens(msgs []llm.Message) int {
	raw, err := json.Marshal(msgs)
	if err != nil {
		return 0
	}
	return len(raw) / 4
}

// lastReportedUsage returns the newest assistant usage in the model view.
func lastReportedUsage(msgs []llm.Message) llm.Usage {
	for i := range slices.Backward(msgs) {
		if msgs[i].Role != llm.RoleAssistant {
			continue
		}
		if usage := msgs[i].Usage; usage.PromptTokens > 0 || usage.TotalTokens > 0 {
			return usage
		}
	}
	return llm.Usage{}
}

// requestCompact validates and records a model-requested compaction. The
// engine applies it at the next tool-round boundary (see Loop); the
// transcript itself stays append-only. The PrepareCompact here is an early
// answer to the model ("nothing to compact yet"), not a cached decision:
// runCompaction re-prepares at the boundary on fresh state.
func (engine *Engine) requestCompact() error {
	if engine.pendingCompact {
		return errors.New("compaction already scheduled for this round boundary")
	}
	prep, err := compaction.PrepareCompactManual(engine.session.PathEntries(), compaction.DefaultSettings())
	if err != nil {
		return err
	}
	if !prep.HasWork() {
		return errors.New("nothing to compact: no uncompacted history to summarize yet")
	}
	engine.pendingCompact = true
	return nil
}

func (engine *Engine) streamTurn(
	ctx context.Context,
	yield func(session.Event, error) bool,
	messages []llm.Message,
) (llm.Message, session.Event, bool) {
	id := fmt.Sprintf("assistant-%d", time.Now().UnixNano())
	started := time.Now()
	model := engine.modelCfg.Name
	var thinking, text string
	var final llm.Message
	var finish string
	var thinkStart, thinkEnd time.Time
	gotDone := false
	// thinkingSpan is the round's reasoning wall time: first reasoning delta
	// to first text delta (or to stream end when reasoning ran to the wire).
	thinkingSpan := func() time.Duration {
		if thinkStart.IsZero() || thinkEnd.IsZero() || thinkEnd.Before(thinkStart) {
			return 0
		}
		return thinkEnd.Sub(thinkStart)
	}

	for event, err := range engine.client.Stream(ctx, messages) {
		if err != nil {
			if thinking != "" || text != "" {
				if !yield(
					emitMessage(
						id,
						session.StateError,
						session.StopNone,
						thinking,
						text,
						nil,
						llm.Usage{},
						model,
						started,
						thinkingSpan(),
					),
					nil,
				) {
					return llm.Message{}, nil, false
				}
			}
			yield(nil, err)
			return llm.Message{}, nil, false
		}

		switch event.Type {
		case llm.StreamEventTypeError:
			errText := event.Err
			if errText == "" {
				errText = "stream error"
			}
			yield(nil, fmt.Errorf("%s", errText))
			return llm.Message{}, nil, false

		case llm.StreamEventTypeDelta:
			if event.Delta.ReasoningContent != "" {
				if thinkStart.IsZero() {
					thinkStart = time.Now()
				}
				thinking += event.Delta.ReasoningContent
			}
			if event.Delta.Content != "" {
				if !thinkStart.IsZero() && thinkEnd.IsZero() {
					thinkEnd = time.Now()
				}
				text += event.Delta.Content
			}
			if !yield(
				emitMessage(
					id,
					session.StateStreaming,
					session.StopNone,
					thinking,
					text,
					nil,
					llm.Usage{},
					model,
					started,
					thinkingSpan(),
				),
				nil,
			) {
				return llm.Message{}, nil, false
			}

		case llm.StreamEventTypeDone:
			if len(event.Partial.Choices) == 0 {
				yield(nil, errors.New("agent: stream finished with no assistant choice"))
				return llm.Message{}, nil, false
			}
			final = event.Partial.Choices[0].Message
			final.Usage = event.Partial.Usage
			finish = event.Partial.Choices[0].FinishReason
			gotDone = true
			if !thinkStart.IsZero() && thinkEnd.IsZero() {
				thinkEnd = time.Now()
			}
			// Prefer fully accumulated message for the complete event.
			if final.ReasoningContent != "" {
				thinking = final.ReasoningContent
			}
			if final.Content != "" {
				text = final.Content
			}
		}
	}

	if !gotDone {
		if ctx.Err() != nil {
			_ = yield(
				emitMessage(
					id,
					session.StateCancelled,
					session.StopNone,
					thinking,
					text,
					nil,
					llm.Usage{},
					model,
					started,
					thinkingSpan(),
				),
				nil,
			)
			return llm.Message{}, nil, false
		}
		yield(nil, errors.New("agent: stream closed without assistant output"))
		return llm.Message{}, nil, false
	}

	blocks := engine.toolCallsToBlocks(final.ToolCalls)
	reason := stopReasonFromFinish(finish, len(blocks) > 0)
	complete := emitMessage(
		id,
		session.StateComplete,
		reason,
		thinking,
		text,
		blocks,
		final.Usage,
		model,
		started,
		thinkingSpan(),
	)
	return final, complete, true
}

// stopReasonFromFinish maps the provider's raw finish signal onto the session
// stop reason: a budget cutoff always wins (the transcript must never render
// a truncated round as a clean end), then tool use, then a normal end turn.
// An empty signal falls back to inferring from the accumulated message.
func stopReasonFromFinish(finish string, hasTools bool) session.StopReason {
	switch finish {
	case "max_tokens", "length":
		return session.StopMaxTokens
	case "tool_use", "tool_calls":
		return session.StopToolUse
	case "end_turn", "stop", "stop_sequence":
		return session.StopEndTurn
	}
	if hasTools {
		return session.StopToolUse
	}
	return session.StopEndTurn
}

func (engine *Engine) toolCallsToBlocks(calls []llm.ToolCall) []session.ContentBlock {
	if len(calls) == 0 {
		return nil
	}
	out := make([]session.ContentBlock, 0, len(calls))
	for _, c := range calls {
		input := c.Function.Arguments
		if tool, ok := engine.executor.registry[c.Function.Name]; ok && tool.DetailFromArgs != nil {
			if d := tool.DetailFromArgs(json.RawMessage(c.Function.Arguments)); d != "" {
				input = d
			}
		}
		out = append(out, session.ContentBlock{
			Type:     session.BlockToolUse,
			ID:       c.ID,
			Name:     c.Function.Name,
			Input:    input,
			Complete: true,
		})
	}
	return out
}

func buildContent(thinking, text string, tools []session.ContentBlock) []session.ContentBlock {
	var out []session.ContentBlock
	if thinking != "" {
		out = append(out, session.ContentBlock{Type: session.BlockThinking, Text: thinking})
	}
	if text != "" {
		out = append(out, session.ContentBlock{Type: session.BlockText, Text: text})
	}
	out = append(out, tools...)
	return out
}

func emitMessage(
	id string,
	state session.State,
	reason session.StopReason,
	thinking,
	text string,
	tools []session.ContentBlock,
	usage llm.Usage,
	model string,
	started time.Time,
	thinkDur time.Duration,
) session.Event {
	msg := session.Message{
		ID:         id,
		State:      state,
		StopReason: reason,
		Content:    buildContent(thinking, text, tools),
		Text:       text,
		Usage: session.TokenUsage{
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			CachedTokens:     usage.CachedTokens(),
			TotalTokens:      usage.TotalTokens,
		},
		Model:            model,
		Started:          started,
		ThinkingDuration: thinkDur,
	}
	if state != session.StateStreaming {
		msg.Ended = time.Now()
	}
	return session.AssistantMessageUpdate{Message: msg}
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
