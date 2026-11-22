package librarytool_test

import (
	"context"
	"encoding/json"
	"errors"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/alvnukov/cozyphi/internal/funclib"
	"github.com/alvnukov/cozyphi/internal/llm"
	"github.com/alvnukov/cozyphi/internal/tools/librarytool"
	"github.com/alvnukov/cozyphi/internal/tools/tooldef"
)

func call(t *testing.T, tool tooldef.Tool, in map[string]any) tooldef.Result {
	t.Helper()
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	res, err := tool.Run(t.Context(), raw)
	require.NoError(t, err, "%v", in["action"])
	require.NotNil(t, res.Structured)
	return res
}

func TestLibraryToolCrossProjectPublication(t *testing.T) {
	store := funclib.New(t.TempDir())
	first, second := t.TempDir(), t.TempDir()
	const source = "export function failedPackages(rows: string[]): number { return rows.length }"
	const tests = "import { equal, test } from 'cozy:test'; test('counts rows', () => equal(2, 2))"
	calls := 0
	check := func(_ context.Context, pkg funclib.Package, mods map[string]string) error {
		calls++
		if mods[pkg.Name+"/index.ts"] != source || mods[pkg.Name+"/index.test.ts"] != tests {
			return errors.New("unexpected modules presented for validation")
		}
		return nil
	}
	a := librarytool.Tool(store, "session-a", first, true, nil, check)
	b := librarytool.Tool(store, "session-b", second, false, nil, nil)
	open := map[string]any{"action": "open", "name": "builds"}
	require.Contains(t, call(t, a, open).Content, `"editableFiles":{}`)
	for file, content := range map[string]string{"index.ts": source, "index.test.ts": tests} {
		res := call(t, a, map[string]any{"action": "create_file", "name": "builds", "file": file, "content": content})
		require.Contains(t, res.Content, "@file")
	}
	require.Contains(t, call(t, a, open).Content, "1#")
	validated := call(t, a, map[string]any{"action": "test", "name": "builds"})
	var receipt struct {
		Revision string `json:"revision"`
	}
	require.NoError(t, json.Unmarshal([]byte(validated.Content), &receipt))
	require.NotEmpty(t, receipt.Revision)
	publish := map[string]any{"action": "publish", "name": "builds", "version": "1.0.0", "revision": receipt.Revision}
	raw, err := json.Marshal(publish)
	require.NoError(t, err)
	require.Contains(t, a.ApprovalPreview(t.Context(), raw), "failedPackages")
	require.Contains(t, call(t, a, publish).Content, `"version":"1.0.0"`)
	require.Equal(t, 2, calls)
	require.Contains(t, call(t, b, map[string]any{"action": "versions", "name": "builds"}).Content, `"version":"1.0.0"`)
	require.Contains(
		t,
		call(t, b, map[string]any{"action": "search", "query": "failedPackages"}).Content,
		"cozy:lib/builds",
	)
	get := map[string]any{"action": "get", "name": "builds", "file": "index.ts"}
	require.Contains(t, call(t, b, get).Content, "function failedPackages")
	_, err = b.Run(t.Context(), raw)
	require.ErrorContains(t, err, "unavailable")
	require.False(t, strings.Contains(call(t, b, map[string]any{"action": "list"}).Content, "return rows.length"))
}

func TestFailedValidationPreservesDraftAndRefusesPublication(t *testing.T) {
	store := funclib.New(t.TempDir())
	tool := librarytool.Tool(store, "session", t.TempDir(), true, nil,
		func(context.Context, funclib.Package, map[string]string) error { return errors.New("assertion failed") })
	call(t, tool, map[string]any{"action": "open", "name": "builds"})
	for _, f := range []string{"index.ts", "index.test.ts"} {
		call(
			t,
			tool,
			map[string]any{"action": "create_file", "name": "builds", "file": f, "content": "export const n = 1"},
		)
	}
	state, err := store.DraftState(t.Context(), "session", "builds")
	require.NoError(t, err)
	raw, err := json.Marshal(
		map[string]string{"action": "publish", "name": "builds", "version": "1.0.0", "revision": state.Digest},
	)
	require.NoError(t, err)
	_, err = tool.Run(t.Context(), raw)
	require.ErrorContains(t, err, "assertion failed")
	_, err = store.Get(t.Context(), "builds", "1.0.0")
	require.Error(t, err)
	require.Contains(t, call(t, tool, map[string]any{"action": "open", "name": "builds"}).Content, "1#")
}

