---
id: interactive-child-sessions-design
title: Design full interactive child sessions and parent wake events
status: done
priority: high
model_level: very_high
task_type: design
parent_id: multisession-mode
tags:
    - multisession
    - agents
    - design
branch: design/interactive-child-sessions-design
worktree_path: .worktrees/interactive-child-sessions-design
acceptance_criteria:
    - Текущие agent jobs и интерактивные сессии исследованы с ссылками на код.
    - Спроектированы lifecycle, isolation и доставка событий для интерактивных детей.
    - План реализации учитывает существующие multisession-задачи, отдельные effort и проверки.
verification_plan:
    - Проверить исходники и тесты agent jobs, Controller/Editor и фоновых событий на зафиксированной ревизии.
    - Сопоставить с multisession runtime/registry/UI задачами; указать противоречия и неопределённости.
    - Представить план реализации с effort, зависимостями и проверками гонок и permissions.
created_at: "2026-09-05T14:41:24.235349Z"
updated_at: "2026-09-05T15:11:41.825804Z"
---

## Body

Read-only исследование и архитектурный план по согласованному запросу пользователя. Ребёнок — полноценная сессия с очередью ввода, сменой модели, отдельным прерыванием хода; выбор ● main / ○ имя ребёнка. Переключение с работающего ребёнка не останавливает его; уход из прерванного без продолжения останавливает задание с сохранением истории. Итог/ошибка/остановка будит родителя событием вместо обязательного agent_wait. Реализация и изменения multisession-тикетов только после отдельного согласования плана.

**Started (2026-09-05).** Исследование без изменений кода на main@37c3a4a4d37603e98e65596c8e3f47bf6a23f6e9. В main уже были посторонние изменения go.sum и две untracked task notes; не трогаем. Реализация не начата.

**Note (2026-09-05).** Read-only TRACE/working complete at main@37c3a4a4d37603e98e65596c8e3f47bf6a23f6e9. SOURCE: EngineRunner.Run (internal/agent/engine_runner.go:51) constructs a local engine and executes one Loop; Manager.run (internal/job/manager.go:247) persists terminal outcome but sends no lifecycle push. Progress subscribers are lossy and unsuitable for mandatory parent results. Child session ID is not structured in Meta. Controller StartPrompt/runLoop inject accepted input at tool-round boundaries; Cancel preserves accepted queue; finishRun clears streamStopped and may launch the next queued prompt. SetModel/SetModelEffort require idle. Editor callbacks, focus, history navigation, overlays and transcript need per-session ownership; swapping ctrl or using /resume is not live activation. Existing multisession tasks are todo; reuse their runtime/registry direction, but ● must mean selection (old panel uses unread). plangate.Runtime is shared immutable policy, not session plan/approval state. Watch scheduler is reusable, but its drop-oldest queue is not valid for child terminal outcomes. doc/agent-backends.md is proposed only, not a prerequisite. RUNTIME: targeted go test -mod=readonly checks passed: controller watcher cases 4.430s; job spawn/wait/cancel/close/progress 3.418s; agent runner/cancel/injection/workdir confinement 0.478s; controller bus/queue/cancel/model 0.989s; editor interrupt/queue/recall 2.114s. Offline deps for latter checks; no full/race suite or live TUI. Explorer summaries retained at ~/.cozyphi/jobs/job_20260905T144100_01a420f03f730d6f/result.md and job_20260905T144100_310c23bb9fcfe9da/result.md. No code changed. Design remains next; added low-effort ledger-only closeout step, which invalidated plan approval and requires confirmation before further gated work.

**Note (2026-09-05).** Design completed (2026-09-05). This is a proposed implementation contract, not implemented behavior. The implementation plan still needs separate approval. The user selected high effort for design; xhigh was unsupported by the configured model. Main advanced from inspected 37c3a4a4d37603e98e65596c8e3f47bf6a23f6e9 to c855a20c2f3f8a8c050930d50cd8fe2bbbca9958 during the session; git diff confirmed no changes in the inspected agent/job/controller/editor/composer/overlays/history paths or referenced design/domain documents. Only this ledger file is owned by this task.

