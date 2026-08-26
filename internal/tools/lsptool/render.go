package lsptool

import (
	"fmt"
	"strings"

	"github.com/alvnukov/cozyphi/internal/lsp"
)

// render turns a normalized Result into bounded model-facing text. It is the
// only path from lsp.Result into the transcript, so raw protocol data cannot
// leak here.
func render(op lsp.Operation, res lsp.Result) string {
	var b strings.Builder
	switch op {
	case lsp.OpDefinition, lsp.OpReferences:
		renderLocations(&b, op, res)
	case lsp.OpHover:
		renderHover(&b, res)
	case lsp.OpSymbols:
		renderSymbols(&b, res)
	case lsp.OpCalls:
		renderCalls(&b, res)
	case lsp.OpDiagnostics:
		renderDiagnostics(&b, res)
	case lsp.OpLanguages:
		renderLanguages(&b, res)
	}
	appendWarnings(&b, res)
	return bound(b.String(), lsp.MaxOutputBytes)
}

func renderLocations(b *strings.Builder, op lsp.Operation, res lsp.Result) {
	if len(res.Locations) == 0 {
		fmt.Fprintf(b, "%s: no results\n", op)
		return
	}
	fmt.Fprintf(b, "%s: %d location(s)\n", op, len(res.Locations))
	for _, l := range res.Locations {
		fmt.Fprintf(b, "%s:%d:%d-%d:%d\n", l.File, l.Line, l.Character, l.EndLine, l.EndCharacter)
	}
}

func renderHover(b *strings.Builder, res lsp.Result) {
	if res.Hover == nil {
		b.WriteString("hover: no results\n")
		return
	}
	h := res.Hover
	if h.Line > 0 {
		fmt.Fprintf(b, "hover @ %d:%d\n", h.Line, h.Character)
	}
	b.WriteString(h.Text)
	if !strings.HasSuffix(h.Text, "\n") {
		b.WriteByte('\n')
	}
}

func renderSymbols(b *strings.Builder, res lsp.Result) {
	if len(res.Symbols) == 0 {
		b.WriteString("symbols: no results\n")
		return
	}
	fmt.Fprintf(b, "symbols: %d result(s)\n", len(res.Symbols))
	for _, s := range res.Symbols {
		fmt.Fprintf(b, "%s (%s) @ %s:%d:%d\n", s.Name, s.Kind, s.Location.File, s.Location.Line, s.Location.Character)
	}
}

func renderCalls(b *strings.Builder, res lsp.Result) {
	if len(res.Calls) == 0 {
		b.WriteString("calls: no results\n")
		return
	}
	fmt.Fprintf(b, "calls: %d result(s)\n", len(res.Calls))
	for _, c := range res.Calls {
		fmt.Fprintf(
			b,
			"%s -> %s @ %s:%d:%d\n",
			c.From.Name,
			c.To.Name,
			c.Location.File,
			c.Location.Line,
			c.Location.Character,
		)
	}
}

func renderDiagnostics(b *strings.Builder, res lsp.Result) {
	status := res.Status
	if status == "" {
		status = lsp.StatusFresh
	}
	if len(res.Diagnostics) == 0 {
		fmt.Fprintf(b, "diagnostics: none (%s)\n", status)
		return
	}
	fmt.Fprintf(b, "diagnostics: %d result(s) (%s)\n", len(res.Diagnostics), status)
	for _, d := range res.Diagnostics {
		fmt.Fprintf(b, "%s: %s @ %s:%d:%d\n", d.Severity, d.Message, d.File, d.Line, d.Character)
	}
}

func renderLanguages(b *strings.Builder, res lsp.Result) {
	if len(res.Languages) == 0 {
		b.WriteString("languages: none configured\n")
		return
	}
	for _, l := range res.Languages {
		fmt.Fprintf(b, "%s/%s configured=%t installed=%t running=%t roots=%d\n",
			l.Language, l.Server, l.Configured, l.Installed, l.Running, l.ActiveRoots)
		if l.Error != "" {
			fmt.Fprintf(b, "error: %s\n", l.Error)
		}
		if l.InstallHint != "" {
			fmt.Fprintf(b, "install: %s\n", l.InstallHint)
		}
	}
}

func appendWarnings(b *strings.Builder, res lsp.Result) {
	if res.Truncated {
		b.WriteString("truncated: result exceeded limits\n")
	}
	if res.Omitted > 0 {
		fmt.Fprintf(b, "omitted: %d result(s)\n", res.Omitted)
	}
	for _, w := range res.Warnings {
		fmt.Fprintf(b, "warning: %s\n", w)
	}
}

// bound truncates output to max bytes, never leaving a partial surrogate
// problem since it works on raw bytes of already-valid UTF-8 text.
func bound(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n... truncated"
}
