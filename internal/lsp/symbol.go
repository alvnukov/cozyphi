package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// symbolKinds maps LSP SymbolKind numbers to the stable names exposed by the
// frozen contract. Unknown numbers stay visible as unknown:<n> instead of
// being silently coerced to a neighbor.
var symbolKinds = [...]string{
	"file", "module", "namespace", "package", "class", "method", "property",
	"field", "constructor", "enum", "interface", "function", "variable",
	"constant", "string", "number", "boolean", "array", "object", "key",
	"null", "enummember", "struct", "event", "operator", "typeparameter",
}

func symbolKindName(kind int) string {
	if kind < 1 || kind > len(symbolKinds) {
		return fmt.Sprintf("unknown:%d", kind)
	}
	return symbolKinds[kind-1]
}

// flatSymbol is one flattened navigation symbol from either wire form.
type flatSymbol struct {
	name      string
	detail    string
	kind      int
	container string
	uri       string
	selection wireRange
}

// decodeDocumentSymbols flattens a DocumentSymbol[] or SymbolInformation[]
// reply for fileURI in document order (parents before children).
func decodeDocumentSymbols(raw json.RawMessage, fileURI string) ([]flatSymbol, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, nil
	}
	if raw[0] != '[' {
		return nil, newError(ErrProtocol, "documentSymbol: unexpected payload")
	}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, newError(ErrProtocol, "documentSymbol: %v", err)
	}
	var out []flatSymbol
	for _, item := range arr {
		switch {
		case jsonHas(item, "selectionRange"):
			var d wireDocumentSymbol
			if err := json.Unmarshal(item, &d); err != nil {
				return nil, newError(ErrProtocol, "documentSymbol: %v", err)
			}
			out = appendSymbolTree(out, d, "", fileURI)
		case jsonHas(item, "location"):
			var s wireSymbolInformation
			if err := json.Unmarshal(item, &s); err != nil {
				return nil, newError(ErrProtocol, "documentSymbol: %v", err)
			}
			if s.Location == nil {
				return nil, newError(ErrProtocol, "documentSymbol: flat symbol without location")
			}
			out = append(out, flatSymbol{
				name: s.Name, kind: s.Kind, container: s.ContainerName,
				uri: s.Location.URI, selection: s.Location.Range,
			})
		default:
			return nil, newError(ErrProtocol, "documentSymbol: unknown item shape")
		}
	}
	return out, nil
}

// appendSymbolTree flattens one hierarchy pre-order so parents precede their
// children deterministically.
func appendSymbolTree(out []flatSymbol, d wireDocumentSymbol, container, uri string) []flatSymbol {
	out = append(out, flatSymbol{
		name: d.Name, detail: d.Detail, kind: d.Kind,
		container: container, uri: uri, selection: d.SelectionRange,
	})
	for _, child := range d.Children {
		out = appendSymbolTree(out, child, d.Name, uri)
	}
	return out
}

// decodeWorkspaceSymbols flattens a SymbolInformation[] or WorkspaceSymbol[]
// reply. Items without a location are counted, not returned: they can be
// neither contained nor positioned.
func decodeWorkspaceSymbols(raw json.RawMessage) (flats []flatSymbol, skipped int, err error) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, 0, nil
	}
	if raw[0] != '[' {
		return nil, 0, newError(ErrProtocol, "workspace/symbol: unexpected payload")
	}
	var arr []wireSymbolInformation
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, 0, newError(ErrProtocol, "workspace/symbol: %v", err)
	}
	for _, s := range arr {
		if s.Location == nil {
			skipped++
			continue
		}
		flats = append(flats, flatSymbol{
			name: s.Name, kind: s.Kind, container: s.ContainerName,
			uri: s.Location.URI, selection: s.Location.Range,
		})
	}
	return flats, skipped, nil
}

