package util

import (
	"fmt"
	"path/filepath"
	"strings"
)

// DiffResult holds a unified diff and the first changed line.
type DiffResult struct {
	Diff             string
	FirstChangedLine int
}

type change struct {
	OldIdx int
	NewIdx int
	Kind   string
	line   string
}

type hunk struct {
	start int
	end   int
}

// GenerateDiffString computes a unified diff between oldContent and newContent.
func GenerateDiffString(oldContent, newContent string, contextLines int) DiffResult {
	var oldLines, newLines []string
	if oldContent == "" {
		oldLines = []string{}
	} else {
		oldLines = strings.Split(oldContent, "\n")
	}
	if newContent == "" {
		newLines = []string{}
	} else {
		newLines = strings.Split(newContent, "\n")
	}

	changes := computeChanges(oldLines, newLines)
	diff := generateUnifiedDiffFromChanges(changes, contextLines)
	firstChangedLine := computeFirstChangedLine(changes)

	return DiffResult{
		Diff:             diff,
		FirstChangedLine: firstChangedLine,
	}
}

// GenerateFileDiff returns a unified diff with ---/+++ headers.
func GenerateFileDiff(filePath, oldContent, newContent string, contextLines int) string {
	result := GenerateDiffString(oldContent, newContent, contextLines)
	baseName := filepath.Base(filePath)
	header := fmt.Sprintf("--- a/%s\n", baseName)
	header += fmt.Sprintf("+++ b/%s\n", baseName)
	return header + result.Diff
}

func generateUnifiedDiffFromChanges(changes []change, contextLines int) string {
	if len(changes) == 0 {
		return ""
	}

	var diff strings.Builder
	var hunks []hunk
	for idx := range changes {
		if changes[idx].Kind != "equal" {
			start := max(0, idx-contextLines)
			end := min(len(changes)-1, idx+contextLines)
			if len(hunks) == 0 || start > hunks[len(hunks)-1].end+1 {
				hunks = append(hunks, hunk{start: start, end: end})
			} else if end > hunks[len(hunks)-1].end {
				hunks[len(hunks)-1].end = end
			}
		}
	}

	for _, h := range hunks {
		hunkChanges := changes[h.start : h.end+1]

		hasOldLines, hasNewLines := false, false
		for _, c := range hunkChanges {
			if (c.Kind == "delete" || c.Kind == "equal") && c.OldIdx >= 0 {
				hasOldLines = true
			}
			if (c.Kind == "add" || c.Kind == "equal") && c.NewIdx >= 0 {
				hasNewLines = true
			}
		}

		oldStart, oldCount := 1, 0
		newStart, newCount := 1, 0

		switch {
		case hasOldLines && hasNewLines:
			for _, c := range hunkChanges {
				if (c.Kind == "delete" || c.Kind == "equal") && c.OldIdx >= 0 {
					if oldCount == 0 {
						oldStart = c.OldIdx + 1
					}
					oldCount++
				}
				if (c.Kind == "add" || c.Kind == "equal") && c.NewIdx >= 0 {
					if newCount == 0 {
						newStart = c.NewIdx + 1
					}
					newCount++
				}
			}
		case !hasOldLines && hasNewLines:
			for _, c := range hunkChanges {
				if c.Kind == "add" && c.NewIdx >= 0 {
					if newCount == 0 {
						newStart = c.NewIdx + 1
					}
					newCount++
				}
			}
			anchorOld := -1
			for i := h.start - 1; i >= 0; i-- {
				if changes[i].OldIdx >= 0 {
					anchorOld = changes[i].OldIdx + 1
					break
				}
			}
			if anchorOld == -1 {
				oldStart = 1
			} else {
				oldStart = anchorOld + 1
			}
			oldCount = 0
		case hasOldLines && !hasNewLines:
			for _, c := range hunkChanges {
				if c.OldIdx >= 0 {
					if oldCount == 0 {
						oldStart = c.OldIdx + 1
					}
					oldCount++
				}
			}
			anchorNew := -1
			for i := h.start - 1; i >= 0; i-- {
				if changes[i].NewIdx >= 0 {
					anchorNew = changes[i].NewIdx + 1
					break
				}
			}
			if anchorNew == -1 {
				newStart = 1
			} else {
				newStart = anchorNew + 1
			}
			newCount = 0
		}

		fmt.Fprintf(&diff, "@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount)

		for _, c := range hunkChanges {
			switch c.Kind {
			case "equal":
				fmt.Fprintf(&diff, " %s\n", c.line)
			case "delete":
				fmt.Fprintf(&diff, "-%s\n", c.line)
			case "add":
				fmt.Fprintf(&diff, "+%s\n", c.line)
			}
		}
	}

	return diff.String()
}

func computeChanges(oldLines, newLines []string) []change {
	n, m := len(oldLines), len(newLines)
	if n == 0 && m == 0 {
		return nil
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}

	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			switch {
			case oldLines[i-1] == newLines[j-1]:
				dp[i][j] = dp[i-1][j-1] + 1
			case dp[i-1][j] >= dp[i][j-1]:
				dp[i][j] = dp[i-1][j]
			default:
				dp[i][j] = dp[i][j-1]
			}
		}
	}

	var changes []change
	var backtrack func(i, j int)
	backtrack = func(i, j int) {
		if i > 0 && j > 0 && oldLines[i-1] == newLines[j-1] {
			backtrack(i-1, j-1)
			changes = append(changes, change{
				Kind: "equal", line: oldLines[i-1], OldIdx: i - 1, NewIdx: j - 1,
			})
			return
		}
		if j > 0 && (i == 0 || dp[i][j-1] >= dp[i-1][j]) {
			backtrack(i, j-1)
			changes = append(changes, change{
				Kind: "add", line: newLines[j-1], OldIdx: -1, NewIdx: j - 1,
			})
			return
		}
		if i > 0 && (j == 0 || dp[i][j-1] < dp[i-1][j]) {
			backtrack(i-1, j)
			changes = append(changes, change{
				Kind: "delete", line: oldLines[i-1], OldIdx: i - 1, NewIdx: -1,
			})
			return
		}
	}
	backtrack(n, m)
	return changes
}

func computeFirstChangedLine(changes []change) int {
	for i, c := range changes {
		if c.Kind != "equal" {
			if c.NewIdx >= 0 {
				return c.NewIdx + 1
			}
			for j := i + 1; j < len(changes); j++ {
				if changes[j].NewIdx >= 0 {
					return changes[j].NewIdx + 1
				}
			}
			return 0
		}
	}
	return 0
}
