# Command Line Arena

A six-file command-line simulation example using restricted Int-heavy logic, allocation concepts, and O(n^2) style computation.
## Files

- `first.klang` is the project entry file.
- `app.klang` contains the main example module when present.
- Extra `.klang` files are local modules used by this example.

## Try It

Check the example:

```sh
go run . check examples/commandlinearena
```

Run the example through the interpreter:

```sh
go run . run examples/commandlinearena
```

Package it as a browser WASM bundle:

```sh
go run . package examples/commandlinearena --backend=WASM
```

Serve it directly with the built-in browser runtime server:

```sh
go run . serve examples/commandlinearena --port=8080
```
