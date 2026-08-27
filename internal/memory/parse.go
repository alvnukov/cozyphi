package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Sentinel errors returned when a memory file cannot be read as one fact.
var (
	ErrNoFrontmatter   = errors.New("memory file must start with YAML frontmatter (---)")
	ErrOpenFrontmatter = errors.New("memory file frontmatter is not closed with ---")
)

const (
	frontmatterDelim = "---"
	bom              = "\ufeff"
)

// linkPattern matches the [[other-memory]] cross-reference form.
var linkPattern = regexp.MustCompile(`\[\[([^\[\]]+)\]\]`)

// ParseFile reads one memory file. The file name is the memory's identity:
// a missing or empty frontmatter name falls back to it, so a file the model
// wrote in a hurry still lands in the index instead of vanishing.
func ParseFile(path string) (Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Entry{}, err
	}
	entry, err := parse(string(data))
	if err != nil {
		return Entry{}, fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	entry.Path = path
	entry.File = filepath.Base(path)
	if entry.Name == "" {
		entry.Name = strings.TrimSuffix(entry.File, fileExt)
	}
	return entry, nil
}

// parse splits frontmatter from body. The frontmatter grammar is deliberately
// narrow: flat "key: value" lines plus the one nested block this format uses,
// metadata.type. A flat "type:" is accepted too, because that is the mistake
// a model makes when it writes the file from memory.
func parse(raw string) (Entry, error) {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i >= len(lines) || strings.TrimSpace(strings.TrimPrefix(lines[i], bom)) != frontmatterDelim {
		return Entry{}, ErrNoFrontmatter
	}
	i++

	var (
		fields []string
		closed bool
	)
	for ; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == frontmatterDelim {
			closed = true
			i++
			break
		}
		fields = append(fields, lines[i])
	}
	if !closed {
		return Entry{}, ErrOpenFrontmatter
	}

	entry := parseFrontmatter(fields)
	entry.Body = strings.TrimSpace(strings.Join(lines[i:], "\n"))
	entry.Links = parseLinks(entry.Body)
	return entry, nil
}

func parseFrontmatter(fields []string) Entry {
	var (
		entry  Entry
		parent string
	)
	for _, line := range fields {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		nested := line != trimmed
		key, value, ok := strings.Cut(trimmed, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = unquote(strings.TrimSpace(value))
		if !nested {
			parent = ""
			if value == "" {
				parent = key
			}
		}
		switch {
		case key == "name" && !nested:
			entry.Name = value
		case key == "description" && !nested:
			entry.Description = value
		case key == "type" && (!nested || parent == "metadata"):
			entry.Kind = ParseKind(value)
		case key == "pin" && (!nested || parent == "metadata"):
			entry.Pinned = truthy(value)
		}
	}
	return entry
}

// truthy reads the handful of spellings a model reaches for when it means yes.
func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "yes", "y", "1", "on":
		return true
	default:
		return false
	}
}

func parseLinks(body string) []string {
	matches := linkPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	links := make([]string, 0, len(matches))
	for _, m := range matches {
		name := strings.TrimSpace(m[1])
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		links = append(links, name)
	}
	return links
}

func unquote(value string) string {
	if len(value) < 2 {
		return value
	}
	quote := value[0]
	if (quote == '"' || quote == '\'') && value[len(value)-1] == quote {
		return value[1 : len(value)-1]
	}
	return value
}
