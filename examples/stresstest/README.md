# Stress Test

Stress-style example that imports stdlib helpers and exercises repeated runtime operations.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/stresstest
```

Run the example through the interpreter:

```sh
go run . run examples/stresstest
```

Package it as a browser WASM bundle:

```sh
go run . package examples/stresstest --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/stresstest --port=8080
```
