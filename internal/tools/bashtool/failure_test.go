package bashtool

import (
	"strings"
	"testing"
)

func TestFailureMarkersMatchToolchainsOnly(t *testing.T) {
	positives := []string{
		"--- FAIL: TestFoo (0.01s)",
		"FAIL\tgithub.com/alvnukov/cozyphi/internal/x\t0.532s",
		"FAIL",
		"internal/x/y.go:12:3: undefined: foo",
		"internal/x/y.go:12:3: unused variable (unused)",
		"panic: test timed out after 10m0s",
		"src/main.c:3:5: error: expected ';'",
		"src/main.c:3:5: fatal error: missing.h: No such file",
		"src/app.ts(3,5): error TS2322: Type 'string' is not assignable",
		"error[E0308]: mismatched types",
		"error: could not compile `x`",
		"Error: ENOENT: no such file or directory",
		"FAILED tests/test_x.py::test_a - AssertionError",
		"Traceback (most recent call last):",
		"ValueError: invalid literal for int()",
		"requests.exceptions.ConnectionError: refused",
		"make: *** [Makefile:12: build] Error 2",
		"fatal: not a git repository",
	}
	for _, line := range positives {
		if !isFailureMarker(line) {
			t.Errorf("expected marker: %q", line)
		}
	}
	negatives := []string{
		"ok  \tgithub.com/alvnukov/cozyphi/internal/x\t0.310s",
		"--- PASS: TestFoo (0.00s)",
		"PASS",
		"=== RUN   TestFoo",
		"    Error Trace:\tx_test.go:12",
		"\tError:      \tNot equal",
		"error handling improved in this release",
		"errors.New(\"boom\")",
		"go: downloading golang.org/x/tools v0.1.0",
		"npm WARN deprecated left-pad",
		"Failed to fetch, retrying",
		"README.md: 12 lines",
		"",
	}
	for _, line := range negatives {
		if isFailureMarker(line) {
			t.Errorf("unexpected marker: %q", line)
		}
	}
}

const goTestFailure = `=== RUN   TestFoo
--- FAIL: TestFoo (0.01s)
    foo_test.go:42: expected 1, got 2
    foo_test.go:43: and this
=== RUN   TestBar
--- PASS: TestBar (0.00s)
FAIL
FAIL	github.com/x/y	0.532s
ok  	github.com/x/z	0.010s
`

func TestScanFailuresCollectsMarkersWithContextAndNumbers(t *testing.T) {
	scan := scanFailures(goTestFailure)
	if scan.Total != 9 {
		t.Fatalf("total lines = %d, want 9", scan.Total)
	}
	if scan.Markers != 3 || scan.Shown != 3 {
		t.Fatalf("markers = %d shown = %d, want 3/3", scan.Markers, scan.Shown)
	}
	want := []failureLine{
		{2, "--- FAIL: TestFoo (0.01s)"},
		{3, "    foo_test.go:42: expected 1, got 2"},
		{4, "    foo_test.go:43: and this"},
		{7, "FAIL"},
		{8, "FAIL\tgithub.com/x/y\t0.532s"},
	}
	if len(scan.Lines) != len(want) {
		t.Fatalf("lines = %+v, want %+v", scan.Lines, want)
	}
	for i := range want {
		if scan.Lines[i] != want[i] {
			t.Errorf("line %d = %+v, want %+v", i, scan.Lines[i], want[i])
		}
	}
}

func TestScanFailuresBoundsContextAndDeduplicates(t *testing.T) {
	out := "error: boom\n  a\n  b\n  c\n  d\n  e\nerror: boom\nerror: boom\n"
	scan := scanFailures(out)
	if scan.Markers != 3 {
		t.Fatalf("markers = %d, want 3 (duplicates still count)", scan.Markers)
	}
	if scan.Shown != 1 {
		t.Fatalf("shown = %d, want 1 (duplicate text reported once)", scan.Shown)
	}
	if len(scan.Lines) != 1+maxFailureContext {
		t.Fatalf("lines = %d, want marker plus %d context", len(scan.Lines), maxFailureContext)
	}
	if scan.Lines[0].Number != 1 {
		t.Fatalf("duplicate kept a later line number: %+v", scan.Lines[0])
	}
}

func TestScanFailuresStopsCollectingAtBudget(t *testing.T) {
	var b strings.Builder
	for i := range 100 {
		b.WriteString("--- FAIL: Test")
		b.WriteString(strings.Repeat("x", i%7))
		b.WriteString(strings.Repeat("y", i))
		b.WriteString("\n")
	}
	scan := scanFailures(b.String())
	if scan.Markers != 100 {
		t.Fatalf("markers = %d, want 100", scan.Markers)
	}
	if len(scan.Lines) != maxFailureLines || scan.Shown != maxFailureLines {
		t.Fatalf("lines = %d shown = %d, want %d", len(scan.Lines), scan.Shown, maxFailureLines)
	}
}

