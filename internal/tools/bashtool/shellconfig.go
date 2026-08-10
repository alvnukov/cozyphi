package bashtool

// Shell resolution for the bash tool:
//
//   - Windows: Git Bash at known locations (%ProgramFiles%\Git\bin\bash.exe,
//     %ProgramFiles(x86)%\Git\bin\bash.exe), then bash.exe on PATH
//     (Cygwin, MSYS2, WSL).
//   - WSL's legacy bash.exe shim (C:\Windows\System32\bash.exe) can't take
//     "-c" arguments, so commands are fed via stdin ("bash -s").
//   - Unix: /bin/bash, then bash on PATH, then fall back to sh.

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"
)

// shellConfig describes how to launch the resolved shell.
type shellConfig struct {
	shell     string
	args      []string
	stdinMode bool // feed the command via stdin ("-s") instead of "-c <cmd>"
}

var (
	shellCfgMu  sync.Mutex
	shellCfg    shellConfig
	shellCfgSet bool
)

// resolveShellConfig returns the cached shell config. On failure nothing is
// cached, so installing Git Bash while phi runs takes effect on the next call.
func resolveShellConfig() (shellConfig, error) {
	shellCfgMu.Lock()
	defer shellCfgMu.Unlock()
	if shellCfgSet {
		return shellCfg, nil
	}
	cfg, err := resolveShellConfigUncached()
	if err == nil {
		shellCfg, shellCfgSet = cfg, true
	}
	return cfg, err
}

func resolveShellConfigUncached() (shellConfig, error) {
	if runtime.GOOS == "windows" {
		var searched []string
		for _, dir := range gitBashDirs() {
			p := filepath.Join(dir, "Git", "bin", "bash.exe")
			searched = append(searched, p)
			if isFile(p) {
				return configForShell(p), nil
			}
		}
		if p := findBashOnPath(); p != "" {
			return configForShell(p), nil
		}
		return shellConfig{}, fmt.Errorf(
			"no bash shell found: install Git for Windows (https://git-scm.com/download/win) "+
				"or add bash (Cygwin/MSYS2/WSL) to PATH; searched: %s",
			strings.Join(searched, ", "))
	}
	if isFile("/bin/bash") {
		return configForShell("/bin/bash"), nil
	}
	if p := findBashOnPath(); p != "" {
		return configForShell(p), nil
	}
	return shellConfig{shell: "sh", args: []string{"-c"}}, nil
}

func gitBashDirs() []string {
	var dirs []string
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if v := os.Getenv(key); v != "" {
			dirs = append(dirs, v)
		}
	}
	return dirs
}

var wslBashRe = regexp.MustCompile(`^[a-z]:\\windows\\(?:system32|sysnative)\\bash\.exe$`)

// isLegacyWslBashPath reports whether path is Windows' legacy WSL bash shim,
// which doesn't handle "-c" arguments well.
func isLegacyWslBashPath(path string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(path, "/", `\`))
	return wslBashRe.MatchString(normalized)
}

func configForShell(path string) shellConfig {
	if isLegacyWslBashPath(path) {
		return shellConfig{shell: path, args: []string{"-s"}, stdinMode: true}
	}
	return shellConfig{shell: path, args: []string{"-c"}}
}

// findBashOnPath locates bash via `where bash.exe` (Windows) / `which bash`
// (Unix). `where` can list paths that don't exist, so results are verified.
func findBashOnPath() string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	prog, arg := "which", "bash"
	if runtime.GOOS == "windows" {
		prog, arg = "where", "bash.exe"
	}
	out, err := exec.CommandContext(ctx, prog, arg).Output()
	if err != nil {
		return ""
	}
	first := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	if first == "" {
		return ""
	}
	if runtime.GOOS == "windows" && !isFile(first) {
		return ""
	}
	return first
}

func isFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
