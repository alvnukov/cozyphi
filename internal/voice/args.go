package voice

import (
	"fmt"
	"strings"
)

// splitArgs splits a command line into argv the way a shell would for the
// simple cases: whitespace separates words, single and double quotes group
// them, and a backslash escapes the next character. There is no expansion of
// any kind — the argv goes straight to exec, never through a shell.
func splitArgs(line string) ([]string, error) {
	var (
		argv  []string
		word  strings.Builder
		open  bool
		quote rune
	)
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '\\' && i+1 < len(runes):
			i++
			word.WriteRune(runes[i])
			open = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
			open = true
		case r == ' ' || r == '\t' || r == '\n':
			if open {
				argv = append(argv, word.String())
				word.Reset()
				open = false
			}
		default:
			word.WriteRune(r)
			open = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("unbalanced %q in command line", string(quote))
	}
	if open {
		argv = append(argv, word.String())
	}
	return argv, nil
}

// expandArgs replaces {name} placeholders inside each argument. An argument
// that is exactly one placeholder with an empty value disappears, and so does
// the flag right before it — so an unused "--prompt {hint}" leaves no dangling
// flag behind.
func expandArgs(argv []string, values map[string]string) []string {
	out := make([]string, 0, len(argv))
	for _, arg := range argv {
		if name, ok := solePlaceholder(arg); ok && values[name] == "" {
			if n := len(out); n > 0 && strings.HasPrefix(out[n-1], "-") {
				out = out[:n-1]
			}
			continue
		}
		for name, value := range values {
			arg = strings.ReplaceAll(arg, "{"+name+"}", value)
		}
		out = append(out, arg)
	}
	return out
}

// solePlaceholder reports the placeholder name when the argument is nothing
// but one placeholder.
func solePlaceholder(arg string) (string, bool) {
	if !strings.HasPrefix(arg, "{") || !strings.HasSuffix(arg, "}") {
		return "", false
	}
	name := arg[1 : len(arg)-1]
	if name == "" || strings.ContainsAny(name, "{}") {
		return "", false
	}
	return name, true
}
