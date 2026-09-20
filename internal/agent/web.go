package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	cozyconfig "github.com/alvnukov/cozy-tools/config"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/webtool"
	"github.com/alvnukov/cozyphi/internal/webpreflight"
)

// WebOptions is the engine's half of the web configuration: the library
// policy plus the legacy quarantine setting, carried as data. The zero value
// has web disabled only if the policy says so — cozyconfig.WebPolicy defaults
// to enabled — so callers that want no web tool pass an explicitly disabled
// policy.
type WebOptions struct {
	Policy cozyconfig.WebPolicy
	// Quarantine records the configured web.quarantine mode for observation
	// (diag reports it). It authorizes nothing: unchecked delivery is gone,
	// and the web tool fails closed until an explicit web model binding
	// exists — see webtool.Deps.Ready.
	Quarantine bool
	// resolveBinding re-reads the model catalog at each tool admission. Provider
	// connect/remove and route changes must invalidate the prior verdict without
	// requiring a new engine.
	resolveBinding func() project.WebBinding
	// admit is the shared account admission hook for the capability
	// preflight. The seam belongs to routing-openai-account-admission; until
	// that task delivers an adapter it stays nil in production, and the
	// preflight reports unavailable naming that task. Tests inject a fake
	// here — nowhere else.
	admit webpreflight.AdmissionFunc
}

// WebOptionsFrom translates the resolved `web:` config section into what an
// engine needs. It is the one place the config's quarantine vocabulary turns
// into a boolean, so a new mode cannot silently read as "off". The boolean
// is data for observation only: it no longer authorizes unchecked delivery,
// and web.enabled: true is not a protected-readiness claim.
//
// find resolves a model name against the session's catalog (configured
// models plus connected providers); nil leaves any pin unresolved, which the
// binding reports as missing rather than guessing.
func WebOptionsFrom(cfg project.WebConfig, find func(string) (llm.ModelConfig, bool)) WebOptions {
	return WebOptions{
		Policy:         cfg.Policy,
		Quarantine:     cfg.Quarantine != project.WebQuarantineOff,
		resolveBinding: func() project.WebBinding { return cfg.Binding(find) },
	}
}

// enabled reports whether this engine should carry a web tool at all.
func (o WebOptions) enabled() bool {
	return o.Policy.IsEnabled() && strings.TrimSpace(o.Policy.CacheDir) != ""
}

// turnWeb is the per-turn web state the permission layer reads. It answers
// two questions and records nothing else: has untrusted page text entered
// this turn's context, and which egress hosts has the turn already reached
// with an approved call.
//
// It implements permission.Taint. The engine owns it because the turn is the
// engine's; the gate only ever asks.
type turnWeb struct {
	mu      sync.Mutex
	tainted bool
	hosts   map[string]bool
}

func newTurnWeb() *turnWeb { return &turnWeb{} }

// Tainted reports whether web text reached the model this turn.
func (t *turnWeb) Tainted() bool {
	if t == nil {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.tainted
}

// HostSeen reports whether an approved call already reached this host during
// the turn.
func (t *turnWeb) HostSeen(host string) bool {
	if t == nil {
		return false
	}
	host = strings.ToLower(strings.TrimSpace(host))
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.hosts[host]
}

// mark records that untrusted web text entered the context. It is one-way
// within a turn: nothing un-taints a context that has already read a page.
func (t *turnWeb) mark() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tainted = true
}

// seeHost records an egress destination the turn has been allowed to reach,
// so a second call to the same host is not re-asked after every page.
func (t *turnWeb) seeHost(host string) {
	if t == nil {
		return
	}
	host = strings.ToLower(strings.TrimSpace(host))
	if host == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hosts == nil {
		t.hosts = make(map[string]bool)
	}
	t.hosts[host] = true
}

// reset clears the turn's web state. Loop calls it at the top of every turn:
// the taint is a property of one context-building turn, not of the session.
func (t *turnWeb) reset() {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	t.tainted = false
	t.hosts = nil
}

