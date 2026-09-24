package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	// SkillFileName is the required file name inside each skill directory.
	SkillFileName        = "SKILL.md"
	frontmatterDelimiter = "---"
)

// Sentinel errors returned when parsing a skill file.
var (
	ErrInvalidFrontmatter   = errors.New("skill file must start with YAML front matter (---)")
	ErrFrontmatterNotClosed = errors.New("skill file frontmatter not properly closed with ---")
	ErrInvalidYAML          = errors.New("skill file frontmatter is invalid")
)

// Skill represents a single skill loaded from a SKILL.md file.
type Skill struct {
	Name          string
	Description   string
	License       string
	Compatibility string
	Body          string
	Path          string
	SkillFilePath string
}

// Parse parses a single SKILL.md file, extracting YAML frontmatter and body.
func Parse(skillFilePath string) (*Skill, error) {
	content, err := os.ReadFile(skillFilePath)
	if err != nil {
		return nil, err
	}

	contentStr := string(content)
	if !strings.HasPrefix(contentStr, frontmatterDelimiter) {
		return nil, ErrInvalidFrontmatter
	}

	parts := strings.SplitN(contentStr, frontmatterDelimiter, 3)
	if len(parts) < 3 {
		return nil, ErrFrontmatterNotClosed
	}

	skill, err := parseFrontmatter(parts[1])
	if err != nil {
		return nil, err
	}

	skill.Body = strings.TrimSpace(parts[2])
	skill.Path = filepath.Dir(skillFilePath)
	skill.SkillFilePath = skillFilePath
	return skill, nil
}

// parseFrontmatter reads flat "key: value" lines and common YAML block scalars
// (>, >-, |, |-). Nested YAML maps/lists are not supported.
func parseFrontmatter(fm string) (*Skill, error) {
	skill := &Skill{}
	lines := strings.Split(fm, "\n")
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		// Continuation lines belong to a previous block scalar — skip orphans.
		if raw != "" && (raw[0] == ' ' || raw[0] == '\t') {
			return nil, ErrInvalidYAML
		}
		key, val, ok := strings.Cut(trimmed, ":")
		if !ok {
			return nil, ErrInvalidYAML
		}
		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)
		if key == "" {
			return nil, ErrInvalidYAML
		}
		if isYAMLBlockScalar(val) {
			var body string
			body, i = readBlockScalar(lines, i+1)
			val = body
		} else {
			var err error
			val, err = unquote(val)
			if err != nil {
				return nil, err
			}
		}
		switch key {
		case "name":
			skill.Name = val
		case "description":
			skill.Description = val
		case "license":
			skill.License = val
		case "compatibility":
			skill.Compatibility = val
		}
	}
	return skill, nil
}

func isYAMLBlockScalar(val string) bool {
	switch val {
	case ">", ">-", ">|", "|", "|-", "|+":
		return true
	default:
		return false
	}
}

// readBlockScalar collects indented continuation lines for >, >-, |, |-.
// Folded style (>) joins lines with spaces; literal style (|) keeps newlines.
func readBlockScalar(lines []string, start int) (string, int) {
	var parts []string
	i := start
	for ; i < len(lines); i++ {
		raw := lines[i]
		if strings.TrimSpace(raw) == "" {
			parts = append(parts, "")
			continue
		}
		if raw == "" || (raw[0] != ' ' && raw[0] != '\t') {
			break
		}
		parts = append(parts, strings.TrimSpace(raw))
	}
	// Caller loop will i++ after return; step back one so the next key is seen.
	return strings.Join(parts, " "), i - 1
}

func unquote(val string) (string, error) {
	if val == "" {
		return "", nil
	}
	q := val[0]
	if q != '"' && q != '\'' {
		return val, nil
	}
	if len(val) < 2 || val[len(val)-1] != q {
		return "", ErrInvalidYAML
	}
	return val[1 : len(val)-1], nil
}

// ToPromptMarkdown converts skills to a Markdown fragment for injection into
// the system prompt. Returns an empty string if the list is empty.
func ToPromptMarkdown(skills []*Skill) string {
	if len(skills) == 0 {
		return ""
	}

	var sb strings.Builder
	for _, s := range skills {
		sb.WriteString("### ")
		sb.WriteString(s.Name)
		sb.WriteString("\n\n")
		sb.WriteString(s.Description)
		sb.WriteString("\n\n")
		sb.WriteString("**Location:** `")
		sb.WriteString(s.SkillFilePath)
		sb.WriteString("`\n\n")
	}
	return sb.String()
}

// Find resolves a skill by name: exact, then case-insensitive, then a bare
// name — the part after "<namespace>:" or the skill's directory name — that
// matches exactly one skill. A bare name shared by several skills is an
// error naming them; no match is nil, nil.
func Find(list []*Skill, name string) (*Skill, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, nil
	}
	for _, s := range list {
		if s.Name == name {
			return s, nil
		}
	}
	for _, s := range list {
		if strings.EqualFold(s.Name, name) {
			return s, nil
		}
	}
	var candidates []*Skill
	for _, s := range list {
		if strings.EqualFold(bareName(s.Name), name) || strings.EqualFold(filepath.Base(s.Path), name) {
			candidates = append(candidates, s)
		}
	}
	switch len(candidates) {
	case 0:
		return nil, nil
	case 1:
		return candidates[0], nil
	}
	full := make([]string, 0, len(candidates))
	for _, s := range candidates {
		full = append(full, s.Name)
	}
	return nil, fmt.Errorf("skill %q is ambiguous — use one of: %s", name, strings.Join(full, ", "))
}

// bareName strips a plugin namespace: "superpowers:tdd" → "tdd".
func bareName(name string) string {
	if _, bare, ok := strings.Cut(name, ":"); ok {
		return bare
	}
	return name
}
