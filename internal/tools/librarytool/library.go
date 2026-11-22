// Package librarytool binds the shared function library to a session's tool
// registry. The caller supplies session and checkout identity outside model input.
package librarytool

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/alvnukov/cozyphi/internal/codemode"
	"github.com/alvnukov/cozyphi/internal/funcadvisor"
	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/redact"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
	"github.com/alvnukov/cozyphi/internal/tools/writetool"
	"github.com/alvnukov/cozyphi/internal/util"
)

const description = `Use and maintain the shared, versioned TypeScript function library across projects.
search returns export signatures, tested behavior and imports; list returns resolved packages; versions lists all releases of one package; get reads a published file.
Use import {name} from "cozy:lib/package" in code to invoke an export. Project locks choose versions.
open creates or resumes this session's private draft and returns editable source and tests. create_file adds an absent
index.ts, index.test.ts or package.json. Use read for editable lines, then edit with its hash and LINE#HASH anchors.
test runs type checks and unit tests using cozy:test and exact tool fixtures. diff previews the draft. publish validates
the supplied draft revision against its digest and creates an immutable package version. rebase reconciles concurrent work; pin explicitly selects
a published version in this project's lock for upgrades or rollback. package.json accepts description and exact package dependencies.`

type input struct {
	Versions map[string]string    `json:"versions"`
	Action   string               `json:"action"`
	Name     string               `json:"name"`
	Version  string               `json:"version"`
	Revision string               `json:"revision"`
	Query    string               `json:"query"`
	File     string               `json:"file"`
	Content  *string              `json:"content"`
	Hash     string               `json:"hash"`
	Edits    []writetool.FlatEdit `json:"edits"`
	Offset   int                  `json:"offset"`
	Limit    int                  `json:"limit"`
}

type binding struct {
	store       *funclib.Store
	session     string
	project     string
	actions     []string
	definitions []llm.ToolDefinition
	check       funclib.Validator
}

// Tool constructs a role-scoped library tool. A nil checker runs the production
// compiler and fixture runner. The ordinary dispatcher remains responsible for consent.
func Tool(store *funclib.Store, session, project string, writable bool,
	definitions []llm.ToolDefinition, check funclib.Validator,
) tooldef.Tool {
	b := binding{
		store: store, session: session, project: project,
		actions: tooldef.LibraryActions(writable), definitions: definitions, check: check,
	}
	desc := description
	if !writable {
		desc = "Explore the shared, versioned TypeScript library. search returns callable imports and tested behavior; " +
			"list shows project-resolved packages; get reads a published file. read, diff and test inspect this session's own draft."
	}
	return tooldef.Tool{
		Definition: llm.ToolDefinition{Name: "library", Description: desc, Params: params(b.actions)},
		Run:        b.run, DetailFromArgs: func(raw json.RawMessage) string {
			var in input
			if json.Unmarshal(raw, &in) != nil {
				return "library"
			}
			return in.detail()
		},
		ApprovalPreview: b.preview,
	}
}

func (in input) detail() string {
	if in.Action == "pin" && len(in.Versions) > 0 {
		return fmt.Sprintf("pin %d package versions", len(in.Versions))
	}
	name := in.Name
	if in.Version != "" {
		name += "@" + in.Version
	}
	return strings.TrimSpace(strings.Join([]string{in.Action, name, in.File}, " "))
}

func (b binding) run(ctx context.Context, raw json.RawMessage) (tooldef.Result, error) {
	var in input
	if err := json.Unmarshal(raw, &in); err != nil {
		return tooldef.Result{}, fmt.Errorf("decode library request: %w", err)
	}
	if !slices.Contains(b.actions, in.Action) {
		return tooldef.Result{}, fmt.Errorf("library action %q is unavailable in this session", in.Action)
	}
	if b.store == nil || b.session == "" || b.project == "" {
		return tooldef.Result{}, errors.New("library session or project binding unavailable")
	}
	if in.Offset < 0 || in.Limit < 0 {
		return tooldef.Result{}, errors.New("library offset and limit must be nonnegative")
	}
	value, err := b.execute(ctx, in)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("library %s: %w", in.Action, err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return tooldef.Result{}, fmt.Errorf("encode library result: %w", err)
	}
	return tooldef.Result{Content: string(body), Output: string(body), Structured: value, Detail: in.detail()}, nil
}

