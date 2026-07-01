# kLang Conformance

The authoritative machine-readable backend matrix is
`src/engine/conformance/backend_features.json`. Its schema and stable feature
identifiers are validated by ordinary Go tests.

The matrix distinguishes:

- `Standalone`: the Go interpreter and native host primitives;
- `JS`: native JavaScript code generation;
- `WASM`: browser packaging, including explicit interpreter fallback; and
- `bytecode`: the compiled `KBC1` VM subset used by WASM when supported.

Support statuses are `interpreted`, `compiled`, `host-provided`, `fallback`,
and `rejected`. A backend must never omit a feature or silently claim support.
Unsupported backend diagnostics carry a matrix `FeatureID`.

Run correctness and differential checks with:

```sh
go test ./...
```

Run bounded fuzz seeds with the ordinary suite, or extended fuzzing separately:

```sh
go test ./src/lexer -fuzz=FuzzLexer -fuzztime=30s
go test ./src/parser -fuzz=FuzzParser -fuzztime=30s
go test ./src/engine/bytecode -fuzz=FuzzBytecode -fuzztime=30s
go test ./src/engine/runtime -fuzz=FuzzListIndexing -fuzztime=30s
go test ./src/engine/type_checker -fuzz=FuzzTypeString -fuzztime=30s
```

Performance is measured separately and is never used to relax correctness:

```sh
go test ./src/engine/conformance -run='^$' -bench=. -benchmem
```
