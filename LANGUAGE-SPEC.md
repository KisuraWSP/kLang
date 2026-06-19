1. Function first language
2. Has Support for first class functions
3. Has small standard library containing important modules
4. Simple Module System
5. All Important Data Types are built into the language
6. Language Operates as file-based system (Meaning each file can execute as a script unless defined as a entry point to a project via the first.klang file)
7. Alias functions can define constructor-like custom data types and extension methods.
8. Arrays and slices can be attached to user-defined memory regions and always index from 0.
9. Builtin allocator/value wrappers include Box, Ref, RefMut, RefCell, HeapAllocator, RegionAllocator, BumpAllocator, and ArenaAllocator.
10. Table is a builtin Lua-style dynamic data type and is the only dynamically typed container.
11. Async functions return Awaitable values, and await unwraps the completed value.
12. Iterators and coroutines are builtin first-class runtime values.
13. `spawn`, `join`, and `thread_status` provide multi-threaded interpreter workers represented by `Thread[T]`.
14. `Args` is a builtin immutable `List[String]` containing the command line arguments for the current workspace.
15. `copy` and `clone` create cloned values without moving from the source binding.
16. Function and lambda parameters are immutable by default; use `mut` before the parameter name to allow mutation.
17. `--raw-lang` disables stdlib module resolution while preserving local workspace imports.
18. `let`, `val`, `var`, and `const` are inferred declaration keywords with strict type checking.
19. Builtin type names expose `.sizeof`, which returns an `Int` size value.
20. Functions may return multiple values through tuple-style return signatures.
21. `private` hides functions and namespaces from other files where the checker can enforce file ownership.
22. Here strings use `//` delimiters in expression-start positions and produce multiline `String` values.
23. `Any` is a fully dynamic wildcard type and cannot be restricted.
24. `defer` schedules statements or blocks to run at the end of the current runtime block.
25. `inline` marks functions and alias functions as eager inline candidates for compiler/runtime optimization.
26. Alias functions use block syntax with `: type`, hook blocks such as `[new] { ... }`, and `#extend { ... }`.
27. `#set_entry_point_to_here` marks the following function as the runtime entry point.
28. `Atomic[T]` plus `Atomic`, `atomic_load`, `atomic_store`, and `atomic_add` provide race-safe runtime cells.
29. `Program`, `BuildSystem`, and `WorkSpace` are builtin meta-programming values for describing custom workspaces and compact build plans.
30. `debug`, `debug_type`, `debug_stack`, and `breakpoint` are builtin debugger helpers.
31. JavaScript FFI is filesystem-only through `JSModule` and `JSCall` descriptors loaded from `.js` files.
32. Variable destructuring can unpack Lists, Tables, Maps, and object fields through parser lowering into inferred declarations.
33. `_` is a discard identifier for ignored values and can be reused without creating a binding.
34. `lazy` variable declarations delay initializer evaluation until the binding is first accessed.
35. Builtin values participate in shared selector protocols: collection-like values expose `.count`, strings/chars expose case conversion methods, and integer values expose `.times(callback)`.
36. `enum` declarations define typed ordinal enum values with implicit iota-style ordinals and optional explicit integer ordinals.
37. Aggregate values use copy-on-write storage for ordinary assignment.
38. Every loaded workspace has a compiler/runtime `Context`, and failures are reported as source-aware `ErrorContext` diagnostics.
39. Stdlib imports use a function lookup table by default so only module functions referenced through the imported module namespace are collected, type checked, and registered. `module_caller(call_entire_module : True);` in the importing source opts back into whole-module stdlib imports.
40. A module source may declare `module(disabled : True);` to reject imports of that module until the directive is removed or set to false.
41. Stdlib modules may declare `global namespace Name { ... }`; functions inside that namespace are loaded into an internal compiler/runtime symbol table and can be called without an import or namespace qualifier.
42. `run` marks a block or single statement as a priority runtime action that executes before ordinary statements in the same block.
43. Multiline comments use `(* ... *)` delimiters and may span lines.
44. Numeric parent types support child-width types through `.child(bits)` and globally available aliases such as `i8`, `u32`, `float64`, and `complex128`.
45. The checker reports warnings for unused local variables and unused function parameters.
46. Qualified module calls can infer imports; `list.append(...)` loads a resolvable `list` module even without an explicit import.
47. Integer literals support optional leading `-` and base prefixes `0x`/`0X`, `0o`/`0O`, and `0b`/`0B`. Identifiers may use Unicode letters, marks, and symbols, including Sinhala text and emoji, but may not begin with a digit.
48. Modules may import other modules. Stdlib-to-stdlib imports remain stdlib imports, so selective function loading still applies to dependency modules.
49. `Type` is the parent runtime metadata type for all language types. `SomeType.get_runtime_type_info()` returns metadata for serialization, data introspection, and memory layout interpretation.
50. `assert expression;` is a builtin statement keyword. The expression must be `Bool`; runtime execution fails with an assertion error when it is false.
51. CLI `doc --sourcefile=[...]` generates a static HTML documentation UI for one or more Klang source files or folder projects, including declaration cards and source-code chapters for each file.
52. `report expression;` is a builtin runtime reporting statement. It evaluates the expression, prints the expression text, value, runtime type, and current stack trace, then continues execution.

