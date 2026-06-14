# JSON Parser

Demonstrates JSON-like parsing/formatting workflows using tables, strings, and typed helpers.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/jsonparser
```

Run the example through the interpreter:

```sh
go run . run examples/jsonparser
```

Package it as a browser WASM bundle:

```sh
go run . package examples/jsonparser --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/jsonparser --port=8080
```
