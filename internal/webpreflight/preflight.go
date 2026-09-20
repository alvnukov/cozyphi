// Package webpreflight runs the one-time capability check of the configured
// web model route (spec protected-web-research.md, D2): a bounded fixture
// set sent to the pinned model's provider to observe, before web is used,
// that the route accepts image input, answers executor-less tool-call
// rounds, and replies in a strict structured form.
//
// Two gates run before any request. Shared account admission first: with no
// adapter delivered (routing-openai-account-admission), the verdict is an
// honest unavailable that names the blocker — not an improvised bypass, and
// not a consent question for a route that cannot run. Explicit consent
// second: the spend is quota on the model provider, a recipient no
// permissions.web.allow entry ever consented to (D8), so the user is asked
// once, with the route identity and the exact request budget.
//
// The preflight is evidence, not certification: a route that later refuses
// at runtime still fails closed at the tool boundary, and nothing here
// overrides that.
package webpreflight

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/llm/client"
)

// MaxRequests is the fixed request budget of one preflight run: one per
// probe, no retries. It is exported so the consent question can state the
// exact spend it is asking about.
const MaxRequests = 3

// Status aggregates the probe outcomes into the verdict a caller acts on.
type Status string

// Verdict statuses. NotReady means the route was examined and failed a
// requirement; Unavailable means the run could not even start, for a reason
// the caller can act on; Limited means the text pipeline works but a
// declared capability is missing — image reads stay off, never substituted.
const (
	StatusReady       Status = "ready"
	StatusLimited     Status = "limited"
	StatusNotReady    Status = "not-ready"
	StatusUnavailable Status = "unavailable"
)

// AdmissionFunc is the shared account admission hook. It reports whether the
// shared-account machinery allows this preflight's spend right now. The seam
// belongs to routing-openai-account-admission; until that lands there is no
// adapter, and nil here is the honest production state.
type AdmissionFunc func(ctx context.Context) error

// ConsentFunc asks the user for the one-time preflight approval. identity is
// the route's non-secret identity line; budget is MaxRequests. A non-nil
// error is a denial or a failed question — either way, fail closed.
type ConsentFunc func(ctx context.Context, identity string, budget int) error

// Runner runs the preflight against one pinned web model route.
type Runner struct {
	model    llm.ModelConfig
	identity string
	tools    []llm.ToolDefinition
	admit    AdmissionFunc
	consent  ConsentFunc
}

// New builds a runner. tools are the decoy tool definitions offered to the
// route: real definitions, so the offer is indistinguishable from the
// session's own tool list, but the runner holds no executor — a tool call in
// a reply is recorded, never dispatched.
func New(
	model llm.ModelConfig,
	identity string,
	tools []llm.ToolDefinition,
	admit AdmissionFunc,
	consent ConsentFunc,
) *Runner {
	return &Runner{model: model, identity: identity, tools: tools, admit: admit, consent: consent}
}

// Verdict is the outcome of one preflight run.
type Verdict struct {
	Status Status
	// Blocker names what stands in the way when the status is not ready:
	// the undelivered adapter for unavailable, the failed requirement
	// otherwise. Fixed vocabulary only — never raw model text.
	Blocker string
	// Vision, Tools and Structured are the individual probe outcomes.
	Vision     Probe
	Tools      Probe
	Structured Probe
	// Requests is the number of provider requests actually sent.
	Requests int
}

// Probe is one capability observation: pass, or fail with a structural
// reason. Unproven is fail — the preflight never upgrades an unknown to a
// pass.
type Probe struct {
	Pass   bool
	Reason string
}

// Run executes the preflight: admission, consent, then the bounded probe
// set. The returned error covers only the paths that are not verdicts —
// denied consent and cancellation. Cancellation is honored between probes;
// an in-flight request finishes or the stream breaks, never both probes.
func (r *Runner) Run(ctx context.Context) (Verdict, error) {
	if r.admit == nil {
		return Verdict{
			Status:  StatusUnavailable,
			Blocker: "shared account admission adapter is not delivered (routing-openai-account-admission)",
		}, nil
	}
	if err := r.admit(ctx); err != nil {
		return Verdict{
			Status:  StatusUnavailable,
			Blocker: fmt.Sprintf("shared account admission: %v", err),
		}, nil
	}
	if r.consent == nil {
		// No one wired to ask means no one can approve. Fail closed.
		return Verdict{}, errors.New("preflight consent: no consent hook wired")
	}
	if err := r.consent(ctx, r.identity, MaxRequests); err != nil {
		return Verdict{}, fmt.Errorf("preflight consent: %w", err)
	}

	v := Verdict{Requests: 0}
	v.Structured = r.probeStructured(ctx)
	v.Requests++
	if ctx.Err() != nil {
		return Verdict{}, fmt.Errorf("preflight cancelled: %w", ctx.Err())
	}
	v.Tools = r.probeTools(ctx)
	v.Requests++
	if ctx.Err() != nil {
		return Verdict{}, fmt.Errorf("preflight cancelled: %w", ctx.Err())
	}
	v.Vision = r.probeVision(ctx)
	v.Requests++

	switch {
	case !v.Structured.Pass || !v.Tools.Pass:
		v.Status = StatusNotReady
		v.Blocker = notReadyBlocker(v)
	case !v.Vision.Pass:
		v.Status = StatusLimited
		v.Blocker = "vision probe failed; image reads stay unavailable — " + v.Vision.Reason
	default:
		v.Status = StatusReady
	}
	return v, nil
}

