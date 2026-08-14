package bashtool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// shellWaitDelay is how long Cmd.Wait waits after Cancel for the process tree
// to exit before sending a hard kill (TerminateProcess / SIGKILL). Prevents
// indefinite hangs when taskkill fails or a child breaks out of the tree.
const shellWaitDelay = 3 * time.Second

// shellEnv returns the environment for shell commands: the parent env with
// phi's bin dir (~/.phi/bin, where fd/ripgrep are downloaded) prepended to
// PATH.
func shellEnv() []string {
	env := os.Environ()
	homeDir, err := os.UserHomeDir()
	if err != nil || homeDir == "" {
		return env
	}
	binDir := filepath.Join(homeDir, ".phi", "bin")
	if _, err := os.Stat(binDir); err != nil {
		return env
	}
	return prependPathEntry(env, binDir)
}

// prependPathEntry prepends dir to the PATH entry of env (the PATH key is
// matched case-insensitively, as Windows uses "Path"). Returns env unchanged
// when dir is already present.
func prependPathEntry(env []string, dir string) []string {
	pathKey, pathIdx := "PATH", -1
	for i, kv := range env {
		key, _, ok := strings.Cut(kv, "=")
		if ok && strings.EqualFold(key, "PATH") {
			pathKey, pathIdx = key, i
			break
		}
	}
	if pathIdx < 0 {
		return append(env, pathKey+"="+dir)
	}
	cur := strings.TrimPrefix(env[pathIdx], pathKey+"=")
	for _, seg := range filepath.SplitList(cur) {
		if samePath(seg, dir) {
			return env
		}
	}
	updated := make([]string, len(env))
	copy(updated, env)
	updated[pathIdx] = pathKey + "=" + dir + string(os.PathListSeparator) + cur
	return updated
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// buildShellCommand builds the shell command for command, applying the
// resolved shell config, phi's bin dir on PATH, and process-group/tree
// cancellation: on context cancellation the whole tree is killed
// (taskkill /T on Windows, process-group SIGKILL elsewhere).
func buildShellCommand(ctx context.Context, command string) (*exec.Cmd, error) {
	cfg, err := resolveShellConfig()
	if err != nil {
		return nil, err
	}
	var cmd *exec.Cmd
	if cfg.stdinMode {
		cmd = exec.CommandContext(ctx, cfg.shell, cfg.args...) //nolint:gosec // G204: shell is the bash tool's purpose
		cmd.Stdin = strings.NewReader(command)
	} else {
		//nolint:gosec // G204: shell is the bash tool's purpose
		cmd = exec.CommandContext(ctx, cfg.shell, append(cfg.args, command)...)
	}
	cmd.Env = shellEnv()
	cmd.SysProcAttr = processGroupAttr()
	cmd.WaitDelay = shellWaitDelay
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return killProcessTree(cmd.Process.Pid)
	}
	return cmd, nil
}
