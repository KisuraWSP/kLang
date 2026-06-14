# Web App

Web-app themed example that models frontend/runtime data flow in Klang source.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/webapp
```

Run the example through the interpreter:

```sh
go run . run examples/webapp
```

Package it as a browser WASM bundle:

```sh
go run . package examples/webapp --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/webapp --port=8080
```