// symbols implements the file (documentSymbol) and query (workspace/symbol)
// modes behind one normalized, bounded, deduplicated symbol list.
func (m *Manager) symbols(ctx context.Context, c *client, q Query) (Result, error) {
	var flats []flatSymbol
	skipped := 0
	if q.File != "" {
		if err := c.requireCapability("documentSymbolProvider", q.Op); err != nil {
			return Result{}, err
		}
		if _, err := c.syncDocument(ctx, q.File); err != nil {
			return Result{}, err
		}
		raw, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
			"textDocument": map[string]any{"uri": uriFromPath(q.File)},
		})
		if err != nil {
			return Result{}, requestError(ctx, q.Op, err)
		}
		flats, err = decodeDocumentSymbols(raw, uriFromPath(q.File))
		if err != nil {
			return Result{}, err
		}
		// file plus query narrows the outline instead of conflicting with it.
		if q.Query != "" {
			needle := strings.ToLower(q.Query)
			kept := flats[:0]
			for _, fs := range flats {
				if strings.Contains(strings.ToLower(fs.container+"."+fs.name), needle) {
					kept = append(kept, fs)
				}
			}
			flats = kept
		}
	} else {
		if err := c.requireCapability("workspaceSymbolProvider", q.Op); err != nil {
			return Result{}, err
		}
		raw, err := c.request(ctx, "workspace/symbol", map[string]any{"query": q.Query})
		if err != nil {
			return Result{}, requestError(ctx, q.Op, err)
		}
		flats, skipped, err = decodeWorkspaceSymbols(raw)
		if err != nil {
			return Result{}, err
		}
	}

	syms := make([]Symbol, 0, len(flats))
	outside := 0
	for _, fs := range flats {
		sym, ok, err := m.normalizeSymbol(q.Op, fs)
		if err != nil {
			return Result{}, err
		}
		if !ok {
			outside++
			continue
		}
		syms = append(syms, sym)
	}
	bounded, omitted := finalize(syms, q.Limit, compareSymbol)
	res := Result{Symbols: bounded, Omitted: omitted + outside + skipped, Truncated: omitted > 0}
	if outside > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("omitted %d symbol(s) outside the workspace", outside))
	}
	if skipped > 0 {
		res.Warnings = append(res.Warnings, fmt.Sprintf("omitted %d symbol(s) without a location", skipped))
	}
	return res, nil
}

// normalizeSymbol bounds one flattened symbol and locates it. ok=false means
// the symbol's file fell outside the workspace. Name and detail are capped at
// MaxTextFieldBytes; a cut that extreme never identifies a real symbol.
func (m *Manager) normalizeSymbol(op Operation, fs flatSymbol) (Symbol, bool, error) {
	loc, _, ok, err := locate(m.workspace, op, wireLocation{URI: fs.uri, Range: fs.selection})
	if err != nil || !ok {
		return Symbol{}, ok, err
	}
	name, _ := boundText(fs.name)
	detail, _ := boundText(fs.detail)
	return Symbol{
		Name:      name,
		Kind:      symbolKindName(fs.kind),
		Detail:    detail,
		Container: fs.container,
		Location:  loc,
	}, true, nil
}

