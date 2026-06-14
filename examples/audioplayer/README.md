# Audio Player

Models a small audio-player style workflow with typed state and playback controls.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/audioplayer
```

Run the example through the interpreter:

```sh
go run . run examples/audioplayer
```

Package it as a browser WASM bundle:

```sh
go run . package examples/audioplayer --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/audioplayer --port=8080
```
