# Restricted Generics

Shows restricted generic syntax and strict checking of allowed generic types.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/restrict
```

Run the example through the interpreter:

```sh
go run . run examples/restrict
```

Package it as a browser WASM bundle:

```sh
go run . package examples/restrict --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/restrict --port=8080
```
