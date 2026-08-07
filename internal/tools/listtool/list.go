package listtool

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/pulseaiclub/phi/internal/tools/tooldef"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/pulseaiclub/phi/internal/llm"
)

const (
	listDescription = `List files and directories as an ASCII tree.

Use limit and max_depth to control output size. Hidden files and common
cache directories are skipped. Use glob to find files by name pattern.`
	truncatedMessage = "[Tree truncated after %d files. Use limit=<n> to see more.]\n\n"
)

const (
	defaultMaxFiles = 500
	defaultMaxDepth = 3
	collapsePreview = 20
)

// ListTool returns the list tool definition + handler.
func ListTool() tooldef.Tool {
	return tooldef.Tool{
		Definition: llm.ToolDefinition{
			Name:        "list",
			Description: listDescription,
			Params: &llm.FunctionParameters{
				Type: "object",
				Properties: llm.Object{
					"path": llm.Object{
						"type":        "string",
						"description": "Directory to list. Example: /repo or .",
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
			var in listInput
			_ = json.Unmarshal(input, &in)
			return strings.TrimSpace(in.Path)
		},
		Run: runList,
	}
}

type listInput struct {
	Path     string `json:"path,omitempty"`
	Limit    int    `json:"limit,omitempty"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

func normalizeOptions(limit int, maxDepth int) (int, int) {
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

func runList(ctx context.Context, input json.RawMessage) (tooldef.Result, error) {
	var in listInput
	if err := json.Unmarshal(input, &in); err != nil {
		// Try as a plain string path.
		var s string
		if err2 := json.Unmarshal(input, &s); err2 != nil || strings.TrimSpace(s) == "" {
			return tooldef.Result{}, fmt.Errorf("failed to parse list arguments: %w", err)
		}
		in.Path = strings.TrimSpace(s)
	}

	dir, err := tooldef.ResolveToCwd(in.Path)
	if err != nil {
		return tooldef.Result{}, err
	}
	dir = filepath.Clean(dir)

	info, err := os.Stat(dir)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("path not found or inaccessible: %s. Check the path and permissions", dir)
	}
	if !info.IsDir() {
		return tooldef.Result{}, fmt.Errorf("not a directory: %s. Use read for files or glob to search", dir)
	}

	limit, maxDepth := normalizeOptions(in.Limit, in.MaxDepth)

	var fileCount int
	root := buildTree(ctx, dir, &fileCount, limit, 0, maxDepth)
	if root == nil {
		return tooldef.Result{}, fmt.Errorf("failed to build tree for directory %s", dir)
	}

	treeStr := renderTree(dir, root.Children)

	if fileCount < limit {
		return tooldef.Result{Content: treeStr, Detail: dir, Output: treeStr}, nil
	}

	truncated := fmt.Sprintf(truncatedMessage, limit) + treeStr
	return tooldef.Result{Content: truncated, Detail: dir, Output: truncated}, nil
}

func shouldSkip(name string) bool {
	return (len(name) > 0 && name[0] == '.') || skipDirs[name]
}

func buildTree(ctx context.Context, dir string, fileCount *int, limit int, currentDepth int, maxDepth int) *treeNode {
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
	fmt.Fprintf(&b, "%s%c\n", rootPath, os.PathSeparator)
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
