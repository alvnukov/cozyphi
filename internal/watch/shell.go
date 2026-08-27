package watch

import (
	"context"

	"github.com/alvnukov/cozyphi/internal/tools/bashtool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

// shellCollectLimit is what a watch retains of a command's output. A watch
// reads its output as it streams and never asks for the collected tail, so
// this only has to be big enough that the collector is not the bottleneck —
// the bash tool's 8 MB budget would be held for the life of every watch.
const shellCollectLimit = 64 * 1024

// defaultShell runs a command the way the bash tool does: same shell
// resolution, same environment, same working directory rules. A watch that
// ran something else would be a second, undocumented shell.
func defaultShell(ctx context.Context, command string, onChunk func(string)) (ShellResult, error) {
	res, err := bashtool.ExecShell(ctx, command, bashtool.ShellExecOptions{
		OnChunk:      onChunk,
		CollectLimit: shellCollectLimit,
	})
	if err != nil {
		return ShellResult{}, err
	}
	return ShellResult{ExitCode: res.ExitCode, Canceled: res.Canceled}, nil
}

// withCwd puts the working directory where the shell looks for it.
func withCwd(ctx context.Context, dir string) context.Context {
	if dir == "" {
		return ctx
	}
	return tooldef.WithCwd(ctx, dir)
}
