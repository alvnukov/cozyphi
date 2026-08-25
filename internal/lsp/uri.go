package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
)

// pathFromURI decodes a local file:// URI into an absolute cleaned path.
// Only file URIs with no host (or localhost) are accepted; anything else fails
// closed so a server cannot smuggle a remote or non-file reference.
func pathFromURI(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("bad uri %q: %w", raw, err)
	}
	if u.Scheme != "file" {
		return "", fmt.Errorf("non-file uri %q", raw)
	}
	if u.Host != "" && u.Host != "localhost" {
		return "", fmt.Errorf("remote file uri %q", raw)
	}
	path := u.Path
	if path == "" {
		path = u.Opaque
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("relative file uri %q", raw)
	}
	return filepath.Clean(path), nil
}

// filepathRel renders abs as a slash-separated path relative to workspace when
// contained, otherwise absolute. The Manager checks containment first; this
// only formats the already-contained result.
func filepathRel(workspace, abs string) (string, error) {
	rel, err := filepath.Rel(workspace, abs)
	if err != nil {
		return "", err
	}
	if rel == "." {
		return ".", nil
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("path escapes workspace")
	}
	return filepath.ToSlash(rel), nil
}
