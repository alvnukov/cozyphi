package codemode

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const testTypes = `export declare function test(name: string, run: () => unknown): void;
export declare function equal(actual: unknown, expected: unknown): void;
export declare function fixture(name: string, args: Record<string, unknown>, value: unknown): void;
export declare function fixtureError(name: string, args: Record<string, unknown>, message: string): void;
`

// Check performs semantic TypeScript checking offline, without executing module
// code. The checker sees only the staged, pinned package graph and the guest SDK.
func Check(ctx context.Context, p Program) error {
	if err := validateProgram(p); err != nil {
		return err
	}
	deno, err := exec.LookPath("deno")
	if err != nil {
		return errors.New("library type check requires Deno 2 on PATH")
	}
	dir, err := os.MkdirTemp("", "cozyphi-types-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	absolute, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return err
	}
	dir = absolute
	files := make(map[string]string, len(p.Modules)+3)
	maps.Copy(files, p.Modules)
	imports, body := splitCell(p.Source)
	files["cell.ts"] = imports + "\nexport async function cell(){\n" + body + "\n}"
	files["sdk.d.ts"], err = sdkDeclarations(p.Tools)
	if err != nil {
		return fmt.Errorf("declare tool SDK: %w", err)
	}
	files["test.d.ts"] = testTypes
	// Map imports from a staging root explicitly. Deno resolves all absolute
	// file URLs through the catch-all deny mapping, including ones found in types.
	aliases := map[string]any{"file:///": nil, "cozy:test": "./test.d.ts"}
	names := make([]string, 0, len(files))
	for name, src := range files {
		if strings.Contains(src, "<reference") || strings.Contains(src, "@deno-types") ||
			strings.Contains(src, "@ts-types") ||
			strings.Contains(src, "@jsxImportSource") ||
			strings.Contains(src, "@ts-nocheck") {
			return fmt.Errorf("type checker directives are not supported in %s", name)
		}
		file := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(file), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(file, []byte(src), 0o600); err != nil {
			return err
		}
		key := fileURL(file)
		aliases[key] = key
		if pkg, _, ok := strings.Cut(name, "/"); ok {
			if root, resolveErr := resolveModule(p, "", "cozy:lib/"+pkg); resolveErr == nil {
				aliases["cozy:lib/"+pkg] = "./" + root
			}
		}
		names = append(names, name)
	}
	sort.Strings(names)
	config := map[string]any{"imports": aliases, "compilerOptions": map[string]any{
		"lib": []string{"es2022"}, "strict": true, "types": []string{"./sdk.d.ts"}, "noImplicitOverride": true,
	}}
	cfg, err := json.Marshal(config)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "deno.json"), cfg, 0o600); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	args := []string{"check", "--no-remote", "--no-npm", "--no-lock", "--config", "deno.json"}
	args = append(args, names...)
	cmd := exec.CommandContext(ctx, deno, args...)
	cmd.Dir = dir
	cmd.Env = []string{
		"DENO_DIR=" + filepath.Join(dir, "cache"),
		"DENO_NO_UPDATE_CHECK=1",
		"DENO_NO_PROMPT=1",
		"DENO_NO_PACKAGE_JSON=1",
		"NO_COLOR=1",
	}
	if systemRoot := os.Getenv("SystemRoot"); systemRoot != "" {
		cmd.Env = append(cmd.Env, "SystemRoot="+systemRoot)
	}
	out := &cappedOutput{}
	cmd.Stdout = out
	cmd.Stderr = out
	if err = cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf(
			"TypeScript check: %w: %s",
			err,
			strings.ReplaceAll(strings.TrimSpace(out.buf.String()), dir+"/", ""),
		)
	}
	return nil
}

func fileURL(filename string) string {
	name := filepath.ToSlash(filename)
	if !strings.HasPrefix(name, "/") {
		name = "/" + name
	}
	return (&url.URL{Scheme: "file", Path: name}).String()
}

type cappedOutput struct{ buf bytes.Buffer }

func (w *cappedOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16384 - w.buf.Len()
	if remaining > 0 {
		_, _ = w.buf.Write(p[:min(n, remaining)])
	}
	return n, nil
}
