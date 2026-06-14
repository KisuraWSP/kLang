# Lambda Functions

Shows lambda and first-class function usage in a compact project.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/lambdafunctions
```

Run the example through the interpreter:

```sh
go run . run examples/lambdafunctions
```

Package it as a browser WASM bundle:

```sh
go run . package examples/lambdafunctions --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/lambdafunctions --port=8080
```
