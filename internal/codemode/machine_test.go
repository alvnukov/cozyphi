package codemode

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alvnukov/cozyphi/internal/llm"
)

func TestNestedCall(t *testing.T) {
	result, err := Run(
		t.Context(),
		Program{Source: `const v: number = await tools.read({n: 41}); return v+1;`},
		func(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
			if name != "read" || string(args) != `{"n":41}` {
				t.Fatalf("unexpected request %s %s", name, args)
			}
			return json.RawMessage("41"), nil
		},
	)
	if err != nil || string(result) != "42" {
		t.Fatalf("result %q: %v", result, err)
	}
}

func TestLibraryExecutionAndFixtures(t *testing.T) {
	p := Program{Modules: map[string]string{
		"builds/index.ts": `export async function statuses(path: string): Promise<number[]> {const data = await tools.read({path});return data.statuses;}`,
		"builds/index.test.ts": `import {statuses} from "./index.ts"; import {test, equal, fixture} from "cozy:test";
test("gets statuses",async()=>{fixture("read",{path:"abc"},{statuses:[1,2]});equal(await statuses("abc"),[1,2])})`,
	}}
	if result, err := Test(t.Context(), p, func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		t.Fatal("live host called by test")
		return nil, nil
	}); err != nil || !strings.Contains(string(result), "\"passed\":1") {
		t.Fatalf("test result %s: %v", result, err)
	}
	p.Source = `import { statuses } from "cozy:lib/builds"; return await statuses("abc")`
	if names := Imports(p.Source); len(names) != 1 || names[0] != "builds" {
		t.Fatalf("imports %v", names)
	}
	result, err := Run(t.Context(), p, func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		return json.RawMessage(`{"statuses":[17]}`), nil
	})
	if err != nil || string(result) != "[17]" {
		t.Fatalf("library returned %s: %v", result, err)
	}
	p.Source = ""
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("optional type checker not installed")
	}
	if err := Check(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	p.Modules["builds/index.ts"] = `export const wrong: number = "text"`
	if err := Check(t.Context(), p); err == nil || !strings.Contains(err.Error(), "not assignable") {
		t.Fatalf("expected semantic type error, got %v", err)
	}
}

func TestNoAmbientAccess(t *testing.T) {
	p := Program{Source: `return [typeof Deno,typeof process,typeof fetch,typeof require]`}
	r, err := Run(t.Context(), p, nil)
	if err != nil || string(r) != `["undefined","undefined","undefined","undefined"]` {
		t.Fatalf("unexpected ambient APIs %s: %v", r, err)
	}
	for _, spec := range []string{"file:///etc/hosts", "node:fs", "https://invalid.example/x", "../../root.ts"} {
		p.Source = fmt.Sprintf("import %q; return 5", spec)
		if _, err = Run(t.Context(), p, nil); err == nil {
			t.Fatalf("unexpected import %s", spec)
		}
	}
}

func TestInterruptBusyCell(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	if _, err := Run(ctx, Program{Source: "while(true) {}"}, nil); err == nil {
		t.Fatal("busy cell not interrupted")
	}
}

func TestDiscoveryAndUnawaitedEffects(t *testing.T) {
	p := Program{
		Tools:  []llm.ToolDefinition{{Name: "read", Description: "read file"}},
		Source: `return [tools.list(),tools.describe("read").description]`,
	}
	r, err := Run(t.Context(), p, nil)
	if err != nil || string(r) != `[["read"],"read file"]` {
		t.Fatalf("discovery %s: %v", r, err)
	}
	p.Source = `tools.delete({}); return 1`
	_, err = Run(t.Context(), p, func(context.Context, string, json.RawMessage) (json.RawMessage, error) {
		t.Fatal("unawaited call effected")
		return nil, nil
	})
	if err == nil {
		t.Fatal("expected unawaited call failure")
	}
}

func TestFailingAndMissingTests(t *testing.T) {
	for _, src := range []string{"", `import {test, equal} from "cozy:test"; test("bad",()=>equal(1,2))`, `import {test, fixture} from "cozy:test"; test("unused",()=>fixture("read",{},2))`} {
		if _, err := Test(
			t.Context(),
			Program{Modules: map[string]string{"builds/index.test.ts": src}},
			nil,
		); err == nil {
			t.Fatalf("accepted incomplete test %q", src)
		}
	}
}

func TestParallelReadBatchesStopAtMutation(t *testing.T) {
	p := Program{
		ParallelReads: true,
		Source:        `return Promise.all([tools.read({path:"a"}),tools.ls({}),tools.write({path:"b"}),tools.find({})])`,
	}
	var names []string
	result, err := Run(
		t.Context(),
		p,
		func(_ context.Context, name string, args json.RawMessage) (json.RawMessage, error) {
			names = append(names, name)
			if name == "cozy:parallel_read" {
				var in struct{ Calls []batchRequest }
				if err := json.Unmarshal(
					args,
					&in,
				); err != nil || len(in.Calls) != 2 || in.Calls[0].Name != "read" ||
					in.Calls[1].Name != "ls" {
					t.Fatalf("unexpected batch %s", args)
				}
				return json.RawMessage(`[{"value":1},{"value":2}]`), nil
			}
			return json.RawMessage("3"), nil
		},
	)
	if err != nil || string(result) != "[1,2,3,3]" || strings.Join(names, ",") != "cozy:parallel_read,write,find" {
		t.Fatalf("calls %v result %s: %v", names, result, err)
	}
}

