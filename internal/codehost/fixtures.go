package codehost

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	"github.com/alvnukov/cozyphi/internal/codemode"
	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools"
)

type mockCall struct {
	Name   string          `json:"name"`
	Args   json.RawMessage `json:"args"`
	Result json.RawMessage `json:"result"`
	Error  string          `json:"error"`
}

type cellCase struct {
	Name  string          `json:"name"`
	Calls []mockCall      `json:"calls"`
	Want  json.RawMessage `json:"want"`
}

type cellCheck struct {
	Source string     `json:"source"`
	Cases  []cellCase `json:"cases"`
}

func cellTestTool(store *funclib.Store, checkout string, definitions []llm.ToolDefinition) tools.Tool {
	call := llm.Object{"type": "object", "required": []string{"name", "args"}, "properties": llm.Object{
		"name": llm.Object{"type": "string"}, "args": llm.Object{"type": "object"},
		"result": llm.Object{}, "error": llm.Object{"type": "string"},
	}}
	cases := llm.Object{"type": "array", "minItems": 1, "maxItems": 16, "items": llm.Object{
		"type": "object", "required": []string{"name", "want"}, "properties": llm.Object{
			"name": llm.Object{"type": "string"}, "want": llm.Object{},
			"calls": llm.Object{"type": "array", "maxItems": 128, "items": call},
		},
	}}
	return tools.Tool{
		Definition: llm.ToolDefinition{
			Name:        "code_test",
			Readable:    true,
			Description: "Unit-test the exact source of a one-off TypeScript cell. Each named case specifies the expected JSON return value (want) and optional ordered mock calls with name, args, and result or error. Only fixtures can answer tools; side effects never run.",
			Params: &llm.FunctionParameters{
				Type:     "object",
				Required: []string{"source", "cases"},
				Properties: llm.Object{
					"source": llm.Object{"type": "string"}, "cases": cases,
				},
			},
		},
		Run: func(ctx context.Context, input json.RawMessage) (tools.Result, error) {
			var in cellCheck
			if len(input) > 512<<10 {
				return tools.Result{}, errors.New("cell tests exceed 512 KiB")
			}
			if err := json.Unmarshal(input, &in); err != nil {
				return tools.Result{}, fmt.Errorf("decode cell tests: %w", err)
			}
			if in.Source == "" || len(in.Cases) == 0 || len(in.Cases) > 16 {
				return tools.Result{}, errors.New("provide source and 1 to 16 named test cases")
			}
			ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			defer cancel()
			modules, _, err := store.PreviewPinned(ctx, checkout, codemode.Imports(in.Source))
			if err != nil {
				return tools.Result{}, fmt.Errorf("resolve test imports: %w", err)
			}
			p := codemode.Program{Source: in.Source, Modules: modules, Tools: definitions}
			for i, test := range in.Cases {
				if err := test.run(ctx, p); err != nil {
					return tools.Result{}, fmt.Errorf("cell test %d (%q): %w", i+1, test.Name, err)
				}
			}
			report := struct {
				Passed       int    `json:"passed"`
				SourceSHA256 string `json:"sourceSHA256"`
			}{len(in.Cases), fmt.Sprintf("%x", sha256.Sum256([]byte(in.Source)))}
			raw, err := json.Marshal(report)
			return tools.Result{
				Structured: report,
				Content:    string(raw),
				Detail:     fmt.Sprintf("%d cell tests passed", report.Passed),
			}, err
		},
	}
}

func (test cellCase) run(ctx context.Context, p codemode.Program) error {
	if test.Name == "" || len(test.Name) > 160 || len(test.Want) == 0 || len(test.Calls) > 128 {
		return errors.New("provide a short name, a JSON expectation and up to 128 mock calls")
	}
	for _, call := range test.Calls {
		if call.Name == "" || len(call.Args) == 0 || (len(call.Result) == 0) == (call.Error == "") {
			return errors.New("each mock needs a name, arguments, and either a JSON result or an error")
		}
	}
	next := 0
	var unexpected error
	out, err := codemode.Run(
		ctx,
		p,
		func(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
			if unexpected != nil {
				return nil, unexpected
			}
			if next >= len(test.Calls) {
				unexpected = fmt.Errorf("unexpected fixture call %q", name)
				return nil, unexpected
			}
			mock := test.Calls[next]
			if mock.Name != name || !sameJSON(mock.Args, args) {
				unexpected = fmt.Errorf(
					"fixture %d expected %q with its supplied arguments, received %q with %s",
					next+1,
					mock.Name,
					name,
					shortJSON(args),
				)
				return nil, unexpected
			}
			next++
			if mock.Error != "" {
				return nil, errors.New(mock.Error)
			}
			return mock.Result, nil
		},
	)
	if unexpected != nil {
		return unexpected
	}
	if err != nil {
		return err
	}
	if next != len(test.Calls) {
		return fmt.Errorf("consumed %d of %d fixture calls", next, len(test.Calls))
	}
	if !sameJSON(test.Want, out) {
		return fmt.Errorf("expected %s, received %s", shortJSON(test.Want), shortJSON(out))
	}
	return nil
}

func sameJSON(a, b json.RawMessage) bool {
	var x, y any
	return json.Unmarshal(a, &x) == nil && json.Unmarshal(b, &y) == nil && reflect.DeepEqual(x, y)
}

func shortJSON(raw json.RawMessage) string {
	if len(raw) > 320 {
		return string(raw[:320]) + "…"
	}
	return string(raw)
}
