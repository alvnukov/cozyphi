package controller

import (
	"github.com/alvnukov/cozyphi/internal/agent"
	"github.com/alvnukov/cozyphi/internal/diag"
	"github.com/alvnukov/cozyphi/internal/lsp"
	"github.com/alvnukov/cozyphi/internal/mcp"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/version"
)

// tuiMode is what the runtime category reports as the process shape. It is the
// counterpart of the headless run's "headless", and it is the only thing about
// the harness view that differs between the two entry points.
const tuiMode = "tui"

// newDiagnostics builds one session's read-only view of the harness, or nil
// when the process was not started with --developer-mode.
//
// One registry per user session rather than one per process: its accessors are
// bound to this controller, so switching or closing a session can never answer
// with another session's identity. The registry owns nothing and borrows
// nothing that needs closing — a session ends without taking a shared resource
// with it.
func (r *Runtime) newDiagnostics(c *Controller) *diag.Registry {
	if c == nil || !r.developerModeGranted() {
		return nil
	}
	return diag.NewRegistry(nil, diag.DefaultLimits(),
		diag.NewRuntimeCollector(diag.RuntimeDeps{
			Version:   version.Version,
			Mode:      tuiMode,
			Enabled:   true,
			Workspace: func() string { return c.cwd },
			SessionID: c.routingSessionID,
		}),
		diag.NewModelCollector(diag.ModelDeps{
			Configured:       c.configuredModelFacts,
			ConfiguredSource: c.configuredModelSource,
			State:            c.modelState,
			Providers:        c.providerFacts,
			Import:           c.importFacts,
		}),
		diag.NewPermissionCollector(diag.PermissionDeps{
			Configured: c.configuredPermissionFacts,
			Defaults:   permission.DefaultObservation,
			Gate:       c.gateFacts,
			Overlay:    c.permissionOverlay,
		}),
		diag.NewToolCollector(diag.ToolDeps{State: c.toolState}),
		diag.NewContextCollector(diag.ContextDeps{State: c.contextState}),
		diag.NewPlanCollector(diag.PlanDeps{State: c.planState}),
		diag.NewIntegrationCollector(diag.IntegrationDeps{MCP: c.mcpState, LSP: c.lspState}),
	)
}

// mcpState is the server pool's own account of itself, taken under the
// pool's lock together with what the workspace knew when it loaded it. The
// pool belongs to the workspace and is borrowed, never owned: a session ends
// without taking it away, and a workspace that never got one answers
// unavailable rather than reporting that nothing is configured.
//
// Reading it starts no server, opens no connection, lists no tools and
// closes no client. A server the model has not called yet is reported as one.
func (c *Controller) mcpState() diag.MCPState {
	if c == nil {
		return diag.MCPState{}
	}
	return mcp.Observe(c.mcpPool, c.mcpLoad)
}

// lspState is the language-server manager's own account of itself, taken
// under the manager's lock together with what the workspace knew when it
// opened it. The manager belongs to the workspace and is borrowed, never
// owned: a session ends without taking it away, and a workspace that never
// got one answers unavailable rather than reporting that no language server
// is configured.
//
// Reading it starts no server, downloads nothing, synchronizes no workspace,
// runs no query and asks for no diagnostics. A workspace nobody has queried
// yet is reported as one.
func (c *Controller) lspState() diag.LSPState {
	if c == nil {
		return diag.LSPState{}
	}
	return lsp.Observe(c.lspMgr, c.lspOpen)
}

// planState is the engine's own account of the durable plan: where it stands,
// what policy the gate compiles from, and what the step in progress may do.
// It is read through the published pointer like the layers above, so a resume
// or a rebind is answered for the engine this session is on now.
//
// Reading it approves nothing, starts and finishes no step, files no
// evidence, and leaves the plan revision where it stood.
func (c *Controller) planState() diag.PlanState {
	if c == nil {
		return diag.PlanState{}
	}
	return c.engineRef.Load().PlanObservation()
}

// contextState is the engine's own account of its context window: what it
// budgets against, what is in it, and what the system prompt was assembled
// from. It is read through the published pointer like the layers above, so a
// resume or a rebind is answered for the engine this session is on now.
//
// Reading it compacts nothing, trims nothing, loads no memory, re-reads no
// instruction file or skill, and leaves the token calibration where it stood.
func (c *Controller) contextState() diag.ContextState {
	if c == nil {
		return diag.ContextState{}
	}
	return c.engineRef.Load().ContextObservation()
}

// toolState is the engine's own answer about its tool layer, read through the
// published pointer for the same reason the model layer is: a rebind replaces
// the engine, and an observation must describe the one this session is on now
// rather than the one the registry was built around. Reading it dispatches no
// tool and asks for no approval.
func (c *Controller) toolState() diag.ToolState {
	if c == nil {
		return diag.ToolState{}
	}
	return c.engineRef.Load().ToolObservation()
}

