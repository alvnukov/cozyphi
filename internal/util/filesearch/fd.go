// Package filesearch finds workspace files via the fd binary.
package filesearch

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

var (
	fdPathOnce sync.Once
	fdPath     string
	fdPathErr  error
)

// ResolveFD returns the path to the fd binary: ~/.phi/bin/fd first, then PATH.
func ResolveFD() (string, error) {
	fdPathOnce.Do(func() {
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			custom := filepath.Join(homeDir, ".phi", "bin", "fd")
			if _, err := os.Stat(custom); err == nil {
				fdPath = custom
				return
			}
		}
		p, err := exec.LookPath("fd")
		if err != nil {
			fdPathErr = errors.New("fd is not available: install to ~/.phi/bin or PATH")
			return
		}
		fdPath = p
	})
	return fdPath, fdPathErr
}

// ResetResolveFDForTest clears the cached fd path (tests only).
func ResetResolveFDForTest() {
	fdPathOnce = sync.Once{}
	fdPath = ""
	fdPathErr = nil
}

// Search runs fd under cwd and returns relative paths (slash-separated), at most limit.
// An empty query lists files without a name filter.
func Search(ctx context.Context, cwd, query string, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 20
	}
	bin, err := ResolveFD()
	if err != nil {
		return nil, err
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			return nil, err
		}
	}

	args := []string{
		"--type", "f",
		"--color", "never",
		"--max-results", strconv.Itoa(limit),
	}
	query = strings.TrimSpace(query)
	if query != "" {
		// Match against the full relative path; escape regex metacharacters
		// so the query is treated as a literal substring.
		args = append(args, "--full-path", "--", escapeRegex(query))
	}

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = cwd
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// fd exits 1 when there are no matches.
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && stdout.Len() == 0 {
			return nil, nil
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("fd: %s", msg)
	}

	lines := strings.Split(stdout.String(), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line == "." {
			continue
		}
		line = strings.TrimPrefix(line, "./")
		line = filepath.ToSlash(line)
		out = append(out, line)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func escapeRegex(s string) string {
	var b strings.Builder
	b.Grow(len(s) * 2)
	for _, r := range s {
		switch r {
		case '.', '+', '*', '?', '(', ')', '[', ']', '{', '}', '\\', '|', '^', '$':
			_ = b.WriteByte('\\')
			_, _ = b.WriteRune(r)
		default:
			_, _ = b.WriteRune(r)
		}
	}
	return b.String()
}
