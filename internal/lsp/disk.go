package lsp

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Disk notifications cover unopened dependencies too. Unlike a background watcher,
// this bounded scan has no goroutine, registration lifetime or idle polling cost.
const maxSourceFiles = 50000

type diskStamp struct {
	size     int64
	modified time.Time
}

type fileChange struct {
	URI  string `json:"uri"`
	Type int    `json:"type"`
}

// beginQuery serializes the disk/sync barrier, not server requests. The caller
// samples sourceEpoch and releases it before running its operation.
func (c *client) beginQuery(ctx context.Context, target string) (func(), error) {
	select {
	case c.queryGate <- struct{}{}:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-c.done:
		return nil, c.failure()
	}
	release := func() { <-c.queryGate }
	if err := c.refreshDisk(ctx, target); err != nil {
		release()
		return nil, err
	}
	return release, nil
}

func (c *client) refreshDisk(ctx context.Context, target string) error {
	next, err := scanSources(ctx, c.root)
	if err != nil {
		return err
	}
	if c.disk == nil {
		c.disk = next
		return nil
	}
	var changes []fileChange
	for path, stamp := range next {
		previous, exists := c.disk[path]
		if exists && previous == stamp {
			continue
		}
		kind := 2 // changed
		if !exists {
			kind = 1 // created
		}
		changes = append(changes, fileChange{URI: uriFromPath(path), Type: kind})
	}
	for path := range c.disk {
		if _, exists := next[path]; !exists {
			changes = append(changes, fileChange{URI: uriFromPath(path), Type: 3})
		}
	}
	if len(changes) == 0 {
		return nil
	}
	// A target's version alone cannot validate diagnostics after a dependency
	// changes. Drop all derived results and bump targets at their next sync so
	// late publications bearing the old target version cannot pass as fresh.
	c.sourceEpoch.Add(1)
	c.diag.invalidate()
	c.docs.mu.Lock()
	for _, doc := range c.docs.docs {
		doc.invalidated = true
	}
	c.docs.mu.Unlock()
	for _, change := range changes {
		path, err := pathFromURI(change.URI)
		if err != nil {
			return err
		}
		c.docs.mu.Lock()
		doc := c.docs.docs[change.URI]
		notified := doc != nil && doc.notified
		if doc != nil && change.Type == 3 {
			delete(c.docs.docs, change.URI)
			c.docs.total -= len(doc.text)
		}
		c.docs.mu.Unlock()
		// The handler synchronizes its target last, after all dependencies and
		// disk notifications, giving publications a new unambiguous version.
		if doc == nil || (path == target && change.Type != 3) {
			continue
		}
		if change.Type == 3 {
			if notified {
				if err := c.notify(ctx, "textDocument/didClose", map[string]any{
					"textDocument": map[string]any{"uri": change.URI},
				}); err != nil {
					return err
				}
			}
		} else if _, err := c.syncDocument(ctx, path); err != nil {
			return err
		}
	}
	if err := c.notify(ctx, "workspace/didChangeWatchedFiles", map[string]any{"changes": changes}); err != nil {
		return err
	}
	c.disk = next
	return nil
}

func scanSources(ctx context.Context, root string) (map[string]diskStamp, error) {
	files := make(map[string]diskStamp)
	visited := 0
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > maxSourceFiles {
			return newError(
				ErrUnavailable,
				"source sync exceeds %d entries; use a smaller Go workspace",
				maxSourceFiles,
			)
		}
		if entry.IsDir() {
			if path != root && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "node_modules") {
				return filepath.SkipDir
			}
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 || !isSourceFile(entry.Name()) {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode().IsRegular() {
			files[path] = diskStamp{size: info.Size(), modified: info.ModTime()}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, newError(ErrUnavailable, "synchronize Go source tree: %v", err)
	}
	return files, nil
}

func isSourceFile(name string) bool {
	return strings.HasSuffix(name, ".go") || name == "go.mod" || name == "go.sum" || name == "go.work" ||
		name == "go.work.sum"
}