// configuredPermissionFacts is the permissions block the configuration
// resolved: the built-in defaults with the config file's permissions section
// merged over them. It is what was asked for, and it stops being the truth
// the moment plan mode, a role ceiling or a bypass is applied to it — which
// is why it is a separate layer from the gate below rather than a stand-in
// for one.
func (c *Controller) configuredPermissionFacts() diag.PermissionFacts {
	cfg := c.loadedConfig()
	if cfg == nil {
		return diag.PermissionFacts{}
	}
	return permission.PolicyObservation(cfg.Permissions)
}

// gateFacts observes the boundary this session actually judges tool calls
// with — the one published to the engine, wrappers included. It reads the
// gate through the owner's projection and never hands it a request, so an
// observation decides nothing, grants nothing and leaves every later decision
// exactly as it was.
func (c *Controller) gateFacts() diag.GateFacts {
	if c == nil {
		return diag.GateFacts{}
	}
	return permission.Observe(c.currentGate())
}

// permissionOverlay names what this session put between the configured policy
// and the assembled one. A sub-agent runs under its role's ceiling, and plan
// mode overlays readonly on whatever the configuration says; with neither in
// force nothing narrowed the policy here, and a difference that remains is
// the session's own — a task level changed at runtime, say — rather than an
// origin this controller can name.
func (c *Controller) permissionOverlay() diag.Source {
	if c == nil {
		return diag.Source{}
	}
	switch {
	case c.childRole != "":
		return diag.Source{
			Kind: diag.SourceComputed,
			Ref:  "the sub-agent role ceiling, which narrows the configured rules and never widens them",
		}
	case c.mode == agent.ModePlan:
		return diag.Source{Kind: diag.SourcePlan, Ref: "plan mode overlays readonly"}
	default:
		return diag.Source{}
	}
}

// configuredModelFacts is what the loader resolved: the default model of the
// configuration this process loaded, environment overrides included. It is
// read through the project rather than remembered here, because a new session
// with no model of its own reloads the file — and it is only ever read: an
// observation never reloads anything itself, so a config file edited on disk
// changes nothing until an owner decides to read it again.
func (c *Controller) configuredModelFacts() diag.ModelFacts {
	cfg := c.loadedConfig()
	if cfg == nil {
		return diag.ModelFacts{}
	}
	return agent.ModelFacts(cfg.Model())
}

// configuredModelSource names how that default was chosen. The loader records
// exactly two facts about the choice, and this reports them rather than
// inferring an origin from the result.
func (c *Controller) configuredModelSource() diag.Source {
	cfg := c.loadedConfig()
	if cfg == nil {
		return diag.Source{Kind: diag.SourceUnknown}
	}
	return diag.ModelSelectionSource(cfg.ModelEnvOverride(), cfg.DefaultModel != "")
}

// modelState is the engine's own answer, read through the published pointer
// so a collector running on a tool goroutine sees the engine this session is
// on and not the one it was on when the registry was built.
func (c *Controller) modelState() diag.ModelState {
	if c == nil {
		return diag.ModelState{}
	}
	return c.engineRef.Load().ModelObservation()
}

// providerFacts is the provider manager's own answer about its catalog and
// its credential store. The manager is process-wide and borrowed, never
// owned: this reads what it already holds under its own lock and refreshes
// nothing — no catalog fetch, no re-read of either file, no authentication.
func (c *Controller) providerFacts() diag.ProviderFacts {
	if c == nil || c.runtime == nil {
		return diag.ProviderFacts{}
	}
	return c.runtime.providers.Observation()
}

// importFacts is the opencode import's state as it resolved at startup. The
// import runs once for the process, so this reports what happened then; a
// session that starts later observes the same outcome rather than retrying
// a read the user never asked for.
func (c *Controller) importFacts() diag.ImportFacts {
	if c == nil || c.runtime == nil {
		return diag.ImportFacts{}
	}
	return c.runtime.importState
}

// loadedConfig reads the configuration the project currently holds. The
// project swaps it atomically, so this is safe from a tool goroutine; a
// project without one yields nil and every configured layer says unavailable.
func (c *Controller) loadedConfig() *project.Config {
	if c == nil || c.proj == nil {
		return nil
	}
	return c.proj.Config()
}

// routingSessionID reads the identity this controller publishes for routing.
// It follows every engine replacement — clear, resume, model rebind — so an
// observation names the session the model is actually running in, and reads
// empty before the first engine exists rather than naming a session that does
// not.
func (c *Controller) routingSessionID() string {
	if c == nil {
		return ""
	}
	if id := c.progressSession.Load(); id != nil {
		return *id
	}
	return ""
}
