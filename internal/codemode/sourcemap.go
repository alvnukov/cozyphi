package codemode

import (
	"encoding/json"
	"errors"
	"regexp"
	"strconv"
	"strings"

	"github.com/evanw/esbuild/pkg/api"
)

var bundlePosition = regexp.MustCompile(`cozy-bundle\.js:([0-9]+)(?::([0-9]+))?`)

type sourceMap struct {
	Sources    []string
	Mappings   string
	cellOffset int
}
type location struct {
	name   string
	line   int
	column int
}

func (m sourceMap) at(line, column int) (location, bool) {
	if line < 1 {
		return location{}, false
	}
	source, origLine, origCol := 0, 0, 0
	for index, row := range strings.Split(m.Mappings, ";") {
		if index >= line {
			break
		}
		gen := 0
		var best location
		for part := range strings.SplitSeq(row, ",") {
			if part == "" {
				continue
			}
			d, ok := decodeVLQ(part)
			if !ok {
				return location{}, false
			}
			gen += d[0]
			if index+1 == line && column > 0 && gen > column-1 {
				break
			}
			if len(d) < 4 {
				best = location{}
				continue
			}
			source += d[1]
			origLine += d[2]
			origCol += d[3]
			if source < 0 || source >= len(m.Sources) {
				return location{}, false
			}
			name := strings.TrimPrefix(m.Sources[source], "library:")
			n := origLine + 1
			if name == "cell.ts" {
				n = max(1, n-m.cellOffset)
			}
			best = location{name, n, origCol + 1}
			if index+1 == line && column == 0 {
				break
			}
		}
		if index+1 == line {
			return best, best.name != ""
		}
	}
	return location{}, false
}

func decodeVLQ(s string) ([]int, bool) {
	const digits = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	a := make([]int, 0, 5)
	value, shift := 0, uint(0)
	for i := range len(s) {
		d := strings.IndexByte(digits, s[i])
		if d < 0 || shift >= 30 {
			return nil, false
		}
		value |= (d & 31) << shift
		if d&32 != 0 {
			shift += 5
			continue
		}
		n := value >> 1
		if value&1 != 0 {
			n = -n
		}
		a = append(a, n)
		if len(a) > 5 {
			return nil, false
		}
		value = 0
		shift = 0
	}
	return a, len(a) > 0 && shift == 0
}

func (m sourceMap) annotate(err error) error {
	if err == nil {
		return nil
	}
	message := bundlePosition.ReplaceAllStringFunc(err.Error(), func(s string) string {
		parts := bundlePosition.FindStringSubmatch(s)
		line, _ := strconv.Atoi(parts[1])
		column, _ := strconv.Atoi(parts[2])
		loc, ok := m.at(line, column)
		if !ok {
			return s
		}
		return loc.name + ":" + strconv.Itoa(loc.line) + ":" + strconv.Itoa(loc.column)
	})
	if message == err.Error() {
		return err
	}
	return errors.New(message)
}

type compiledCell struct {
	js      string
	mapping sourceMap
}

func readBundle(files []api.OutputFile) (compiledCell, error) {
	var out compiledCell
	for _, f := range files {
		if strings.HasSuffix(f.Path, ".map") {
			if err := json.Unmarshal(f.Contents, &out.mapping); err != nil {
				return out, err
			}
		} else if strings.HasSuffix(f.Path, ".js") {
			out.js = string(f.Contents)
		}
	}
	if out.js == "" {
		return out, errors.New("missing compiled cell")
	}
	return out, nil
}
