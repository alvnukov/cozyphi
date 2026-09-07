package lsp

import (
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
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
	// u.Path is already percent-decoded, so a drive colon spelled %3A by the
	// server arrives here literal, the same as one left unencoded.
	path := u.Path
	if path == "" {
		path = u.Opaque
	}
	path = osPathFromURIPath(path, runtime.GOOS == "windows")
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("relative file uri %q", raw)
	}
	return filepath.Clean(path), nil
}

// osPathFromURIPath turns the decoded path component of a file:// URI back into
// an OS path. On Windows a URI path is /C:/Users/zx/main.go: the slash in front
// of the drive letter is dropped and the forward slashes become backslashes, so
// the result is C:\Users\zx\main.go. POSIX paths are already OS paths and pass
// through unchanged. The slash conversion is done here rather than left to
// filepath.FromSlash so the Windows rules stay deterministic on any host, which
// keeps the round trip unit testable from POSIX. The windows flag is explicit
// for the same reason.
func osPathFromURIPath(path string, windows bool) string {
	if !windows {
		return path
	}
	path = dropSlashBeforeDrive(path)
	return strings.ReplaceAll(path, "/", `\`)
}

// dropSlashBeforeDrive removes the single leading slash a file:// URI carries in
// front of a Windows drive letter: /C:/Users -> C:/Users. Anything that is not
// a leading "/<letter>:" is left untouched.
func dropSlashBeforeDrive(path string) string {
	if len(path) >= 3 && path[0] == '/' && isDriveLetter(path[1]) && path[2] == ':' {
		return path[1:]
	}
	return path
}

// isDriveLetter reports whether c is an ASCII letter that can name a Windows
// volume.
func isDriveLetter(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
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
