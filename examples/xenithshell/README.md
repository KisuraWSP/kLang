# Xenith Shell

Command-shell themed example using command input, branching, and typed command handling.
Note: this example may prompt for input when run through the interpreter.

## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/xenithshell
```

Run the example through the interpreter:

```sh
go run . run examples/xenithshell
```

Package it as a browser WASM bundle:

```sh
go run . package examples/xenithshell --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/xenithshell --port=8080
```
