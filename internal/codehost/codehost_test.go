package codehost

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/project"
	"github.com/alvnukov/cozyphi/internal/tools"
	"github.com/alvnukov/cozyphi/internal/tools/librarytool"
)

func testProject(t *testing.T) *project.Project {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	p, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	return p
}

func TestCellTestsUseOnlyFixtures(t *testing.T) {
	project := testProject(t)
	store := funclib.New(project.Global().FunctionsDir())
	tool := cellTestTool(store, project.CheckoutRoot(), nil)
	source := "const r = await tools.bash({command:'product-test'}); return r.exit === 0"
	cases := []cellCase{
		{Name: "passing product", Calls: []mockCall{{Name: "bash", Args: json.RawMessage(`{"command":"product-test"}`), Result: json.RawMessage(`{"exit":0}`)}}, Want: json.RawMessage("true")},
		{Name: "failing product", Calls: []mockCall{{Name: "bash", Args: json.RawMessage(`{"command":"product-test"}`), Result: json.RawMessage(`{"exit":1}`)}}, Want: json.RawMessage("false")},
	}
	run := func(code string, cases []cellCase) (string, error) {
		raw, err := json.Marshal(cellCheck{Source: code, Cases: cases})
		require.NoError(t, err)
		r, err := tool.Run(t.Context(), raw)
		return r.Content, err
	}
	out, err := run(source, cases)
	require.NoError(t, err)
	require.Contains(t, out, `"passed":2`)
	_, err = run("try { await tools.write({}); } catch {} return true", []cellCase{{Name: "unknown", Want: json.RawMessage("true")}})
	require.ErrorContains(t, err, "unexpected fixture call")
	cases[0].Want = json.RawMessage("false")
	_, err = run(source, cases)
	require.ErrorContains(t, err, "expected false, received true")
}

func TestPublishedFunctionIsSuggestedAndExecutedInAnotherCheckout(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("Deno 2 required for release type checking")
	}
	p := testProject(t)
	ctx := t.Context()
	store := funclib.New(p.Global().FunctionsDir())
	_, err := store.Draft(ctx, "author", "records", "")
	require.NoError(t, err)
	_, err = store.CreateFile(ctx, "author", "records", "index.ts", "export function countRecords(rows: number[]): number { return rows.length; }")
	require.NoError(t, err)
	_, err = store.CreateFile(ctx, "author", "records", "index.test.ts", `import {test,equal} from "cozy:test";
import {countRecords} from "./index.ts";
test("counts supplied records",()=>equal(countRecords([3,4,4]),3))`)
	require.NoError(t, err)
	release, err := store.Publish(ctx, "author", "records", "1.0.0", librarytool.Check)
	require.NoError(t, err)
	second, err := project.Discover(t.TempDir())
	require.NoError(t, err)
	require.NotEqual(t, p.CheckoutRoot(), second.CheckoutRoot())
	mode := New(second.Global(), project.CodeModeConfig{Discovery: "local"}, nil)
	hints := mode.Hints(ctx, "other-session", second.Root(), "countRecords", nil)
	require.Contains(t, hints, "countRecords")
	require.Contains(t, hints, "cozy:lib/records")
	source := `import { countRecords } from "cozy:lib/records"; return countRecords([7,8,8,9]);`
	out, err := mode.Run(ctx, "other-session", second.Root(), true, source, nil, func(context.Context, string, json.RawMessage) (any, error) {
		t.Fatal("unexpected host effect in pure library")
		return nil, nil
	})
	require.NoError(t, err)
	require.Equal(t, "4", out.Output)
	require.Equal(t, "1.0.0@"+release.Digest, out.Imports["records"])
}

func TestCellCanWriteFixAndTestProjectCode(t *testing.T) {
	p := testProject(t)
	t.Chdir(p.Root())
	testSource := "package sum\nimport \"testing\"\nfunc TestSum(t *testing.T) { if sum(3,4) != 7 { t.Fatal(\"wrong sum\") } }\n"
	require.NoError(t, os.WriteFile(filepath.Join(p.Root(), "sum_test.go"), []byte(testSource), 0o600))
	const src = `const wrote = await tools.write({path:"sum.go",content:"package sum\nfunc sum(a,b int) int { return a-b }\n"});
const red = await tools.bash({command:"go test sum.go sum_test.go"});
await tools.edit({path:"sum.go",hash:wrote.hash,edits:[{from:wrote.anchors[1],to:wrote.anchors[1],content:"func sum(a,b int) int { return a+b }"}]});
const green = await tools.bash({command:"go test sum.go sum_test.go"});
return {failed:red.exitCode!==0 && red.output.includes("wrong sum"), final:green.exitCode};`
	native := tools.DefaultTools()
	reg := tools.NewRegistry(native)
	mode := New(p.Global(), project.CodeModeConfig{Discovery: "manual"}, nil)
	out, err := mode.Run(t.Context(), "test-author", p.Root(), true, src, tools.Definitions(native), func(ctx context.Context, name string, args json.RawMessage) (any, error) {
		handler, ok := reg[name]
		require.True(t, ok)
		result, err := handler.Run(ctx, args)
		return result.Structured, err
	})
	require.NoError(t, err)
	require.JSONEq(t, `{"failed":true,"final":0}`, out.Output)
	result, err := os.ReadFile("sum.go")
	require.NoError(t, err)
	require.Equal(t, "package sum\nfunc sum(a,b int) int { return a+b }\n", string(result))
}
