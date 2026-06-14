# Threads

Demonstrates spawn, join, Thread[T], and atomic-style concurrency helpers.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/threads
```

Run the example through the interpreter:

```sh
go run . run examples/threads
```

Package it as a browser WASM bundle:

```sh
go run . package examples/threads --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/threads --port=8080
```
