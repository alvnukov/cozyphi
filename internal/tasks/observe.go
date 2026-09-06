package tasks

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// DiscoverFacts is what the caller knew at the moment it looked for a
// registry and cannot recover afterwards. Discover returns a nil registry
// both for a repository that has none and for a config it refused, so a
// session holding nothing cannot tell the two apart on its own. Re-running
// Discover to find out is not allowed: it reads the helper's config and
// stats the registry directory, and an observation does not read disk.
type DiscoverFacts struct {
	// Attempted is whether a discovery happened at all. A zero DiscoverFacts
	// is a caller that never looked, and the whole layer reports unavailable
	// rather than "this repository has no tasks".
	Attempted bool
	// Failed is whether the discovery returned an error — a config that
	// could not be read or parsed, or one naming a path outside the
	// repository.
	Failed bool
}

// ObserveDiscover records the outcome of one Discover call. It is called
// where the discovery is, because that is the only place both answers are
// known.
//
// The error is taken and dropped on purpose. It quotes the registry path the
// config named and can name the file it failed to parse; Failed says that
// the discovery failed, and none of the error's text leaves this call.
func ObserveDiscover(err error) DiscoverFacts {
	return DiscoverFacts{Attempted: true, Failed: err != nil}
}

// Observe reports the registry's state for the harness view: whether one was
// found, and where it sits inside the repository.
//
// It reads what the registry already holds and does nothing else. The
// directory is not listed, no note is opened or parsed, no task is created,
// updated or recovered, and no config is re-read. That is also why no count
// of tasks is reported: the registry keeps none in memory, every listing
// parses the whole directory, and a number here would be a read this view
// must not do.
//
// Nothing that could carry a secret is copied out: not a task id, title,
// status, body, acceptance criterion, branch, worktree path or note path.
// What leaves is two states and a repository-relative directory name.
func Observe(r *Registry, load DiscoverFacts) diag.TaskStoreFacts {
	facts := diag.TaskStoreFacts{
		Known:      load.Attempted || r != nil,
		Attempted:  load.Attempted,
		Failed:     load.Failed,
		Found:      r != nil,
		DefaultDir: DefaultDir,
	}
	if r == nil {
		return facts
	}
	facts.Dir = relativeDir(r.root, r.dir)
	facts.Revision = fmt.Sprintf("f%t.d%t", facts.Failed, facts.Dir != "")
	return facts
}

// relativeDir names the registry inside its own repository. Discovery
// already refuses a path that climbs out of the root, so this normally
// yields a plain directory name; anything that still would not is reported
// as nothing rather than as a path on this machine.
func relativeDir(root, dir string) string {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return rel
}
