// Package funcadvisor finds optional code-mode hints in the libraries and tools available to a session.
package funcadvisor

import (
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"text/scanner"
	"unicode"
	"unicode/utf8"

	"github.com/evanw/esbuild/pkg/api"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
)

// Card describes one real candidate. Its ID is valid only inside a catalog snapshot.
type Card struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Use       string   `json:"use"`
	Version   string   `json:"version,omitempty"`
	Digest    string   `json:"revision,omitempty"`
	Signature string   `json:"signature,omitempty"`
	Purpose   string   `json:"purpose,omitempty"`
	Test      string   `json:"test,omitempty"`
	Draft     bool     `json:"draft,omitempty"`
	Requires  []string `json:"tools,omitempty"`
}

type token struct {
	text       string
	start, end int
}

func tokens(src string) []token {
	var s scanner.Scanner
	s.Init(strings.NewReader(src))
	s.Mode = scanner.ScanIdents | scanner.ScanStrings | scanner.ScanChars | scanner.ScanRawStrings | scanner.ScanComments | scanner.SkipComments
	s.IsIdentRune = func(r rune, i int) bool {
		return r == '_' || r == '$' || unicode.IsLetter(r) || i > 0 && unicode.IsDigit(r)
	}
	// Single-quoted JavaScript strings can contain more than one character.
	s.Error = func(*scanner.Scanner, string) {}
	var out []token
	for t := s.Scan(); t != scanner.EOF; t = s.Scan() {
		start := s.Offset
		v := s.TokenText()
		out = append(out, token{v, start, start + len(v)})
	}
	return out
}

func brief(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n] + "…"
}

// The compiler determines which symbols actually exist after TypeScript erasure.
// Lexical details enrich their cards without evaluating any package initializer.
func exports(p funclib.Package, draft bool) []Card {
	list := tokens(p.Source)
	tests := testCases(p.Tests)
	names := publicNames(p.Source)
	var out []Card
	byName := make(map[string]Card)
	for i := range list {
		if list[i].text != "export" {
			continue
		}
		j := i + 1
		if j < len(list) && list[j].text == "async" {
			j++
		}
		if j+2 >= len(list) {
			continue
		}
		kind := list[j].text
		if kind != "function" && kind != "const" {
			continue
		}
		name := list[j+1].text
		if !validName(name) {
			continue
		}
		signature := callableSignature(p.Source, list[j+1:], kind)
		if signature == "" {
			continue
		}
		description := docBefore(p.Source, list[i].start)
		if description == "" {
			description = p.Description
		}
		card := Card{
			ID:   p.Name + "/" + name,
			Name: name, Version: p.Version, Digest: p.Digest, Draft: draft,
			Use:       fmt.Sprintf("import { %s } from %q", name, "cozy:lib/"+p.Name),
			Signature: brief(signature, 240), Purpose: brief(description, 180),
		}
		end := j + 2
		for end < len(list) && list[end].text != "export" {
			end++
		}
		card.Requires = requiredTools(list[j+2 : end])
		for _, test := range tests {
			if slices.Contains(test.symbols, name) {
				card.Test = brief(test.name, 120)
				break
			}
		}
		byName[name] = card
	}
	for _, name := range names {
		card, ok := byName[name]
		if !ok {
			use := fmt.Sprintf("import { %s } from %q", name, "cozy:lib/"+p.Name)
			if name == "default" {
				use = fmt.Sprintf("import libraryDefault from %q", "cozy:lib/"+p.Name)
			}
			if !validName(name) {
				use = fmt.Sprintf("import { %q as libraryExport } from %q", name, "cozy:lib/"+p.Name)
			}
			card = Card{
				ID:      p.Name + "/" + name,
				Name:    name,
				Version: p.Version,
				Digest:  p.Digest,
				Purpose: brief(p.Description, 180),
				Use:     use,
			}
		}
		if draft {
			card.Draft = true
			card.ID = "draft/" + card.ID
			card.Use = "Open the current session's draft for " + p.Name
		}
		out = append(out, card)
	}
	return out
}

