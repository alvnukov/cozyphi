package globtool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulseaiclub/phi/internal/tools/tooldef"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/pulseaiclub/phi/internal/llm"
)

const (
	defaultGlobLimit    = 100
	defaultGlobOffset   = 0
	globCollapsePreview = 20
)

var globDescription = fmt.Sprintf(
	`Find files matching a glob pattern and return absolute paths sorted by modification time.

Use path to restrict the search directory. Supports doublestar syntax:
* for one level, ** for recursive, ? for single char, [abc] for sets,
{a,b} for alternatives. Returns at most %d results.
For open-ended exploration that needs many glob/grep rounds, prefer agent_task / agent_spawn when available.`,
	defaultGlobLimit,
)

// GlobTool returns the glob tool definition + handler.
func GlobTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "glob",
			Description: globDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "Directory to search. Example: ./src",
					},
					"pattern": llm.Object{
						"type":        "string",
						"description": "Glob pattern. Example: **/*.go",
					},
				},
				Required: []string{"pattern"},
			},
			Readable: true,
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in globInput
			_ = json.Unmarshal(input, &in)
			pat := strings.TrimSpace(in.Pattern)
			p := strings.TrimSpace(in.Path)
			if p == "" {
				p = "."
			}
			if pat != "" {
				return fmt.Sprintf("glob %q in %s", pat, p)
			}
			return "glob"
		},
		Run: runGlob,
	}
}

type globInput struct {
	Path    string `json:"path,omitempty"`
	Pattern string `json:"pattern"`
}

type fileEntry struct {
	path    string
	mtimeMs int64
}

func runGlob(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in globInput
	if err := json.Unmarshal(input, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("failed to parse glob arguments: %w", err)
	}
	if strings.TrimSpace(in.Pattern) == "" {
		return tooldef.Result{}, errors.New("pattern is required: provide a glob such as *.go or **/*.md")
	}

	searchPath := in.Path
	if strings.TrimSpace(searchPath) == "" {
		searchPath = "."
	}
	absPath, err := tooldef.ResolveToCwd(searchPath)
	if err != nil {
		return tooldef.Result{}, err
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("path not found: %s. Provide an existing directory", absPath)
	}
	if !info.IsDir() {
		return tooldef.Result{}, fmt.Errorf("path is not a directory: %s. Use list to browse directories", absPath)
	}

	files, truncated, err := scanGlob(ctx, in.Pattern, absPath, defaultGlobLimit, defaultGlobOffset)
	if err != nil {
		return tooldef.Result{}, err
	}

	content := renderGlobResult(files, truncated)
	return tooldef.Result{
		Content: content,
		Detail:  fmt.Sprintf("%d files", len(files)),
		Output:  content,
	}, nil
}

func scanGlob(ctx context.Context, pattern, cwd string, limit, offset int) ([]string, bool, error) {
	pattern = strings.TrimSpace(pattern)
	patternLower := strings.ToLower(filepath.ToSlash(pattern))

	entries := make([]fileEntry, 0, limit)
	err := filepath.WalkDir(cwd, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(cwd, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		ok, err := doublestar.Match(patternLower, strings.ToLower(rel))
		if err != nil {
			return fmt.Errorf("invalid glob pattern %q: %w", pattern, err)
		}
		if !ok {
			return nil
		}

		st, err := d.Info()
		if err != nil {
			return nil
		}
		entries = append(entries, fileEntry{
			path:    path,
			mtimeMs: st.ModTime().UnixMilli(),
		})
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].mtimeMs == entries[j].mtimeMs {
			return entries[i].path < entries[j].path
		}
		return entries[i].mtimeMs < entries[j].mtimeMs
	})

	if offset > len(entries) {
		offset = len(entries)
	}
	end := offset + limit
	end = min(end, len(entries))
	truncated := len(entries) > offset+limit

	out := make([]string, 0, end-offset)
	for _, e := range entries[offset:end] {
		out = append(out, e.path)
	}
	return out, truncated, nil
}

func renderGlobResult(files []string, truncated bool) string {
	if len(files) == 0 {
		return "No files found"
	}
	result := strings.Join(files, "\n")
	if truncated {
		result += "\n(Results are truncated. Consider using a more specific path or pattern.)"
	}
	return result
}