func (b binding) execute(ctx context.Context, in input) (any, error) {
	switch in.Action {
	case "list", "search":
		return b.search(ctx, in)
	case "versions":
		releases, err := b.store.Versions(ctx, in.Name)
		if err != nil {
			return nil, err
		}
		start, end := window(len(releases), in.Offset, in.Limit)
		return struct {
			Versions []funclib.Pin `json:"versions"`
			Total    int           `json:"total"`
		}{releases[start:end], len(releases)}, nil
	case "get":
		return b.published(ctx, in)
	case "open", "rebase":
		var pkg funclib.Package
		var err error
		if in.Action == "rebase" {
			pkg, err = b.store.Rebase(ctx, b.session, in.Name)
		} else {
			version, resolveErr := b.selectedVersion(ctx, in.Name, in.Version)
			if resolveErr != nil {
				return nil, resolveErr
			}
			pkg, err = b.store.Draft(ctx, b.session, in.Name, version)
		}
		if err != nil {
			return nil, err
		}
		return b.opened(ctx, pkg, in.Query)
	case "read":
		text, err := b.store.ReadRange(ctx, b.session, in.Name, in.File, in.Offset, in.Limit)
		return b.fileResult(ctx, in, text, err)
	case "create_file":
		if in.Content == nil {
			return nil, errors.New("create_file requires content")
		}
		text, err := b.store.CreateFile(ctx, b.session, in.Name, in.File, *in.Content)
		return b.fileResult(ctx, in, text, err)
	case "edit":
		text, err := b.store.Edit(ctx, b.session, in.Name, in.File, in.Hash, in.Edits)
		return b.fileResult(ctx, in, text, err)
	case "diff":
		pkg, text, err := b.store.DraftDiff(ctx, b.session, in.Name)
		if err != nil {
			return nil, err
		}
		return struct {
			Revision string `json:"revision"`
			page
		}{pkg.Digest, view(text, in.Offset, in.Limit)}, nil
	case "test":
		var report json.RawMessage
		var revision string
		check := func(ctx context.Context, pkg funclib.Package, modules map[string]string) error {
			revision = pkg.Digest
			var err error
			report, err = b.validate(ctx, pkg, modules)
			return err
		}
		if err := b.store.Check(ctx, b.session, in.Name, check); err != nil {
			return nil, err
		}
		return struct {
			Passed   bool            `json:"passed"`
			Revision string          `json:"revision"`
			Report   json.RawMessage `json:"report,omitempty"`
		}{true, revision, report}, nil
	case "publish":
		if in.Revision == "" {
			return nil, errors.New("publish requires the exact revision returned by open, read, edit, diff or test")
		}
		var related []funcadvisor.Card
		validate := func(ctx context.Context, candidate funclib.Package, modules map[string]string) error {
			if _, err := b.validate(ctx, candidate, modules); err != nil {
				return err
			}
			if candidate.Name == in.Name {
				related = b.related(ctx, candidate, in.Query)
			}
			return nil
		}
		pkg, err := b.store.PublishAt(ctx, b.session, in.Name, in.Version, in.Revision, validate)
		if err != nil {
			return nil, err
		}
		return struct {
			packageSummary
			Exports []funcadvisor.Card `json:"exports"`
			Related []funcadvisor.Card `json:"related,omitempty"`
		}{summary(pkg), funcadvisor.IndexRelease(b.store, pkg), related}, nil
	case "pin":
		if len(in.Versions) > 0 {
			if in.Name != "" || in.Version != "" {
				return nil, errors.New("pin accepts either name and version or a versions object")
			}
			return b.store.PinSet(ctx, b.project, in.Versions)
		}
		if in.Version == "" {
			return nil, errors.New("pin requires an explicit release version")
		}
		pkg, err := b.store.Pin(ctx, b.project, in.Name, in.Version)
		return summary(pkg), err
	default:
		return nil, fmt.Errorf("unknown library action %q", in.Action)
	}
}

// Check checks types and executes the release's unit tests inside a fixture-only runtime.
func Check(ctx context.Context, _ funclib.Package, modules map[string]string) error {
	_, err := runTests(ctx, modules, nil)
	return err
}

func (b binding) validate(
	ctx context.Context,
	pkg funclib.Package,
	modules map[string]string,
) (json.RawMessage, error) {
	if b.check != nil {
		return nil, b.check(ctx, pkg, modules)
	}
	return runTests(ctx, modules, b.definitions)
}

func runTests(
	ctx context.Context,
	modules map[string]string,
	definitions []llm.ToolDefinition,
) (json.RawMessage, error) {
	program := codemode.Program{Modules: modules, Tools: definitions}
	if err := codemode.Check(ctx, program); err != nil {
		return nil, fmt.Errorf("package type check: %w", err)
	}
	return codemode.Test(ctx, program, nil)
}

func (b binding) opened(ctx context.Context, pkg funclib.Package, query string) (any, error) {
	files := make(map[string]string)
	for _, file := range []string{"index.ts", "index.test.ts", "package.json"} {
		text, err := b.store.ReadEditable(ctx, b.session, pkg.Name, file)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		files[file] = text
	}
	return struct {
		Package packageSummary     `json:"package"`
		Files   map[string]string  `json:"editableFiles"`
		Related []funcadvisor.Card `json:"related,omitempty"`
	}{summary(pkg), files, b.related(ctx, pkg, query)}, nil
}

