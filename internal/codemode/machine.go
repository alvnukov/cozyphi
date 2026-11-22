package codemode

import (
	"context"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

//go:embed vendorwasm/qjs.wasm
var runtimeImage []byte

//go:embed guest.js
var guestJS string

// The compilation cache stores machine code only. Each execution gets fresh
// guest memory, handles, module graph, WASI namespace and tool output.
var cache = sync.OnceValue(wazero.NewCompilationCache)

type machine struct {
	ctx context.Context
	mod api.Module
	js  uint64
}

func (m machine) invoke(name string, args ...uint64) (uint64, error) {
	fn := m.mod.ExportedFunction(name)
	if fn == nil {
		return 0, fmt.Errorf("missing JS runtime export %s", name)
	}
	v, err := fn.Call(m.ctx, args...)
	if err != nil {
		return 0, fmt.Errorf("JS runtime %s: %w", name, err)
	}
	if len(v) == 0 {
		return 0, nil
	}
	return v[0], nil
}

func newMachine(ctx context.Context, r wazero.Runtime) (machine, error) {
	var m machine
	if _, err := wasi_snapshot_preview1.Instantiate(ctx, r); err != nil {
		return m, err
	}
	// The upstream ABI imports this callback even when no proxy classes exist.
	// Only the explicit, validated host request returned by take() can perform I/O.
	if _, err := r.NewHostModuleBuilder("env").
		NewFunctionBuilder().
		WithFunc(func(uint32, uint64, uint32, uint32) uint64 { return 0 }).
		Export("jsFunctionProxy").
		Instantiate(ctx); err != nil {
		return m, err
	}
	compiled, err := r.CompileModule(ctx, runtimeImage)
	if err != nil {
		return m, err
	}
	// No mounted directories, environment, sockets, stdout or stdin.
	mod, err := r.InstantiateModule(ctx, compiled, wazero.NewModuleConfig().WithStartFunctions())
	if err != nil {
		return m, err
	}
	m.ctx = ctx
	m.mod = mod
	handle, err := m.invoke("New_QJS", 32<<20, 512<<10, 0, 0)
	if err != nil {
		return m, err
	}
	if handle == 0 {
		return m, errors.New("cannot allocate JS heap")
	}
	m.js, err = m.invoke("QJS_GetContext", handle)
	return m, err
}

func (m machine) cstring(s string) (uint64, error) {
	ptr, err := m.invoke("malloc", uint64(len(s)+1))
	if err != nil {
		return 0, err
	}
	if ptr == 0 || !m.mod.Memory().WriteString(uint32(ptr), s) ||
		!m.mod.Memory().WriteByte(uint32(ptr)+uint32(len(s)), 0) {
		return 0, errors.New("JS heap exhausted")
	}
	return ptr, nil
}

// eval reads a bounded string from the QuickJS C ABI. Unlike standard output,
// this value can never fabricate a host-owned approval or delivery receipt.
func (m machine) eval(source string) (string, error) { return m.evalAt(source, "cozy-host.js") }

func (m machine) evalAt(source, filename string) (string, error) {
	ptr, err := m.cstring(source)
	if err != nil {
		return "", err
	}
	file, err := m.cstring(filename)
	if err != nil {
		return "", err
	}
	opts, err := m.invoke("QJS_CreateEvalOption", ptr, 0, 0, file, 0)
	if err != nil {
		return "", err
	}
	value, err := m.invoke("QJS_Eval", m.js, opts)
	if err != nil {
		return "", err
	}
	for _, p := range []uint64{ptr, file, opts} {
		if _, err = m.invoke("free", p); err != nil {
			return "", err
		}
	}
	threw, err := m.invoke("JS_HasException", m.js)
	if err != nil {
		return "", err
	}
	if threw != 0 {
		value, err = m.invoke("JS_GetException", m.js)
		if err != nil {
			return "", err
		}
	}
	packed, err := m.invoke("QJS_ToCString", m.js, value)
	if err != nil {
		return "", err
	}
	data, ok := m.mod.Memory().Read(uint32(packed), 8)
	if !ok || packed == 0 {
		return "", errors.New("cannot decode JS value")
	}
	n := binary.LittleEndian.Uint32(data[:4])
	offset := binary.LittleEndian.Uint32(data[4:])
	if n > maxValue {
		return "", errors.New("JS output exceeds limit")
	}
	bytes, ok := m.mod.Memory().Read(offset, n)
	if !ok {
		return "", errors.New("invalid JS output")
	}
	result := string(bytes)
	if _, err = m.invoke("QJS_FreeValue", m.js, value); err != nil {
		return "", err
	}
	if _, err = m.invoke("JS_FreeCString", m.js, uint64(offset)); err != nil {
		return "", err
	}
	if _, err = m.invoke("free", packed); err != nil {
		return "", err
	}
	if threw != 0 {
		return "", fmt.Errorf("cell: %s", result)
	}
	return result, nil
}

func Run(ctx context.Context, p Program, host Host) (json.RawMessage, error) {
	return execute(ctx, p, host, false)
}

// Test always uses fixture tools. The argument has the same shape as Run for adapters.
func Test(ctx context.Context, p Program, _ Host) (json.RawMessage, error) {
	return execute(ctx, p, nil, true)
}

func execute(ctx context.Context, p Program, h Host, testing bool) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	js, err := bundle(p, testing)
	if err != nil {
		return nil, err
	}
	defs, err := json.Marshal(p.Tools)
	if err != nil {
		return nil, err
	}
	if len(defs) > maxModuleBytes {
		return nil, errors.New("tool catalog exceeds limit")
	}
	ctx, cancel := context.WithTimeout(ctx, maxDuration)
	defer cancel()
	config := wazero.NewRuntimeConfig().
		WithCloseOnContextDone(true).
		WithMemoryLimitPages(1024).
		WithCompilationCache(cache())
	r := wazero.NewRuntimeWithConfig(ctx, config)
	defer r.Close(context.Background())
	m, err := newMachine(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("initialize code mode: %w", err)
	}
	startup := fmt.Sprintf("%s(%s,%t,%t);void 0", guestJS, defs, testing, p.ParallelReads && !testing)
	if _, err = m.eval(startup); err != nil {
		return nil, err
	}
	if _, err = m.evalAt(js.js+"\nvoid 0", "cozy-bundle.js"); err != nil {
		return nil, js.mapping.annotate(err)
	}
	value, err := drive(m, h, p.ParallelReads && !testing)
	return value, js.mapping.annotate(err)
}

