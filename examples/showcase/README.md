# Showcase

Large multi-file showcase covering many current language features in one project.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/showcase
```

Run the example through the interpreter:

```sh
go run . run examples/showcase
```

Package it as a browser WASM bundle:

```sh
go run . package examples/showcase --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/showcase --port=8080
```
