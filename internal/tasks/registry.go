// Package tasks reads and writes the mcp-ai-helper task registry: one
// markdown note per task, YAML frontmatter plus Body, Acceptance Criteria and
// Verification Plan sections, kept under the main checkout of a repository.
//
// The registry is the helper's, and stays the helper's: this package speaks
// its file format so a session can pick a task, start it, and close it
// without a server in between, while the helper keeps reading the same
// notes. Discovery follows the helper's config (`.mcp-ai-helper.yaml`), and a
// note the helper would refuse is skipped with a diagnostic, never rewritten.
package tasks

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// ConfigFile is the helper's repository config, read for the registry path.
	ConfigFile = ".mcp-ai-helper.yaml"
	// DefaultDir is where the registry lives when the config does not say.
	DefaultDir = "obsidian-tasks"
	// WorktreeDir is the directory task worktrees are created under, relative
	// to the main checkout.
	WorktreeDir = ".worktrees"
)

// Status is a task's place in its life: todo, in_progress, blocked or done.
type Status string

// The four statuses the helper knows.
const (
	StatusTodo       Status = "todo"
	StatusInProgress Status = "in_progress"
	StatusBlocked    Status = "blocked"
	StatusDone       Status = "done"
)

// Statuses lists the valid statuses in workflow order.
var Statuses = []Status{StatusTodo, StatusInProgress, StatusBlocked, StatusDone}

// Priorities lists the valid priorities, most urgent first.
var Priorities = []string{"critical", "high", "medium", "low"}

// Levels lists the valid model levels, most capable first.
var Levels = []string{"very_high", "high", "medium", "low"}

// Errors callers tell apart.
var (
	ErrNotFound = errors.New("task not found")
	ErrExists   = errors.New("task already exists")
)

// Task is one registry note in memory.
type Task struct {
	ID         string
	Title      string
	Status     Status
	Priority   string // one of Priorities, or empty
	ModelLevel string // one of Levels, or empty
	Type       string // task_type: feature, bug, chore, epic, …
	ParentID   string
	Tags       []string
	// Branch and WorktreePath are the helper's conventions, filled in when a
	// task starts: `<type>/<id>` and `.worktrees/<id>`.
	Branch             string
	WorktreePath       string
	AcceptanceCriteria []string
	VerificationPlan   []string
	Body               string
	CreatedAt          time.Time
	UpdatedAt          time.Time
	// Path is the note file, absolute. Empty on a task not yet written.
	Path string
}

// IsEpic reports whether the task is a container: an epic, or a goal. Neither
// is worked directly; their children are.
func (t Task) IsEpic() bool {
	return t.Type == "epic" || slices.Contains(t.Tags, "goal")
}

// Executable reports whether a session can take the task: open, and not a
// container.
func (t Task) Executable() bool {
	return (t.Status == StatusTodo || t.Status == StatusInProgress) && !t.IsEpic()
}

// Diagnostic names a note the registry could not read. The file stays as it
// is; the helper reports the same problem the same way.
type Diagnostic struct {
	File string
	Err  error
}

// Registry is one task directory.
type Registry struct {
	root string
	dir  string
	now  func() time.Time
}

