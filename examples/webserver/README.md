# Web Server

Simple webserver-themed project that loads an index.html-style page.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/webserver
```

Run the example through the interpreter:

```sh
go run . run examples/webserver
```

Package it as a browser WASM bundle:

```sh
go run . package examples/webserver --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/webserver --port=8080
```