type frame struct {
	Done   bool
	ID     int
	Name   string
	Args   json.RawMessage
	Result json.RawMessage
	Error  string
	Batch  []frame
}

func validateCall(f frame) error {
	if f.ID <= 0 || len(f.Name) > 256 || f.Name == "" || strings.HasPrefix(f.Name, "cozy:") ||
		len(f.Args) == 0 || f.Args[0] != '{' || len(f.Batch) > 0 || f.Done {
		return errors.New("invalid cell tool request")
	}
	return nil
}

func drive(m machine, h Host, parallelReads bool) (json.RawMessage, error) {
	calls := 0
	for {
		s, err := m.eval("cozyCell.take()")
		if err != nil {
			return nil, err
		}
		var f frame
		if err = json.Unmarshal([]byte(s), &f); err != nil {
			return nil, fmt.Errorf("invalid cell response: %w", err)
		}
		if f.Done {
			if f.Error != "" {
				return nil, errors.New(f.Error)
			}
			if len(f.Result) == 0 {
				return nil, errors.New("cell result must be a JSON value")
			}
			return f.Result, nil
		}
		if len(f.Batch) > 0 {
			calls += len(f.Batch)
			if !parallelReads || len(f.Batch) < 2 || len(f.Batch) > 8 || calls > maxCalls {
				return nil, errors.New("invalid parallel read batch")
			}
			if err = m.batch(h, f.Batch); err != nil {
				return nil, err
			}
			continue
		}
		calls++
		if calls > maxCalls {
			return nil, errors.New("cell tool call limit exceeded")
		}
		if err = validateCall(f); err != nil {
			return nil, err
		}
		var answer json.RawMessage
		if err = m.ctx.Err(); err != nil {
			return nil, err
		}
		if h == nil {
			err = fmt.Errorf("tool %q unavailable", f.Name)
		} else {
			answer, err = h(m.ctx, f.Name, f.Args)
		}
		if err = m.settle(f.ID, answer, err); err != nil {
			return nil, err
		}
	}
}

type batchRequest struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}
type batchAnswer struct {
	Value json.RawMessage `json:"value"`
	Error string          `json:"error"`
}

func (m machine) batch(h Host, batch []frame) error {
	if h == nil {
		return errors.New("parallel read host unavailable")
	}
	calls := make([]batchRequest, 0, len(batch))
	for _, f := range batch {
		if err := validateCall(f); err != nil {
			return err
		}
		switch f.Name {
		case "ls", "find":
		case "read":
			var arg struct{ Mode string }
			if json.Unmarshal(f.Args, &arg) != nil || (arg.Mode != "" && arg.Mode != "view") {
				return errors.New("unsupported parallel read mode")
			}
		default:
			return errors.New("unsupported parallel read operation")
		}
		calls = append(calls, batchRequest{f.Name, f.Args})
	}
	args, err := json.Marshal(map[string]any{"calls": calls})
	if err != nil {
		return err
	}
	if err = m.ctx.Err(); err != nil {
		return err
	}
	raw, err := h(m.ctx, "cozy:parallel_read", args)
	if err != nil {
		return err
	}
	if len(raw) > len(batch)*(2<<20) {
		return errors.New("parallel result exceeds limit")
	}
	var replies []batchAnswer
	if err = json.Unmarshal(raw, &replies); err != nil || len(replies) != len(batch) {
		return errors.New("invalid parallel read response")
	}
	for i, reply := range replies {
		var callErr error
		if reply.Error != "" {
			callErr = errors.New(reply.Error)
		}
		if err = m.settle(batch[i].ID, reply.Value, callErr); err != nil {
			return err
		}
	}
	return nil
}

func (m machine) settle(id int, answer json.RawMessage, callErr error) error {
	if err := m.ctx.Err(); err != nil {
		return err
	}
	if callErr == nil {
		if len(answer) == 0 {
			answer = json.RawMessage("null")
		}
		if len(answer) > 2<<20 || !json.Valid(answer) {
			return errors.New("invalid or oversized host result")
		}
	} else {
		msg := callErr.Error()
		if len(msg) > 4096 {
			msg = msg[:4096]
		}
		answer, _ = json.Marshal(msg)
	}
	// Pass host output as JSON data, including keys with special JavaScript syntax.
	encoded, _ := json.Marshal(string(answer))
	command := fmt.Sprintf("cozyCell.settle(%d,%t,JSON.parse(%s));void 0", id, callErr == nil, encoded)
	_, err := m.eval(command)
	return err
}
