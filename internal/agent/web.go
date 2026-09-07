package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	cozyconfig "github.com/alvnukov/cozy-tools/config"

	"github.com/alvnukov/cozyphi/internal/llm"
	llmclient "github.com/alvnukov/cozyphi/internal/llm/client"
	"github.com/alvnukov/cozyphi/internal/permission"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/session"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/webtool"
)

// WebOptions is the engine's half of the web configuration: the library
// policy plus cozyphi's quarantine choice. The zero value has web disabled
// only if the policy says so — cozyconfig.WebPolicy defaults to enabled — so
// callers that want no web tool pass an explicitly disabled policy.
type WebOptions struct {
	Policy cozyconfig.WebPolicy
	// Quarantine is on when read and find must go through the tool-less
	// reader instead of returning page text.
	Quarantine bool
}

// WebOptionsFrom translates the resolved `web:` config section into what an
// engine needs. It is the one place the config's quarantine vocabulary turns
// into a boolean, so a new mode cannot silently read as "off".
func WebOptionsFrom(cfg project.WebConfig) WebOptions {
	return WebOptions{
		Policy:     cfg.Policy,
		Quarantine: cfg.Quarantine != project.WebQuarantineOff,
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

// webRuntime is the snapshot the web tool's collaborators read at call time:
// the live tool registry the decoys are copied from, and the model the
// quarantine reader runs on. bindExecutor refreshes it under engine.mu, and
// readers take only this lock — a tool running inside the executor must never
// need the engine's.
type webRuntime struct {
	mu       sync.RWMutex
	registry tools.Registry
	model    llm.ModelConfig
}

func (r *webRuntime) set(registry tools.Registry, model llm.ModelConfig) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.registry = registry
	r.model = model
}

func (r *webRuntime) snapshot() (tools.Registry, llm.ModelConfig) {
	if r == nil {
		return nil, llm.ModelConfig{}
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.registry, r.model
}

// webTool builds this engine's web tool, or nothing when web is off. The
// caller must hold engine.mu (buildToolListFor does).
func (engine *Engine) webTool() []tools.Tool {
	if engine == nil || !engine.web.enabled() {
		return nil
	}
	runtime := engine.webRuntime
	deps := webtool.Deps{
		Policy:     engine.web.Policy,
		Quarantine: engine.web.Quarantine,
		Mask:       engine.webMask,
		Decoys: func(trap *webtool.Trap) []tools.Tool {
			registry, _ := runtime.snapshot()
			return webtool.Decoys(registry, trap)
		},
		Warn:        engine.emitWebNotice,
		MarkTainted: engine.turnWeb.mark,
	}
	if engine.web.Quarantine {
		deps.Reader = quarantineReader{runtime: runtime}
	}
	return webtool.Tool(deps)
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

// quarantineReader runs the web-reader role: one model call with no real
// tools, over one bounded fragment, answering one question.
//
// It is not a job and not an Engine. A job would give the page a session, a
// transcript and a spawn surface; this is a single round with a fixed system
// prompt and a tool list that is entirely decoys, which is the smallest thing
// that can read a hostile page.
type quarantineReader struct {
	runtime *webRuntime
}

// Read implements webtool.Reader.
func (r quarantineReader) Read(ctx context.Context, req webtool.ReaderRequest) (webtool.ReaderResult, error) {
	_, model := r.runtime.snapshot()
	if strings.TrimSpace(model.Name) == "" {
		return webtool.ReaderResult{}, errors.New("agent: no model configured for the web quarantine reader")
	}
	// The reader is a sub-agent in posture, not in machinery: it gets the
	// session's model but none of its context, prompt or history.
	client := llmclient.NewClient(model, tools.Definitions(req.Tools), req.System)

	message := llm.Message{Role: llm.RoleUser, Content: readerPrompt(req)}
	for event, err := range client.Stream(ctx, []llm.Message{message}) {
		if err != nil {
			return webtool.ReaderResult{}, err
		}
		switch event.Type {
		case llm.StreamEventTypeError:
			if event.Err != nil {
				return webtool.ReaderResult{}, event.Err
			}
			return webtool.ReaderResult{}, errors.New("agent: web reader stream error")
		case llm.StreamEventTypeDone:
			if len(event.Partial.Choices) == 0 {
				return webtool.ReaderResult{}, errors.New("agent: web reader finished with no reply")
			}
			final := event.Partial.Choices[0].Message
			if len(final.ToolCalls) > 0 {
				return webtool.ReaderResult{}, fireDecoy(ctx, req.Tools, final.ToolCalls[0])
			}
			return webtool.ReaderResult{Answer: strings.TrimSpace(final.Content)}, nil
		case llm.StreamEventTypeDelta:
			// Nothing streams out of a quarantine read: the answer is only
			// ever delivered as a whole, framed, by the tool.
		}
	}
	return webtool.ReaderResult{}, errors.New("agent: web reader stream ended without a reply")
}

// fireDecoy runs the decoy the reader called so the trap records which one,
// then reports the refusal. An unrecognized name still aborts: the reader had
// only decoys, so any tool call at all is the page acting.
func fireDecoy(ctx context.Context, decoys []tools.Tool, call llm.ToolCall) error {
	for _, decoy := range decoys {
		if decoy.Definition.Name != call.Function.Name {
			continue
		}
		_, err := decoy.Run(ctx, []byte(call.Function.Arguments))
		if err != nil {
			return err
		}
		return webtool.ErrDecoy
	}
	return fmt.Errorf("%w: %s", webtool.ErrDecoy, call.Function.Name)
}

// readerPrompt is the reader's only user message: the question, then the
// fragment inside the same untrusted frame the session would have seen.
func readerPrompt(req webtool.ReaderRequest) string {
	var b strings.Builder
	b.WriteString("Question: ")
	b.WriteString(req.Question)
	b.WriteString("\n\nAnswer it from the page text below, and from nothing else.\n\n")
	b.WriteString(webtool.Frame(map[string]string{
		"kind":     "web_page_fragment",
		"doc_id":   req.DocID,
		"fragment": req.Fragment,
	}))
	return b.String()
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