func TestLibraryValidationUsesRealFixturesAndTypeChecks(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("Deno type checker unavailable")
	}
	modules := map[string]string{
		"builds/index.ts": `export async function status(id: string): Promise<number> {
			return (await tools.call("mcp_call", {id})).status;
		}`,
		"builds/index.test.ts": `import { equal,fixture,test } from "cozy:test";
		import { status } from "./index.ts";
		test("status is read via the expected tool arguments", async () => {
			fixture("mcp_call",{id:"42"},{status:200});
			equal(await status("42"),200);
		});`,
	}
	require.NoError(t, librarytool.Check(t.Context(), funclib.Package{}, modules))
	modules["builds/index.test.ts"] = strings.Replace(modules["builds/index.test.ts"], "status:200", "status:400", 1)
	require.Error(t, librarytool.Check(t.Context(), funclib.Package{}, modules))
	modules["builds/index.ts"] = `export async function status(id: string): Promise<number> { return "wrong"; }`
	require.ErrorContains(t, librarytool.Check(t.Context(), funclib.Package{}, modules), "type check")
}

func TestPublicationRejectsWrongNativeArguments(t *testing.T) {
	if _, err := exec.LookPath("deno"); err != nil {
		t.Skip("Deno type checker unavailable")
	}
	store := funclib.New(t.TempDir())
	definitions := []llm.ToolDefinition{{Name: "read", Params: &llm.FunctionParameters{
		Type: "object", Required: []string{"path"}, Properties: llm.Object{"path": llm.Object{"type": "string"}},
	}}}
	tool := librarytool.Tool(store, "schema-test", t.TempDir(), true, definitions, nil)
	call(t, tool, map[string]any{"action": "open", "name": "bad-reader"})
	contents := map[string]string{
		"index.ts": `export async function readBadly() { return await tools.read({path: 42}); }`,
		"index.test.ts": `import {test,equal,fixture} from "cozy:test";
			import {readBadly} from "./index.ts";
			test("reads the fixture",async()=>{fixture("read",{path:42},1);equal(await readBadly(),1)})`,
	}
	for file, content := range contents {
		call(t, tool, map[string]any{"action": "create_file", "name": "bad-reader", "file": file, "content": content})
	}
	pkg, err := store.DraftState(t.Context(), "schema-test", "bad-reader")
	require.NoError(t, err)
	for _, action := range []string{"test", "publish"} {
		raw, err := json.Marshal(
			map[string]string{"action": action, "name": "bad-reader", "version": "1.0.0", "revision": pkg.Digest},
		)
		require.NoError(t, err)
		_, err = tool.Run(t.Context(), raw)
		require.ErrorContains(t, err, "type check")
		require.ErrorContains(t, err, "string")
	}
	_, err = store.Get(t.Context(), "bad-reader", "1.0.0")
	require.Error(t, err)
}

func TestLibraryToolPinsMultiplePackages(t *testing.T) {
	store := funclib.New(t.TempDir())
	check := func(context.Context, funclib.Package, map[string]string) error { return nil }
	for _, name := range []string{"first", "second"} {
		_, err := store.Draft(t.Context(), "seed", name, "")
		require.NoError(t, err)
		for _, file := range []string{"index.ts", "index.test.ts"} {
			_, err = store.CreateFile(t.Context(), "seed", name, file, "export const item = 1")
			require.NoError(t, err)
		}
		_, err = store.Publish(t.Context(), "seed", name, "1.0.0", check)
		require.NoError(t, err)
	}
	tool := librarytool.Tool(store, "author", t.TempDir(), true, nil, nil)
	res := call(
		t,
		tool,
		map[string]any{"action": "pin", "versions": map[string]string{"first": "1.0.0", "second": "1.0.0"}},
	)
	var pins map[string]funclib.Pin
	require.NoError(t, json.Unmarshal([]byte(res.Content), &pins))
	require.Len(t, pins, 2)
	for _, pin := range pins {
		require.Equal(t, "1.0.0", pin.Version)
		require.NotEmpty(t, pin.Digest)
	}
}