**Outcome and scope**
A native child is a retained interactive Session, not a one-shot job with a log viewer. The user can select it, type ordinary queued messages, interrupt a turn, continue, change model/effort and answer its interactions. Working children continue while unselected. Leaving an interrupted child without continuing stops its assignment and preserves its conversation. Completion/error/stop delivers a bounded result to its parent without mandatory agent_wait. Selection never follows an event automatically. No intermediate read-only-viewer release.

Reuse the existing multisession backlog and native engine, not a second session registry or scheduler. First delivery covers a parent and its children with validated child workdirs, including task worktrees. General multi-project navigation, restored sets of open sessions, title-generation tools, external Claude/Codex backends and split-screen views are not prerequisites. Existing role restrictions, workspace confinement, no nested agent tools and absence of child memory/task/watch tools remain; ordinary interaction is not unrestricted capability parity.

**Ownership and interface design**
1. Process runtime owns shared provider/catalog resources, job manager, history persistence and shutdown coordination. Shared plangate.Runtime contains immutable default policy only; actual plans, approval state, evidence and edit capabilities remain session-local.
2. Workspace resources have explicit canonical cwd/config ownership. Do not use git root alone as a cache key where two worktrees have different cwd, hooks or MCP configuration. Sharing requires an explicit lifetime contract, not duplicate Close calls.
3. A retained per-session Controller owns exactly one engine and serializes its turns, input queue, cancellation, interactions and model changes. Reuse this path for main and native interactive children. Do not implement another prompt queue inside job.Runner. Session-specific gate/profile and callbacks are constructor inputs; constructors remain fully initialized and use explicit parameters, not dependency bags.
4. Extend the existing job module for assignment lifecycle and correlation. Keep its concurrency/workdir validation and model-facing spawn/wait/list/cancel interface. An interactive runner adapter connects a job to a retained Controller; the existing headless runner is the second real adapter at this seam. Assembly in cmd/runtime avoids agent/job importing TUI and avoids reverse pointers to Editor. Ending an assignment does not dispose of its retained session.
5. The UI registry owns per-session Views, not job execution. Retain composer, attachments, pending skills, transcript/scroll/selection, overlays, submitter and origin-bound callbacks. A per-session Bus plus one process redraw relay lets every View consume updates while only the selected one draws. Session creation must be recoverable through a snapshot/cursor so a fast child cannot finish before the UI subscribes and disappear.
6. Share the history corpus/writer, not its navigation cursor and recalled draft. Focus, voice routing, editing mode and delayed picker results must follow session identity. Background overlays store requests without hiding the active pickers or stealing App focus. Panel navigation must remain available while an overlay is open.

**Identity and lifecycle**
Distinguish SessionID (retained conversation), JobID (delegated assignment), and TurnID/generation (one running loop). Persist ParentSessionID, child SessionID, originating tool ID and canonical workdir in structured metadata. Scope asynchronous control and replies to the expected session/turn generation; do not use the selected view or footer activity as identity.

Turn states include idle, running, interrupting and interrupted; waiting permission/question is attached to the active turn and may contain multiple requests. Assignment terminal outcomes remain compatible with completed/failed/cancelled/timed_out. User-left-interrupted is a structured stop reason, not a successful result. The UI may label it stopped without changing old wait consumers' cancelled vocabulary.

