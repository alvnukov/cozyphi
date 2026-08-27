// Package watchtool exposes background watches as one model-facing tool.
//
// A watch is how the agent stops polling. Instead of running `gh pr checks` in
// a loop and burning a turn on each answer, it starts a watch and is told when
// something happens — mid-turn if a turn is running, or by waking the session
// if none is. Nothing here delivers events: the tool starts and stops watches,
// and the session decides what an event does.
package watchtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/watch"
)

// defaultLogLimit is a readable page of one watch's history.
const defaultLogLimit = 20

const description = `Watch something in the background and be told when it happens, instead of
polling for it. Events reach you on their own: mid-turn if a turn is running,
otherwise by starting one. Never call this in a loop to check on a watch.

Three shapes, chosen by what you pass:

- command, no every: streaming. Every matching line of output is an event.
  Use match to filter — an unfiltered log is a firehose and gets stopped.
  Set on=exit instead to get one event when the command finishes, with its
  exit code and tail: that is the shape for a long build or test run.
- command and every: polling. The command runs on each tick and an event
  fires only when its output changed. The first run is the silent baseline.
  This is the shape for a remote API — a CI run, a queue, a deploy.
- every, no command: a reminder. The label comes back when the interval is up.
  Add once=true for a one-shot.

Rules that matter:
- A watch outlives the turn that started it and runs until stopped or until
  cozyphi exits. Stop what you no longer need.
- Filter for what you would act on, including failure. A watch that greps
  only the success line stays silent through a crash, and silence reads
  exactly like "still running".
- Do not use a watch for something that finishes in seconds — run it with
  bash and read the answer.`

// Deps binds the tool to a manager. A nil manager yields a tool that explains
// it has nowhere to run rather than failing: sub-agents carry no manager.
type Deps struct {
	Manager *watch.Manager
}

// Tool returns the watch tool definition and handler.
func Tool(deps Deps) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "watch",
			Description: description,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type": "string",
						"enum": []string{"start", "list", "log", "stop"},
						"description": "start: begin watching. list: what is running. " +
							"log: what one watch has seen. stop: end one.",
					},
					"command": llm.Object{
						"type":        "string",
						"description": "Shell command to watch. Omit for a plain reminder (start action).",
					},
					"label": llm.Object{
						"type": "string",
						"description": "What is being watched, in a few words. It titles every event, " +
							"and it is what a reminder comes back with. Required without a command.",
					},
					"match": llm.Object{
						"type": "string",
						"description": "Regexp filter on output lines. Cover failure too, not just the " +
							"happy path: \"passed|failed|error|panic\".",
					},
					"on": llm.Object{
						"type":        "string",
						"enum":        []string{"line", "exit"},
						"description": "line (default): every matching line. exit: one event when the command finishes.",
					},
					"every": llm.Object{
						"type": "string",
						"description": fmt.Sprintf(
							"Interval as a duration (%q, %q). Turns a command into a poll and a label into "+
								"a reminder. Minimum %s.", "30s", "5m", watch.MinInterval),
					},
					"once": llm.Object{
						"type":        "boolean",
						"description": "Fire one time, then stop. Needs every.",
					},
					"id": llm.Object{
						"type":        "string",
						"description": "Which watch to act on, as list names it (log and stop actions).",
					},
					"limit": llm.Object{
						"type":        "integer",
						"minimum":     1,
						"description": fmt.Sprintf("Events to show; default %d (log action).", defaultLogLimit),
					},
				},
			},
		},
		DetailFromArgs: detail,
		Run:            run(deps.Manager),
	}
}

type input struct {
	Action  string `json:"action"`
	Command string `json:"command"`
	Label   string `json:"label"`
	Match   string `json:"match"`
	On      string `json:"on"`
	Every   string `json:"every"`
	Once    bool   `json:"once"`
	ID      string `json:"id"`
	Limit   *int   `json:"limit"`
	// PlanStep is injected by the plan gate and consumed before this tool runs;
	// it is accepted here so strict decoding never rejects a gate-valid call.
	PlanStep *int `json:"plan_step"`
}

func run(mgr *watch.Manager) tooldef.Handler {
	return func(_ context.Context, raw json.RawMessage) (tooldef.Result, error) {
		in, err := parse(raw)
		if err != nil {
			return tooldef.Result{}, err
		}
		if mgr == nil {
			return tooldef.Result{}, errors.New(
				"watch: this session runs no watches; do the work directly instead")
		}
		switch in.Action {
		case "list":
			return list(mgr)
		case "log":
			return watchLog(mgr, in.ID, limitOf(in.Limit))
		case "stop":
			return stop(mgr, in.ID)
		default:
			return start(mgr, in)
		}
	}
}