func publicNames(source string) []string {
	result := api.Build(api.BuildOptions{
		Stdin:  &api.StdinOptions{Contents: source, Loader: api.LoaderTS},
		Format: api.FormatESModule, Metafile: true, LogLevel: api.LogLevelSilent,
	})
	var meta struct {
		Outputs map[string]struct {
			Exports []string `json:"exports"`
		} `json:"outputs"`
	}
	if len(result.Errors) > 0 || json.Unmarshal([]byte(result.Metafile), &meta) != nil {
		return nil
	}
	for _, file := range meta.Outputs {
		slices.Sort(file.Exports)
		return file.Exports
	}
	return nil
}

func requiredTools(tokens []token) []string {
	var required []string
	for i := 0; i+3 < len(tokens); i++ {
		if tokens[i].text != "tools" || tokens[i+1].text != "." || tokens[i+3].text != "(" {
			continue
		}
		name := tokens[i+2].text
		switch name {
		case "list", "describe":
			continue
		case "call":
			if i+4 >= len(tokens) {
				continue
			}
			v := tokens[i+4].text
			if len(v) < 3 || (v[0] != '"' && v[0] != '\'') {
				continue
			}
			name = v[1 : len(v)-1]
		}
		if !slices.Contains(required, name) {
			required = append(required, name)
		}
	}
	slices.Sort(required)
	return required
}

func validName(name string) bool {
	for i, r := range name {
		if r == '_' || r == '$' || unicode.IsLetter(r) || i > 0 && (unicode.IsDigit(r) || unicode.IsMark(r)) {
			continue
		}
		return false
	}
	return name != ""
}

func callableSignature(src string, list []token, kind string) string {
	paren, bracket, depth := 0, 0, 0
	hadArgs := false
	for j := 1; j < len(list); j++ {
		t := list[j].text
		switch t {
		case "(":
			paren++
			hadArgs = true
		case ")":
			paren--
		case "[":
			bracket++
		case "]":
			bracket--
		case "{":
			if kind == "function" && hadArgs && paren == 0 && bracket == 0 && depth == 0 {
				return src[list[0].start:list[j-1].end]
			}
			depth++
		case "}":
			depth--
		case "=":
			if kind == "const" && j+1 < len(list) && list[j+1].text == ">" && paren == 0 && depth == 0 {
				return src[list[0].start:list[j-1].end]
			}
		case ";", "export":
			if paren == 0 && bracket == 0 && depth == 0 {
				if kind == "function" && hadArgs {
					return src[list[0].start:list[j-1].end]
				}
				return ""
			}
		}
		if list[j].end-list[0].start > 1400 || paren < 0 || bracket < 0 || depth < 0 {
			return ""
		}
	}
	return ""
}

func docBefore(src string, start int) string {
	prefix := strings.TrimRightFunc(src[:start], unicode.IsSpace)
	if !strings.HasSuffix(prefix, "*/") {
		return ""
	}
	i := strings.LastIndex(prefix, "/*")
	if i < 0 || !strings.HasPrefix(prefix[i:], "/**") {
		return ""
	}
	lines := strings.Split(strings.TrimSuffix(prefix[i+3:], "*/"), "\n")
	var out []string
	for _, line := range lines {
		line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "*"))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, " ")
}

type testCase struct {
	name    string
	symbols []string
}

func testCases(src string) []testCase {
	t := tokens(src)
	var result []testCase
	for j := range t {
		if j+2 < len(t) && (t[j].text == "test" || t[j].text == "it") && t[j+1].text == "(" {
			v := t[j+2].text
			if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
				result = append(result, testCase{name: v[1 : len(v)-1]})
			}
		}
		if len(result) > 0 && j+1 < len(t) && t[j+1].text == "(" && validName(t[j].text) {
			n := len(result) - 1
			result[n].symbols = append(result[n].symbols, t[j].text)
		}
	}
	return result
}

var sentenceEnd = regexp.MustCompile(`[\r\n]|[.!?]\s`)

func toolCard(d llm.ToolDefinition) Card {
	description := d.Description
	if end := sentenceEnd.FindStringIndex(description); end != nil {
		description = description[:end[0]]
	}
	fields := []string(nil)
	if d.Params != nil {
		fields = append(fields, d.Params.Required...)
	}
	return Card{
		ID: "tool/" + d.Name, Name: d.Name, Purpose: brief(description, 180),
		Use:       "tools.call(" + strconv.Quote(d.Name) + ", args)",
		Signature: strings.Join(fields, ", "),
	}
}
