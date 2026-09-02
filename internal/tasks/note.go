package tasks

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ErrHeading rejects a body that would be cut short on disk: the helper's
// parser treats every `## ` line as the start of a new section, so text after
// one silently disappears. Bold labels carry structure inside a body instead.
var ErrHeading = errors.New("body must not contain a `## ` heading (use a bold label instead)")

// note is the on-disk shape of a task: the frontmatter mcp-ai-helper marshals
// with yaml.v3, in this field order, followed by the three markdown sections.
// Both tools read and write the same files, so the shape is the helper's,
// byte for byte, and a note that changed hands does not churn its diff.
type note struct {
	ID                 string     `yaml:"id"`
	Title              string     `yaml:"title"`
	Status             string     `yaml:"status"`
	Priority           string     `yaml:"priority,omitempty"`
	ModelLevel         string     `yaml:"model_level,omitempty"`
	TaskType           string     `yaml:"task_type,omitempty"`
	ParentID           string     `yaml:"parent_id,omitempty"`
	Tags               []string   `yaml:"tags,omitempty"`
	Branch             string     `yaml:"branch,omitempty"`
	WorktreePath       string     `yaml:"worktree_path,omitempty"`
	AcceptanceCriteria stringList `yaml:"acceptance_criteria,omitempty"`
	VerificationPlan   stringList `yaml:"verification_plan,omitempty"`
	CreatedAt          string     `yaml:"created_at,omitempty"`
	UpdatedAt          string     `yaml:"updated_at,omitempty"`
}

// stringList reads the list shapes a hand-edited note ends up with: a plain
// sequence, a single scalar, or items that YAML parsed as `key: value` pairs
// because nobody quoted the colon.
type stringList []string

func (l *stringList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		out := make([]string, 0, len(node.Content))
		for _, item := range node.Content {
			value, err := listItem(item)
			if err != nil {
				return err
			}
			out = append(out, value)
		}
		*l = out
		return nil
	case yaml.ScalarNode:
		if strings.TrimSpace(node.Value) == "" {
			*l = nil
			return nil
		}
		*l = stringList{node.Value}
		return nil
	default:
		return fmt.Errorf("expected a list of strings, got YAML node kind %d", node.Kind)
	}
}

func listItem(node *yaml.Node) (string, error) {
	switch node.Kind {
	case yaml.ScalarNode:
		return node.Value, nil
	case yaml.MappingNode:
		if len(node.Content) == 2 && node.Content[0].Kind == yaml.ScalarNode &&
			node.Content[1].Kind == yaml.ScalarNode {
			return node.Content[0].Value + ": " + node.Content[1].Value, nil
		}
		return "", fmt.Errorf("expected a string list item, got a mapping with %d nodes", len(node.Content))
	default:
		return "", fmt.Errorf("expected a string list item, got YAML node kind %d", node.Kind)
	}
}

// parseNote decodes one note file. stem is the file name without `.md`; the
// frontmatter id must agree with it, or the registry would hold a task that
// no path finds.
func parseNote(data []byte, stem string) (Task, error) {
	fm, body, err := splitFrontmatter(string(data))
	if err != nil {
		return Task{}, err
	}
	var n note
	if err := yaml.Unmarshal([]byte(fm), &n); err != nil {
		repaired := quotePlainScalars(fm)
		if repaired == fm {
			return Task{}, fmt.Errorf("frontmatter: %w", err)
		}
		if retry := yaml.Unmarshal([]byte(repaired), &n); retry != nil {
			return Task{}, fmt.Errorf("frontmatter: %w", err)
		}
	}
	task, err := n.task(stem)
	if err != nil {
		return Task{}, err
	}
	mainBody, criteria, plan := splitBody(body)
	task.Body = mainBody
	if len(criteria) > 0 {
		task.AcceptanceCriteria = criteria
	}
	if len(plan) > 0 {
		task.VerificationPlan = plan
	}
	return task, nil
}

