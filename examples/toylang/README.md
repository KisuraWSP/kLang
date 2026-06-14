# Toy Language

Small toy-language style example showing parser/interpreter-like data flow in Klang.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/toylang
```

Run the example through the interpreter:

```sh
go run . run examples/toylang
```

Package it as a browser WASM bundle:

```sh
go run . package examples/toylang --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/toylang --port=8080
```