- Interrupt requests cancellation of the current turn, not of the job. Do not declare it fully interrupted or start another Loop until the old loop exits. Preserve normal accepted-message queue behavior.
- Selection is independent. Leaving a running child does nothing to execution. Leaving during interrupting/interrupted commits a stop intent when no subsequent continuation has started. This intent prevents finishRun, queued work, background wakes and late stream events from reviving the assignment.
- Serialize submit, interrupt, leave and turn-end decisions. If ordinary queued work has already started a new turn before Leave is processed, that is a running child and continues. If Leave wins while interruption is pending, retain queued messages visibly but do not execute them until explicit resume. Test both orders rather than rely on wall-clock timing.
- Typing a draft, changing a model or merely selecting a child does not count as continuation. Opening settings or another modal within that child is not leaving the session.
- While the child remains selected, interrupt then submit/continue retains its SessionID and current nonterminal JobID. Natural quiescence after all accepted input is handled completes the assignment once; an intermediate assistant/tool-round completion is not a job result.
- A terminal JobID/result is immutable. Proposed default for a later user follow-up to a completed/stopped child: retain SessionID and create a linked follow-up JobID, preserving the prior result and wait semantics. Opening its history alone runs nothing.
- Run completion, interrupt and leave races produce one authoritative outcome. Cancellation timeouts remain visibly stopping/uncertain and do not release concurrency slots while tools are still active. Retained idle views have a separate budget from live assignments; existing job concurrency limits remain enforced.
- Explicit parent-session close/process shutdown stops owned live assignments under a bounded shutdown contract. Merely switching parent or child views does not. Restart restores history only in this scope, not autonomous running work. Late events remain tied to the original parent identity and cannot enter a replacement /clear or /resume conversation.

**Permissions, input and model selection**
Keep PreHooks -> Gate/Ask -> Run -> PostHooks. Interactive worker children can ask the human instead of treating every Ask as unattended rejection. Explore/review ceilings remain read-only and must survive every model/mode/config change. Neither the parent model nor clicking a child can answer a permission request. Responses identify session, turn and interaction, and stale/post-cancel/mismatched responses fail closed. Headless children remain unattended and fail closed; no implicit allow-all inheritance. Cancel/close releases all pending reply waits.

Input follows current ordinary semantics: queue while working, inject at a tool-round seam, and dispatch remaining messages after a text-only turn. Esc recall and layered Ctrl+C behavior remain intact. Switching never resubmits a draft or appends a duplicate user message.

Model/effort changes are user-only. Use one consistent behavior in main and child: resolve and validate the pair before mutation, then atomically commit it for the next inference request; an already-running request and its tool round retain their snapshot. Invalid or unsupported choices leave the effective model unchanged with an actionable error. Show selected/pending/effective state when different. This intentionally extends today's Controller idle-only picker and must not be implemented by simply removing requireRunIdle: swapModel currently also changes gate, callbacks, jobs and hooks through multiple calls. Provide one coherent reconfiguration commit that preserves the child profile and does not widen tools or permission policy. Existing plan-model precedence and user override semantics remain; record the effective model/effort per round. An automatic child inheritance or view activation never rewrites main's selected model or global last-used preferences.

**Parent result delivery**
Use a typed parent inbox, fed by both existing watch delivery and new child lifecycle events where their scheduling is shared. Do not turn lifecycle results into watch.Event or feed them into its drop-oldest queue. Job progress/text deltas may coalesce; accepted terminal outcomes and interaction requests may not silently disappear.

Persist a correlated terminal outcome and bounded summary/result reference before delivery. Envelope identifies parent session, child session, job, outcome/stop reason, sequence/event ID and whether the user intervened. Use the existing result size bound (12,000 bytes) and include status even when no assistant text exists. Treat result text as child output, never as a user instruction or permission approval.

Within the owning process, route outcomes to the exact parent inbox. Append delivery identity with the parent context record before acknowledging consumption; retry/reconcile by that identity. An explicit agent_wait and automatic push share a result identity so a consumed result does not trigger a redundant autonomous wake. Do not promise cross-crash exactly-once behavior. Store failures leave an explicit undelivered/error state rather than report delivery success. Bound memory with durable pending references/backpressure; do not drop the only terminal event under pressure. Recovery must not mark another live process's jobs failed merely because the jobs directory is shared; verify ownership before reusing existing recovery.

