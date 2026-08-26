package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
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
// symbols: no match is a successful empty result, a unique match yields its
// selection start, and several matches either return candidate locations
// (definition, references) or fail with the typed ambiguity error (hover,
// calls). done=true means res is the final result for this query.
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
	if _, err := c.syncDocument(ctx, q.File); err != nil {
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
		if fs.name == q.Symbol {
			matches = append(matches, fs)
		}
	}
	switch len(matches) {
	case 0:
		return wirePosition{}, true, Result{}, nil
	case 1:
		return matches[0].selection.Start, false, Result{}, nil
	}
	if !locCandidates {
		return wirePosition{}, false, Result{}, newError(
			ErrAmbiguous, "%s: symbol %q has %d declarations in %s",
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
	return wirePosition{}, true, Result{
		Locations: bounded,
		Omitted:   omitted,
		Truncated: omitted > 0,
		Warnings:  []string{fmt.Sprintf("ambiguous symbol %q: %d candidate declaration(s)", q.Symbol, len(matches))},
	}, nil
}
