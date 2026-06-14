# Markdown to HTML Converter

Converts markdown-like text into HTML-like output using strings and table/list helpers.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/markdowntohtmlconvertor
```

Run the example through the interpreter:

```sh
go run . run examples/markdowntohtmlconvertor
```

Package it as a browser WASM bundle:

```sh
go run . package examples/markdowntohtmlconvertor --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/markdowntohtmlconvertor --port=8080
```
