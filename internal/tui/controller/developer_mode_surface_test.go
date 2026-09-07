package controller

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// The settable surface of the harness, read off the source rather than off a
// list somebody remembered to update. It is the other half of the coverage
// inventory: the inventory says where every setting is reported, and this
// says what the settings are, so a setting added to a loader and to nothing
// else fails a test on the day it is added.
//
// Three sources, because there are three ways to tell the harness something:
// a key in a config file, a variable in the environment, a flag on the
// command line. Each is found the way the loader itself finds it — the yaml
// tags it decodes through, the literal it passes to os.Getenv, the string it
// compares an argument against — so a new one cannot be introduced without
// this scan seeing it.
//
// What it deliberately cannot see is a name assembled at run time: a variable
// whose name is itself a setting, a platform lookup keyed off a list. Those
// are in the inventory under the runtime surface, named by hand, because the
// fixed decision on this ticket is that runtime-only entries carry an
// explicit list rather than a reflection dump.

// surfaceKind is which of the three ways a setting is given.
type surfaceKind string

const (
	surfaceConfig  surfaceKind = "config"
	surfaceEnv     surfaceKind = "env"
	surfaceFlag    surfaceKind = "flag"
	surfaceRuntime surfaceKind = "runtime"
)

// scannedSurface is everything the source says can be set, by kind.
type scannedSurface struct {
	config []string
	env    []string
	flags  []string
}

// ids returns one kind's surfaces, sorted, for comparison with the inventory.
func (s scannedSurface) ids(kind surfaceKind) []string {
	switch kind {
	case surfaceConfig:
		return s.config
	case surfaceEnv:
		return s.env
	case surfaceFlag:
		return s.flags
	case surfaceRuntime:
		return nil
	}
	return nil
}

// longFlag is a command line flag as cmd spells it when it compares an
// argument against one. The optional "=" is the same flag written with its
// value attached, and is folded away.
var longFlag = regexp.MustCompile(`^--[a-z][a-z0-9-]*=?$`)

// envName is a variable name as a loader passes it to os.Getenv.
var envName = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// scanSettableSurface parses every non-test source file in the module and
// returns what can be set. It reads source and nothing else: no build, no
// reflection, no run.
func scanSettableSurface(t *testing.T) scannedSurface {
	t.Helper()
	root := moduleRoot(t)
	fset := token.NewFileSet()
	found := scannedSurface{}
	consts := map[string]string{}
	files := map[string]*ast.File{}

	walkSource(t, root, func(rel, path string) {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", rel, err)
		}
		files[rel] = file
		collectStringConsts(filepath.Dir(rel), file, consts)
	})

	for rel, file := range files {
		pkg := filepath.Dir(rel)
		scanFile(pkg, file, consts, &found)
	}
	found.config = tidy(found.config)
	found.env = tidy(found.env)
	found.flags = tidy(found.flags)
	return found
}

// scanFile adds one file's settings to found.
func scanFile(pkg string, file *ast.File, consts map[string]string, found *scannedSurface) {
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSpec:
			if st, ok := node.Type.(*ast.StructType); ok {
				found.config = append(found.config, schemaPaths(pkg, node.Name.Name, st)...)
			}
		case *ast.ValueSpec:
			// A schema declared where it is decoded rather than as a named
			// type: `var cfg struct{ ... }` is still a config file's shape.
			st, ok := node.Type.(*ast.StructType)
			if !ok || len(node.Names) == 0 {
				return true
			}
			found.config = append(found.config, schemaPaths(pkg, node.Names[0].Name, st)...)
		case *ast.CallExpr:
			scanCall(pkg, node, consts, found)
		case *ast.BasicLit:
			if pkg != "cmd" || node.Kind != token.STRING {
				return true
			}
			value, err := strconv.Unquote(node.Value)
			if err != nil || !longFlag.MatchString(value) {
				return true
			}
			found.flags = append(found.flags, strings.TrimSuffix(value, "="))
		}
		return true
	})
}

