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
34. BuildSystem // Compact build descriptor containing project_name, number_of_files, files, and backend; JS selects experimental native JavaScript code generation
35. WorkSpace // Meta workspace combining Program and BuildSystem
36. JSModule // Filesystem-only JavaScript module descriptor loaded from a .js file
37. JSCall // Filesystem-only JavaScript API call descriptor
38. Enum // User-defined typed ordinal enum values declared with `enum`
39. Context // Compiler/runtime source context for a workspace, including files, entry point, backend, and diagnostics
40. ErrorContext // Source-aware diagnostic containing phase, file, line, column span, rule, message, hint, source line, suggestions, and type context
41. Type // Runtime metadata for every language type, returned by `SomeType.get_runtime_type_info()`
42. JSON // Immutable parsed JSON value with object, array, string, number, bool, and null variants
43. Parsable[$Item] // Immutable source, AST, runtime argument, Program, BuildSystem, and WorkSpace metadata for one Klang program

All builtin type names expose a compile-time size query through `.sizeof`, which returns an `Int`.
For example, `Int.sizeof` returns the runtime size used for an `Int` value.

`type name = ExistingType;` declares a workspace-scoped compile-time type alias. Aliases may use lowercase or snake_case names, may refer forward to aliases declared later, may chain through other aliases, and may appear recursively inside generic types such as `Option[string_list]`. They are exact synonyms: they create no runtime value, allocation, or nominal type distinction. Cyclic aliases and aliases targeting unknown types are rejected.

Numeric parent types expose child-width types through `.child(bits)`. `Int` and `UInt` default to 64 bits and support 8, 16, 32, and 64 bit children. `Float` defaults to 64 bits and supports 32 and 64 bit children. `Complex` defaults to 128 bits and supports 64 and 128 bit children. The builtin aliases `i8`, `i16`, `i32`, `i64`, `u8`, `u16`, `u32`, `u64`, `float32`, `float64`, `complex64`, and `complex128` are globally available through the internal `types` namespace without imports.

Integer values may be written in decimal, hexadecimal (`0x`/`0X`), octal (`0o`/`0O`), or binary (`0b`/`0B`) form. Numeric literals may use `_` separators between digits for readability, such as `1_000_000`, `0xFF_FF`, or `12_345.67_89`. Signed integer and float literals use a leading `-`, and child-width range checks apply after the literal is parsed.

Every builtin and user-visible type conforms to the parent `Type` metadata model. `SomeType.get_runtime_type_info()` returns a `Type` object with fields such as `name`, `size`, `alignment`, `footprint`, `fields`, `field_count`, `serialization`, `introspection`, `layout`, `supports_serialization`, `supports_introspection`, and `supports_memory_layout`.

Struct-style alias functions create user-visible object types. Their constructor parameters are statically checked fields, generic constructor arguments flow into those field and method types, and `#extend` methods are checked with `this` bound to the alias receiver. Alias generic lists may appear before the alias name (`alias function[T Printable] Box(...)`) or after it (`alias function Box[T Printable](...)`). Trait, named, and nested `restrict[...]` constraints propagate through constructor fields and specializations. A constructor field may declare a JSON name with ``this.field `json:"name"`;``; the mapping is retained by object copies and exposed through the alias `Type` metadata.

`JSON` is a distinct immutable builtin type. `JSON(source : String)` parses JSON directly and is especially useful with here strings; `JSON(structValue)` converts a struct-style alias using its JSON field tags. `json_parse(source)` provides the non-throwing `Result[JSON, String]` form. Object fields and array items are accessed with selectors or indexes and remain typed as `JSON`; `json_string`, `json_int`, `json_float`, and `json_bool` safely extract scalar values as `Option[T]`. JSON also provides `.kind`, `.count` for objects, arrays, and strings, `json_get`, `json_is_null`, and `json_stringify`, which accepts either JSON or a struct alias. The `json` stdlib module exposes these operations as `parse`, `must_parse`, `stringify`, `get_field`, `get_index`, `kind`, `as_string`, `as_int`, `as_float`, `as_bool`, `is_null`, `null_json`, and `encode_json` while retaining the older String encoder helpers for compatibility.

