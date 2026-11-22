// Package codemode runs model-authored programs in a fresh capability-limited JavaScript VM.
package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/alvnukov/cozyphi/internal/llm"
)

const (
	maxSource      = 256 << 10
	maxModuleBytes = 4 << 20
	maxModules     = 256
	maxValue       = 256 << 10
	maxCalls       = 128
	maxDuration    = 2 * time.Minute
)

type Program struct {
	Source string
	// Modules contains resolved, immutable virtual files keyed by package/path.ts.
	Modules map[string]string
	Tools   []llm.ToolDefinition
	// ParallelReads enables the executor callback for bounded groups of independent reads.
	ParallelReads bool
}

type Host func(context.Context, string, json.RawMessage) (json.RawMessage, error)

// Imports lists the statically resolvable library roots in a cell using the same parser as execution.
func Imports(source string) []string {
	names := map[string]bool{}
	var mu sync.Mutex
	if len(source) > maxSource || (!strings.Contains(source, "import") && !strings.Contains(source, "require")) {
		return nil
	}
	prelude, body := splitCell(source)
	api.Build(
		api.BuildOptions{
			Stdin: &api.StdinOptions{
				Contents: prelude + "\nasync function __cell(){\n" + body + "\n}\n__cell()",
				Loader:   api.LoaderTS,
			},
			Bundle:      true,
			Format:      api.FormatESModule,
			TreeShaking: api.TreeShakingFalse,
			LogLevel:    api.LogLevelSilent,
			Plugins: []api.Plugin{{Name: "imports", Setup: func(b api.PluginBuild) {
				b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(a api.OnResolveArgs) (api.OnResolveResult, error) {
					if name, ok := strings.CutPrefix(a.Path, "cozy:lib/"); ok && validPackage(name) {
						mu.Lock()
						names[name] = true
						mu.Unlock()
					}
					return api.OnResolveResult{Path: a.Path, External: true}, nil
				})
			}}},
		},
	)
	out := make([]string, 0, len(names))
	for name := range names {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func validPackage(name string) bool {
	return name != "" && !strings.ContainsAny(name, "/\\:") && fs.ValidPath(name) && name != "."
}

func validateProgram(p Program) error {
	if len(p.Source) > maxSource {
		return errors.New("cell exceeds the source limit")
	}
	if len(p.Modules) > maxModules {
		return errors.New("library contains too many modules")
	}
	n := 0
	for name, source := range p.Modules {
		if !fs.ValidPath(name) || !strings.Contains(name, "/") || strings.ContainsAny(name, "\\:\x00\n\r?#") ||
			!strings.HasSuffix(name, ".ts") {
			return fmt.Errorf("invalid virtual module path %q", name)
		}
		n += len(source)
		if n > maxModuleBytes {
			return errors.New("library source exceeds the module limit")
		}
	}
	return nil
}

// splitCell leaves leading static imports at module scope and puts the rest of the cell
// in an async function. esbuild validates actual TypeScript and resolves every import.
func splitCell(s string) (string, string) {
	end := 0
	for {
		i := skipTrivia(s, end)
		if !strings.HasPrefix(s[i:], "import") {
			break
		}
		j := i + 6
		if j < len(s) &&
			(s[j] == '_' || s[j] == '$' || s[j] >= 'A' && s[j] <= 'Z' || s[j] >= 'a' && s[j] <= 'z' || s[j] >= '0' && s[j] <= '9') {
			break
		}
		j = skipTrivia(s, j)
		if j == len(s) || s[j] == '(' || s[j] == '.' {
			break
		}
		depth := 0
		done := false
		for j < len(s) {
			j = skipTrivia(s, j)
			if j >= len(s) {
				break
			}
			c := s[j]
			j++
			switch c {
			case '{':
				depth++
			case '}':
				depth--
			case '\'', '"':
				for j < len(s) && s[j] != c {
					if s[j] == '\\' && j+1 < len(s) {
						j++
					}
					j++
				}
				if j < len(s) {
					j++
				}
				if depth == 0 {
					if next := skipTrivia(s, j); next < len(s) && s[next] == ';' {
						j = next + 1
					}
					done = true
				}
			}
			if done {
				break
			}
		}
		if !done {
			break
		}
		end = j
	}
	return s[:end], s[end:]
}

func skipTrivia(s string, i int) int {
	for i < len(s) {
		switch {
		case strings.ContainsRune(" \r\n\t", rune(s[i])):
			i++
		case strings.HasPrefix(s[i:], "//"):
			n := strings.IndexByte(s[i:], '\n')
			if n < 0 {
				return len(s)
			}
			i += n + 1
		case strings.HasPrefix(s[i:], "/*"):
			n := strings.Index(s[i+2:], "*/")
			if n < 0 {
				return i
			}
			i += 4 + n
		default:
			return i
		}
	}
	return i
}

func resolveModule(p Program, importer, spec string) (string, error) {
	if spec == "cozy:test" {
		return "cozy:test", nil
	}
	if name, ok := strings.CutPrefix(spec, "cozy:lib/"); ok && validPackage(name) {
		for _, f := range []string{name + "/index.ts", name + "/src/index.ts"} {
			if _, found := p.Modules[f]; found {
				return f, nil
			}
		}
	} else if strings.HasPrefix(spec, "./") || strings.HasPrefix(spec, "../") {
		f := path.Join(path.Dir(importer), spec)
		root, _, _ := strings.Cut(importer, "/")
		if strings.HasPrefix(f, root+"/") {
			if _, ok := p.Modules[f]; ok {
				return f, nil
			}
		}
	}
	return "", fmt.Errorf("module %q is outside the pinned program or unavailable", spec)
}

func bundle(p Program, testing bool) (compiledCell, error) {
	imports, body := splitCell(p.Source)
	entry := imports + "\ncozyCell.start(async()=>{\n" + body + "\n});\n"
	testCount := 0
	if testing {
		var preamble strings.Builder
		tests := make([]string, 0)
		for k := range p.Modules {
			if strings.HasSuffix(k, ".test.ts") {
				tests = append(tests, k)
			}
		}
		sort.Strings(tests)
		testCount = len(tests)
		for _, k := range tests {
			q, _ := json.Marshal("cozy:fixture/" + k)
			fmt.Fprintf(&preamble, "import %s;\n", q)
		}
		entry = preamble.String() + imports + "\ncozyCell.start(async()=>{\nawait(async()=>{\n" + body + "\n})();\nreturn await cozyCell.runTests();\n});\n"
	}
	b := api.Build(
		api.BuildOptions{
			Stdin:          &api.StdinOptions{Contents: entry, Loader: api.LoaderTS, Sourcefile: "cell.ts"},
			Bundle:         true,
			Platform:       api.PlatformNeutral,
			Sourcemap:      api.SourceMapExternal,
			SourcesContent: api.SourcesContentExclude,
			Outfile:        "cozy-bundle.js",
			TsconfigRaw:    "{}",
			Define:         map[string]string{"require": "undefined"},
			Format:         api.FormatIIFE,
			Target:         api.ES2022,
			LogLevel:       api.LogLevelSilent,
			Plugins: []api.Plugin{{Name: "pinned-library", Setup: func(build api.PluginBuild) {
				build.OnResolve(
					api.OnResolveOptions{Filter: ".*"},
					func(a api.OnResolveArgs) (api.OnResolveResult, error) {
						if f, ok := strings.CutPrefix(
							a.Path,
							"cozy:fixture/",
						); ok && testing &&
							a.Importer == "cell.ts" {
							if _, ok = p.Modules[f]; ok {
								return api.OnResolveResult{Path: f, Namespace: "library"}, nil
							}
						}
						f, err := resolveModule(p, a.Importer, a.Path)
						if err != nil {
							return api.OnResolveResult{}, err
						}
						if f == "cozy:test" && !testing {
							return api.OnResolveResult{}, errors.New("cozy:test is available during tests only")
						}
						return api.OnResolveResult{Path: f, Namespace: "library"}, nil
					},
				)
				build.OnLoad(
					api.OnLoadOptions{Filter: ".*", Namespace: "library"},
					func(a api.OnLoadArgs) (api.OnLoadResult, error) {
						src := p.Modules[a.Path]
						if a.Path == "cozy:test" {
							src = "export const {test, equal, fixture, fixtureError} = cozyCell"
						}
						return api.OnLoadResult{Contents: &src, Loader: api.LoaderTS}, nil
					},
				)
			}}},
		},
	)
	if len(b.Errors) > 0 {
		return compiledCell{}, fmt.Errorf(
			"compile cell: %s",
			strings.TrimSpace(
				strings.Join(api.FormatMessages(b.Errors, api.FormatMessagesOptions{Kind: api.ErrorMessage}), "\n"),
			),
		)
	}
	compiled, err := readBundle(b.OutputFiles)
	compiled.mapping.cellOffset = 2
	if testing {
		compiled.mapping.cellOffset = 3 + testCount
	}
	return compiled, err
}