// scanCall reads the two calls that name a setting: an environment lookup and
// a config file path.
func scanCall(pkg string, call *ast.CallExpr, consts map[string]string, found *scannedSurface) {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		owner, ok := fun.X.(*ast.Ident)
		if !ok {
			return
		}
		switch {
		case owner.Name == "os" && (fun.Sel.Name == "Getenv" || fun.Sel.Name == "LookupEnv"):
			found.env = append(found.env, envArgs(pkg, call.Args, consts)...)
		case owner.Name == "configfile" && (fun.Sel.Name == "Set" || fun.Sel.Name == "Lookup"):
			if path := literalPath(call.Args); path != "" {
				found.config = append(found.config, "config.yaml:"+path)
			}
		}
	case *ast.Ident:
		// firstEnv is the project loader's own variadic lookup: the names it
		// tries are as much the environment surface as os.Getenv's are.
		if fun.Name == "firstEnv" {
			found.env = append(found.env, envArgs(pkg, call.Args, consts)...)
		}
	}
}

// envArgs resolves a lookup's arguments to variable names, following a
// package level constant once — the loaders that keep their variable name in
// a const are as explicit as the ones that write it inline.
func envArgs(pkg string, args []ast.Expr, consts map[string]string) []string {
	var names []string
	for _, arg := range args {
		var value string
		switch a := arg.(type) {
		case *ast.BasicLit:
			if a.Kind != token.STRING {
				continue
			}
			unquoted, err := strconv.Unquote(a.Value)
			if err != nil {
				continue
			}
			value = unquoted
		case *ast.Ident:
			value = consts[pkg+"."+a.Name]
		}
		if envName.MatchString(value) {
			names = append(names, value)
		}
	}
	return names
}

// literalPath joins a config file path written entirely in literals. A path
// assembled from a variable is not a key this scan can name, and is left to
// the inventory's runtime surface.
func literalPath(args []ast.Expr) string {
	var parts []string
	for _, arg := range args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return ""
		}
		parts = append(parts, value)
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, ".")
}

// schemaPaths returns every config key one struct decodes. Configuration is
// yaml everywhere except the persisted TUI preferences, which are json and
// live in the package that owns configuration — so json counts as a schema
// there and nowhere else. Widening it further would pull in every wire format
// in the module: an LLM request body and a jsonl event are not settings.
func schemaPaths(pkg, owner string, st *ast.StructType) []string {
	paths := tagPaths("yaml", pkg+":"+owner, "", st)
	if pkg == "internal/project" {
		paths = append(paths, tagPaths("json", pkg+":"+owner, "", st)...)
	}
	return paths
}

// tagPaths returns every config key a struct decodes, nested structs
// included, as owner:path. A field with no such tag is not a config key.
func tagPaths(tag, owner, prefix string, st *ast.StructType) []string {
	var paths []string
	for _, field := range st.Fields.List {
		if field.Tag == nil {
			continue
		}
		raw, err := strconv.Unquote(field.Tag.Value)
		if err != nil {
			continue
		}
		name, _, _ := strings.Cut(reflect.StructTag(raw).Get(tag), ",")
		if name == "" || name == "-" {
			continue
		}
		paths = append(paths, owner+"."+prefix+name)
		if nested := structOf(field.Type); nested != nil {
			paths = append(paths, tagPaths(tag, owner, prefix+name+".", nested)...)
		}
	}
	return paths
}

// structOf unwraps the pointer and slice a schema field is usually written
// as, and returns the struct underneath when there is one.
func structOf(expr ast.Expr) *ast.StructType {
	switch typed := expr.(type) {
	case *ast.StructType:
		return typed
	case *ast.StarExpr:
		return structOf(typed.X)
	case *ast.ArrayType:
		return structOf(typed.Elt)
	}
	return nil
}

// collectStringConsts records package level string constants, so a lookup
// written against a named constant resolves to the variable it names.
func collectStringConsts(pkg string, file *ast.File, into map[string]string) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				lit, ok := value.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if unquoted, err := strconv.Unquote(lit.Value); err == nil {
					into[pkg+"."+name.Name] = unquoted
				}
			}
		}
	}
}

// walkSource visits every non-test Go file in the module. The task registry,
// the worktrees and the git directory are not the harness's source.
func walkSource(t *testing.T, root string, visit func(rel, path string)) {
	t.Helper()
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".worktrees", "vendor", "obsidian-tasks", "testdata", "node_modules":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		visit(rel, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
}

// moduleRoot walks up from the package directory to the go.mod, so the scan
// covers the module rather than the one package the test happens to live in.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the package directory")
		}
		dir = parent
	}
}

// tidy sorts and deduplicates, so two loaders naming the same variable are
// one surface rather than two.
func tidy(values []string) []string {
	slices.Sort(values)
	return slices.Compact(values)
}