`Parsable[T]` is an immutable metaprogramming value created from a source `String`, commonly a here string, plus an optional `List[String]` of source arguments. It exposes `source`, `original_source`, `ast : List[Table]`, `statement_count`, `runtime_type`, `runtime_info`, `cli_args`, `source_args`, combined `args`, `program`, `build_system`, `workspace`, and `keywords`. `parsable_with_source`, `parsable_replace`, and `parsable_append` reparse changed source and return `Result[Parsable[T], String]`, preventing stale AST metadata. `parsable_source`, `parsable_ast`, `parsable_args`, `parsable_runtime_info`, and `parsable_workspace` are typed accessors.

Builtin values expose a small shared protocol surface through selector syntax:
- `String`, `List[T]`, `Set[T]`, `Map[K, V]`, `Table`, `JSON`, `SIMD[T]`, and `Iterator[T]` provide `.count : Int`. JSON count is defined for object, array, and string values.
- `String` and `Char` provide `.uppercase()` and `.lowercase()`.
- `Int` and `UInt` provide `.times(callback : Function[Int, T])`, which calls the callback for each zero-based index and returns the callback's last result.

Generic type parameters may use named constraints in addition to explicit `restrict[...]` allow-lists. `T numeric` accepts numeric parent and child-width types, `T comparable` accepts primitive comparable values, `T hashable` accepts the safe primitive key space used by `Table` and `Set`, `T iterable` accepts values supported by `iter`/`len`, `T allocator_like` accepts builtin allocator/value wrappers, and `T TraitName` requires an `impl TraitName for ConcreteType` declaration.

`Option[T]` and `Result[T, E]` are the standard absence and fallibility types. `Some(value)`/`None()` construct options, and `Ok(value)`/`Err(error)` construct results. Use `.some` and `.ok` for presence checks, `!` to propagate a `Result` error from a fallible expression, and helpers such as `option_map`, `option_unwrap_or`, `option_and_then`, `result_map`, `result_map_err`, `result_unwrap_or`, and `result_and_then` to transform or recover values without unsafe `.value` access.

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

Backend compilation uses a Go `Backend` contract with `Check`, `Emit`, and `Package` phases. The JS backend lowers the supported typed core to backend-neutral IR and emits JavaScript. Its initial value surface is `Int`, `UInt`, `Float`, `Bool`, `String`, `Char`, and recursively typed `List[T]`, with variables, arithmetic/comparison/boolean expressions, ordinary functions, namespaces, imported modules, single returns, `if`/`unless`, `while`, range loops, assignment, `break`/`continue`, `assert`, `throw`, and `print`. Qualified namespace names are flattened into collision-safe JavaScript identifiers, while selective module filters keep unused imported functions out of generated artifacts.

The JS backend implements `String` concatenation, `len(value)`, `.count`, Unicode code-point indexing, `.uppercase()`, `.lowercase()`, and primitive-to-String casts. Its generated helpers use `Array.from` for code-point length/index behavior and kLang display formatting for mixed concatenation, preserving values such as `True` rather than JavaScript's lowercase `true`.

JS-generated `List[T]` values support literals, zero values, `len`/`.count`, checked indexing, indexed growth and compound mutation, `for index := range(len(values))`, and filtered or mapped list comprehensions. Generated binding, call, return, indexing, and comprehension boundaries copy nested lists eagerly, preserving the observable isolation required by kLang's copy-on-write collection semantics.

The JS backend supports struct-style alias constructors, typed constructor fields, trailing default arguments, field selectors, `#extend` methods, and nested struct/List value isolation. `JSON(structValue)` and `json_stringify(structValue)` recursively serialize supported fields, apply `json:"name"` tags, omit internal metadata, and order object keys deterministically. Alias lifecycle/allocator hooks and nested trait or impl bodies remain backend errors until the native JS runtime has matching facilities.

Every native JS build emits `program.js.map` using Source Map v3 and links it from `program.js`. Maps use portable `src/...` paths, include the original source content, and map generated functions and statements back to kLang locations. Generated functions retain kLang call frames; uncaught entry-point failures print the JS error kind, message, kLang function stack, source excerpts, and carets. JS backend compile diagnostics carry a stable rule identifier and source span through `ErrorContext`.

The compiler and runtime track symbol state for globals, locals, temporary variables, regions, temporary regions, parameters, named returns, and function return values. Compile-time state records include the declaration kind, name, type, function, mutability, file, and line. Runtime state records include the phase, event (`define`, `bind`, `assign`, `move`, or `return`), kind, declared type, runtime type, function, mutability, and moved status. Temporary variables use the state kind `temporary`, and temporary memory regions use `temporary_region`. `debug_state()` returns the runtime records as `List[Table]`.
