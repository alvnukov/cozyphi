//go:build integration

package lsp

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

// TestRealGoplsSmoke exercises the production seams against a real gopls
// binary: Open -> languages (no process) -> definition (spawns a generation).
// Run explicitly with: go test -tags integration -run TestRealGoplsSmoke ./internal/lsp/
func TestRealGoplsSmoke(t *testing.T) {
	ws := filepath.Clean(filepath.Join(".", "..", ".."))
	abs, err := filepath.Abs(ws)
	if err != nil {
		t.Fatal(err)
	}

	cfg := DefaultConfig()
	cfg.Gopls.Command = []string{"gopls"}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	m, err := Open(ctx, abs, cfg)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() {
		cctx, ccancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer ccancel()
		_ = m.Close(cctx)
	}()

	res, err := m.Query(ctx, Query{Op: OpLanguages})
	if err != nil {
		t.Fatalf("languages: %v", err)
	}
	if len(res.Languages) != 1 || !res.Languages[0].Installed {
		t.Fatalf("gopls not resolvable: %+v", res.Languages)
	}
	t.Logf("languages: %+v", res.Languages[0])

	// 69:6 is the identifier LoadConfig; 69:1 is the func keyword, which gopls
	// correctly reports as "not an identifier".
	file := filepath.Join(abs, "internal", "lsp", "config.go")
	res, err = m.Query(ctx, Query{Op: OpDefinition, File: file, Line: 69, Character: 6})
	if err != nil {
		t.Fatalf("definition: %v", err)
	}
	if len(res.Locations) == 0 {
		t.Fatal("definition returned no locations")
	}
	t.Logf("definition -> %s:%d:%d", res.Locations[0].File, res.Locations[0].Line, res.Locations[0].Character)

	// Symbol-target resolution must land on the identifier, not the func
	// keyword: with hierarchical document symbols gopls returns a precise
	// selectionRange. This pins the capability we advertise.
	res, err = m.Query(ctx, Query{Op: OpDefinition, File: file, Symbol: "LoadConfig"})
	if err != nil {
		t.Fatalf("definition by symbol: %v", err)
	}
	if len(res.Locations) == 0 {
		t.Fatal("definition by symbol returned no locations")
	}
	if res.Locations[0].Line != 69 || res.Locations[0].Character != 6 {
		t.Fatalf("definition by symbol = %s:%d:%d, want 69:6",
			res.Locations[0].File, res.Locations[0].Line, res.Locations[0].Character)
	}
	t.Logf("definition by symbol -> %s:%d:%d", res.Locations[0].File, res.Locations[0].Line, res.Locations[0].Character)

	res, err = m.Query(ctx, Query{Op: OpHover, File: file, Symbol: "LoadConfig"})
	if err != nil {
		t.Fatalf("hover by symbol: %v", err)
	}
	if res.Hover == nil || res.Hover.Text == "" {
		t.Fatal("hover by symbol returned no text")
	}
	t.Logf("hover by symbol: %d chars", len(res.Hover.Text))

	res, err = m.Query(ctx, Query{Op: OpReferences, File: file, Symbol: "LoadConfig"})
	if err != nil {
		t.Fatalf("references by symbol: %v", err)
	}
	if len(res.Locations) == 0 {
		t.Fatal("references by symbol returned no locations")
	}
	t.Logf("references by symbol: %d locations", len(res.Locations))

	res, err = m.Query(ctx, Query{Op: OpCalls, File: file, Symbol: "LoadConfig", Direction: DirectionIncoming})
	if err != nil {
		t.Fatalf("calls incoming by symbol: %v", err)
	}
	if len(res.Calls) == 0 {
		t.Fatal("calls incoming by symbol returned no edges")
	}
	t.Logf("calls incoming by symbol: %d edges", len(res.Calls))

	// gopls names methods "(*Manager).Query" in documentSymbol output; the bare
	// name and the Manager.Query spelling must resolve the method itself
	// instead of falling back to a doc comment occurrence.
	mgrFile := filepath.Join(abs, "internal", "lsp", "manager.go")
	res, err = m.Query(ctx, Query{Op: OpDefinition, File: mgrFile, Symbol: "Query"})
	if err != nil {
		t.Fatalf("definition by method symbol: %v", err)
	}
	if len(res.Locations) == 0 {
		t.Fatal("definition by method symbol returned no locations")
	}
	if res.Locations[0].Line != 84 || res.Locations[0].Character != 19 {
		t.Fatalf("definition by method symbol = %s:%d:%d, want 84:19",
			res.Locations[0].File, res.Locations[0].Line, res.Locations[0].Character)
	}
	t.Logf(
		"definition by method symbol -> %s:%d:%d",
		res.Locations[0].File,
		res.Locations[0].Line,
		res.Locations[0].Character,
	)

	res, err = m.Query(ctx, Query{Op: OpHover, File: mgrFile, Symbol: "Query"})
	if err != nil {
		t.Fatalf("hover by method symbol: %v", err)
	}
	if res.Hover == nil || res.Hover.Text == "" {
		t.Fatal("hover by method symbol returned no text")
	}
	t.Logf("hover by method symbol: %d chars", len(res.Hover.Text))

	res, err = m.Query(ctx, Query{Op: OpCalls, File: mgrFile, Symbol: "Manager.Query", Direction: DirectionIncoming})
	if err != nil {
		t.Fatalf("calls incoming by method symbol: %v", err)
	}
	if len(res.Calls) == 0 {
		t.Fatal("calls incoming by method symbol returned no edges")
	}
	t.Logf("calls incoming by method symbol: %d edges", len(res.Calls))

	// References, hover, document symbols, and diagnostics all ride the same
	// synced generation; exercising them end-to-end proves the seam against a
	// real gopls, not just the fake server.
	res, err = m.Query(ctx, Query{Op: OpReferences, File: file, Line: 69, Character: 6})
	if err != nil {
		t.Fatalf("references: %v", err)
	}
	if len(res.Locations) == 0 {
		t.Fatal("references returned no locations")
	}
	t.Logf("references: %d locations", len(res.Locations))

	res, err = m.Query(ctx, Query{Op: OpHover, File: file, Line: 69, Character: 6})
	if err != nil {
		t.Fatalf("hover: %v", err)
	}
	if res.Hover == nil || res.Hover.Text == "" {
		t.Fatal("hover returned no text")
	}
	t.Logf("hover: %d chars", len(res.Hover.Text))

	res, err = m.Query(ctx, Query{Op: OpSymbols, File: file})
	if err != nil {
		t.Fatalf("symbols: %v", err)
	}
	if len(res.Symbols) == 0 {
		t.Fatal("symbols returned none")
	}
	t.Logf("symbols: %d", len(res.Symbols))

	res, err = m.Query(ctx, Query{Op: OpDiagnostics, File: file})
	if err != nil {
		t.Fatalf("diagnostics: %v", err)
	}
	if res.Status == "" {
		t.Fatal("diagnostics returned no status")
	}
	t.Logf("diagnostics: status=%s count=%d", res.Status, len(res.Diagnostics))

	// Call hierarchy rides the same synced generation and pins the wire method
	// names (textDocument/prepareCallHierarchy + incomingCalls/outgoingCalls)
	// that gopls v0.23.0 actually registers.
	res, err = m.Query(ctx, Query{Op: OpCalls, File: file, Line: 69, Character: 6, Direction: DirectionIncoming})
	if err != nil {
		t.Fatalf("calls incoming: %v", err)
	}
	if len(res.Calls) == 0 {
		t.Fatal("calls incoming returned no edges")
	}
	t.Logf("calls incoming: %d edges", len(res.Calls))

	res, err = m.Query(ctx, Query{Op: OpCalls, File: file, Line: 69, Character: 6, Direction: DirectionOutgoing})
	if err != nil {
		t.Fatalf("calls outgoing: %v", err)
	}
	if len(res.Calls) == 0 {
		t.Fatal("calls outgoing returned no edges")
	}
	t.Logf("calls outgoing: %d edges", len(res.Calls))
}