func TestTestSourceMayReturnWithoutSkippingTests(t *testing.T) {
	if _, err := Test(t.Context(), Program{Source: `return {passed: 20}`}, nil); err == nil {
		t.Fatal("accepted test program without registered cases")
	}
}

func TestNoFileImportDuringCheck(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("optional type checker not installed")
	}
	canary := filepath.Join(t.TempDir(), "external.ts")
	if err := os.WriteFile(canary, []byte("export type External = string"), 0o600); err != nil {
		t.Fatal(err)
	}
	name, _ := json.Marshal(fileURL(canary))
	p := Program{
		Modules: map[string]string{
			"sample/index.ts": "import type { External } from " + string(
				name,
			) + "; export const leaked: External = 'outside'",
		},
	}
	if err := Check(t.Context(), p); err == nil {
		t.Fatal("external file import accepted by type checker")
	}
}

func TestThrownErrorIdentifiesLibrarySource(t *testing.T) {
	program := Program{
		Source:  `import {crash} from "cozy:lib/demo"; return crash()`,
		Modules: map[string]string{"demo/index.ts": "export function crash() {\n  throw new Error('broken');\n}"},
	}
	_, err := Run(t.Context(), program, nil)
	if err == nil || !strings.Contains(err.Error(), "broken") || !strings.Contains(err.Error(), "demo/index.ts:2") {
		t.Fatalf("expected named source and original line, got %v", err)
	}
}

func TestBoundToolSDKRejectsWrongInput(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("optional type checker not installed")
	}
	p := Program{
		Source: `await tools.read({path:17})`,
		Tools: []llm.ToolDefinition{
			{
				Name: "read",
				Params: &llm.FunctionParameters{
					Type:       "object",
					Properties: llm.Object{"path": llm.Object{"type": "string"}},
					Required:   []string{"path"},
				},
			},
		},
	}
	if err := Check(t.Context(), p); err == nil || !strings.Contains(err.Error(), "not assignable") {
		t.Fatalf("expected invalid path type, got %v", err)
	}
}

func TestPermissionDenialSurvivesQueuedEffect(t *testing.T) {
	p := Program{Source: `return Promise.all([tools.read({}),tools.write({})])`}
	_, err := Run(t.Context(), p, func(_ context.Context, name string, _ json.RawMessage) (json.RawMessage, error) {
		if name != "read" {
			t.Fatalf("executed queued effect: %s", name)
		}
		return nil, errors.New("permission denied")
	})
	if err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("missing denial: %v", err)
	}
}

func TestCellBudgets(t *testing.T) {
	if _, err := Run(
		t.Context(),
		Program{Source: `return "x".repeat(300000)`},
		nil,
	); err == nil ||
		!strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("oversized output: %v", err)
	}
	var calls int
	_, err := Run(
		t.Context(),
		Program{Source: `for(let i=0;i<129;i++) await tools.read({})`},
		func(context.Context, string, json.RawMessage) (json.RawMessage, error) { calls++; return nil, nil },
	)
	if err == nil || calls != 128 {
		t.Fatalf("call limit: %d, %v", calls, err)
	}
}

func TestBundleCacheSeparatesCodeAndAuthority(t *testing.T) {
	for _, step := range []struct {
		source string
		names  []string
		want   string
	}{
		{"export const n = 1", []string{"first"}, "[1,null,[\"first\"]]"},
		{"export const n = 2", []string{"second"}, "[2,null,[\"second\"]]"},
		{"export const n = 1", []string{"third"}, "[1,null,[\"third\"]]"},
	} {
		p := Program{
			Source:  `import {n} from "cozy:lib/values"; const prior = globalThis.saved; globalThis.saved = 41; return [n, prior ?? null, await tools.list()]`,
			Modules: map[string]string{"values/index.ts": step.source},
		}
		for _, name := range step.names {
			p.Tools = append(p.Tools, llm.ToolDefinition{Name: name})
		}
		got, err := Run(t.Context(), p, nil)
		if err != nil || string(got) != step.want {
			t.Fatalf("module = %q: got %s, error %v; want %s", step.source, got, err, step.want)
		}
	}
}

func BenchmarkFreshCell(b *testing.B) {
	p := Program{Source: `const n: number = 40; return n + 2`}
	start := time.Now()
	if _, err := Run(b.Context(), p, nil); err != nil {
		b.Fatal(err)
	}
	b.Logf("initial call %v", time.Since(start))
	b.ResetTimer()
	for b.Loop() {
		if _, err := Run(b.Context(), p, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func TestConcurrentInstancesRemainIsolated(t *testing.T) {
	for _, name := range []string{"alpha", "beta", "gamma", "delta"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			p := Program{Source: `globalThis.owner = await tools.who({}); return globalThis.owner`}
			want := json.RawMessage(fmt.Sprintf("%q", name))
			got, err := Run(
				t.Context(),
				p,
				func(context.Context, string, json.RawMessage) (json.RawMessage, error) { return want, nil },
			)
			if err != nil || string(got) != string(want) {
				t.Fatalf("result %s, want %s: %v", got, want, err)
			}
		})
	}
}
