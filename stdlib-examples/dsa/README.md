# dsa stdlib example

This example is a comprehensive tour of the `dsa` standard library module.
It demonstrates:

- exact type aliases for scalar, list, option, and DSA alias-struct types
- chained aliases such as `score_history -> scores -> List[score]`
- aliases nested in `Option[T]` and used in function signatures
- struct-family aliases for `Stack`, `Queue`, and `OrderedMap`
- immutable Stack construction, push, peek, pop, values, count, and empty state
- mutable Stack bindings that repeatedly assign push/pop results
- immutable Queue construction, enqueue, peek, dequeue, values, and empty state
- mutable Queue bindings that repeatedly assign enqueue/dequeue results
- direct `Stack`, `Queue`, and `OrderedMap` alias methods
- ordered-map construction from parallel lists
- duplicate-key handling and shortest-list construction
- ordered lookup, replacement, insertion, removal, key/value views, and pairs
- mutable OrderedMap and `arrayhashmap` workflows
- the `dsa.arrayhashmap` compatibility namespace

Assertions are silent. The program prints only the actual structures and values
produced by each operation.

Run it from the repository root:

```sh
go run . run stdlib-examples/dsa
```

All operations return new values. A mutable binding can be reassigned to each
result, while snapshots of earlier values remain unchanged. A final return
value of `0` means every assertion passed.

Type aliases in this example are compile-time synonyms. They improve the
domain vocabulary and shorten nested types without creating wrappers or
changing the runtime representation of a value.
