# types stdlib example

This example is an in-depth tour of the builtin `types` namespace and numeric parent/child type features.

It demonstrates:

- Direct child-width types such as `Int.child(8)` and `UInt.child(32)`.
- Global aliases such as `i8`, `i16`, `u8`, `float32`, `complex64`, and `complex128`.
- Namespaced aliases such as `types.u8`.
- `.sizeof` on parent types, aliases, and direct child types.
- Assigning smaller child types into wider child or parent types.
- Runtime behavior shared with parent values, such as calling `.times` on an `i8`.
- Commented-out examples of range failures.

Run it from the repository root:

```sh
go run . run stdlib-examples/types
```
