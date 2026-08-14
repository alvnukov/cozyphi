package util

import (
	"hash/fnv"
	"strings"
	"unsafe"
)

const hashMod = 10000

// dict contains 10000 two-character hash codes (aa-zz + numeric pattern).
var dict [hashMod]string

func init() {
	chars := "abcdefghijklmnopqrstuvwxyz"
	idx := 0
	for i := 0; i < 26 && idx < hashMod; i++ {
		for j := 0; j < 26 && idx < hashMod; j++ {
			dict[idx] = string([]byte{chars[i], chars[j]})
			idx++
		}
	}
	for idx < hashMod {
		n := idx % 100
		dict[idx] = string([]byte{byte('0' + n/10), byte('0' + n%10)})
		idx++
	}
}

// removeWhitespace removes all whitespace characters from s (optimized, no regex).
func removeWhitespace(s string) string {
	hasWhitespace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' || (c >= 0x0B && c <= 0x0D) {
			hasWhitespace = true
			break
		}
	}
	if !hasWhitespace {
		return s
	}
	result := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c != ' ' && c != '\t' && c != '\n' && c != '\r' && (c < 0x0B || c > 0x0D) {
			result = append(result, c)
		}
	}
	if len(result) == 0 {
		return ""
	}
	return unsafe.String(&result[0], len(result))
}

// ComputeLineHash computes a 2-character hash for a line. Whitespace is
// stripped before hashing, so only non-whitespace content affects the result.
func ComputeLineHash(line string) string {
	if line != "" && line[len(line)-1] == '\r' {
		line = line[:len(line)-1]
	}
	line = removeWhitespace(line)
	h := fnv.New64a()
	h.Write([]byte(line))
	return dict[h.Sum64()%hashMod]
}

// FormatHashLines formats file content with hashline prefixes for display.
// Each line becomes "LINENUM#HASH|CONTENT" where LINENUM is 1-indexed.
func FormatHashLines(content string, startLine int) string {
	if startLine == 0 {
		startLine = 1
	}
	if content == "" {
		return formatLinePrefix(startLine, ComputeLineHash(""))
	}

	lineCount := strings.Count(content, "\n") + 1
	estimatedSize := len(content) + lineCount*8

	var sb strings.Builder
	sb.Grow(estimatedSize)

	lineNum := startLine
	start := 0
	for i := 0; i <= len(content); i++ {
		if i == len(content) || content[i] == '\n' {
			line := content[start:i]
			hash := ComputeLineHash(line)
			writeLinePrefix(&sb, lineNum, hash)
			sb.WriteString(line)
			if i < len(content) {
				sb.WriteByte('\n')
			}
			lineNum++
			start = i + 1
		}
	}
	return sb.String()
}

// ---- line prefix cache for fast "N#hash|" formatting ----

var (
	linePrefixCache [1000][4]byte
	linePrefixLen   [1000]int8
)

func init() {
	for i := range 1000 {
		n := i + 1
		p := linePrefixCache[i][:]
		if n >= 100 {
			p[0] = byte('0' + n/100)
			p[1] = byte('0' + (n/10)%10)
			p[2] = byte('0' + n%10)
			p[3] = '#'
			linePrefixLen[i] = 4
		} else if n >= 10 {
			p[0] = byte('0' + n/10)
			p[1] = byte('0' + n%10)
			p[2] = '#'
			linePrefixLen[i] = 3
		} else {
			p[0] = byte('0' + n)
			p[1] = '#'
			linePrefixLen[i] = 2
		}
	}
}

func writeLinePrefix(sb *strings.Builder, lineNum int, hash string) {
	if lineNum >= 1 && lineNum <= 1000 {
		idx := lineNum - 1
		sb.Write(linePrefixCache[idx][:linePrefixLen[idx]])
	} else {
		n := lineNum
		if n >= 10000 {
			sb.WriteByte(byte('0' + n/10000))
			n %= 10000
		}
		if n >= 1000 {
			sb.WriteByte(byte('0' + n/1000))
			n %= 1000
		}
		if n >= 100 {
			sb.WriteByte(byte('0' + n/100))
			n %= 100
		}
		if n >= 10 {
			sb.WriteByte(byte('0' + n/10))
			n %= 10
		}
		sb.WriteByte(byte('0' + n))
		sb.WriteByte('#')
	}
	sb.WriteString(hash)
	sb.WriteByte('|')
}

func formatLinePrefix(lineNum int, hash string) string {
	var sb strings.Builder
	sb.Grow(12)
	writeLinePrefix(&sb, lineNum, hash)
	return sb.String()
}

// ValidateHash checks whether the hash at the given 1-based line matches.
func ValidateHash(line int, hash string, fileContent []string) bool {
	if line < 1 || line > len(fileContent) {
		return false
	}
	expected := strings.ToLower(hash)
	actual := ComputeLineHash(fileContent[line-1])
	return expected == actual
}