An active parent consumes outcomes at the next tool-round seam; an idle parent gets a coalesced wake. A result after the final tool boundary still wakes it. Preserve user draft/focus. Share a bounded autonomous wake streak with other background sources; explicit interruption suppresses automatic continuation while retaining events for the next user action. User interaction requests draw attention but do not start recursive model turns just to wait for a human. Headless keeps optional agent_wait as its dependency barrier and still reaps jobs on process exit; it gains no hidden interactive loop.

**Implementation sequence and effort**
1. Runtime ownership — high. Reuse/update multisession-runtime-split. Explicit process/workspace/session ownership, scoped model/permission state and history writer/navigation split; two Controllers share resources without cross-close or identity leakage. Covers cmd assembly and headless invariants. No UI changes required yet.
2. Retained interactive Views — high, after 1. Reuse/update multisession-registry and the necessary part of multisession-switch-cues. Full per-session interaction state, origin-bound callbacks, all-bus draining, focus routing, inactive overlays and branch-watch cleanup. Verify two live fake-server sessions before children use this path.
3. Interactive child lifecycle — high, after 1 and integrating with 2. New child-specific work under multisession-mode, not a duplicate registry task. Retained native Controller adapter for job.Runner; structured identity, turn interrupt versus assignment stop, stop-on-leave, continued input and human permission/question/continue routing. Role ceilings and fail-closed headless behavior are acceptance criteria of this step, not later hardening.
4. Atomic per-session model/effort selection — high, after 1; can be developed independently from most of 2/3 in its own worktree. Reuse the existing engine round snapshot; concentrate coherent reconfiguration in one module. Verify queued choice, invalid effort rollback, concurrent tool round, role ceiling and plan override behavior in main and child.
5. Parent inbox and wake — high, after 3 (interface contract can be developed alongside it). New child outcome delivery work, sharing scheduler behavior with watches. Persisted correlation, no terminal loss, wait/push dedup, user-stop precedence, result-after-final-boundary and burst handling. Existing multisession-background-attention supplies UI notices but is not a substitute for model inbox delivery.
6. Child selector and attention UI — medium, after 2/3/5. Reuse/update multisession-sessions-panel, multisession-hotkeys and multisession-switch-cues/background-attention. Render clickable '● main / ○ child-name'; dot means selection only, with separate running/waiting/interrupted/stopped/error/unread marks. Keyboard access and previous-session navigation use the existing keys table. Header/composer identify the current session; focus never follows background completion. Use spawn description/fallback names; no title-generation tool prerequisite. Small terminals and overlays must remain navigable.
7. Integration and lifecycle hardening — high, after 3-6. Extend multisession-hardening with deterministic ordering tests and public-interface integration tests, then race tests and live TUI smoke. No second prompt scheduler or child-only queue/model semantics remains.
8. Documentation and gates — low, after 7. Update doc/tui.md, watch guidance where lifetime wording changes, child tool/system guidance, domain/architecture notes if needed and CHANGELOG Unreleased. agent_spawn guidance says push-driven in interactive mode, wait only for explicit barriers/headless. Run make fmt-check lint test, ownership/race checks and final smoke; diagnose substantive failures in an appropriately high-effort step rather than bury them in low-effort closeout. Commit code and ledger in task worktrees/main according to repository conventions; merge only after verification.

Existing multisession tasks remain unchanged until implementation-plan approval. Do not mark the broad multi-project, restore or title epics done merely because this focused slice ships.