// resolveTarget resolves the query target to a 0-based UTF-16 wire position
// inside q.File. Position mode converts the 1-based code-point contract
// through the synced snapshot. Symbol mode resolves the flattened document
// symbols — a plain name or a qualified Container.Name — and tolerates the
// natural over- and under-specification: a position refines an ambiguous
// symbol to its nearest declaration, and a symbol that is not declared in the
// file falls back to the given position or to the name's occurrence in the
// text, so a name only used in the file still resolves. Several declarations
// without a line hint either return candidate locations (definition,
// references) or fail with the typed ambiguity error (hover, calls).
// done=true means res is the final result for this query.
func (m *Manager) resolveTarget(
	ctx context.Context,
	c *client,
	q Query,
	locCandidates bool,
) (wirePosition, bool, Result, error) {
	if q.Symbol == "" {
		snap, err := c.syncDocument(ctx, q.File)
		if err != nil {
			return wirePosition{}, false, Result{}, err
		}
		line := lineText(snap.text, q.Line)
		return wirePosition{Line: q.Line - 1, Character: utf16Column(line, q.Character)}, false, Result{}, nil
	}
	if err := c.requireCapability("documentSymbolProvider", q.Op); err != nil {
		return wirePosition{}, false, Result{}, err
	}
	snap, err := c.syncDocument(ctx, q.File)
	if err != nil {
		return wirePosition{}, false, Result{}, err
	}
	raw, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]any{"uri": uriFromPath(q.File)},
	})
	if err != nil {
		return wirePosition{}, false, Result{}, requestError(ctx, q.Op, err)
	}
	flats, err := decodeDocumentSymbols(raw, uriFromPath(q.File))
	if err != nil {
		return wirePosition{}, false, Result{}, err
	}
	var matches []flatSymbol
	for _, fs := range flats {
		if symbolMatches(fs, q.Symbol) {
			matches = append(matches, fs)
		}
	}
	switch len(matches) {
	case 0:
		return occurrenceTarget(q, snap.text)
	case 1:
		return matches[0].selection.Start, false, Result{}, nil
	}
	if q.Line > 0 {
		return nearestSelection(matches, q.Line), false, Result{}, nil
	}
	if !locCandidates {
		return wirePosition{}, false, Result{}, newError(
			ErrAmbiguous, "%s: symbol %q has %d declarations in %s; add line to pick one or qualify as Container.Name",
			q.Op, q.Symbol, len(matches), filepath.Base(q.File),
		)
	}
	cands := make([]Location, 0, len(matches))
	for _, fs := range matches {
		loc, _, ok, err := locate(m.workspace, q.Op, wireLocation{URI: uriFromPath(q.File), Range: fs.selection})
		if err != nil {
			return wirePosition{}, false, Result{}, err
		}
		if ok {
			cands = append(cands, loc)
		}
	}
	bounded, omitted := finalize(cands, q.Limit, compareLocation)
	m.attachSnippets(bounded)
	return wirePosition{}, true, Result{
		Locations: bounded,
		Omitted:   omitted,
		Truncated: omitted > 0,
		Warnings:  []string{fmt.Sprintf("ambiguous symbol %q: %d candidate declaration(s)", q.Symbol, len(matches))},
	}, nil
}

// symbolMatches accepts the plain declaration name and its qualified
// Container.Name form, so a model that copies "Manager.symbols" from a
// previous answer targets the method directly.
func symbolMatches(fs flatSymbol, symbol string) bool {
	if fs.name == symbol {
		return true
	}
	return fs.container != "" && fs.container+"."+fs.name == symbol
}

