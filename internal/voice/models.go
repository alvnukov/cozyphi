package voice

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// modelBaseURL is where whisper.cpp publishes the ggml weights.
const modelBaseURL = "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/"

// DefaultModel is what the Ctrl+G offer and a bare /voice install download:
// small transcribes an 11 s clip in ~3 s on an M2, which is what the dialog
// mode needs; bigger models are one /voice install away.
const DefaultModel = "small"

// modelFilePrefix and modelFileSuffix bracket the model name in a file name.
const (
	modelFilePrefix = "ggml-"
	modelFileSuffix = ".bin"
)

// Model is one whisper.cpp ggml model cozyphi knows how to fetch.
type Model struct {
	// Name is the catalog name, e.g. "small".
	Name string
	// File is the file name in a model directory, e.g. "ggml-small.bin".
	File string
	// URL is where Install downloads it from.
	URL string
	// ApproxBytes is for display only; verification never uses it, because
	// the published files change size between releases.
	ApproxBytes int64
	// rank orders auto-selection when several models are installed: higher is
	// better. It stays unexported so it cannot become an API promise.
	rank int
}

// Byte units for FormatBytes and the catalog sizes.
const (
	kib int64 = 1 << 10
	mib int64 = 1 << 20
	gib int64 = 1 << 30
)

// catalog is the fetchable models, in the order /voice models lists them.
var catalog = []Model{
	{Name: "tiny", ApproxBytes: 75 * mib, rank: 0},
	{Name: "base", ApproxBytes: 142 * mib, rank: 1},
	{Name: "small", ApproxBytes: 466 * mib, rank: 2},
	{Name: "medium", ApproxBytes: 1536 * mib, rank: 3},
	{Name: "large-v3", ApproxBytes: 3328599654, rank: 4},
	{Name: "large-v3-turbo", ApproxBytes: 1717986918, rank: 5},
}

func init() {
	for i := range catalog {
		catalog[i].File = ModelFileName(catalog[i].Name)
		catalog[i].URL = modelBaseURL + catalog[i].File
	}
}

// ModelFileName is the ggml file name for a catalog name.
func ModelFileName(name string) string { return modelFilePrefix + name + modelFileSuffix }

// Catalog returns the models cozyphi can download, best-known-first order kept
// stable for the /voice models line.
func Catalog() []Model { return append([]Model(nil), catalog...) }

// LookupModel finds a catalog model by name, file stem or file name, so
// "small", "ggml-small" and "ggml-small.bin" all name the same model.
func LookupModel(name string) (Model, bool) {
	want := modelName(name)
	if want == "" {
		return Model{}, false
	}
	for _, m := range catalog {
		if m.Name == want {
			return m, true
		}
	}
	return Model{}, false
}

// modelName strips the ggml-*.bin decoration from a model reference.
func modelName(name string) string {
	want := strings.ToLower(strings.TrimSpace(name))
	want = strings.TrimSuffix(want, modelFileSuffix)
	return strings.TrimPrefix(want, modelFilePrefix)
}

// FormatBytes renders a size the way the catalog and the download progress
// show it: whole megabytes, one decimal for gigabytes.
func FormatBytes(n int64) string {
	switch {
	case n >= gib:
		return fmt.Sprintf("%.1f GB", float64(n)/float64(gib))
	case n >= mib:
		return fmt.Sprintf("%d MB", (n+mib/2)/mib)
	case n >= kib:
		return fmt.Sprintf("%d KB", (n+kib/2)/kib)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// Installed is one ggml-*.bin found in a model directory.
type Installed struct {
	// Path is the file itself.
	Path string
	// Bytes is its size on disk.
	Bytes int64
	// Name is the catalog model this file provides — "medium" for both
	// ggml-medium.bin and ggml-medium-q5_0.bin — or "" when it matches none.
	Name string
	// Rank is the catalog rank auto-selection compares, -1 when Name is empty.
	Rank int
}

// InstalledModels lists every ggml-*.bin in dirs, in directory order and
// alphabetically within a directory. Unreadable directories are skipped: a
// missing model directory is the normal state before the first install.
func InstalledModels(dirs []string) []Installed {
	var out []Installed
	seen := make(map[string]bool)
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		matches, err := filepath.Glob(filepath.Join(dir, modelFilePrefix+"*"+modelFileSuffix))
		if err != nil || len(matches) == 0 {
			continue
		}
		sort.Strings(matches)
		for _, path := range matches {
			if seen[path] {
				continue
			}
			info, err := os.Stat(path)
			if err != nil || info.IsDir() {
				continue
			}
			seen[path] = true
			name, rank := catalogOf(filepath.Base(path))
			out = append(out, Installed{Path: path, Bytes: info.Size(), Name: name, Rank: rank})
		}
	}
	return out
}

// catalogOf maps a ggml file name to the catalog model it provides: the
// longest catalog name that prefixes the part between "ggml-" and ".bin", so
// quantized variants (ggml-large-v3-turbo-q8_0.bin) count as their base model.
func catalogOf(file string) (string, int) {
	stem := modelName(file)
	best := ""
	rank := -1
	for _, m := range catalog {
		if strings.HasPrefix(stem, m.Name) && len(m.Name) > len(best) {
			best, rank = m.Name, m.rank
		}
	}
	return best, rank
}
