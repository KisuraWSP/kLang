# Hello World

Minimal project showing the standard first.klang entry point importing app.klang.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/helloworld
```

Run the example through the interpreter:

```sh
go run . run examples/helloworld
```

Package it as a browser WASM bundle:

```sh
go run . package examples/helloworld --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/helloworld --port=8080
```
