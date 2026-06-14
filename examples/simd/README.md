# SIMD

Demonstrates SIMD values and vector-style numeric operations.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/simd
```

Run the example through the interpreter:

```sh
go run . run examples/simd
```

Package it as a browser WASM bundle:

```sh
go run . package examples/simd --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/simd --port=8080
```