func parse(raw json.RawMessage) (input, error) {
	in := input{Action: "start"}
	if len(raw) == 0 {
		return in, nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&in); err != nil {
		return input{}, fmt.Errorf("watch: invalid arguments: %w", err)
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	if in.Action == "" {
		in.Action = "start"
	}
	switch in.Action {
	case "start", "list", "log", "stop":
	default:
		return input{}, fmt.Errorf("watch: unknown action %q (use start, list, log or stop)", in.Action)
	}
	if (in.Action == "log" || in.Action == "stop") && strings.TrimSpace(in.ID) == "" {
		return input{}, fmt.Errorf("watch: %s needs an id; call action=list to see what is running", in.Action)
	}
	return in, nil
}

func limitOf(limit *int) int {
	if limit == nil || *limit <= 0 {
		return defaultLogLimit
	}
	return *limit
}

func start(mgr *watch.Manager, in input) (tooldef.Result, error) {
	every, err := parseEvery(in.Every)
	if err != nil {
		return tooldef.Result{}, err
	}
	w, err := mgr.Start(watch.Spec{
		Label:   in.Label,
		Command: in.Command,
		Match:   in.Match,
		On:      watch.Trigger(strings.ToLower(strings.TrimSpace(in.On))),
		Every:   every,
		Once:    in.Once,
	})
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("watch: %w", err)
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Watching %s as %s.\n\n%s\n\n", w.Label, w.ID, shape(w))
	sb.WriteString("Events will reach you on their own — do not call this tool to check on it.\n")
	fmt.Fprintf(&sb, "Stop it with action=stop, id=%s when it is no longer useful.", w.ID)
	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("%s %s", w.ID, w.Label)}, nil
}

// shape says in one line what this watch will actually do, so the model does
// not have to re-derive it from the arguments it passed.
func shape(w watch.Watch) string {
	switch {
	case w.Command == "":
		return fmt.Sprintf("Every %s it comes back with its label.", w.Every)
	case w.Every > 0:
		return fmt.Sprintf(
			"It runs every %s and fires when the output changes; the first run is the baseline.", w.Every)
	case w.On == watch.OnExit:
		return "It fires once, when the command finishes, with the exit code and the tail."
	default:
		return "Every matching line is an event."
	}
}

func list(mgr *watch.Manager) (tooldef.Result, error) {
	watches := mgr.List()
	if len(watches) == 0 {
		return tooldef.Result{Content: "No watches. Start one with action=start.", Detail: "none"}, nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d watches, %d still running:\n\n", len(watches), mgr.Live())
	for _, w := range watches {
		fmt.Fprintf(&sb, "- %s %s — %s, %d events%s\n", w.ID, w.Label, state(w), w.Events, reason(w))
		if w.Command != "" {
			fmt.Fprintf(&sb, "    %s\n", w.Command)
		}
	}
	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("list (%d)", len(watches))}, nil
}

func state(w watch.Watch) string {
	if w.Live {
		return "running"
	}
	return "stopped"
}

func reason(w watch.Watch) string {
	if w.Err == "" {
		return ""
	}
	return " (" + w.Err + ")"
}

func watchLog(mgr *watch.Manager, id string, limit int) (tooldef.Result, error) {
	events, err := mgr.Log(id, limit)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("watch: %w", err)
	}
	if len(events) == 0 {
		return tooldef.Result{
			Content: id + " has seen nothing yet.",
			Detail:  id + " (empty)",
		}, nil
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Last %d events from %s:\n\n", len(events), id)
	for _, ev := range events {
		fmt.Fprintf(&sb, "[%s] %s\n", ev.Time.Format(time.TimeOnly), ev.Text)
	}
	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("%s (%d)", id, len(events))}, nil
}

func stop(mgr *watch.Manager, id string) (tooldef.Result, error) {
	if err := mgr.Stop(id); err != nil {
		return tooldef.Result{}, fmt.Errorf("watch: %w", err)
	}
	return tooldef.Result{Content: fmt.Sprintf("Stopped %s.", id), Detail: "stopped " + id}, nil
}

// parseEvery accepts a duration the way a human writes one. The floor itself
// is the manager's rule, not this one.
func parseEvery(text string) (time.Duration, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, nil
	}
	every, err := time.ParseDuration(text)
	if err != nil {
		return 0, fmt.Errorf("watch: every %q is not a duration (try \"30s\", \"5m\", \"1h\"): %w", text, err)
	}
	return every, nil
}

// detail is the one line the UI shows before the tool runs — and the line the
// permission overlay shows beside the command, so the interval belongs in it.
func detail(raw json.RawMessage) string {
	var in input
	_ = json.Unmarshal(raw, &in)
	action := strings.TrimSpace(in.Action)
	if action == "" {
		action = "start"
	}
	if action != "start" {
		return strings.TrimSpace(action + " " + in.ID)
	}
	subject := strings.TrimSpace(in.Command)
	if subject == "" {
		subject = strings.TrimSpace(in.Label)
	}
	if every := strings.TrimSpace(in.Every); every != "" {
		return fmt.Sprintf("every %s: %s", every, subject)
	}
	return subject
}