func (n note) task(stem string) (Task, error) {
	id := strings.TrimSpace(n.ID)
	title := strings.TrimSpace(n.Title)
	switch {
	case id == "":
		return Task{}, errors.New("frontmatter: id is required")
	case title == "":
		return Task{}, errors.New("frontmatter: title is required")
	case id != stem && id != NormalizeID(stem):
		return Task{}, fmt.Errorf("frontmatter id %q does not match the file name %q", id, stem)
	}
	status := Status(normalizeEnum(n.Status))
	if !validStatus(status) {
		return Task{}, fmt.Errorf("invalid status %q", n.Status)
	}
	priority := normalizeEnum(n.Priority)
	if priority != "" && !validPriority(priority) {
		return Task{}, fmt.Errorf("invalid priority %q", n.Priority)
	}
	level := normalizeEnum(n.ModelLevel)
	if level != "" && !validLevel(level) {
		return Task{}, fmt.Errorf("invalid model_level %q", n.ModelLevel)
	}
	return Task{
		ID:                 id,
		Title:              title,
		Status:             status,
		Priority:           priority,
		ModelLevel:         level,
		Type:               strings.ToLower(strings.TrimSpace(n.TaskType)),
		ParentID:           strings.TrimSpace(n.ParentID),
		Tags:               normalizeTags(n.Tags),
		Branch:             strings.TrimSpace(n.Branch),
		WorktreePath:       strings.TrimSpace(n.WorktreePath),
		AcceptanceCriteria: []string(n.AcceptanceCriteria),
		VerificationPlan:   []string(n.VerificationPlan),
		CreatedAt:          parseTime(n.CreatedAt),
		UpdatedAt:          parseTime(n.UpdatedAt),
	}, nil
}

// renderNote is the inverse of parseNote in the helper's own layout: yaml.v3
// frontmatter, then `## Body`, `## Acceptance Criteria` as bullets and
// `## Verification Plan` numbered.
func renderNote(t Task) ([]byte, error) {
	n := note{
		ID:                 t.ID,
		Title:              t.Title,
		Status:             string(t.Status),
		Priority:           t.Priority,
		ModelLevel:         t.ModelLevel,
		TaskType:           t.Type,
		ParentID:           t.ParentID,
		Tags:               t.Tags,
		Branch:             t.Branch,
		WorktreePath:       t.WorktreePath,
		AcceptanceCriteria: stringList(t.AcceptanceCriteria),
		VerificationPlan:   stringList(t.VerificationPlan),
		CreatedAt:          formatTime(t.CreatedAt),
		UpdatedAt:          formatTime(t.UpdatedAt),
	}
	fm, err := yaml.Marshal(n)
	if err != nil {
		return nil, fmt.Errorf("encode frontmatter: %w", err)
	}
	var buf bytes.Buffer
	buf.WriteString("---\n")
	buf.Write(fm)
	buf.WriteString("---\n")
	if body := strings.TrimSpace(t.Body); body != "" {
		buf.WriteString("\n## Body\n\n")
		buf.WriteString(body)
		buf.WriteString("\n")
	}
	if len(t.AcceptanceCriteria) > 0 {
		buf.WriteString("\n## Acceptance Criteria\n")
		for _, c := range t.AcceptanceCriteria {
			buf.WriteString("\n- " + c)
		}
		buf.WriteString("\n")
	}
	if len(t.VerificationPlan) > 0 {
		buf.WriteString("\n## Verification Plan\n")
		for i, v := range t.VerificationPlan {
			fmt.Fprintf(&buf, "\n%d. %s", i+1, v)
		}
		buf.WriteString("\n")
	}
	return buf.Bytes(), nil
}

func splitFrontmatter(text string) (fm, body string, err error) {
	const opening = "---\n"
	if !strings.HasPrefix(text, opening) {
		return "", "", errors.New("missing opening --- frontmatter fence")
	}
	rest := text[len(opening):]
	start := 0
	for start <= len(rest) {
		end := strings.IndexByte(rest[start:], '\n')
		if end < 0 {
			break
		}
		end += start
		line := strings.TrimSpace(rest[start:end])
		if line == "---" || line == "..." {
			return rest[:start], rest[end+1:], nil
		}
		start = end + 1
	}
	return "", "", errors.New("missing closing --- frontmatter fence")
}

