# Built into the language by default
1. Int
2. UInt
3. String
4. Float
5. Bool
6. Char
7. Map[$Key, $Value]
8. List[...$Items]
9. Set[$Item]
10. Option[$Item]
11. Result[$Ok, $Err]
12. Complex
13. SIMD[$Lane]
14. Function[...$Args, $Return]
15. T // Builtin Generic Type Value containing Information of respective data type
16. T[$Region] // Region-backed array/slice storage with zero-based indexing and region capacity checks; `temp region` marks storage as temporary
17. Box[$Item]
18. Ref[$Item]
19. RefMut[$Item]
20. RefCell[$Item]
21. HeapAllocator
22. RegionAllocator
23. BumpAllocator
24. ArenaAllocator
25. Table // Lua-style dynamic table; this is the only dynamically typed container
26. Awaitable[$Item]
27. Iterator[$Item]
28. Coroutine[$Item]
29. Thread[$Item] // Multi-threaded interpreter worker handle returned by spawn
30. Args // Builtin immutable List[String] containing command line arguments passed to the program workspace
31. Any // Fully dynamic wildcard type; unlike T, it cannot be restricted and accepts any value
32. Atomic[$Item] // Runtime synchronized cell for race-safe shared numeric/value updates
33. Program // Meta-programming descriptor containing module : List[String]
34. BuildSystem // Compact build descriptor containing project_name, number_of_files, files, and backend
35. WorkSpace // Meta workspace combining Program and BuildSystem
36. JSModule // Filesystem-only JavaScript module descriptor loaded from a .js file
37. JSCall // Filesystem-only JavaScript API call descriptor
38. Enum // User-defined typed ordinal enum values declared with `enum`
39. Context // Compiler/runtime source context for a workspace, including files, entry point, backend, and diagnostics
40. ErrorContext // Source-aware diagnostic containing phase, file, line, column span, rule, message, hint, source line, suggestions, and type context
41. Type // Runtime metadata for every language type, returned by `SomeType.get_runtime_type_info()`

All builtin type names expose a compile-time size query through `.sizeof`, which returns an `Int`.
For example, `Int.sizeof` returns the runtime size used for an `Int` value.

Numeric parent types expose child-width types through `.child(bits)`. `Int` and `UInt` default to 64 bits and support 8, 16, 32, and 64 bit children. `Float` defaults to 64 bits and supports 32 and 64 bit children. `Complex` defaults to 128 bits and supports 64 and 128 bit children. The builtin aliases `i8`, `i16`, `i32`, `i64`, `u8`, `u16`, `u32`, `u64`, `float32`, `float64`, `complex64`, and `complex128` are globally available through the internal `types` namespace without imports.

Integer values may be written in decimal, hexadecimal (`0x`/`0X`), octal (`0o`/`0O`), or binary (`0b`/`0B`) form. Numeric literals may use `_` separators between digits for readability, such as `1_000_000`, `0xFF_FF`, or `12_345.67_89`. Signed integer and float literals use a leading `-`, and child-width range checks apply after the literal is parsed.

Every builtin and user-visible type conforms to the parent `Type` metadata model. `SomeType.get_runtime_type_info()` returns a `Type` object with fields such as `name`, `size`, `alignment`, `footprint`, `fields`, `field_count`, `serialization`, `introspection`, `layout`, `supports_serialization`, `supports_introspection`, and `supports_memory_layout`.

Builtin values expose a small shared protocol surface through selector syntax:
- `String`, `List[T]`, `Set[T]`, `Map[K, V]`, `Table`, `SIMD[T]`, and `Iterator[T]` provide `.count : Int`.
- `String` and `Char` provide `.uppercase()` and `.lowercase()`.
- `Int` and `UInt` provide `.times(callback : Function[Int, T])`, which calls the callback for each zero-based index and returns the callback's last result.

Generic type parameters may use named constraints in addition to explicit `restrict[...]` allow-lists. `T numeric` accepts numeric parent and child-width types, `T comparable` accepts primitive comparable values, `T hashable` accepts the safe primitive key space used by `Table` and `Set`, `T iterable` accepts values supported by `iter`/`len`, `T allocator_like` accepts builtin allocator/value wrappers, and `T TraitName` requires an `impl TraitName for ConcreteType` declaration.

User-defined `enum` declarations create typed ordinal values inspired by Go `const`/`iota` enums. Variants are selected as `EnumName.Variant`, compare only with variants from the same enum type, and expose `.ordinal`, `.name`, and `.variant`.

Aggregate collection values use copy-on-write storage for ordinary assignment. Shared `List`, `Set`, `Map`, `Table`, and `SIMD` storage is detached when a mutable binding is written, while explicit `copy` and `clone` still create eager clones.

`Set[T]` stores unique primitive values in deterministic insertion order. `Set([items...])` builds a set from a list and deduplicates repeated values. Set items use the same safe primitive key space as `Table`: `String`, `Int`, `UInt`, `Float`, `Bool`, and `Char`. `set_has(set, value)` tests membership, and `iter(set)` yields the unique values in insertion order.

`Table` is the only fully dynamic container. It stores mixed values and accepts only primitive value keys: `String`, `Int`, `UInt`, `Float`, `Bool`, and `Char`. Keys compare by their normalized primitive kind plus value, so `1`, `"1"`, and `'1'` are distinct keys. Unsafe dynamic keys such as `Table`, `List`, functions, refs, allocator objects, and other runtime objects are rejected. `table["name"]` reads a user field and reports a missing-key diagnostic when absent; reads never create fields. Indexed mutation detaches shared storage before writing. `data.count` is the builtin protocol count for the table's own entries, while `data["count"]` reads the user field named `"count"`. Other non-conflicting selectors such as `data.name` remain sugar for `data["name"]`.

Table helper builtins are available without imports:
- `table_has(table, key)` and `has_key(table, key)` return `Bool`.
- `table_delete(table, key)` returns a new `Table` without the key; assigning the result applies deletion.
- `table_keys(table)`, `table_values(table)`, and `table_entries(table)` return insertion-order lists. Entries are `Table` records with `key` and `value` fields.
- `table_sequence_count(table)` returns the contiguous zero-based numeric length and never replaces `table.count`.
- `table_set_fallback(child, parent)` returns a `Table` that reads missing keys from `parent`.

`iter(table)` yields insertion-order `{key, value}` entry tables. Table iteration order is deterministic insertion order for own entries; fallback entries are visible through lookup but are not included in `.count`, `table_keys`, `table_values`, `table_entries`, or direct iteration.

The language engine builds a `Context` for each loaded workspace and reports failures through `ErrorContext`. This diagnostic context is used by module resolution, parsing, type checking, runtime execution, packaging, and WASM backend generation. Type diagnostics may include source spans, "did you mean" suggestions, import hints, and expected/found type trees.

The compiler and runtime track symbol state for globals, locals, temporary variables, regions, temporary regions, parameters, named returns, and function return values. Compile-time state records include the declaration kind, name, type, function, mutability, file, and line. Runtime state records include the phase, event (`define`, `bind`, `assign`, `move`, or `return`), kind, declared type, runtime type, function, mutability, and moved status. Temporary variables use the state kind `temporary`, and temporary memory regions use `temporary_region`. `debug_state()` returns the runtime records as `List[Table]`.