// nearestSelection picks the declaration whose selection starts closest to
// the 1-based hint line; document order breaks ties.
func nearestSelection(matches []flatSymbol, line int) wirePosition {
	best := matches[0]
	bestDist := absInt(best.selection.Start.Line - (line - 1))
	for _, fs := range matches[1:] {
		if d := absInt(fs.selection.Start.Line - (line - 1)); d < bestDist {
			best, bestDist = fs, d
		}
	}
	return best.selection.Start
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// occurrenceTarget resolves a symbol that is not declared in the file: an
// exact position wins, then the name's occurrence on the given line, then its
// first occurrence anywhere in the text. done=true with a warning when the
// name never appears, so the empty answer says why it is empty.
func occurrenceTarget(q Query, text string) (wirePosition, bool, Result, error) {
	if q.Line > 0 && q.Character > 0 {
		line := lineText(text, q.Line)
		return wirePosition{Line: q.Line - 1, Character: utf16Column(line, q.Character)}, false, Result{}, nil
	}
	if q.Line > 0 {
		line := lineText(text, q.Line)
		if col, ok := occurrenceColumn(line, q.Symbol); ok {
			return wirePosition{Line: q.Line - 1, Character: utf16Column(line, col)}, false, Result{}, nil
		}
	}
	for i, line := range strings.Split(text, "\n") {
		if col, ok := occurrenceColumn(line, q.Symbol); ok {
			return wirePosition{Line: i, Character: utf16Column(line, col)}, false, Result{}, nil
		}
	}
	return wirePosition{}, true, Result{
		Warnings: []string{fmt.Sprintf("symbol %q does not appear in %s", q.Symbol, filepath.Base(q.File))},
	}, nil
}

// occurrenceColumn returns the 1-based code-point column of the first
// identifier-bounded occurrence of symbol's name in line. A qualified name
// searches for its last segment, which is what the source spells at use
// sites.
func occurrenceColumn(line, symbol string) (int, bool) {
	name := symbol
	if i := strings.LastIndex(symbol, "."); i >= 0 {
		name = symbol[i+1:]
	}
	if name == "" {
		return 0, false
	}
	for from := 0; ; {
		idx := strings.Index(line[from:], name)
		if idx < 0 {
			return 0, false
		}
		idx += from
		if identBounded(line, idx, len(name)) {
			return utf8.RuneCountInString(line[:idx]) + 1, true
		}
		from = idx + len(name)
	}
}

// identBounded reports that line[idx:idx+n] is not embedded in a larger
// identifier.
func identBounded(line string, idx, n int) bool {
	if idx > 0 {
		if r, _ := utf8.DecodeLastRuneInString(line[:idx]); identRune(r) {
			return false
		}
	}
	if idx+n < len(line) {
		if r, _ := utf8.DecodeRuneInString(line[idx+n:]); identRune(r) {
			return false
		}
	}
	return true
}

func identRune(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// resolveWorkspaceSymbol turns a file-less navigational symbol into a
// concrete file position through workspace/symbol on the workspace-root
// client. A unique declaration continues the query at its location; anything
// else is final: several declarations return the candidates to pick from
// (locations for definition — they are its answer — and symbols otherwise),
// and no exact match returns the nearest workspace symbols with a warning
// instead of a silent miss.
func (m *Manager) resolveWorkspaceSymbol(ctx context.Context, q Query) (Query, bool, Result, error) {
	ctx, cancel := context.WithTimeout(ctx, workspaceSymbolTimeout)
	defer cancel()
	c, err := m.clientFor(ctx, m.workspace)
	if err != nil {
		return q, false, Result{}, err
	}
	if err := c.requireCapability("workspaceSymbolProvider", q.Op); err != nil {
		return q, false, Result{}, err
	}
	raw, err := c.request(ctx, "workspace/symbol", map[string]any{"query": q.Symbol})
	if err != nil {
		return q, false, Result{}, requestError(ctx, q.Op, err)
	}
	flats, _, err := decodeWorkspaceSymbols(raw)
	if err != nil {
		return q, false, Result{}, err
	}
	var exact, folded, near []Symbol
	for _, fs := range flats {
		sym, ok, err := m.normalizeSymbol(q.Op, fs)
		if err != nil {
			return q, false, Result{}, err
		}
		if !ok {
			continue
		}
		switch {
		case symbolMatches(fs, q.Symbol):
			exact = append(exact, sym)
		case strings.EqualFold(fs.name, q.Symbol):
			folded = append(folded, sym)
		default:
			near = append(near, sym)
		}
	}
	if len(exact) == 0 {
		exact = folded
	}
	switch len(exact) {
	case 0:
		res := Result{Warnings: []string{fmt.Sprintf("no declaration named %q in the workspace", q.Symbol)}}
		if len(near) > 0 {
			res.Symbols, res.Omitted = finalize(near, q.Limit, compareSymbol)
			res.Truncated = res.Omitted > 0
			res.Warnings = []string{fmt.Sprintf("no declaration named %q; nearest workspace symbols listed", q.Symbol)}
		}
		return q, true, res, nil
	case 1:
		loc := exact[0].Location
		q.File = filepath.Join(m.workspace, filepath.FromSlash(loc.File))
		q.Line = loc.Line
		q.Character = loc.Character
		q.Symbol = ""
		return q, false, Result{}, nil
	}
	warning := fmt.Sprintf(
		"symbol %q has %d declarations in the workspace; pass file to pick one", q.Symbol, len(exact),
	)
	if q.Op == OpDefinition {
		locs := make([]Location, 0, len(exact))
		for _, sym := range exact {
			locs = append(locs, sym.Location)
		}
		bounded, omitted := finalize(locs, q.Limit, compareLocation)
		m.attachSnippets(bounded)
		return q, true, Result{
			Locations: bounded, Omitted: omitted, Truncated: omitted > 0, Warnings: []string{warning},
		}, nil
	}
	bounded, omitted := finalize(exact, q.Limit, compareSymbol)
	return q, true, Result{
		Symbols: bounded, Omitted: omitted, Truncated: omitted > 0, Warnings: []string{warning},
	}, nil
}