// notReadyBlocker joins the failed probes' reasons. Reasons are structural
// by construction, so the join cannot smuggle model text into a verdict.
func notReadyBlocker(v Verdict) string {
	// Fixed order, not map order: the blocker is a refusal message users
	// compare across runs, so it must render identically every time.
	var parts []string
	for _, p := range []struct {
		name  string
		probe Probe
	}{
		{"structured replies", v.Structured},
		{"tool-call round", v.Tools},
	} {
		if !p.probe.Pass {
			parts = append(parts, p.name+" failed: "+p.probe.Reason)
		}
	}
	return strings.Join(parts, "; ")
}

// ask runs one client round and returns the assembled reply text and the
// tool calls it carried. One request, no retries, no executor: the iteration
// ends when the stream does.
func (r *Runner) ask(
	ctx context.Context,
	system string,
	msgs []llm.Message,
	tools []llm.ToolDefinition,
) (string, []llm.ToolCall, error) {
	c := client.NewClient(r.model, tools, system)
	var text strings.Builder
	var calls []llm.ToolCall
	for ev, err := range c.Stream(ctx, msgs) {
		if err != nil {
			return text.String(), calls, err
		}
		if ev.Type == llm.StreamEventTypeDelta {
			text.WriteString(ev.Delta.Content)
			calls = append(calls, ev.Delta.ToolCalls...)
		}
	}
	return text.String(), calls, nil
}

// strictReply decodes a reply that must be exactly one JSON object with the
// given keys present. Anything else — prose around it, missing keys, a
// truncated object — is a fail, not a pass.
func strictReply(text string, keys ...string) (map[string]string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") || !strings.HasSuffix(trimmed, "}") {
		return nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		return nil, false
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		s, ok := raw[k].(string)
		if !ok {
			return nil, false
		}
		out[k] = strings.ToLower(strings.TrimSpace(s))
	}
	return out, true
}

// probeStructured checks that the route answers a strict single-object JSON
// request. This is the shape every later protected-web exchange relies on.
func (r *Runner) probeStructured(ctx context.Context) Probe {
	// echoMarker is a fixed recognition string, not a secret: the structured
	// probe asks the route to echo it back inside strict JSON.
	const echoMarker = "preflight-echo-7f3a"
	text, _, err := r.ask(ctx,
		"You are a capability probe. Reply with exactly one JSON object {\"echo\":\"<token>\"} and nothing else.",
		[]llm.Message{{Role: llm.RoleUser, Content: "token: " + echoMarker}},
		nil,
	)
	if err != nil {
		return Probe{Reason: fmt.Sprintf("route did not answer: %v", err)}
	}
	reply, ok := strictReply(text, "echo")
	if !ok || reply["echo"] != echoMarker {
		return Probe{Reason: "reply was not the expected JSON object"}
	}
	return Probe{Pass: true}
}

// probeTools checks that the route can be offered tools and will issue a
// tool call when asked. The call is recorded and never executed: the runner
// holds no executor, and the decoy definitions stay offers, not abilities.
func (r *Runner) probeTools(ctx context.Context) Probe {
	if len(r.tools) == 0 {
		return Probe{Reason: "no tool definitions were provided to offer"}
	}
	_, calls, err := r.ask(ctx,
		"You are a capability probe. Use the offered tool exactly as instructed; do not answer in prose.",
		[]llm.Message{{Role: llm.RoleUser, Content: "Call the bash tool with command \"echo preflight\"."}},
		r.tools,
	)
	if err != nil {
		return Probe{Reason: fmt.Sprintf("route did not answer: %v", err)}
	}
	offered := make(map[string]bool, len(r.tools))
	for _, t := range r.tools {
		offered[t.Name] = true
	}
	for _, c := range calls {
		if offered[c.Function.Name] {
			return Probe{Pass: true}
		}
	}
	return Probe{Reason: "model issued no tool call for an offered tool"}
}

// probeVision checks that the route accepts an inline image and reads it.
// The fixture is a 2x2 PNG, red on top and blue on bottom; the reply must
// name both halves. A blind guess has to land two colors, and a route that
// rejects image input fails on the transport instead — either way the probe
// does not pass, and image reads stay unavailable rather than degrading to
// OCR-only (D2).
func (r *Runner) probeVision(ctx context.Context) Probe {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	img.Set(1, 0, color.RGBA{R: 255, A: 255})
	img.Set(0, 1, color.RGBA{B: 255, A: 255})
	img.Set(1, 1, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return Probe{Reason: "fixture image could not be encoded"}
	}
	media := llm.Media{MediaType: "image/png", Data: base64.StdEncoding.EncodeToString(buf.Bytes())}

	text, _, err := r.ask(
		ctx,
		"You are a capability probe. Look at the attached image and reply with exactly one JSON object {\"top\":\"<color>\",\"bottom\":\"<color>\"}; one word per color.",
		[]llm.Message{{
			Role:    llm.RoleUser,
			Content: "What color is the top half, and what color is the bottom half?",
			Media:   []llm.Media{media},
		}},
		nil,
	)
	if err != nil {
		return Probe{Reason: fmt.Sprintf("route rejected or failed the image request: %v", err)}
	}
	reply, ok := strictReply(text, "top", "bottom")
	if !ok || !strings.Contains(reply["top"], "red") || !strings.Contains(reply["bottom"], "blue") {
		return Probe{Reason: "model did not identify the fixture image"}
	}
	return Probe{Pass: true}
}
