# Built into the language by default
1. Int
2. UInt
3. String
4. Float
5. Bool
6. Char
7. Map[$Key, $Value]
8. List[...$Items]
9. Option[$Item]
10. Result[$Ok, $Err]
11. Complex
12. SIMD[$Lane]
13. Function[...$Args, $Return]
14. T // Builtin Generic Type Value containing Information of respective data type
15. T[$Region] // Region-backed array/slice storage with zero-based indexing and region capacity checks
16. Box[$Item]
17. Ref[$Item]
18. RefMut[$Item]
19. RefCell[$Item]
20. HeapAllocator
21. RegionAllocator
22. BumpAllocator
23. ArenaAllocator
24. Table // Lua-style dynamic table; this is the only dynamically typed container
25. Awaitable[$Item]
26. Iterator[$Item]
27. Coroutine[$Item]
28. Thread[$Item] // Multi-threaded interpreter worker handle returned by spawn
29. Args // Builtin immutable List[String] containing command line arguments passed to the program workspace
30. Any // Fully dynamic wildcard type; unlike T, it cannot be restricted and accepts any value
31. Atomic[$Item] // Runtime synchronized cell for race-safe shared numeric/value updates
32. Program // Meta-programming descriptor containing module : List[String]
33. BuildSystem // Compact build descriptor containing project_name, number_of_files, files, and backend
34. WorkSpace // Meta workspace combining Program and BuildSystem
35. JSModule // Filesystem-only JavaScript module descriptor loaded from a .js file
36. JSCall // Filesystem-only JavaScript API call descriptor
37. Enum // User-defined typed ordinal enum values declared with `enum`
38. Context // Compiler/runtime source context for a workspace, including files, entry point, backend, and diagnostics
39. ErrorContext // Source-aware diagnostic containing phase, file, line, column, rule, message, hint, and source line

All builtin type names expose a compile-time size query through `.sizeof`, which returns an `Int`.
For example, `Int.sizeof` returns the runtime size used for an `Int` value.

Builtin values expose a small shared protocol surface through selector syntax:
- `String`, `List[T]`, `Map[K, V]`, `Table`, `SIMD[T]`, and `Iterator[T]` provide `.count : Int`.
- `String` and `Char` provide `.uppercase()` and `.lowercase()`.
- `Int` and `UInt` provide `.times(callback : Function[Int, T])`, which calls the callback for each zero-based index and returns the callback's last result.

User-defined `enum` declarations create typed ordinal values inspired by Go `const`/`iota` enums. Variants are selected as `EnumName.Variant`, compare only with variants from the same enum type, and expose `.ordinal`, `.name`, and `.variant`.

Aggregate collection values use copy-on-write storage for ordinary assignment. Shared `List`, `Map`, `Table`, and `SIMD` storage is detached when a mutable binding is written, while explicit `copy` and `clone` still create eager clones.

The language engine builds a `Context` for each loaded workspace and reports failures through `ErrorContext`. This diagnostic context is used by module resolution, parsing, type checking, runtime execution, packaging, and WASM backend generation.
