package lstool

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulseaiclub/phi/internal/tools/tooldef"

	"github.com/pulseaiclub/phi/internal/llm"
)

const (
	lsDescription = `List files and directories as an ASCII tree.

Use limit and max_depth to control output size. Hidden files and common
cache directories are skipped.`
	truncatedMessage = "[Tree truncated after %d files. Use limit=<n> to see more.]\n\n"
)

const (
	defaultMaxFiles = 500
	defaultMaxDepth = 3
	collapsePreview = 20
)

// LsTool returns the ls tool definition + handler.
func LsTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "ls",
			Description: lsDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "Directory to list. Example: . or src",
					},
					"limit": llm.Object{
						"type":        "integer",
						"description": "Max files to scan. Example: 100 (default: 500)",
					},
					"max_depth": llm.Object{
						"type":        "integer",
						"description": "Max directory depth to expand. Example: 2 (default: 3)",
					},
				},
				Required: []string{"path"},
			},
			Readable: true,
		},
		DetailFromArgs: func(input json.RawMessage) string {
			var in lsInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Path)
		},
		Run: runLs,
	}
}

type lsInput struct {
	Path     string `json:"path,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

func normalizeOptions(limit, maxDepth int) (int, int) {
	if limit <= 0 {
		limit = defaultMaxFiles
	}
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	return limit, maxDepth
}

type treeNode struct {
	Name     string      `json:"name"`
	IsDir    bool        `json:"isDir"`
	Type     string      `json:"type"`
	Children []*treeNode `json:"children,omitempty"`
}

var skipDirs = map[string]bool{
	"__pycache__":    true,
	"node_modules":   true,
	"venv":           true,
	".venv":          true,
	"vendor":         true,
	".idea":          true,
	".vscode":        true,
	"target":         true,
	"dist":           true,
	"build":          true,
	".pytest_cache":  true,
	".mypy_cache":    true,
	".tox":           true,
	"__pypackages__": true,
	".git":           true,
	".svn":           true,
	".hg":            true,
}

func runLs(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in lsInput
	if err := json.Unmarshal(input, &in); err != nil {
		// Try as a plain string path.
		var s string
		if err2 := json.Unmarshal(input, &s); err2 != nil || strings.TrimSpace(s) == "" {
			return tooldef.Result{}, fmt.Errorf("failed to parse ls arguments: %w", err)
		}
		in.Path = strings.TrimSpace(s)
	}

	dir, err := tooldef.ResolveToCwd(ctx, in.Path)
	if err != nil {
		return tooldef.Result{}, err
	}
	dir = filepath.Clean(dir)

	info, err := os.Stat(dir)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("path not found or inaccessible: %s. Check the path and permissions", dir)
	}
	if !info.IsDir() {
		return tooldef.Result{}, fmt.Errorf("not a directory: %s (ls expects a directory path)", dir)
	}

	limit, maxDepth := normalizeOptions(in.Limit, in.MaxDepth)

	var fileCount int
	root := buildTree(ctx, dir, &fileCount, limit, 0, maxDepth)
	if root == nil {
		return tooldef.Result{}, fmt.Errorf("failed to build tree for directory %s", dir)
	}

	display := tooldef.RelToCwd(ctx, dir)
	treeStr := renderTree(display, root.Children)

	if fileCount < limit {
		return tooldef.Result{Content: treeStr, Detail: display, Output: treeStr}, nil
	}

	truncated := fmt.Sprintf(truncatedMessage, limit) + treeStr
	return tooldef.Result{Content: truncated, Detail: display, Output: truncated}, nil
}

func shouldSkip(name string) bool {
	return (name != "" && name[0] == '.') || skipDirs[name]
}

func buildTree(ctx context.Context, dir string, fileCount *int, limit, currentDepth, maxDepth int) *treeNode {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool {
		return strings.ToLower(entries[i].Name()) < strings.ToLower(entries[j].Name())
	})

	node := &treeNode{
		Name:     filepath.Base(dir),
		IsDir:    true,
		Type:     "directory",
		Children: []*treeNode{},
	}

	for _, entry := range entries {
		if ctx.Err() != nil {
			return nil
		}

		name := entry.Name()
		if shouldSkip(name) {
			continue
		}

		if *fileCount >= limit {
			break
		}

		childPath := filepath.Join(dir, name)
		if entry.IsDir() {
			if currentDepth+1 >= maxDepth {
				node.Children = append(node.Children, &treeNode{
					Name:  name,
					IsDir: true,
					Type:  "directory",
				})
				continue
			}
			child := buildTree(ctx, childPath, fileCount, limit, currentDepth+1, maxDepth)
			if child != nil {
				node.Children = append(node.Children, child)
			} else {
				// If child directory cannot be read, still show directory node.
				node.Children = append(node.Children, &treeNode{
					Name:  name,
					IsDir: true,
					Type:  "directory",
				})
			}
		} else {
			*fileCount++
			node.Children = append(node.Children, &treeNode{
				Name:  name,
				IsDir: false,
				Type:  "file",
			})
		}
	}

	return node
}

func renderTree(rootPath string, children []*treeNode) string {
	var b strings.Builder
	root := filepath.ToSlash(rootPath)
	if !strings.HasSuffix(root, "/") {
		root += "/"
	}
	fmt.Fprintf(&b, "%s\n", root)
	for i, node := range children {
		renderTreeNode(&b, node, "", i == len(children)-1)
	}
	return b.String()
}

func renderTreeNode(b *strings.Builder, node *treeNode, prefix string, isLast bool) {
	connector := "├── "
	nextPrefix := prefix + "│   "
	if isLast {
		connector = "└── "
		nextPrefix = prefix + "    "
	}

	name := node.Name
	if node.IsDir || node.Type == "directory" {
		name += string(os.PathSeparator)
	}
	b.WriteString(prefix)
	b.WriteString(connector)
	b.WriteString(name)
	b.WriteString("\n")

	for i, child := range node.Children {
		renderTreeNode(b, child, nextPrefix, i == len(node.Children)-1)
	}
}
