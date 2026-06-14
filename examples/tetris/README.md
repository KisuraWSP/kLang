# Tetris

Simple Tetris-style game-logic example with board and piece state.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/tetris
```

Run the example through the interpreter:

```sh
go run . run examples/tetris
```

Package it as a browser WASM bundle:

```sh
go run . package examples/tetris --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/tetris --port=8080
```
