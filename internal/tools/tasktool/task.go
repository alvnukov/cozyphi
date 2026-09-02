// Package tasktool is the model-facing side of the task registry: how a
// session asks what to work on, takes a task, and closes it, against the
// same notes mcp-ai-helper keeps. Every answer ends with what to do next, so
// the registry reads like a workflow and not like a file format.
package tasktool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tasks"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

const description = `Work the repository's task registry: one markdown note per task under the
main checkout, shared with mcp-ai-helper. A task has an id, title, status
(todo, in_progress, blocked, done), priority, model_level, type, parent,
tags, body, acceptance criteria and verification plan.

Actions:
- current (default): what to work on — ready tasks best first (in_progress,
  then by priority), blocked ones apart. Call it before choosing work, even
  when a task id was named.
- list: every task on one line each; narrow with status, type, tag or parent.
- get: one task in full, by id.
- create: a new task from title (+ id, body, type, priority, model_level,
  parent, tags, acceptance_criteria, verification_plan, status).
- update: change those fields on an existing task; lists replace whole.
- start: take a task: in_progress, plus the branch and worktree to work in.
- done: close it. note is required: what changed and where it landed.
- block: park it. note is required: what is in the way.
- reopen: back to todo.
- note: append a dated paragraph to the body without changing status.

Notes are tracked files: mutations name the file, commit it with the work.
Body text is markdown without ` + "`## `" + ` headings (they cut the note); use bold
labels for structure.`

// Tool binds a registry into the model-facing tool.
func Tool(reg *tasks.Registry) tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "task",
			Description: description,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"action": llm.Object{
						"type": "string",
						"enum": []string{
							"current",
							"list",
							"get",
							"create",
							"update",
							"start",
							"done",
							"block",
							"reopen",
							"note",
						},
						"description": "current (default): what to work on. list, get: read. create, update: edit. " +
							"start, done, block, reopen: move. note: record progress.",
					},
					"id": llm.Object{
						"type":        "string",
						"description": "Task id (every action but current, list and create; create derives one from the title when omitted).",
					},
					"title": llm.Object{"type": "string", "description": "One-line title (create, update)."},
					"body": llm.Object{
						"type":        "string",
						"description": "Markdown body: what and why (create, update). No `## ` headings.",
					},
					"type": llm.Object{
						"type":        "string",
						"description": "Task type, also the branch prefix: feature, bug, chore, epic, … (create, update; list filter).",
					},
					"status": llm.Object{
						"type":        "string",
						"enum":        []string{"todo", "in_progress", "blocked", "done"},
						"description": "Initial status (create) or filter (list).",
					},
					"priority": llm.Object{
						"type":        "string",
						"enum":        tasks.Priorities,
						"description": "Priority (create, update).",
					},
					"model_level": llm.Object{
						"type":        "string",
						"enum":        tasks.Levels,
						"description": "How capable a model the task needs (create, update).",
					},
					"parent": llm.Object{
						"type":        "string",
						"description": "Parent task id, usually an epic (create, update; list filter).",
					},
					"tag": llm.Object{"type": "string", "description": "Keep only tasks carrying this tag (list)."},
					"tags": llm.Object{
						"type":        "array",
						"items":       llm.Object{"type": "string"},
						"description": "Tags (create, update). The tag goal marks a container, like type epic.",
					},
					"acceptance_criteria": llm.Object{
						"type":        "array",
						"items":       llm.Object{"type": "string"},
						"description": "What must be true when the task is done (create, update).",
					},
					"verification_plan": llm.Object{
						"type":        "array",
						"items":       llm.Object{"type": "string"},
						"description": "How to check it, in order (create, update).",
					},
					"note": llm.Object{
						"type": "string",
						"description": "Text recorded on the task under a dated label: required for done and block, " +
							"optional for start and reopen, the whole point of note.",
					},
				},
			},
		},
		DetailFromArgs: detail,
		Run:            run(reg),
	}
}

