# Coroutines

Demonstrates coroutine values, resume flow, and Option-based completion handling.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/coroutines
```

Run the example through the interpreter:

```sh
go run . run examples/coroutines
```

Package it as a browser WASM bundle:

```sh
go run . package examples/coroutines --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/coroutines --port=8080
```
