# Static Page Server

Shows a static page server style project with an index.html asset beside Klang source.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/staticpageserver
```

Run the example through the interpreter:

```sh
go run . run examples/staticpageserver
```

Package it as a browser WASM bundle:

```sh
go run . package examples/staticpageserver --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/staticpageserver --port=8080
```