// webRuntime is the snapshot the web tool's collaborators read at call
// time: the live tool registry the decoys are copied from. bindExecutor
// refreshes it under engine.mu, and readers take only this lock — a tool
// running inside the executor must never need the engine's.
type webRuntime struct {
	mu       sync.RWMutex
	registry tools.Registry
	// verdictFP/verdict cache one preflight verdict for the route
	// fingerprint it was earned against. It never survives a restart, and
	// a fingerprint change leaves it behind — provider connect/remove and
	// route edits re-probe rather than reuse (project.WebBinding.
	// Fingerprint is the invalidation key).
	verdictFP string
	verdict   webpreflight.Verdict
}

func (r *webRuntime) set(registry tools.Registry) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registry = registry
}

func (r *webRuntime) snapshot() tools.Registry {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry
}

// cachedVerdict returns the preflight verdict recorded for this exact route
// fingerprint, if any. A zero Status means nothing is cached.
func (r *webRuntime) cachedVerdict(fp string) (webpreflight.Verdict, bool) {
	if r == nil || fp == "" {
		return webpreflight.Verdict{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.verdictFP != fp || r.verdict.Status == "" {
		return webpreflight.Verdict{}, false
	}
	return r.verdict, true
}

// rememberVerdict records the preflight verdict for a route fingerprint,
// replacing any verdict earned against a different route.
func (r *webRuntime) rememberVerdict(fp string, v webpreflight.Verdict) {
	if r == nil || fp == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.verdictFP = fp
	r.verdict = v
}

// webTool builds this engine's web tool, or nothing when web is off. The
// caller must hold engine.mu (buildToolListFor does).
//
// Readiness runs through webAdmission: the binding is resolved, then the
// consented capability preflight's verdict — cached per route fingerprint —
// decides. Deps.Ready stays unset; the admission closure is the one entry
// that guards acquisition and model calls, so no gate mode can weaken it,
// and the session model is never a substitute, the legacy wiring that ran
// the reader on it is gone.
func (engine *Engine) webTool() []tools.Tool {
	if engine == nil || !engine.web.enabled() {
		return nil
	}
	runtime := engine.webRuntime
	return webtool.Tool(webtool.Deps{
		Policy:    engine.web.Policy,
		Mask:      engine.webMask,
		Admission: engine.webAdmission,
		Decoys: func(trap *webtool.Trap) []tools.Tool {
			return webtool.Decoys(runtime.snapshot(), trap)
		},
		Warn:        engine.emitWebNotice,
		MarkTainted: engine.turnWeb.mark,
	})
}

// webAdmission is the web tool's call-time readiness verdict: the binding
// must resolve, and the consented capability preflight must have passed for
// exactly this route. The verdict is cached per route fingerprint; with no
// shared account admission adapter delivered the preflight reports
// unavailable naming routing-openai-account-admission, and no consent
// question and no provider request are spent. A consent the user refused is
// cached as a refusal for the route — one denial, not one per call; a restart
// or a fingerprint change re-asks. Policy and mode denials re-check per call:
// they can change without the route changing.
func (engine *Engine) webAdmission(ctx context.Context) (bool, string) {
	if engine.web.resolveBinding == nil {
		return false, project.WebBinding{}.NotReadyReason()
	}
	binding := engine.web.resolveBinding()
	model, ok := binding.Model()
	if !ok {
		return false, binding.NotReadyReason()
	}
	fp := binding.Fingerprint()
	if v, cached := engine.webRuntime.cachedVerdict(fp); cached {
		return v.Status == webpreflight.StatusReady, verdictRefusal(binding.Identity(), v)
	}
	identity := binding.Identity()
	runner := webpreflight.New(
		model,
		identity,
		engine.webDecoyDefinitions(),
		engine.web.admit,
		engine.webPreflightConsent,
	)
	verdict, err := runner.Run(ctx)
	if err != nil {
		// Only the user's explicit refusal is a durable fact worth caching
		// for the route. A failed ask channel is transport noise, and a
		// policy or mode denial can change without the route changing;
		// neither is remembered — they refuse this call and leave the route
		// uncached so the next call re-checks.
		verdict = webpreflight.Verdict{
			Status:  webpreflight.StatusNotReady,
			Blocker: fmt.Sprintf("consent for the web model preflight was not given: %v", err),
		}
		if errors.Is(err, errPreflightConsentRefused) {
			engine.webRuntime.rememberVerdict(fp, verdict)
		}
		return false, verdictRefusal(identity, verdict)
	}
	engine.webRuntime.rememberVerdict(fp, verdict)
	return verdict.Status == webpreflight.StatusReady, verdictRefusal(identity, verdict)
}

// verdictRefusal renders a non-ready verdict as the tool's refusal reason.
// Identity and blocker are both fixed-vocabulary, non-secret text.
func verdictRefusal(identity string, v webpreflight.Verdict) string {
	if v.Status == webpreflight.StatusReady {
		return ""
	}
	return fmt.Sprintf("capability preflight on %s: %s", identity, v.Blocker)
}

// webDecoyDefinitions offers the preflight the same tool names the
// session's decoys carry, definitions only — the runner holds no executor,
// so a tool call in a reply is recorded, never dispatched.
func (engine *Engine) webDecoyDefinitions() []llm.ToolDefinition {
	registry := engine.webRuntime.snapshot()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	sort.Strings(names)
	live := make([]tools.Tool, 0, len(names))
	for _, name := range names {
		live = append(live, registry[name])
	}
	return tools.Definitions(live)
}

// errPreflightConsentRefused marks consent outcomes that are an explicit
// no — a user denial or a policy denial — as opposed to a failed ask
// channel or cancellation, which carry no answer at all.
var errPreflightConsentRefused = errors.New("preflight consent refused")

// webPreflightConsent asks the user for the one-time preflight approval
// through the engine's permission gate — the same gate every tool call
// passes, with the dedicated web_preflight action, so no mode approves what
// the gate would not. The question names the recipient route and the exact
// request budget it is asking to spend. A nil gate reads as allow-all (the
// caller explicitly opted out); a nil ask channel fails closed without
// marking the refusal.
func (engine *Engine) webPreflightConsent(ctx context.Context, identity string, budget int) error {
	req := permission.Request{Action: permission.ActionWebPreflight, Tool: "web", Target: identity}
	gate := engine.gate
	if gate == nil {
		return nil
	}
	dec, reason := gate.Check(ctx, req)
	if dec == permission.Ask {
		reason = fmt.Sprintf("%s — up to %d model requests to %s", reason, budget, identity)
	}
	switch dec {
	case permission.Allow:
		return nil
	case permission.Deny:
		// Not wrapped in errPreflightConsentRefused: a policy or mode
		// denial is re-checked on every call rather than cached, because
		// the policy can change without the route changing.
		return fmt.Errorf("denied by policy: %s", reason)
	default: // Ask
		if engine.ask == nil {
			return fmt.Errorf("no approval channel wired; consent denied: %s", reason)
		}
		res, err := engine.ask(ctx, req, reason)
		if err != nil {
			return fmt.Errorf("asking for consent failed: %w", err)
		}
		if !res.Approved {
			return fmt.Errorf("%w: denied by user", errPreflightConsentRefused)
		}
		return nil
	}
}

// emitWebNotice publishes the user-facing row for a web warning — today a
// suspected injection. A nil sink (headless runs, tests) drops the row; the
// model is told the same thing through the tool result either way.
func (engine *Engine) emitWebNotice(text string) {
	if engine == nil {
		return
	}
	if sink := engine.sessionEvents; sink != nil {
		sink(session.WebNotice{Label: "web: prompt injection suspected", Text: text})
	}
}

// observeWebApproval remembers an egress destination the turn was allowed to
// reach. A later call to the same host is not re-asked after a page arrives:
// the turn already went there with the user's consent, so re-asking would
// train the user to click through rather than to read.
func (engine *Engine) observeWebApproval(req permission.Request) {
	if engine == nil || req.Action != permission.ActionWeb {
		return
	}
	engine.turnWeb.seeHost(req.Host)
}

// googleKeyFromEnv resolves the Google CSE key the way websearch does — from
// the named environment variable, never from a literal in the config — so the
// egress check can recognize it in a URL the model proposes.
func googleKeyFromEnv(policy cozyconfig.WebPolicy) string {
	name := strings.TrimSpace(policy.GoogleAPIKeyEnv)
	if name == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(name))
}