// splitBody mirrors the helper's reader: the text after the frontmatter is a
// run of `## ` sections, of which Body, Acceptance Criteria and Verification
// Plan are known. Text before the first heading is the whole body, sections
// or not, which is why bodies may not carry headings of their own.
func splitBody(body string) (mainBody string, criteria, plan []string) {
	body = strings.TrimLeft(body, "\n")
	if body == "" {
		return "", nil, nil
	}
	if !strings.HasPrefix(body, "## ") {
		idx := strings.Index(body, "\n## ")
		if idx < 0 {
			return strings.TrimSpace(body), nil, nil
		}
		if pre := strings.TrimSpace(body[:idx]); pre != "" {
			return pre, nil, nil
		}
		body = body[idx+1:]
	}
	var criteriaText, planText string
	for strings.HasPrefix(body, "## ") {
		var heading string
		if nl := strings.IndexByte(body, '\n'); nl < 0 {
			heading, body = strings.TrimSpace(body[3:]), ""
		} else {
			heading, body = strings.TrimSpace(body[3:nl]), body[nl+1:]
		}
		var content string
		if next := strings.Index(body, "\n## "); next < 0 {
			content, body = body, ""
		} else {
			content, body = body[:next], body[next+1:]
		}
		content = strings.TrimSpace(content)
		switch heading {
		case "Body":
			mainBody = content
		case "Acceptance Criteria":
			criteriaText = content
		case "Verification Plan":
			planText = content
		}
	}
	return mainBody, parseList(criteriaText), parseList(planText)
}

// parseList reads `- item`, `* item` and `1. item` lines alike.
func parseList(text string) []string {
	var out []string
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		for strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			line = line[2:]
		}
		if line != "" && line[0] >= '0' && line[0] <= '9' {
			if dot := strings.Index(line, ". "); dot > 0 {
				line = line[dot+2:]
			}
		}
		if line = strings.TrimSpace(line); line != "" {
			out = append(out, line)
		}
	}
	return out
}

// quotePlainScalars is the repair the helper applies to a note whose title or
// list items hold an unquoted `: ` — the one YAML mistake a hand edit makes.
func quotePlainScalars(fm string) string {
	lines := strings.Split(fm, "\n")
	changed := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "- ") {
			value := strings.TrimSpace(trimmed[2:])
			if value != "" && !quoted(value) {
				prefix := line[:strings.Index(line, "-")+2]
				lines[i] = prefix + " " + strconv.Quote(value)
				changed = true
			}
			continue
		}
		idx := strings.Index(line, ": ")
		if idx <= 0 {
			continue
		}
		key, value := line[:idx], line[idx+2:]
		if !plainScalarKey(key) || !strings.Contains(value, ": ") || quoted(value) {
			continue
		}
		lines[i] = key + ": " + strconv.Quote(value)
		changed = true
	}
	if !changed {
		return fm
	}
	return strings.Join(lines, "\n")
}

func quoted(value string) bool {
	return strings.HasPrefix(value, `"`) || strings.HasPrefix(value, "'")
}

func plainScalarKey(key string) bool {
	switch key {
	case "id", "title", "status", "priority", "model_level", "task_type", "parent_id", "branch", "worktree_path":
		return true
	default:
		return false
	}
}

// checkBody refuses text the on-disk format cannot hold.
func checkBody(text string) error {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "## ") {
			return ErrHeading
		}
	}
	return nil
}

func parseTime(value string) time.Time {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return time.Time{}
	}
	return t
}

func formatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func normalizeEnum(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "_")
	return strings.ReplaceAll(value, " ", "_")
}

func normalizeTags(tags []string) []string {
	var out []string
	for _, tag := range tags {
		if tag = strings.ToLower(strings.TrimSpace(tag)); tag != "" {
			out = append(out, tag)
		}
	}
	return out
}
