package lsp

import (
	"os"
	"path/filepath"
	"strings"
)

// resolveBinary maps a configured program name to an executable path without
// ever consulting the process working directory. Absolute names pass through
// when the file exists and is executable; bare names resolve through ownerBin
// first and then each PATH entry. Empty, dot, and non-absolute PATH entries
// are skipped so a hostile PATH can never select a working-directory binary.
// The lookup never downloads or installs anything.
func resolveBinary(name, ownerBin string, pathEntries []string) (string, bool) {
	if name == "" {
		return "", false
	}
	if filepath.IsAbs(name) {
		if isExecutable(name) {
			return name, true
		}
		return "", false
	}
	// Belt and braces on top of the config loader: a name carrying separators
	// or a volume colon is never a bare lookup target.
	if strings.ContainsAny(name, `/\\:`) {
		return "", false
	}
	if p := filepath.Join(ownerBin, name); isExecutable(p) {
		return p, true
	}
	for _, dir := range pathEntries {
		if dir == "" || dir == "." || !filepath.IsAbs(dir) {
			continue
		}
		if p := filepath.Join(dir, name); isExecutable(p) {
			return p, true
		}
	}
	return "", false
}

// resolveGopls resolves the configured gopls executable for this machine:
// an absolute command is taken directly, a bare name goes through
// ~/.cozyphi/bin and then the inherited PATH.
func resolveGopls(command []string) (string, bool) {
	if len(command) == 0 {
		return "", false
	}
	ownerBin := ""
	if home, err := os.UserHomeDir(); err == nil {
		ownerBin = filepath.Join(home, ".cozyphi", "bin")
	}
	return resolveBinary(command[0], ownerBin, filepath.SplitList(os.Getenv("PATH")))
}
