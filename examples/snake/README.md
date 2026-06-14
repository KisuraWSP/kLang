# Snake

Simple game-logic example modeling snake state and board updates.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/snake
```

Run the example through the interpreter:

```sh
go run . run examples/snake
```

Package it as a browser WASM bundle:

```sh
go run . package examples/snake --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/snake --port=8080
```