type input struct {
	Action             string   `json:"action"`
	ID                 string   `json:"id"`
	Title              string   `json:"title"`
	Body               string   `json:"body"`
	Type               string   `json:"type"`
	Status             string   `json:"status"`
	Priority           string   `json:"priority"`
	ModelLevel         string   `json:"model_level"`
	Parent             string   `json:"parent"`
	Tag                string   `json:"tag"`
	Tags               []string `json:"tags"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	VerificationPlan   []string `json:"verification_plan"`
	Note               string   `json:"note"`
	// PlanStep is injected by the plan gate and consumed before this tool runs;
	// it is accepted here so strict decoding never rejects a gate-valid call.
	PlanStep tooldef.PlanStep `json:"plan_step"`
}

// readActions is the set the permission gate lets through as reads; the
// extractor in internal/permission mirrors it.
var readActions = map[string]bool{"current": true, "list": true, "get": true}

func run(reg *tasks.Registry) tooldef.Handler {
	return func(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
		in, err := parse(raw)
		if err != nil {
			return tooldef.Result{}, err
		}
		switch in.Action {
		case "list":
			return list(reg, in)
		case "get":
			return get(ctx, reg, in.ID)
		case "create":
			return create(ctx, reg, in)
		case "update":
			return update(ctx, reg, in)
		case "start":
			return start(ctx, reg, in)
		case "done":
			return move(ctx, reg, in, tasks.StatusDone)
		case "block":
			return move(ctx, reg, in, tasks.StatusBlocked)
		case "reopen":
			return move(ctx, reg, in, tasks.StatusTodo)
		case "note":
			return note(ctx, reg, in)
		default:
			return current(reg)
		}
	}
}

func parse(raw json.RawMessage) (input, error) {
	in := input{Action: "current"}
	if err := tooldef.DecodeStrict(raw, &in); err != nil {
		return input{}, fmt.Errorf("task: invalid arguments: %w", err)
	}
	in.Action = strings.ToLower(strings.TrimSpace(in.Action))
	if in.Action == "" {
		in.Action = "current"
	}
	switch in.Action {
	case "current", "list", "get", "create", "update", "start", "done", "block", "reopen", "note":
	default:
		return input{}, fmt.Errorf(
			"task: unknown action %q (use current, list, get, create, update, start, done, block, reopen or note)",
			in.Action,
		)
	}
	in.ID = strings.TrimSpace(in.ID)
	if in.ID == "" && !readActions[in.Action] && in.Action != "create" {
		return input{}, fmt.Errorf("task: %s needs an id; call action=current to see the open tasks", in.Action)
	}
	if in.Action == "get" && in.ID == "" {
		return input{}, errors.New("task: get needs an id; call action=current or action=list to see them")
	}
	if (in.Action == "done" || in.Action == "block") && strings.TrimSpace(in.Note) == "" {
		return input{}, fmt.Errorf("task: %s needs a note: %s", in.Action, noteHint(in.Action))
	}
	if in.Action == "note" && strings.TrimSpace(in.Note) == "" {
		return input{}, errors.New("task: note needs text in note")
	}
	return in, nil
}

func noteHint(action string) string {
	if action == "block" {
		return "say what is in the way and who or what clears it"
	}
	return "say what changed and where it landed (commit, merge, tag)"
}

func current(reg *tasks.Registry) (tooldef.Result, error) {
	cur, err := reg.Current()
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("task: %w", err)
	}
	var sb strings.Builder
	switch {
	case len(cur.Ready) == 0 && len(cur.Blocked) == 0:
		sb.WriteString("No open tasks.")
	case len(cur.Ready) == 0:
		sb.WriteString("Nothing is ready to work on.")
	default:
		fmt.Fprintf(&sb, "Ready (%d), best first:\n", len(cur.Ready))
		for i, task := range cur.Ready {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, row(task))
		}
	}
	if len(cur.Blocked) > 0 {
		fmt.Fprintf(&sb, "\nBlocked (%d):\n", len(cur.Blocked))
		for _, task := range cur.Blocked {
			fmt.Fprintf(&sb, "- %s\n", row(task))
		}
	}
	writeDiagnostics(&sb, cur.Diagnostics)
	switch {
	case len(cur.Ready) == 0 && len(cur.Blocked) == 0:
		sb.WriteString(
			"\nNext: task create (title, body, acceptance_criteria) to open one; task list status=done for what was finished.",
		)
	case len(cur.Ready) == 0:
		sb.WriteString("\nNext: task reopen <id> when a blocker clears, or task create to open new work.")
	default:
		sb.WriteString("\nNext: task get <id> for the full note, then task start <id> to take it.")
	}
	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("current (%d ready)", len(cur.Ready))}, nil
}

func list(reg *tasks.Registry, in input) (tooldef.Result, error) {
	all, diags, err := reg.List()
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("task: %w", err)
	}
	var (
		filters []string
		kept    []tasks.Task
		counts  = map[tasks.Status]int{}
	)
	status := tasks.Status(strings.ToLower(strings.TrimSpace(in.Status)))
	typ := strings.ToLower(strings.TrimSpace(in.Type))
	tag := strings.ToLower(strings.TrimSpace(in.Tag))
	parent := tasks.NormalizeID(in.Parent)
	if status != "" {
		filters = append(filters, "status="+string(status))
	}
	if typ != "" {
		filters = append(filters, "type="+typ)
	}
	if tag != "" {
		filters = append(filters, "tag="+tag)
	}
	if parent != "" {
		filters = append(filters, "parent="+parent)
	}
	for _, task := range all {
		counts[task.Status]++
		if (status != "" && task.Status != status) || (typ != "" && task.Type != typ) ||
			(tag != "" && !slices.Contains(task.Tags, tag)) || (parent != "" && task.ParentID != parent) {
			continue
		}
		kept = append(kept, task)
	}

	var sb strings.Builder
	switch {
	case len(all) == 0:
		sb.WriteString("The registry is empty.")
		writeDiagnostics(&sb, diags)
		sb.WriteString("\nNext: task create (title, body, acceptance_criteria) to open the first task.")
		return tooldef.Result{Content: sb.String(), Detail: "list (0)"}, nil
	case len(kept) == 0:
		fmt.Fprintf(
			&sb,
			"No task matches %s; %d in the registry (%s).",
			strings.Join(filters, " "),
			len(all),
			summarize(counts),
		)
		writeDiagnostics(&sb, diags)
		sb.WriteString("\nNext: task list without the filter, or task current for what is open.")
		return tooldef.Result{Content: sb.String(), Detail: "list (0)"}, nil
	case len(filters) > 0:
		fmt.Fprintf(&sb, "%d of %d tasks (%s):\n", len(kept), len(all), strings.Join(filters, " "))
	default:
		fmt.Fprintf(&sb, "%d tasks (%s):\n", len(all), summarize(counts))
	}
	for _, task := range kept {
		fmt.Fprintf(&sb, "- %s\n", row(task))
	}
	writeDiagnostics(&sb, diags)
	sb.WriteString("\nNext: task get <id> for the full note; task current ranks the open ones.")
	return tooldef.Result{Content: sb.String(), Detail: fmt.Sprintf("list (%d)", len(kept))}, nil
}

func get(ctx context.Context, reg *tasks.Registry, id string) (tooldef.Result, error) {
	task, err := reg.Get(id)
	if err != nil {
		return tooldef.Result{}, lookupError(err, id)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "%s — %s\n", task.ID, task.Title)
	fmt.Fprintf(&sb, "status: %s · priority: %s · model_level: %s · type: %s\n",
		task.Status, orDash(task.Priority), orDash(task.ModelLevel), orDash(task.Type))
	if task.ParentID != "" || len(task.Tags) > 0 {
		fmt.Fprintf(&sb, "parent: %s · tags: %s\n", orDash(task.ParentID), orDash(strings.Join(task.Tags, ", ")))
	}
	if task.Branch != "" || task.WorktreePath != "" {
		fmt.Fprintf(&sb, "branch: %s · worktree: %s\n", orDash(task.Branch), worktreeState(reg, task))
	}
	fmt.Fprintf(&sb, "file: %s", displayPath(ctx, task.Path))
	if !task.UpdatedAt.IsZero() {
		fmt.Fprintf(&sb, " · updated %s", task.UpdatedAt.Local().Format("2006-01-02"))
	}
	sb.WriteString("\n")
	if task.Body != "" {
		sb.WriteString("\n" + task.Body + "\n")
	}
	if len(task.AcceptanceCriteria) > 0 {
		sb.WriteString("\nAcceptance criteria:\n")
		for _, c := range task.AcceptanceCriteria {
			sb.WriteString("- " + c + "\n")
		}
	}
	if len(task.VerificationPlan) > 0 {
		sb.WriteString("\nVerification plan:\n")
		for i, v := range task.VerificationPlan {
			fmt.Fprintf(&sb, "%d. %s\n", i+1, v)
		}
	}
	sb.WriteString("\nNext: " + nextFor(task))
	return tooldef.Result{Content: sb.String(), Detail: "get " + task.ID}, nil
}

func create(ctx context.Context, reg *tasks.Registry, in input) (tooldef.Result, error) {
	if strings.TrimSpace(in.Title) == "" {
		return tooldef.Result{}, errors.New("task: create needs a title (and an id, unless the title reduces to one)")
	}
	task, err := reg.Create(tasks.Draft{
		ID:                 in.ID,
		Title:              in.Title,
		Body:               in.Body,
		Type:               in.Type,
		Status:             tasks.Status(strings.ToLower(strings.TrimSpace(in.Status))),
		Priority:           in.Priority,
		ModelLevel:         in.ModelLevel,
		ParentID:           in.Parent,
		Tags:               in.Tags,
		AcceptanceCriteria: in.AcceptanceCriteria,
		VerificationPlan:   in.VerificationPlan,
	})
	if err != nil {
		return tooldef.Result{}, wrap(err)
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Created %s (%s", task.ID, task.Status)
	if task.Priority != "" {
		sb.WriteString(", " + task.Priority)
	}
	fmt.Fprintf(&sb, ") — %s\nfile: %s (new, commit it)\n", task.Title, displayPath(ctx, task.Path))
	sb.WriteString("\nNext: " + nextFor(task))
	return tooldef.Result{Content: sb.String(), Detail: "create " + task.ID}, nil
}

func update(ctx context.Context, reg *tasks.Registry, in input) (tooldef.Result, error) {
	var (
		patch   tasks.Patch
		changed []string
	)
	field := func(name, value string, dst **string) {
		if strings.TrimSpace(value) != "" {
			v := value
			*dst = &v
			changed = append(changed, name)
		}
	}
	field("title", in.Title, &patch.Title)
	field("body", in.Body, &patch.Body)
	field("type", in.Type, &patch.Type)
	field("priority", in.Priority, &patch.Priority)
	field("model_level", in.ModelLevel, &patch.ModelLevel)
	field("parent", in.Parent, &patch.ParentID)
	if in.Tags != nil {
		patch.Tags = in.Tags
		changed = append(changed, "tags")
	}
	if in.AcceptanceCriteria != nil {
		patch.AcceptanceCriteria = in.AcceptanceCriteria
		changed = append(changed, "acceptance_criteria")
	}
	if in.VerificationPlan != nil {
		patch.VerificationPlan = in.VerificationPlan
		changed = append(changed, "verification_plan")
	}
	if len(changed) == 0 {
		return tooldef.Result{}, errors.New("task: update changes nothing; pass the fields to set " +
			"(title, body, type, priority, model_level, parent, tags, acceptance_criteria, verification_plan); " +
			"status moves with start, done, block and reopen")
	}
	task, err := reg.Update(in.ID, patch)
	if err != nil {
		return tooldef.Result{}, lookupError(err, in.ID)
	}
	content := fmt.Sprintf("Updated %s: %s.\nfile: %s (commit it)\n\nNext: %s",
		task.ID, strings.Join(changed, ", "), displayPath(ctx, task.Path), nextFor(task))
	return tooldef.Result{Content: content, Detail: "update " + task.ID}, nil
}

func start(ctx context.Context, reg *tasks.Registry, in input) (tooldef.Result, error) {
	task, err := reg.Get(in.ID)
	if err != nil {
		return tooldef.Result{}, lookupError(err, in.ID)
	}
	if task.IsEpic() {
		return tooldef.Result{}, fmt.Errorf("task: %s is a container (epic or goal), not work; "+
			"start one of its children — task list parent=%s — or create one under it", task.ID, task.ID)
	}
	if task.Status == tasks.StatusDone {
		return tooldef.Result{}, fmt.Errorf(
			"task: %s is done; task reopen %s first if it must be redone",
			task.ID,
			task.ID,
		)
	}
	already := task.Status == tasks.StatusInProgress
	task, err = reg.SetStatus(task.ID, tasks.StatusInProgress, in.Note)
	if err != nil {
		return tooldef.Result{}, wrap(err)
	}
	var sb strings.Builder
	if already {
		fmt.Fprintf(&sb, "%s was already in progress.\n", task.ID)
	} else {
		fmt.Fprintf(&sb, "Started %s (in_progress) — %s\n", task.ID, task.Title)
	}
	fmt.Fprintf(&sb, "branch: %s · worktree: %s\n", task.Branch, worktreeState(reg, task))
	fmt.Fprintf(&sb, "file: %s (commit it with the work)\n", displayPath(ctx, task.Path))
	sb.WriteString("\nNext: " + nextFor(task))
	return tooldef.Result{Content: sb.String(), Detail: "start " + task.ID}, nil
}

func move(ctx context.Context, reg *tasks.Registry, in input, status tasks.Status) (tooldef.Result, error) {
	task, err := reg.SetStatus(in.ID, status, in.Note)
	if err != nil {
		return tooldef.Result{}, lookupError(err, in.ID)
	}
	var verb string
	switch status {
	case tasks.StatusDone:
		verb = "Done"
	case tasks.StatusBlocked:
		verb = "Blocked"
	default:
		verb = "Reopened"
	}
	content := fmt.Sprintf("%s %s (%s) — %s\nfile: %s (commit it)\n\nNext: %s",
		verb, task.ID, task.Status, task.Title, displayPath(ctx, task.Path), nextFor(task))
	return tooldef.Result{Content: content, Detail: in.Action + " " + task.ID}, nil
}

func note(ctx context.Context, reg *tasks.Registry, in input) (tooldef.Result, error) {
	task, err := reg.Note(in.ID, in.Note)
	if err != nil {
		return tooldef.Result{}, lookupError(err, in.ID)
	}
	content := fmt.Sprintf("Noted on %s (%s).\nfile: %s (commit it)\n\nNext: %s",
		task.ID, task.Status, displayPath(ctx, task.Path), nextFor(task))
	return tooldef.Result{Content: content, Detail: "note " + task.ID}, nil
}

// nextFor is the one line every answer ends with: the natural next move for
// a task in this state.
func nextFor(task tasks.Task) string {
	if task.IsEpic() {
		return fmt.Sprintf(
			"this is a container; task list parent=%s for its children, task create parent=%s to add one.",
			task.ID,
			task.ID,
		)
	}
	switch task.Status {
	case tasks.StatusTodo:
		return fmt.Sprintf("task start %s when you take it.", task.ID)
	case tasks.StatusInProgress:
		where := ""
		if task.WorktreePath != "" {
			where = fmt.Sprintf(" in %s on %s", task.WorktreePath, task.Branch)
		}
		return fmt.Sprintf("do the work%s; task note %s to record progress; task done %s (note: what landed) "+
			"or task block %s (note: what is in the way).", where, task.ID, task.ID, task.ID)
	case tasks.StatusBlocked:
		return fmt.Sprintf(
			"task reopen %s once the blocker clears, then start it; task current for other work.",
			task.ID,
		)
	default:
		return fmt.Sprintf(
			"nothing, it is closed; task current for what is next, task reopen %s only if it must be redone.",
			task.ID,
		)
	}
}

func worktreeState(reg *tasks.Registry, task tasks.Task) string {
	if task.WorktreePath == "" {
		return "-"
	}
	if _, err := os.Stat(filepath.Join(reg.Root(), filepath.FromSlash(task.WorktreePath))); err == nil {
		return task.WorktreePath + " (exists)"
	}
	return fmt.Sprintf("%s (not created yet: git worktree add %s -b %s, from %s)",
		task.WorktreePath, task.WorktreePath, task.Branch, reg.Root())
}

func row(task tasks.Task) string {
	parts := []string{task.ID, string(task.Status)}
	if task.Priority != "" {
		parts = append(parts, task.Priority)
	}
	if task.Type != "" {
		parts = append(parts, task.Type)
	}
	return strings.Join(parts, " · ") + " — " + task.Title
}

func summarize(counts map[tasks.Status]int) string {
	var parts []string
	for _, status := range tasks.Statuses {
		if n := counts[status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, status))
		}
	}
	return strings.Join(parts, ", ")
}

func writeDiagnostics(sb *strings.Builder, diags []tasks.Diagnostic) {
	if len(diags) == 0 {
		return
	}
	fmt.Fprintf(sb, "\n%d note(s) could not be read and are not listed:\n", len(diags))
	for _, d := range diags {
		fmt.Fprintf(sb, "- %s: %v\n", d.File, d.Err)
	}
}

// displayPath follows the harness rule: cwd-relative inside the session's
// directory, absolute outside it — which a registry read from a worktree
// always is.
func displayPath(ctx context.Context, path string) string {
	return tooldef.RelToCwd(ctx, path)
}

func lookupError(err error, id string) error {
	if errors.Is(err, tasks.ErrNotFound) {
		return fmt.Errorf("task: no task %q; call action=current or action=list to see the ids", tasks.NormalizeID(id))
	}
	return wrap(err)
}

func wrap(err error) error {
	return errors.New(strings.Replace(err.Error(), "tasks: ", "task: ", 1))
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// detail is the row the TUI shows before the call runs.
func detail(raw json.RawMessage) string {
	in, err := parse(raw)
	if err != nil {
		return ""
	}
	switch in.Action {
	case "current":
		return "current"
	case "list":
		var filters []string
		for _, f := range [][2]string{{"status", in.Status}, {"type", in.Type}, {"tag", in.Tag}, {"parent", in.Parent}} {
			if strings.TrimSpace(f[1]) != "" {
				filters = append(filters, f[0]+"="+strings.TrimSpace(f[1]))
			}
		}
		return strings.TrimSpace("list " + strings.Join(filters, " "))
	case "create":
		if in.ID != "" {
			return "create " + tasks.NormalizeID(in.ID)
		}
		return "create " + tasks.NormalizeID(in.Title)
	default:
		return in.Action + " " + tasks.NormalizeID(in.ID)
	}
}