func (b binding) fileResult(ctx context.Context, in input, text string, editErr error) (any, error) {
	if editErr != nil {
		return nil, editErr
	}
	pkg, err := b.store.DraftState(ctx, b.session, in.Name)
	if err != nil {
		return nil, err
	}
	return struct {
		Revision string `json:"revision"`
		Editable string `json:"editable"`
	}{pkg.Digest, text}, nil
}

func (b binding) preview(ctx context.Context, raw json.RawMessage) string {
	var in input
	if b.store == nil || json.Unmarshal(raw, &in) != nil || in.Action != "publish" {
		return ""
	}
	pkg, diff, err := b.store.DraftDiff(ctx, b.session, in.Name)
	if err != nil || in.Revision == "" || pkg.Digest != in.Revision {
		return "Draft revision unavailable or changed. Publication requires a matching draft digest."
	}
	return redact.Redact(util.TruncateBytes(diff, 32<<10))
}

func (b binding) search(ctx context.Context, in input) (any, error) {
	catalog, err := b.store.Catalog(ctx, b.project)
	if err != nil {
		return nil, err
	}
	start, end := window(len(catalog), in.Offset, in.Limit)
	if in.Action == "search" {
		_, count := window(100, 0, in.Limit)
		return funcadvisor.Search(catalog, in.Query, count), nil
	}
	items := make([]packageSummary, 0, end-start)
	for _, pkg := range catalog[start:end] {
		items = append(items, summary(pkg))
	}
	return struct {
		Packages []packageSummary `json:"packages"`
		Total    int              `json:"total"`
	}{items, len(catalog)}, nil
}

func window(total, offset, limit int) (int, int) {
	if limit == 0 {
		limit = 20
	}
	start := min(max(0, offset-1), total)
	return start, start + min(total-start, limit, 100)
}

// A new draft and a published lookup both default to the caller's resolution.
func (b binding) selectedVersion(ctx context.Context, name, version string) (string, error) {
	if version != "" {
		return version, nil
	}
	catalog, err := b.store.Catalog(ctx, b.project)
	if err != nil {
		return "", err
	}
	for _, p := range catalog {
		if p.Name == name {
			return p.Version, nil
		}
	}
	return "", nil
}

func (b binding) related(ctx context.Context, pkg funclib.Package, query string) []funcadvisor.Card {
	packages, err := b.store.List(ctx)
	if err != nil {
		return nil // Optional hints cannot prevent editing or publication.
	}
	packages = slices.DeleteFunc(packages, func(p funclib.Package) bool { return p.Name == pkg.Name })
	fragments := []string{query, pkg.Name, pkg.Description}
	for _, c := range funcadvisor.Exports(pkg) {
		fragments = append(fragments, c.Name, c.Test)
	}
	return funcadvisor.Search(packages, strings.Join(fragments, " "), 2)
}

func (b binding) published(ctx context.Context, in input) (any, error) {
	version, err := b.selectedVersion(ctx, in.Name, in.Version)
	if err != nil {
		return nil, err
	}
	pkg, err := b.store.Get(ctx, in.Name, version)
	if err != nil {
		return nil, err
	}
	switch in.File {
	case "":
		return struct {
			Package packageSummary     `json:"package"`
			Exports []funcadvisor.Card `json:"exports"`
		}{summary(pkg), funcadvisor.Exports(pkg)}, nil
	case "index.ts":
		return view(pkg.Source, in.Offset, in.Limit), nil
	case "index.test.ts":
		return view(pkg.Tests, in.Offset, in.Limit), nil
	case "package.json":
		return summary(pkg), nil
	default:
		return nil, fmt.Errorf("unknown library file %q", in.File)
	}
}

type packageSummary struct {
	Name         string            `json:"name"`
	Version      string            `json:"version,omitempty"`
	Digest       string            `json:"digest,omitempty"`
	Description  string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
}

func summary(p funclib.Package) packageSummary {
	return packageSummary{p.Name, p.Version, p.Digest, p.Description, p.Dependencies}
}

type page struct {
	Text          string `json:"text"`
	NextLine      int    `json:"nextLine,omitempty"`
	TruncatedLine bool   `json:"truncatedLine,omitempty"`
}

func view(content string, offset, limit int) page {
	const maxBytes = 32 << 10
	lines := strings.Split(content, "\n")
	start := min(max(0, offset-1), len(lines))
	if limit == 0 {
		limit = 100
	}
	end := start + min(limit, 500, len(lines)-start)
	var out strings.Builder
	var result page
	for i := start; i < end; i++ {
		line := lines[i]
		if out.Len()+len(line)+1 > maxBytes && i > start {
			end = i
			break
		}
		if i > start {
			out.WriteByte('\n')
		}
		if len(line) > maxBytes {
			line = util.TruncateBytes(line, maxBytes)
			result.TruncatedLine = true
		}
		out.WriteString(line)
	}
	result.Text = out.String()
	if end < len(lines) {
		result.NextLine = end + 1
	}
	return result
}
