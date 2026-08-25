package bashtool

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/alvnukov/cozyphi/internal/proc"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// shellEnv returns the environment for shell commands: the parent env with
// cozyphi's bin dir (~/.cozyphi/bin, where fd/ripgrep are downloaded) prepended to
// PATH.
func shellEnv() []string {
	env := os.Environ()
	binDir := project.GetDefaultProject().Global().BinDir()
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

// buildShellSpec builds the process spec for command: the resolved shell argv,
// cozyphi's bin dir on PATH, and the session cwd. Process-tree ownership and
// termination live in the proc module.
func buildShellSpec(ctx context.Context, command string) (proc.Spec, error) {
	cfg, err := resolveShellConfig()
	if err != nil {
		return proc.Spec{}, err
	}
	dir, _ := tooldef.Cwd(ctx)
	return shellSpec(cfg, dir, command), nil
}

// shellSpec assembles argv/stdin for a resolved shell config.
func shellSpec(cfg shellConfig, dir, command string) proc.Spec {
	spec := proc.Spec{Env: shellEnv(), Dir: dir}
	if cfg.stdinMode {
		spec.Argv = []string{cfg.shell, "-s"}
		spec.Stdin = command
		return spec
	}
	args := append([]string(nil), cfg.args...)
	args = append(args, command)
	spec.Argv = append([]string{cfg.shell}, args...)
	return spec
}
