# Klang WASM Bundle

This bundle contains a browser-hosted Klang runtime.

## Files

- `klang.wasm`: the Go interpreter/runtime compiled with `GOOS=js GOARCH=wasm`.
- `wasm_exec.js`: Go's JavaScript support shim for WASM.
- `klang_browser.js`: browser loader exposing `KlangBrowser.runProject()` and `KlangBrowser.runSource(source, args)`.
- `klang-build.json`: package manifest.
- `src/`: resolved Klang source files.

## Run Locally

Serve this folder through any static file server and open `index.html`:

```sh
python3 -m http.server 8080
```

Then visit http://localhost:8080.