**Verification matrix for implementation**
- Two sessions stream concurrently; mouse/keyboard selection preserves draft, media, skills, scroll, history cursor and model; hidden events do not affect active controls.
- Running-leave continues; interrupt-stay retains job/session; interrupt-submit continues once; interrupt-leave stops; leave before cancellation acknowledgement cannot restart queued work. Test both race orders and same-session clicks.
- Text-only children still appear and deliver results; queued input racing final output is accepted exactly once and reflected in the correct assignment result.
- Permission/question/continue in the background stays attached to its origin; switch is possible while modal; stale or cross-session replies are rejected; cancellation unblocks waits.
- Child model switch during inference takes effect only for the next inference snapshot; unsupported effort is rejected; main model is unchanged; read-only role and edit-capability isolation survive.
- Busy/idle/interrupted parent, final-boundary race, burst of completions, explicit wait versus push, duplicate events, full queues and persistence failure have defined non-lossy outcomes without focus theft or autonomous loops.
- Close and timeout do not leak jobs, streams, reply channels, workspaces or watchers; retained views do not consume job slots after terminalization. Process ownership/recovery and legacy job metadata remain safe.
- Existing headless spawn/wait/cancel results and process-exit cleanup remain compatible. No child gains memory, tasks, watches or nested agents merely by becoming interactive.
- Tests use public module interfaces, fake inference streams and controlled ordering. Run focused packages first, then -race for job/agent/controller/editor/sessions; full repository gates and a real terminal smoke are required before claiming the UI complete.

**Baseline checks actually executed**
At the inspected revision, all passed (not proof of the proposed behavior):
`go test -mod=readonly ./internal/tui/controller -run 'Test(WatchEventWakesAnIdleSession|WatchEventRidesIntoARunningTurn|WakeStreakStopsARunawayWatch|EscCallsOffAPendingWake|TheQueueDropsTheOldestUnderPressure)$' -count=1 -timeout=30s` (4.430s).
`GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/job -run 'Test(SpawnWaitResultOnDisk|CancelStopsRunner|HandleWaitTimeoutDoesNotCancelJob|CloseReapsLiveJobsAndWaitsForFinalWrites|SubscribeProgress)$' -count=1 -timeout=30s` (3.418s).
`GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/agent -run 'Test(EngineRunnerViaJobManager|EngineRunnerCancel|LoopInjectsQueuedPromptAtToolBoundary|LoopAgentSpawnWorkdirEscapeFailsSync)$' -count=1 -timeout=30s` (0.478s).
`GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/tui/controller -run 'Test(Bus|Controller_(CancelKeeps|Recall|QueueInjects|LifecycleMutation)|ControllerSetModelEffort)' -count=1 -timeout=30s` (0.989s).
`GOPROXY=off GOSUMDB=off go test -mod=readonly ./internal/tui/editor -run 'Test(AcceptInterrupt|EditorQueued|EditorEscRecalls|MainScreen)' -count=1 -timeout=30s` (2.114s).
No full suite, race suite or live TUI smoke was run in this design task. Exact source routes and investigator limitations are recorded in the preceding note.

**Trade-offs**
Architecture/locality: one retained session mechanism for main and children, accepting an up-front ownership refactor instead of a parallel viewer. Extensibility: only real interactive/headless runner adapters, no speculative universal backend. Testability/reliability: explicit identities, serialized transitions and persisted outcome delivery add implementation work but make races testable and prevent false completion. Security: normal human interaction without role or permission widening. Readability: three distinct lifetimes and a selection-only dot instead of overloaded job/UI booleans.

**Done (2026-09-05).** Completed the read-only investigation and recorded the proposed eight-step implementation plan, effort levels, Session/Turn/Job lifecycle, role/permission isolation, atomic model selection, parent inbox delivery and verification matrix in obsidian-tasks/interactive-child-sessions-design.md. Focused baseline tests passed; no code changes or implementation started. The implementation contract requires separate approval. This task closes design only, not the multisession feature; its ledger note is the sole file in the bookkeeping commit.

## Acceptance Criteria

- Текущие agent jobs и интерактивные сессии исследованы с ссылками на код.
- Спроектированы lifecycle, isolation и доставка событий для интерактивных детей.
- План реализации учитывает существующие multisession-задачи, отдельные effort и проверки.

## Verification Plan

1. Проверить исходники и тесты agent jobs, Controller/Editor и фоновых событий на зафиксированной ревизии.
2. Сопоставить с multisession runtime/registry/UI задачами; указать противоречия и неопределённости.
3. Представить план реализации с effort, зависимостями и проверками гонок и permissions.
