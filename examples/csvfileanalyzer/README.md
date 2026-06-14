# CSV File Analyzer

Shows a typed CSV-analysis style workflow with lists, parsing helpers, and reporting output.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/csvfileanalyzer
```

Run the example through the interpreter:

```sh
go run . run examples/csvfileanalyzer
```

Package it as a browser WASM bundle:

```sh
go run . package examples/csvfileanalyzer --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/csvfileanalyzer --port=8080
```
