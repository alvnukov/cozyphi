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
			"/.cozyphi/config.yaml",
			"/etc/shadow",
			"/etc/passwd",
		}
	}
	return []string{
		filepath.Join(home, ".ssh"),
		filepath.Join(home, ".cozyphi", "config.yaml"),
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
	return AbsCleanAt(path, "")
}

// AbsCleanAt is AbsClean against an explicit cwd. Empty cwd uses the process wd.
func AbsCleanAt(path, cwd string) (string, error) {
	if path == "" {
		path = "."
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	if cwd == "" {
		dir, err := os.Getwd()
		if err != nil {
			return "", err
		}
		cwd = dir
	} else if !filepath.IsAbs(cwd) {
		abs, err := filepath.Abs(cwd)
		if err != nil {
			return "", err
		}
		cwd = abs
	} else {
		cwd = filepath.Clean(cwd)
	}
	return filepath.Clean(filepath.Join(cwd, path)), nil
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
		if inSingle {
			continue
		}
		// $(), ${} and backticks expand inside double quotes too, so an
		// executing tail must not hide behind a safe quoted prefix.
		if c == '`' {
			return true
		}
		if c == '$' && i+1 < len(cmd) && (cmd[i+1] == '(' || cmd[i+1] == '{') {
			return true
		}
		if inDouble {
			continue
		}
		switch c {
		case '\n', '\r', ';', '|':
			return true
		case '&':
			// `&&` chains; background `&` also chains intent: both are control.
			return true
		case '<':
			// Input redirects and heredocs are unsupported syntax: they change
			// what the command reads. Ask.
			return true
		case '(', ')':
			// Subshell or process substitution: compound syntax, not a simple
			// command.
			return true
		case '>':
			// Allow only >/dev/null and N>/dev/null (stderr noise); other
			// redirects can overwrite files and must not use the allowlist.
			if isDevNullRedirect(cmd, i) {
				continue
			}
			return true
		}
	}
	// Unclosed quoting or a dangling escape is ambiguous syntax: fail closed.
	return inSingle || inDouble || escaped
}

// isDevNullRedirect reports whether cmd[gt] is the '>' of a >/dev/null redirect
// (optionally >>; a preceding FD digit such as `2>` is part of the previous
// word and needs no handling here). The target must be exactly /dev/null: a
// lookalike path such as /dev/nullx is a real file.
func isDevNullRedirect(cmd string, gt int) bool {
	j := gt
	if j+1 < len(cmd) && cmd[j+1] == '>' {
		j++
	}
	rest := strings.TrimLeft(cmd[j+1:], " \t")
	return rest == "/dev/null" || strings.HasPrefix(rest, "/dev/null ")
}
