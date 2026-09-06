package lsp

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alvnukov/cozyphi/internal/diag"
)

// OpenFacts is what the caller knew when it opened a manager and cannot
// recover from the manager afterwards. Open returns (nil, nil) for a
// configuration that switches LSP off, so a nil manager on its own cannot
// tell a subsystem that was told to stay down from one that would not come
// up — and that difference is most of what the answer is worth.
type OpenFacts struct {
	// Attempted is whether an open happened at all. A zero OpenFacts is a
	// caller that never opened LSP, and the whole layer reports unavailable
	// rather than "no language server is configured".
	Attempted bool
	// Disabled is whether the configuration switched the subsystem off.
	Disabled bool
	// Failed is whether the open returned an error.
	Failed bool
}

// ObserveOpen records the outcome of one Open call. It is called where the
// open is, because that is the only place all three answers exist together.
//
// The error is taken and dropped on purpose: it names the workspace it
// rejected and can quote the path it could not stat. Failed says that the
// open failed, and none of the error's text leaves this call.
func ObserveOpen(config Config, err error) OpenFacts {
	return OpenFacts{Attempted: true, Disabled: !config.Enabled, Failed: err != nil}
}

// startResult is what became of one client start attempt, kept as a category
// rather than as a message. The zero value is "nothing has been started",
// which is where every manager begins: a server starts on the first query
// that needs one.
type startResult struct {
	attempted bool
	// kind is the typed category of a failed attempt, and empty when the
	// attempt produced a client.
	kind ErrorKind
}

// recordStart stores what became of one start attempt. It is called with
// m.mu held and keeps two things about it: the bounded sanitized message,
// which the languages operation renders to the owner, and the typed
// category, which is all the harness view may see — the message can name the
// resolved executable or carry the server's own words, and the category
// carries neither.
func (m *Manager) recordStart(err error) {
	m.lastStart = startResult{attempted: true}
	if err == nil {
		m.lastStartErr = ""
		return
	}
	m.lastStart.kind = errKind(err)
	m.lastStartErr, _ = boundText(err.Error())
}

// Observe reports the manager's state for the harness view: whether the
// subsystem is on, whether a manager exists, the one directory it works in,
// the server profile it carries, whether that server is on this machine, and
// which roots it is running for.
//
// It reads what the manager already holds, under the manager's own lock, and
// looks the configured executable up on disk. Nothing else happens: no server
// is started, no binary downloaded, no workspace scanned or synchronized, no
// query dispatched and no diagnostics requested — a workspace nobody has
// queried yet is reported as one, which is the whole point of asking.
//
// Nothing that could carry a secret is copied out: not the configured argv,
// an environment entry, an initialization option, a settings value, a
// resolved executable path or the text of a start error. What leaves is the
// build's own name for the profile, a workspace-relative root, and a state.
func Observe(m *Manager, open OpenFacts) diag.LSPState {
	state := diag.LSPState{
		Known:      open.Attempted,
		Enabled:    !open.Disabled,
		OpenFailed: open.Failed,
		Servers:    []diag.LSPServerFacts{},
		Freshness:  diagnosticProvenance(),
	}
	if m == nil {
		return state
	}
	m.mu.Lock()
	closed := m.closed
	roots := make([]string, 0, len(m.clients))
	for root, c := range m.clients {
		if c.alive() {
			roots = append(roots, root)
		}
	}
	last := m.lastStart
	m.mu.Unlock()
	sort.Strings(roots)

	// config is fixed by Open and never written again, so it is read outside
	// the lock; only the mutable state above needs one.
	state.Opened = true
	state.Closed = closed
	state.Workspace = m.workspace
	state.LastStart = observedStart(last)
	state.Servers = []diag.LSPServerFacts{{
		Name:       goplsServer,
		Language:   goLanguage,
		Installed:  observedInstall(m.config.Gopls.Command),
		Roots:      relativeRoots(m.workspace, roots),
		Operations: append([]string(nil), knownOperations...),
	}}
	state.Revision = managerRevision(state)
	return state
}

// diagnosticProvenance is the vocabulary a diagnostics answer draws from, in
// precedence order. The harness view reports it rather than keeping a copy of
// its own, so the two cannot drift apart.
func diagnosticProvenance() []string {
	return []string{StatusFresh, StatusCached, StatusUnconfirmed, StatusPending}
}

// observedInstall reports whether the configured server is on this machine.
// It is the same lookup the languages operation already performs and it has
// no effect of any kind: an absolute command is checked where it stands, a
// bare name is looked for in ~/.cozyphi/bin and then on PATH. Nothing is
// executed, nothing is downloaded, and nothing is installed. A profile with
// no command at all is unknown rather than missing — there is nothing to
// look for, and this view does not read the configuration file to find one.
func observedInstall(command []string) diag.LSPInstall {
	if len(command) == 0 {
		return diag.LSPInstallUnknown
	}
	if _, ok := resolveGopls(command); ok {
		return diag.LSPInstallPresent
	}
	return diag.LSPInstallMissing
}

// observedStart maps the manager's own record onto the view's vocabulary.
// Every typed category the manager can record has a place here, so a start
// that failed for a reason this view has no word for is still reported as a
// failure rather than as a success.
func observedStart(result startResult) diag.LSPStart {
	if !result.attempted {
		return diag.LSPStartNotAttempted
	}
	switch result.kind {
	case "":
		return diag.LSPStartSucceeded
	case ErrUnavailable:
		return diag.LSPStartUnavailable
	case ErrProtocol:
		return diag.LSPStartProtocol
	case ErrClosed:
		return diag.LSPStartClosed
	default:
		// invalid, ambiguous and unsupported all mean the same thing about a
		// start: it was refused before anything was spawned.
		return diag.LSPStartRejected
	}
}

// relativeRoots renders the live roots relative to the workspace. The manager
// never selects a root outside it, and one that would still come out as an
// escape is dropped rather than reported: this view carries no path the
// workspace does not contain.
func relativeRoots(workspace string, roots []string) []string {
	out := make([]string, 0, len(roots))
	for _, root := range roots {
		rel, err := filepath.Rel(workspace, root)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		out = append(out, rel)
	}
	return out
}

// managerRevision fingerprints what this observation describes: how many
// profiles the manager carries, how many are on this machine, how many roots
// are running, and what the last start attempt came to. Nothing in the
// manager counts these — it is only a way to see that two snapshots taken
// across a first query or a shutdown are of two different states.
func managerRevision(state diag.LSPState) string {
	present, roots := 0, 0
	for _, server := range state.Servers {
		if server.Installed == diag.LSPInstallPresent {
			present++
		}
		roots += len(server.Roots)
	}
	return fmt.Sprintf("s%d.i%d.r%d.%s", len(state.Servers), present, roots, state.LastStart)
}
