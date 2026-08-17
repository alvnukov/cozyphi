package util

import (
	"regexp"
	"strings"
)

var (
	// hashlineDisplayPrefixRe matches patterns like "  5#abc|" or ">>> 5#abc|".
	hashlineDisplayPrefixRe = regexp.MustCompile(
		`^\s*(?:>>>|>>)?\s*(?:\+?\s*(?:\d+\s*#\s*|#\s*)|\+)\s*[0-9a-zA-Z]{2,4}(?:\||:)`,
	)
	// hashlinePlusDiffPrefixRe matches diff + prefix patterns.
	hashlinePlusDiffPrefixRe = regexp.MustCompile(
		`^\s*(?:>>>|>>)?\s*\+\s*(?:\d+\s*#\s*|#\s*)?[0-9a-zA-Z]{2,4}(?:\||:)`,
	)
	// hashlineGrepPathPrefixRe matches grep output: "path:>>LINE#HASH|".
	hashlineGrepPathPrefixRe = regexp.MustCompile(
		`^[^:]+:\s*(?:>>>|>>| {2})\s*\d+\s*#\s*[0-9a-zA-Z]{2,4}\|`,
	)
	// hashlineLegacyColonPipeRe matches older colon-separated patterns.
	hashlineLegacyColonPipeRe = regexp.MustCompile(`^\s*(?:>>>?)?\s*\d+:[0-9a-zA-Z]{1,16}\|`)
)

func lineHasDiffPlusPrefix(line string) bool {
	return strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "++")
}

func stripDiffPlusLine(line string) string {
	if lineHasDiffPlusPrefix(line) {
		return line[1:]
	}
	return line
}

func lineLooksHashlinePrefixed(line string) bool {
	return hashlineGrepPathPrefixRe.MatchString(line) ||
		hashlineDisplayPrefixRe.MatchString(line) ||
		hashlinePlusDiffPrefixRe.MatchString(line) ||
		hashlineLegacyColonPipeRe.MatchString(line)
}

func stripOneHashlinePrefix(line string) string {
	switch {
	case hashlineGrepPathPrefixRe.MatchString(line):
		return hashlineGrepPathPrefixRe.ReplaceAllString(line, "")
	case hashlineDisplayPrefixRe.MatchString(line):
		return hashlineDisplayPrefixRe.ReplaceAllString(line, "")
	case hashlineLegacyColonPipeRe.MatchString(line):
		return hashlineLegacyColonPipeRe.ReplaceAllString(line, "")
	default:
		return line
	}
}

// StripLinePrefixes detects hashline- or diff-prefixed paste blocks and
// strips display noise while preserving code text.
func StripLinePrefixes(lines []string) []string {
	var nonEmpty, hashPrefixCount, diffPlusHashPrefixCount, diffPlusCount int
	for _, l := range lines {
		if l == "" {
			continue
		}
		nonEmpty++
		if lineLooksHashlinePrefixed(l) {
			hashPrefixCount++
		}
		if hashlinePlusDiffPrefixRe.MatchString(l) {
			diffPlusHashPrefixCount++
		}
		if lineHasDiffPlusPrefix(l) {
			diffPlusCount++
		}
	}

	if nonEmpty == 0 {
		return lines
	}

	stripHash := hashPrefixCount > 0 && hashPrefixCount == nonEmpty
	stripPlus := !stripHash && diffPlusHashPrefixCount == 0 && diffPlusCount > 0 &&
		float64(diffPlusCount) >= float64(nonEmpty)*0.5
	if !stripHash && !stripPlus && diffPlusHashPrefixCount == 0 {
		return lines
	}

	result := make([]string, len(lines))
	for i, l := range lines {
		switch {
		case stripHash:
			result[i] = stripOneHashlinePrefix(l)
		case stripPlus:
			result[i] = stripDiffPlusLine(l)
		case diffPlusHashPrefixCount > 0 && hashlinePlusDiffPrefixRe.MatchString(l):
			result[i] = hashlineDisplayPrefixRe.ReplaceAllString(l, "")
		default:
			result[i] = l
		}
	}
	return result
}
