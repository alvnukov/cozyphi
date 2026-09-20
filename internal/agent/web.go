package agent

import (
	"os"
	"strings"
	"sync"

	cozyconfig "github.com/alvnukov/cozy-tools/config"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/webtool"
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
	// Binding is the web.model pin resolved against the model catalog at
	// admission (engine construction). It decides what the not-ready answer
	// names; it never flips Ready on its own — capability preflight has not
	// landed yet.
	Binding project.WebBinding
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
		Policy:     cfg.Policy,
		Quarantine: cfg.Quarantine != project.WebQuarantineOff,
		Binding:    cfg.Binding(find),
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

// webTool builds this engine's web tool, or nothing when web is off. The
// caller must hold engine.mu (buildToolListFor does).
//
// Deps.Ready is deliberately never set here: protected readiness requires a
// consented capability preflight over the pinned web model, and that ticket
// has not landed. The binding is still resolved at admission so the refusal
// can name exactly what is missing — no pin, a stale pin, or the missing
// preflight — instead of one generic answer; and the session model is never
// a substitute, the legacy wiring that ran the reader on it is gone. The
// tool fails closed at its own entry, after the permission gate has had its
// say, so no gate mode can weaken the refusal.
func (engine *Engine) webTool() []tools.Tool {
	if engine == nil || !engine.web.enabled() {
		return nil
	}
	runtime := engine.webRuntime
	return webtool.Tool(webtool.Deps{
		Policy:         engine.web.Policy,
		Mask:           engine.webMask,
		NotReadyReason: engine.web.Binding.NotReadyReason(),
		Decoys: func(trap *webtool.Trap) []tools.Tool {
			return webtool.Decoys(runtime.snapshot(), trap)
		},
		Warn:        engine.emitWebNotice,
		MarkTainted: engine.turnWeb.mark,
	})
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
