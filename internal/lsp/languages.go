package lsp

// knownOperations is the frozen V1 operation set reported by languages. A
// live client still gates each operation on its initialized capabilities at
// query time; the inventory itself stays the known set.
var knownOperations = []string{
	"definition",
	"references",
	"hover",
	"symbols",
	"calls",
	"diagnostics",
}

// goplsInstallHint tells the owner how to install the missing server. The
// harness renders it but never executes it, and never downloads anything.
const goplsInstallHint = "go install golang.org/x/tools/gopls@latest"

// languagesStatus reports the one V1 language record. It never starts or
// touches a server process: installed is a pure filesystem lookup, running
// counts live client generations, and error carries the bounded sanitized
// reason of the last failed start, never a PID, argv, or env value.
func (m *Manager) languagesStatus() Result {
	m.mu.Lock()
	roots := 0
	for _, c := range m.clients {
		if c.alive() {
			roots++
		}
	}
	lastErr := m.lastStartErr
	m.mu.Unlock()

	installed := false
	if len(m.config.Gopls.Command) > 0 {
		_, installed = resolveGopls(m.config.Gopls.Command)
	}
	rec := Language{
		Language:    "go",
		Server:      "gopls",
		Configured:  true,
		Installed:   installed,
		Running:     roots > 0,
		ActiveRoots: roots,
		Error:       lastErr,
		Operations:  knownOperations,
	}
	if !installed {
		rec.InstallHint = goplsInstallHint
	}
	return Result{Languages: []Language{rec}}
}