func TestScanFailuresHandlesCRLF(t *testing.T) {
	scan := scanFailures("ok\r\nFAIL\r\n")
	if scan.Markers != 1 || scan.Lines[0].Text != "FAIL" {
		t.Fatalf("crlf not stripped: %+v", scan)
	}
}

func TestFailureBlockIsSilentForShortOutput(t *testing.T) {
	if got := failureBlock(scanFailures(goTestFailure), false); got != "" {
		t.Fatalf("block for %d-line output: %q", strings.Count(goTestFailure, "\n"), got)
	}
	if got := failureBlock(scanFailures(strings.Repeat("ok\n", 500)), false); got != "" {
		t.Fatalf("block without markers: %q", got)
	}
}

func TestFailureBlockPointsIntoLongOutput(t *testing.T) {
	out := strings.Repeat("ok  \tpkg\t0.1s\n", 800) + goTestFailure + strings.Repeat("ok\n", 800)
	got := failureBlock(scanFailures(out), false)
	if !strings.HasPrefix(got, "[failures: 3 marker lines in 1609 lines of output]\n") {
		t.Fatalf("header: %q", got)
	}
	if !strings.Contains(got, "\n 802: --- FAIL: TestFoo (0.01s)\n 803:     foo_test.go:42: expected 1, got 2\n") {
		t.Fatalf("numbered lines missing: %q", got)
	}
	if strings.Contains(got, "showing the first") {
		t.Fatalf("unbounded block claims a bound: %q", got)
	}
}

func TestFailureBlockReportsBoundAndRetention(t *testing.T) {
	var b strings.Builder
	for i := range 100 {
		b.WriteString("FAIL\tpkg")
		b.WriteString(strings.Repeat("x", i))
		b.WriteString("\n")
	}
	got := failureBlock(scanFailures(b.String()), true)
	want := "[failures: 100 marker lines in the 100 retained lines, showing the first 20]\n"
	if !strings.HasPrefix(got, want) {
		t.Fatalf("header = %q, want prefix %q", got, want)
	}
	if strings.Count(got, "\n") != maxFailureLines {
		t.Fatalf("block has %d lines, want %d", strings.Count(got, "\n"), maxFailureLines)
	}
}

func TestBashReportShortFailureIsPlain(t *testing.T) {
	content, display := bashReport(goTestFailure, false, 1, false)
	if content != display {
		t.Fatalf("short output split readers:\n%q\n%q", content, display)
	}
	if !strings.HasSuffix(content, "\n(exit error: exit status 1)") {
		t.Fatalf("exit footer missing: %q", content)
	}
	if strings.Contains(content, "[failures:") {
		t.Fatalf("block on short output: %q", content)
	}
}

func TestBashReportLongFailureLeadsWithBlockForModelOnly(t *testing.T) {
	out := strings.Repeat("ok\n", 1500) + goTestFailure
	content, display := bashReport(out, false, 1, false)
	if !strings.HasPrefix(content, "[failures: 3 marker lines in 1509 lines of output]\n") {
		t.Fatalf("content does not lead with the block: %q", content[:120])
	}
	if !strings.HasSuffix(content, display) {
		t.Fatal("content does not end with the display body")
	}
	if strings.Contains(display, "[failures:") {
		t.Fatalf("TUI display carries the block: %q", display[:120])
	}
	if !strings.Contains(display, "Showing lines 510-1509 of 1509") {
		t.Fatalf("display tail range changed: %q", display[len(display)-200:])
	}
}

func TestBashReportFlagsMaskedFailure(t *testing.T) {
	content, display := bashReport(goTestFailure, false, 0, false)
	note := "(exit code 0 is not trustworthy: the output carries 3 failure marker line(s), " +
		"and a pipeline reports only its last stage)"
	if !strings.HasSuffix(content, "\n"+note) || !strings.HasSuffix(display, "\n"+note) {
		t.Fatalf("masked note missing:\n%q\n%q", content, display)
	}
	clean, _ := bashReport("ok\tpkg\t0.1s\n", false, 0, false)
	if strings.Contains(clean, "not trustworthy") {
		t.Fatalf("clean success flagged: %q", clean)
	}
	canceled, _ := bashReport(goTestFailure, false, 0, true)
	if !strings.HasSuffix(canceled, "\n(command canceled or timed out)") {
		t.Fatalf("cancellation footer lost: %q", canceled)
	}
}

func TestBashReportEmptyOutput(t *testing.T) {
	content, display := bashReport("", false, 0, false)
	if content != "(no output)" || display != "(no output)" {
		t.Fatalf("empty output = %q / %q", content, display)
	}
}
