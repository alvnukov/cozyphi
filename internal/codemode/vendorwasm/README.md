# Embedded QuickJS WebAssembly module

`qjs.wasm` is the unmodified module from the [fastschema/qjs source revision](https://github.com/fastschema/qjs/tree/461716f4f380f81ffd09378751f1812919cddbca). The Git blob is `b1f0d255cccbdf6a09d736286a29bd33e67d12fa`. Its SHA-256 is `88fcbeead33cca3ec8394feb0fb3eec8047ec9ac5062aba7a63799cee11989b7`.

To rebuild from source, check out that revision with its recursive QuickJS-NG submodule. Install the WASI SDK, CMake and Binaryen as required by upstream, then run `make build` in that checkout. The upstream Makefile selects the module exports and runs wasm-opt. Before replacing the image, review its imports and exports and run the CozyPhi execution-boundary tests. A different toolchain may produce a different digest.

The adjacent MIT licenses cover the fastschema WebAssembly adapter and the QuickJS engine. The Go host instantiates the image using wazero with no filesystem mounts, inherited environment, socket permissions or process streams.
