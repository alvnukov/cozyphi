package permission

import (
	"os"
	"path/filepath"
	"strings"
)

func defaultSensitivePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return []string{
			"/.ssh",
			"/.phi/config.yaml",
			"/etc/shadow",
			"/etc/passwd",
		}
	}
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".phi", "config.yaml"),
		filepath.Join(home, ".aws", "credentials"),
		filepath.Join(home, ".gnupg"),
		"/etc/shadow",
	}
}

// WorkspaceRoot returns the git-root workspace, or cwd if no .git is found.
func WorkspaceRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	start := dir
	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return start
}

// AbsClean resolves path to an absolute cleaned path (no symlink resolve).
func AbsClean(path string) (string, error) {
	if path == "" {
		path = "."
	}
	if !filepath.IsAbs(path) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		path = filepath.Join(cwd, path)
	}
	return filepath.Clean(path), nil
}

// InWorkspace reports whether absPath is inside workspace (or equal to it).
func InWorkspace(absPath, workspace string) bool {
	if workspace == "" || absPath == "" {
		return false
	}
	absPath = filepath.Clean(absPath)
	workspace = filepath.Clean(workspace)
	if absPath == workspace {
		return true
	}
	rel, err := filepath.Rel(workspace, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// IsSensitivePath reports whether absPath matches a sensitive prefix.
func IsSensitivePath(absPath string, prefixes []string) bool {
	absPath = filepath.Clean(absPath)
	for _, p := range prefixes {
		p = filepath.Clean(p)
		if absPath == p {
			return true
		}
		if strings.HasPrefix(absPath, p+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// HostOfURL extracts hostname from a URL string (best-effort, no full parse required).
func HostOfURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// Strip scheme
	if i := strings.Index(raw, "://"); i >= 0 {
		raw = raw[i+3:]
	}
	// Strip path/query
	if i := strings.IndexAny(raw, "/?#"); i >= 0 {
		raw = raw[:i]
	}
	// Strip userinfo
	if i := strings.LastIndex(raw, "@"); i >= 0 {
		raw = raw[i+1:]
	}
	// Strip port
	if i := strings.LastIndex(raw, ":"); i >= 0 {
		// IPv6 in brackets
		if !strings.Contains(raw, "]") {
			raw = raw[:i]
		}
	}
	return strings.ToLower(raw)
}

// bashEligibleForAllowlist reports whether cmd is a single simple command that
// may be auto-allowed. Chaining, pipes, substitutions, and overwrite redirects
// force Ask/Deny evaluation instead of prefix allowlist matches.
func bashEligibleForAllowlist(cmd string) bool {
	return !hasBashControlSyntax(cmd)
}

func hasBashControlSyntax(cmd string) bool {
	inSingle, inDouble, escaped := false, false, false
	for i := 0; i < len(cmd); i++ {
		c := cmd[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && !inSingle {
			escaped = true
			continue
		}
		if c == '\'' && !inDouble {
			inSingle = !inSingle
			continue
		}
		if c == '"' && !inSingle {
			inDouble = !inDouble
			continue
		}
		if inSingle || inDouble {
			continue
		}
		switch c {
		case '\n', '\r', ';', '|', '`':
			return true
		case '&':
			if i+1 < len(cmd) && cmd[i+1] == '&' {
				return true
			}
			// background `&` also chains intent; treat as control
			return true
		case '>':
			// Allow only >/dev/null and N>/dev/null (stderr noise); other
			// redirects can overwrite files and must not use the allowlist.
			if isDevNullRedirect(cmd, i) {
				continue
			}
			return true
		case '$':
			if i+1 < len(cmd) && (cmd[i+1] == '(' || cmd[i+1] == '{') {
				return true
			}
		}
	}
	return false
}

// isDevNullRedirect reports whether cmd[i] is the '>' of a >/dev/null redirect
// (optionally preceded by a FD digit, optionally >>).
func isDevNullRedirect(cmd string, gt int) bool {
	j := gt
	if j+1 < len(cmd) && cmd[j+1] == '>' {
		j++
	}
	rest := strings.TrimSpace(cmd[j+1:])
	return strings.HasPrefix(rest, "/dev/null")
}
