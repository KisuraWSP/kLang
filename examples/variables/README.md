# Variables

Focused tour of variable declarations, inference, mutability, and value operations.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/variables
```

Run the example through the interpreter:

```sh
go run . run examples/variables
```

Package it as a browser WASM bundle:

```sh
go run . package examples/variables --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/variables --port=8080
```