// Discover finds the registry of the repository rooted at root: the helper's
// config names it, or the default directory exists. A repository with neither
// has no registry, and Discover returns nil, nil; the tool that would work it
// is then simply not offered. A config that names another backend is not a
// registry this package can read, and yields nil too.
func Discover(root string) (*Registry, error) {
	root = filepath.Clean(root)
	data, err := os.ReadFile(filepath.Join(root, ConfigFile))
	if errors.Is(err, fs.ErrNotExist) {
		dir := filepath.Join(root, DefaultDir)
		if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
			return Open(root, dir), nil
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("tasks: %s: %w", ConfigFile, err)
	}
	var cfg struct {
		TaskRegistry struct {
			Backend  string `yaml:"backend"`
			Obsidian struct {
				Path string `yaml:"path"`
			} `yaml:"obsidian"`
		} `yaml:"task_registry"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("tasks: %s: %w", ConfigFile, err)
	}
	if backend := strings.ToLower(strings.TrimSpace(cfg.TaskRegistry.Backend)); backend != "" && backend != "obsidian" {
		return nil, nil
	}
	dir := strings.TrimSpace(cfg.TaskRegistry.Obsidian.Path)
	if dir == "" {
		dir = DefaultDir
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, dir)
	}
	dir = filepath.Clean(dir)
	if rel, relErr := filepath.Rel(root, dir); relErr != nil || rel == ".." || strings.HasPrefix(rel, "../") {
		return nil, fmt.Errorf(
			"tasks: %s: registry path %q escapes the repository",
			ConfigFile,
			cfg.TaskRegistry.Obsidian.Path,
		)
	}
	return Open(root, dir), nil
}

// Open binds a registry to an explicit directory under the repository root.
// The directory may not exist yet: reads report an empty registry, and the
// first write creates it.
func Open(root, dir string) *Registry {
	return &Registry{root: filepath.Clean(root), dir: filepath.Clean(dir), now: time.Now}
}

// stamp is the current time as a note stores it: UTC, no monotonic reading,
// so a task compares equal before and after a round trip through disk.
func (r *Registry) stamp() time.Time {
	return r.now().UTC().Round(0)
}

// Root is the repository the registry belongs to: the main checkout, which
// is where the notes are tracked and where task worktrees are made.
func (r *Registry) Root() string { return r.root }

// Dir is the note directory, absolute.
func (r *Registry) Dir() string { return r.dir }

// Path is the note file a task id maps to, whether or not it exists.
func (r *Registry) Path(id string) string {
	return filepath.Join(r.dir, NormalizeID(id)+".md")
}

// List reads every note. Unreadable notes come back as diagnostics, so one
// bad file never hides the rest.
func (r *Registry) List() ([]Task, []Diagnostic, error) {
	entries, err := os.ReadDir(r.dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("tasks: %w", err)
	}
	var (
		list  []Task
		diags []Diagnostic
	)
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".md") || strings.HasPrefix(name, ".") {
			continue
		}
		task, readErr := readTask(filepath.Join(r.dir, name))
		if readErr != nil {
			diags = append(diags, Diagnostic{File: name, Err: readErr})
			continue
		}
		list = append(list, task)
	}
	slices.SortFunc(list, func(a, b Task) int { return strings.Compare(a.ID, b.ID) })
	return list, diags, nil
}

// Get reads one task by id.
func (r *Registry) Get(id string) (Task, error) {
	id = NormalizeID(id)
	if id == "" {
		return Task{}, errors.New("tasks: id is required")
	}
	task, err := readTask(r.Path(id))
	if errors.Is(err, fs.ErrNotExist) {
		return Task{}, fmt.Errorf("tasks: %w: %s", ErrNotFound, id)
	}
	if err != nil {
		return Task{}, fmt.Errorf("tasks: %s: %w", id, err)
	}
	return task, nil
}

func readTask(path string) (Task, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Task{}, err
	}
	task, err := parseNote(data, strings.TrimSuffix(filepath.Base(path), ".md"))
	if err != nil {
		return Task{}, err
	}
	task.Path = path
	return task, nil
}

// Current is what a session should look at before choosing work.
type Current struct {
	// Ready holds the executable tasks, best first: in_progress before todo,
	// then by priority, then most recently touched.
	Ready []Task
	// Blocked holds open tasks that wait on something, most urgent first.
	Blocked []Task
	// Diagnostics names notes that could not be read.
	Diagnostics []Diagnostic
}

// Current ranks the open tasks the way the helper's `task current` does.
func (r *Registry) Current() (Current, error) {
	list, diags, err := r.List()
	if err != nil {
		return Current{}, err
	}
	out := Current{Diagnostics: diags}
	for _, task := range list {
		switch {
		case task.Executable():
			out.Ready = append(out.Ready, task)
		case task.Status == StatusBlocked:
			out.Blocked = append(out.Blocked, task)
		}
	}
	slices.SortStableFunc(out.Ready, rank)
	slices.SortStableFunc(out.Blocked, rank)
	return out, nil
}

func rank(a, b Task) int {
	return cmp.Or(
		cmp.Compare(statusRank(a.Status), statusRank(b.Status)),
		cmp.Compare(priorityRank(a.Priority), priorityRank(b.Priority)),
		b.UpdatedAt.Compare(a.UpdatedAt),
		strings.Compare(a.ID, b.ID),
	)
}

func statusRank(s Status) int {
	if s == StatusInProgress {
		return 0
	}
	return 1
}

func priorityRank(p string) int {
	if i := slices.Index(Priorities, p); i >= 0 {
		return i
	}
	return len(Priorities)
}

// Draft is what a new task is made from. ID is derived from Title when
// empty; Type defaults to "task" and Status to todo.
type Draft struct {
	ID                 string
	Title              string
	Body               string
	Type               string
	Status             Status
	Priority           string
	ModelLevel         string
	ParentID           string
	Tags               []string
	AcceptanceCriteria []string
	VerificationPlan   []string
}

// Create writes a new note. It refuses to overwrite an existing task and to
// point at a parent that is not in the registry.
func (r *Registry) Create(d Draft) (Task, error) {
	id := NormalizeID(d.ID)
	if id == "" {
		id = NormalizeID(d.Title)
	}
	if id == "" {
		return Task{}, errors.New("tasks: an id is required (the title does not reduce to one)")
	}
	now := r.stamp()
	task := Task{
		ID:                 id,
		Title:              strings.TrimSpace(d.Title),
		Status:             d.Status,
		Priority:           d.Priority,
		ModelLevel:         d.ModelLevel,
		Type:               strings.ToLower(strings.TrimSpace(d.Type)),
		ParentID:           d.ParentID,
		Tags:               d.Tags,
		AcceptanceCriteria: cleanList(d.AcceptanceCriteria),
		VerificationPlan:   cleanList(d.VerificationPlan),
		Body:               strings.TrimSpace(d.Body),
		CreatedAt:          now,
		UpdatedAt:          now,
		Path:               r.Path(id),
	}
	if task.Status == "" {
		task.Status = StatusTodo
	}
	if task.Type == "" {
		task.Type = "task"
	}
	if err := r.validate(&task); err != nil {
		return Task{}, err
	}
	if _, err := os.Stat(task.Path); err == nil {
		return Task{}, fmt.Errorf("tasks: %w: %s", ErrExists, id)
	}
	if err := r.write(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

// Patch carries the fields Update changes. A nil pointer keeps the field; a
// nil slice keeps the list, an empty one clears it.
type Patch struct {
	Title              *string
	Body               *string
	Type               *string
	Priority           *string
	ModelLevel         *string
	ParentID           *string
	Tags               []string
	AcceptanceCriteria []string
	VerificationPlan   []string
}

// Update applies a patch to an existing task and rewrites its note.
func (r *Registry) Update(id string, p Patch) (Task, error) {
	task, err := r.Get(id)
	if err != nil {
		return Task{}, err
	}
	set := func(dst, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	set(&task.Title, p.Title)
	set(&task.Body, p.Body)
	set(&task.Type, p.Type)
	set(&task.Priority, p.Priority)
	set(&task.ModelLevel, p.ModelLevel)
	set(&task.ParentID, p.ParentID)
	if p.Tags != nil {
		task.Tags = p.Tags
	}
	if p.AcceptanceCriteria != nil {
		task.AcceptanceCriteria = cleanList(p.AcceptanceCriteria)
	}
	if p.VerificationPlan != nil {
		task.VerificationPlan = cleanList(p.VerificationPlan)
	}
	task.Type = strings.ToLower(task.Type)
	task.UpdatedAt = r.stamp()
	if err := r.validate(&task); err != nil {
		return Task{}, err
	}
	if err := r.write(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

// SetStatus moves a task and records why: text, when given, is appended to
// the body as a dated paragraph under a bold label — `**Done (2026-09-03).**
// merged as 1ab2c3d` — which is how a note carries its own history without
// the headings the format cannot hold. Starting a task fills in its branch
// and worktree when the note has none yet.
func (r *Registry) SetStatus(id string, status Status, text string) (Task, error) {
	if !validStatus(status) {
		return Task{}, fmt.Errorf("tasks: invalid status %q", status)
	}
	task, err := r.Get(id)
	if err != nil {
		return Task{}, err
	}
	task.Status = status
	if status == StatusInProgress {
		if task.Branch == "" {
			task.Branch = BranchFor(task)
		}
		if task.WorktreePath == "" {
			task.WorktreePath = WorktreeFor(task.ID)
		}
	}
	return r.commit(task, statusLabel(status), text)
}

// Note appends a dated paragraph to the body without touching the status.
func (r *Registry) Note(id, text string) (Task, error) {
	task, err := r.Get(id)
	if err != nil {
		return Task{}, err
	}
	if strings.TrimSpace(text) == "" {
		return Task{}, errors.New("tasks: a note needs text")
	}
	return r.commit(task, "Note", text)
}

func (r *Registry) commit(task Task, label, text string) (Task, error) {
	now := r.now()
	if text = strings.TrimSpace(text); text != "" {
		if err := checkBody(text); err != nil {
			return Task{}, fmt.Errorf("tasks: %w", err)
		}
		// The label carries the local date: it is read by people, on the day
		// they remember doing the thing. The stamp below stays UTC.
		paragraph := fmt.Sprintf("**%s (%s).** %s", label, now.Format("2006-01-02"), text)
		if task.Body == "" {
			task.Body = paragraph
		} else {
			task.Body = task.Body + "\n\n" + paragraph
		}
	}
	task.UpdatedAt = now.UTC().Round(0)
	if err := r.write(task); err != nil {
		return Task{}, err
	}
	return task, nil
}

func statusLabel(status Status) string {
	switch status {
	case StatusInProgress:
		return "Started"
	case StatusDone:
		return "Done"
	case StatusBlocked:
		return "Blocked"
	default:
		return "Reopened"
	}
}

func (r *Registry) validate(task *Task) error {
	if task.Title == "" {
		return errors.New("tasks: a title is required")
	}
	if !validStatus(task.Status) {
		return fmt.Errorf("tasks: invalid status %q (use %s)", task.Status, join(Statuses))
	}
	task.Priority = normalizeEnum(task.Priority)
	if task.Priority != "" && !validPriority(task.Priority) {
		return fmt.Errorf("tasks: invalid priority %q (use %s)", task.Priority, strings.Join(Priorities, ", "))
	}
	task.ModelLevel = normalizeEnum(task.ModelLevel)
	if task.ModelLevel != "" && !validLevel(task.ModelLevel) {
		return fmt.Errorf("tasks: invalid model_level %q (use %s)", task.ModelLevel, strings.Join(Levels, ", "))
	}
	if task.Type != "" && !typePattern.MatchString(task.Type) {
		return fmt.Errorf("tasks: invalid type %q (lowercase letters, digits and dashes)", task.Type)
	}
	task.Tags = normalizeTags(task.Tags)
	if err := checkBody(task.Body); err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	if task.ParentID != "" {
		parent := NormalizeID(task.ParentID)
		if parent == task.ID {
			return errors.New("tasks: a task cannot be its own parent")
		}
		if _, err := os.Stat(r.Path(parent)); err != nil {
			return fmt.Errorf("tasks: parent %q is not in the registry", task.ParentID)
		}
		task.ParentID = parent
	}
	return nil
}

// write lands the note through a rename so a reader never sees half a file.
func (r *Registry) write(task Task) error {
	data, err := renderNote(task)
	if err != nil {
		return fmt.Errorf("tasks: %s: %w", task.ID, err)
	}
	if err := os.MkdirAll(r.dir, 0o755); err != nil {
		return fmt.Errorf("tasks: %w", err)
	}
	tmp := task.Path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil { //nolint:gosec // notes are tracked, world-readable files
		return fmt.Errorf("tasks: write %s: %w", task.ID, err)
	}
	if err := os.Rename(tmp, task.Path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("tasks: write %s: %w", task.ID, err)
	}
	return nil
}

var (
	idJunk      = regexp.MustCompile(`[^A-Za-z0-9._-]+`)
	typePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// NormalizeID is the helper's id rule: lowercase, runs of anything but
// letters, digits, dot, underscore and dash become one dash, and the ends are
// trimmed of dots and dashes. The result is a safe file name or empty.
func NormalizeID(id string) string {
	id = strings.ToLower(strings.TrimSpace(id))
	id = idJunk.ReplaceAllString(id, "-")
	return strings.Trim(id, ".-")
}

// BranchFor is the helper's branch convention: `<type>/<id>`.
func BranchFor(t Task) string {
	typ := t.Type
	if typ == "" {
		typ = "task"
	}
	return typ + "/" + t.ID
}

// WorktreeFor is the helper's worktree convention, relative to the main
// checkout: `.worktrees/<id>`.
func WorktreeFor(id string) string {
	return WorktreeDir + "/" + NormalizeID(id)
}

func validStatus(s Status) bool   { return slices.Contains(Statuses, s) }
func validPriority(p string) bool { return slices.Contains(Priorities, p) }
func validLevel(l string) bool    { return slices.Contains(Levels, l) }

func cleanList(items []string) []string {
	var out []string
	for _, item := range items {
		if item = strings.TrimSpace(item); item != "" {
			out = append(out, item)
		}
	}
	return out
}

func join(statuses []Status) string {
	parts := make([]string, len(statuses))
	for i, s := range statuses {
		parts[i] = string(s)
	}
	return strings.Join(parts, ", ")
}
