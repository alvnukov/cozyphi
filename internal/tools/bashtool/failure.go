package bashtool

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	// maxFailureLines bounds the failure block handed to the model. The block
	// points at the failures; it does not reproduce the log.
	maxFailureLines = 20
	// maxFailureContext is how many indented lines may follow a marker. The
	// detail under `--- FAIL:` and the `-->` under a rustc error is where the
	// actual message lives; the marker alone only says that one exists.
	maxFailureContext = 3
)

// failureMarkerPatterns match lines a toolchain prints only when something
// failed. They are deliberately anchored and few: a pattern that fires on
// ordinary output teaches the model to ignore the block, which is worse than
// having no block.
var failureMarkerPatterns = []*regexp.Regexp{
	// go test: per test, and the per-package or bare FAIL summary.
	regexp.MustCompile(`^--- FAIL: `),
	regexp.MustCompile(`^FAIL(\s|$)`),
	// go build, go vet, golangci-lint and gopls all report file:line:col.
	regexp.MustCompile(`^\S+\.go:\d+:\d+: `),
	// Go runtime, including "panic: test timed out".
	regexp.MustCompile(`^panic: `),
	// gcc and clang: file:line:col: error:.
	regexp.MustCompile(`^\S+:\d+:\d+: (fatal )?error: `),
	// tsc: file(line,col): error TSnnnn:.
	regexp.MustCompile(`^\S+\(\d+,\d+\): error TS\d+`),
	// cargo and rustc, and every CLI that starts a line with "error:".
	regexp.MustCompile(`(?i)^error(\[E\d+\])?: `),
	// pytest summary, python tracebacks and the exception line closing them.
	regexp.MustCompile(`^FAILED \S`),
	regexp.MustCompile(`^Traceback \(most recent call last\):`),
	regexp.MustCompile(`^[A-Za-z_][\w.]*(Error|Exception)(: |$)`),
	// make and git.
	regexp.MustCompile(`^make: \*{3} `),
	regexp.MustCompile(`^fatal: `),
}

// failureLine is one line of the failure block: a marker, or an indented
// continuation line under it. Number is the 1-based line number in the
// output the block was scanned from, so it matches the "Showing lines" range
// and an offset read of the full-output file.
type failureLine struct {
	Number int
	Text   string
}

// failureScan is what one pass over a command's output found.
type failureScan struct {
	// Lines is the bounded block, in output order.
	Lines []failureLine
	// Markers counts every marker line in the output, shown or not.
	Markers int
	// Shown counts the markers that made it into Lines.
	Shown int
	// Total is the number of lines scanned.
	Total int
}

// scanFailures walks output once and collects failure markers with their
// indented context. Identical marker text is reported once, at its first
// line: the same diagnostic printed on both streams says nothing new.
func scanFailures(output string) failureScan {
	var scan failureScan
	seen := map[string]struct{}{}
	context := 0
	for line := range strings.Lines(output) {
		scan.Total++
		line = strings.TrimRight(line, "\r\n")
		if context > 0 && isContinuation(line) {
			context--
			if len(scan.Lines) < maxFailureLines {
				scan.Lines = append(scan.Lines, failureLine{Number: scan.Total, Text: line})
			}
			continue
		}
		context = 0
		if !isFailureMarker(line) {
			continue
		}
		scan.Markers++
		if _, dup := seen[line]; dup || len(scan.Lines) >= maxFailureLines {
			continue
		}
		seen[line] = struct{}{}
		scan.Shown++
		scan.Lines = append(scan.Lines, failureLine{Number: scan.Total, Text: line})
		context = maxFailureContext
	}
	return scan
}

// isFailureMarker reports whether line matches one of the anchored patterns.
// Every pattern starts at a non-space character, so indented and empty lines
// skip the regexps entirely — most lines of a verbose log are one or the
// other.
func isFailureMarker(line string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' {
		return false
	}
	for _, pattern := range failureMarkerPatterns {
		if pattern.MatchString(line) {
			return true
		}
	}
	return false
}

func isContinuation(line string) bool {
	return strings.TrimSpace(line) != "" && (line[0] == ' ' || line[0] == '\t')
}

// failureBlock renders the block the model reads before the output. It is
// empty when nothing matched, and also when the output is no longer than the
// block could be: a pointer into twenty lines the model already sees is
// noise, a pointer into two thousand is the point.
func failureBlock(scan failureScan, collectionTruncated bool) string {
	if scan.Markers == 0 || scan.Total <= maxFailureLines {
		return ""
	}
	var b strings.Builder
	b.WriteString("[failures: ")
	b.WriteString(strconv.Itoa(scan.Markers))
	b.WriteString(" marker line")
	if scan.Markers != 1 {
		b.WriteString("s")
	}
	if collectionTruncated {
		fmt.Fprintf(&b, " in the %d retained lines", scan.Total)
	} else {
		fmt.Fprintf(&b, " in %d lines of output", scan.Total)
	}
	if scan.Shown < scan.Markers {
		fmt.Fprintf(&b, ", showing the first %d", scan.Shown)
	}
	b.WriteString("]")
	width := len(strconv.Itoa(scan.Total))
	for _, line := range scan.Lines {
		fmt.Fprintf(&b, "\n%*d: %s", width, line.Number, line.Text)
	}
	return b.String()
}

// maskedFailureNote contradicts a zero exit code that the output does not
// support. A command exits with the status of the last stage of its
// pipeline, so `go test ./... | tail -40` reports success whatever the tests
// did; the exit code cannot be repaired here, but the model can be told not
// to trust it.
func maskedFailureNote(scan failureScan) string {
	if scan.Markers == 0 {
		return ""
	}
	return fmt.Sprintf(
		"(exit code 0 is not trustworthy: the output carries %d failure marker line(s), "+
			"and a pipeline reports only its last stage)",
		scan.Markers,
	)
}
