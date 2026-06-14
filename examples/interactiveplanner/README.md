# Interactive Planner

Interactive command-line planning example that uses input-oriented flows and typed records.
Note: this example may prompt for input when run through the interpreter.

## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/interactiveplanner
```

Run the example through the interpreter:

```sh
go run . run examples/interactiveplanner
```

Package it as a browser WASM bundle:

```sh
go run . package examples/interactiveplanner --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/interactiveplanner --port=8080
```