Rules
- Variables have scopes (either via the global or local keyword)
- Variables are immutable by default unless specified mutable via (mut keyword)
- `let` declares a local inferred immutable variable, and `let mut` declares a local inferred mutable variable.
- `val` declares a global inferred immutable variable, and `var` declares a global inferred mutable variable.
- `const` declares a strictly immutable inferred value in the current scope and requires an initializer.
- Inferred declarations must have an initializer and are checked before runtime.
- `lazy local`, `lazy global`, `lazy let`, `lazy val`, and `lazy var` declarations require an initializer and evaluate it on first access, caching the result afterward.
- Destructuring declarations must have an initializer and lower to ordinary inferred declarations before semantic checking and runtime execution.
- `_ = expression;` evaluates and discards an expression result. Declarations and destructuring bindings named `_` also discard their values instead of entering scope.
- Unused local variables and function parameters produce warnings. Use `_` for intentionally ignored values.
- Multiple return signatures use `(name : Type, mut OtherType)` syntax and return values with `return left, right;`.
- Named return values are zero-initialized in the function body.
- `private { ... }` creates a private lexical block.
- Extension methods declared inside an alias function use `this` as their receiver.
- Alias functions may declare members, traits, impls, allocation hooks, deallocation hooks, side-effect hooks, and extension methods in the same block.
- Function parameters may use `name := DefaultExpression()` to infer the parameter type from the default value.
- Entry-point directives apply to the next function in the current namespace or top-level scope.
- Region-backed array types use the `ElementType[RegionName]` form and must reference an existing `region`.
- Region-backed arrays grow through indexed assignment, but an index must be inside the region count.
- Alias-created objects and allocator wrapper objects are heap allocations for runtime memory tracking.
- Table values allow mixed values and primitive keys only: `String`, `Int`, `UInt`, `Float`, `Bool`, and `Char`. Keys compare by normalized primitive kind plus value, so numeric, string, and char spellings do not collide.
- Table reads report a missing-key diagnostic when the key is absent and never create fields. Indexed mutation detaches shared copy-on-write storage before writing.
- `table["count"]` always means the user field named `"count"`. `table.count` is the builtin count protocol and returns the number of own entries. Other non-reserved selectors such as `table.name` are sugar for `table["name"]`.
- Use `table_has(table, key)` or `has_key(table, key)` to test presence, and assign the result of `table_delete(table, key)` to remove a key.
- `table_keys(table)`, `table_values(table)`, and `table_entries(table)` return insertion-order own-entry lists. `iter(table)` yields insertion-order entry tables with `key` and `value` fields.
- `table_sequence_count(table)` returns the contiguous zero-based numeric length. It is separate from `table.count`, which counts all own entries.
- `table_set_fallback(child, parent)` returns a table that reads missing keys from `parent`. Fallback keys do not affect own-entry count or iteration.
- `next(iterator)` returns Option[T], with None when the iterator is exhausted.
- `resume(coroutine)` returns Option[T], with None after the coroutine has completed.
- `spawn(functionValue, [args...])` starts a child interpreter worker and returns `Thread[T]`; `join(thread)` waits and returns `T`.
- Threaded workers share loaded functions, globals, memory tracking, and output. Use `Atomic[T]` for shared mutable values that need safe read-modify-write behavior.
- Each standalone script or project is resolved as its own workspace. Resolver caches speed repeated imports without sharing visited-state between workspaces.
- `import` statements may appear anywhere in a source file. Qualified module calls such as `math.Add(...)` also infer an import when `math` resolves to a local or stdlib module.
- Imported modules may contain their own `import` statements. The resolver loads these recursively, reports cycles, and treats sibling imports under the stdlib root as stdlib modules so function lookup filters continue to apply.
- Stdlib imports are selectively collected by default. For example, `import "html";` plus `html.Document(...)` collects `html.Document` and its same-module helper dependencies, not every function in `stdlib/html.klang`.
- Place `module_caller(call_entire_module : True);` in a source file to make its stdlib imports load complete modules.
- Place `module(disabled : True);` in a module source to make the resolver reject imports of that module.
- Place `global namespace Name { ... }` in a stdlib module to expose the namespace's functions as unqualified calls through the language's internal symbol table. The symbol table is not accessible from Klang source.
- Use `run { ... }` or `run FunctionName();` to execute initialization code before ordinary statements in the same runtime block. A `run` action cannot return, break, or continue.
- Use `(* ... *)` for multiline comments. Multiline comments are ignored by the lexer before parsing.
- `Int.child(8)`, `UInt.child(16)`, `Float.child(32)`, and `Complex.child(128)` restrict values to the requested parent type width. The aliases in the builtin `types` namespace are available without imports.
- Integer literals are decimal by default and may be written as hexadecimal (`0x2A`), octal (`0o52`), or binary (`0b101010`). Signed integer and float literals may use a leading `-`; exponentiation keeps unary-minus precedence, so `-2 ** 3` is parsed as `-(2 ** 3)`.
- Variable names, function names, and function parameter names may contain Unicode identifier characters, Sinhala letters and marks, and emoji symbols. Identifiers cannot begin with a digit.
- Use `TypeName.get_runtime_type_info()` to obtain a `Type` object. Its fields describe automated serialization hooks, introspection data such as field tables, and layout values such as byte size, alignment, and footprint.
- Use `assert condition;` for runtime invariants. The checker requires the condition to be `Bool`.
- Use `report value;` or `report FunctionCall();` to emit a live diagnostic report containing the evaluated value, runtime type, and stack frames. `report` accepts any expression, respects ordinary expression errors, and does not mutate the reported value.
- Alias functions may contain trait and impl declarations in addition to hooks and extension methods.
- CLI `run` prints runtime OS, architecture, CPU count, Go runtime version, and elapsed execution time.
- CLI `package` checks a program and writes a compact source bundle with `klang-build.json`.
- CLI `doc --sourcefile=["file.klang"] --out=docs.html` parses the provided source files and writes a standalone HTML documentation page listing imports, modules, namespaces, functions, aliases, enums, globals, parse diagnostics, and a source-code chapter for every file. Passing a folder project expands to every `.klang` file in that project, using the same file order as normal project loading.
- `BuildSystem` backend is restricted to `WASM`, `JS`, or `Standalone`; `Standalone` means the packaged program runs through the interpreter engine.
- `WASM` packaging compiles the Go interpreter/runtime to browser WebAssembly, writes `klang.wasm`, `wasm_exec.js`, `klang_browser.js`, and loads resolved Klang source files from the package manifest.
- CLI `serve` and package `--serve` start a built-in static web server for the generated WASM runtime bundle so users can run projects in a browser without manually shipping files first.
- The stdlib `html` module renders escaped text, attributes, fragments, documents, and named HTML tags as strings for browser/WASM-oriented programs.
- JavaScript FFI can load and describe local `.js` files, expose discovered exports, and create call descriptors without executing JavaScript inside the interpreter.
- Shared builtin protocols are statically checked and runtime-backed. `.count` is available on `String`, `List`, `Map`, `Table`, `SIMD`, and `Iterator`; `.uppercase()` and `.lowercase()` are available on `String` and `Char`; `.times(callback)` is available on `Int` and `UInt` and invokes the callback with indexes from `0` to `receiver - 1`.
- Enum variants are selected with `EnumName.Variant`, have the enum name as their static type, and can be used in pattern matches. Enum values expose `.ordinal : Int`, `.name : String`, and `.variant : String`; values from different enum types are not assignable to each other.
- Ordinary assignment of aggregate collection values such as `List`, `Map`, `Table`, and `SIMD` may share storage until one binding is mutated. Indexed mutation detaches the mutated binding first, preserving referential transparency for the other binding. Explicit `copy` and `clone` still request an eager cloned value.
- Table mutation respects mutable bindings. `None()` and `Null` are stored as ordinary values and do not delete keys. Invalid key types such as `Table`, `List`, functions, refs, allocator values, and runtime objects produce diagnostics.
- `Context` tracks the program name, entry point, selected backend, source files, and collected diagnostics. `ErrorContext` includes the failing phase (`SOURCE`, `MODULE`, `PARSE`, `TYPE`, `RUNTIME`, `BACKEND`, or `WASM`), location, source line, rule, message, and fix hint. CLI `check`, `run`, `package`, and WASM packaging must report through this structure.
- Runtime errors raised inside function calls include a stack trace before the error leaves the active call stack. The Go implementation parses loaded source files concurrently, reuses that parsed program through semantic checks, collects runtime symbols per source concurrently, and merges compiler/runtime setup results in source order before sequential program execution.
