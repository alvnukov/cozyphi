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
}
