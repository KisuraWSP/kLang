# Game of Life

Runs a small cellular-automata style simulation over list/grid-like data.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/gameoflife
```

Run the example through the interpreter:

```sh
go run . run examples/gameoflife
```

Package it as a browser WASM bundle:

```sh
go run . package examples/gameoflife --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/gameoflife --port=8080
```
