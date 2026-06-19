1. Function first language
2. Has Support for first class functions
3. Has small standard library containing important modules
4. Simple Module System
5. All Important Data Types are built into the language
6. Language Operates as file-based system (Meaning each file can execute as a script unless defined as a entry point to a project via the first.klang file)
7. Alias functions can define constructor-like custom data types and extension methods. Struct-style alias functions expose their parameters as statically checked fields, and generic constructor calls infer returned alias types.
8. Arrays and slices can be attached to user-defined memory regions, including temporary regions, and always index from 0.
9. Builtin allocator/value wrappers include Box, Ref, RefMut, RefCell, HeapAllocator, RegionAllocator, BumpAllocator, and ArenaAllocator.
10. Table is a builtin Lua-style dynamic data type and is the only dynamically typed container.
11. Async functions return Awaitable values, and await unwraps the completed value.
12. Iterators and coroutines are builtin first-class runtime values.
13. `spawn`, `join`, and `thread_status` provide multi-threaded interpreter workers represented by `Thread[T]`.
14. `Args` is a builtin immutable `List[String]` containing the command line arguments for the current workspace.
15. `copy` and `clone` create cloned values without moving from the source binding.
16. Function and lambda parameters are immutable by default and passed by value; use `mut` before the parameter name to allow mutation of the local copy, or `ref` before the parameter name to pass a mutable caller binding by reference.
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
35. `temp local` and `temp let` declare local temporary variables for short-lived intermediate values. They type check and execute like ordinary local variables, are tracked with state kind `temporary`, and do not produce unused-variable warnings.
36. `temp region` declares a temporary memory region for region-backed arrays. It is tracked with state kind `temporary_region`, keeps the same capacity rules as ordinary regions, and runtime allocations for that region are accounted under temporary memory.
37. Builtin values participate in shared selector protocols: collection-like values expose `.count`, strings/chars expose case conversion methods, and integer values expose `.times(callback)`.
38. `enum` declarations define typed ordinal enum values with implicit iota-style ordinals and optional explicit integer ordinals.
39. Aggregate values use copy-on-write storage for ordinary assignment.
40. Every loaded workspace has a compiler/runtime `Context`, and failures are reported as source-aware `ErrorContext` diagnostics.
41. Stdlib imports use a function lookup table by default so only module functions referenced through the imported module namespace are collected, type checked, and registered. `module_caller(call_entire_module : True);` in the importing source opts back into whole-module stdlib imports.
42. A module source may declare `module(disabled : True);` to reject imports of that module until the directive is removed or set to false.
43. Stdlib modules may declare `global namespace Name { ... }`; functions inside that namespace are loaded into an internal compiler/runtime symbol table and can be called without an import or namespace qualifier.
44. `run` marks a block or single statement as a priority runtime action that executes before ordinary statements in the same block.
45. Multiline comments use `(* ... *)` delimiters and may span lines.
46. Numeric parent types support child-width types through `.child(bits)` and globally available aliases such as `i8`, `u32`, `float64`, and `complex128`.
47. The checker reports warnings for unused local variables and unused function parameters.
48. Qualified module calls can infer imports; `list.append(...)` loads a resolvable `list` module even without an explicit import.
49. Integer literals support optional leading `-`, base prefixes `0x`/`0X`, `0o`/`0O`, and `0b`/`0B`, and `_` separators between digits. Identifiers may use Unicode letters, marks, and symbols, including Sinhala text and emoji, but may not begin with a digit.
50. Modules may import other modules. Stdlib-to-stdlib imports remain stdlib imports, so selective function loading still applies to dependency modules.
51. `Type` is the parent runtime metadata type for all language types. `SomeType.get_runtime_type_info()` returns metadata for serialization, data introspection, and memory layout interpretation.
52. `assert expression;` is a builtin statement keyword. The expression must be `Bool`; runtime execution fails with an assertion error when it is false.
53. CLI `doc --sourcefile=[...]` generates a static HTML documentation UI for one or more Klang source files or folder projects, including declaration cards and source-code chapters for each file.
54. `report expression;` is a builtin runtime reporting statement. It evaluates the expression, prints the expression text, value, runtime type, and current stack trace, then continues execution.
55. CLI `check` and `run` persist a source-fingerprint program cache in `.klang-cache` so repeated startups can skip module resolution and type checking when the full resolved source set is unchanged.
56. `Set[T]` is a builtin deterministic hash set for unique primitive values, constructed with `Set(list)`, counted with `.count` or `len`, iterated with `iter`, and queried with `set_has(set, value)`.
57. `format(pattern : String, values : List[T])` and `printf(pattern : String, values : List[T])` are runtime-backed string formatting builtins. `%` consumes the next value, `%%` emits a literal percent sign, and the number of non-escaped placeholders must match `len(values)`. The stdlib `fmt` module exposes `fmt.Format` and `fmt.Printf` wrappers.
58. Generic type parameters support named constraints beyond `restrict[...]`: `numeric`, `comparable`, `hashable`, `iterable`, `allocator_like`, and trait-bound names such as `T Printable`.
59. `kLang test` is a first-class CLI test runner. It discovers functions named `Test...`, runs them after module resolution, type checking, and parsing, treats failed `assert` statements as test failures, accepts `Bool` returns where `True` passes, accepts `Int` returns where `0` passes, and supports sibling `.golden` output files.
60. Multiple-return functions can be unpacked directly into typed variable declarations, for example `local Table x, Int y = Multi();`.
61. The compiler and runtime track state for variables, function parameters, named returns, and return values. Type-check reports expose compile-time state records, and `debug_state()` returns runtime state records as `List[Table]`.
62. Pattern matching supports exhaustive checks for Bool, enum, Option, and Result values, plus structural cases for List and Table values.
63. Option and Result have checked helper functions for mapping, chaining, fallback recovery, and consistent safe access diagnostics.
64. CLI `fmt`/`format` parse-validates Klang source, writes a canonical four-space style, preserves comments and here strings, supports `--write` for rewrites, and supports `--check` for CI-style formatting verification.

Rules
- Variables have scopes (either via the global or local keyword)
- Variables are immutable by default unless specified mutable via (mut keyword)
- `let` declares a local inferred immutable variable, and `let mut` declares a local inferred mutable variable.
- `val` declares a global inferred immutable variable, and `var` declares a global inferred mutable variable.
- `const` declares a strictly immutable inferred value in the current scope and requires an initializer.
- Inferred declarations must have an initializer and are checked before runtime.
- `lazy local`, `lazy global`, `lazy let`, `lazy val`, and `lazy var` declarations require an initializer and evaluate it on first access, caching the result afterward.
- `temp local` and `temp let` declare local temporary variables for intermediate values. `lazy temp local` and `lazy temp let` are valid. Temporary variables cannot be global, cannot be const, and are excluded from unused-variable warnings.
- Destructuring declarations must have an initializer and lower to ordinary inferred declarations before semantic checking and runtime execution.
- `_ = expression;` evaluates and discards an expression result. Declarations and destructuring bindings named `_` also discard their values instead of entering scope.
- Unused local variables and function parameters produce warnings. Use `_` for intentionally ignored values.
- Pattern matches over Bool, enum, Option, and Result values must be exhaustive unless marked `partial` or given a default `case:`. `Some(x)`, `Ok(x)`, `Err(x)`, List patterns, and Table patterns can bind captured values inside the case body.
- `Option[T]` values expose `.some : Bool` and guarded `.value : T`; `Result[T,E]` values expose `.ok : Bool` and guarded `.value : T`. Accessing `.value` without a proven `Some`/`Ok` state is rejected with a diagnostic that suggests checks, pattern matching, helper functions, or `!` propagation for Result values.
- `option_map`, `option_unwrap_or`, `option_and_then`, `result_map`, `result_map_err`, `result_unwrap_or`, and `result_and_then` are builtin helper functions. They preserve `None`/`Err` without calling success callbacks, and they statically check callback and fallback types.
- The formatter only formats parse-valid Klang source. It normalizes indentation, operator spacing, punctuation spacing, and trailing newlines while preserving line comments, multiline comments, and here-string contents.
- Postfix `?` checks presence/success as `Bool` for Option and Result values. Postfix `!` unwraps successful Result values or propagates the error as a thrown value.
- Multiple return signatures use `(name : Type, mut OtherType)` syntax and return values with `return left, right;`.
- Typed multi-variable declarations use `local Type a, OtherType b = FunctionReturningTwoValues();`. The initializer must be a call to a function with multiple declared return values, the number of bindings must match the number of return values, and each returned value must be assignable to its declared binding type. `_` may be used to discard a returned value.
- Named return values are zero-initialized in the function body.
- Function arguments are pass-by-value by default, so mutating a `mut` parameter changes only the callee's local copy. A `ref name : Type` parameter aliases the caller's mutable variable; calls to `ref` parameters must pass a direct mutable variable, not a literal or temporary expression.
- `private { ... }` creates a private lexical block.
- Extension methods declared inside an alias function use `this` as their receiver.
- Alias functions may declare members, traits, impls, allocation hooks, deallocation hooks, side-effect hooks, and extension methods in the same block.
- Struct-style alias functions are first-class static object types: constructor parameters become readable fields on `this`, `#extend` methods are type checked with the receiver in scope, generic arguments inferred from constructor calls flow into fields and methods, and methods may return either the bare alias name or a specialized alias type from the same alias family.
- Function parameters may use `name := DefaultExpression()` to infer the parameter type from the default value.
- Generic parameters may use `T restrict[Int, Float]` for explicit allow-lists, `T numeric`, `T comparable`, `T hashable`, `T iterable`, `T allocator_like`, or `T TraitName` for trait-bound constraints. Trait-bound constraints require a matching `impl TraitName for ConcreteType` before the concrete type can satisfy the generic call.
- Entry-point directives apply to the next function in the current namespace or top-level scope.
- Region-backed array types use the `ElementType[RegionName]` form and must reference an existing `region`.
- `temp region Name(Type, size, count);` declares a temporary region. Region-backed arrays that reference it use the same `ElementType[Name]` syntax and count checks as ordinary regions.
- Region-backed arrays grow through indexed assignment, but an index must be inside the region count.
- Alias-created objects and allocator wrapper objects are heap allocations for runtime memory tracking.
- Table values allow mixed values and primitive keys only: `String`, `Int`, `UInt`, `Float`, `Bool`, and `Char`. Keys compare by normalized primitive kind plus value, so numeric, string, and char spellings do not collide.
- Set values store unique primitive items only: `String`, `Int`, `UInt`, `Float`, `Bool`, and `Char`. `Set([items...])` deduplicates by normalized primitive kind plus value, preserves insertion order for iteration, and rejects unsafe dynamic items such as `Table`, `List`, functions, refs, allocator objects, and runtime objects.
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
- Each standalone script or project is resolved as its own workspace. Resolver caches speed repeated imports without sharing visited-state between workspaces. Successful CLI checks/runs also write a `.klang-cache` entry keyed by entry point, raw-lang mode, and source fingerprints; valid hits may reuse the resolved and checked source set.
- `import` statements may appear anywhere in a source file. Qualified module calls such as `math.Add(...)` also infer an import when `math` resolves to a local or stdlib module.
- Imported modules may contain their own `import` statements. The resolver loads these recursively, reports cycles, and treats sibling imports under the stdlib root as stdlib modules so function lookup filters continue to apply.
- Stdlib imports are selectively collected by default. For example, `import "html";` plus `html.Document(...)` collects `html.Document` and its same-module helper dependencies, not every function in `stdlib/html.klang`.
- Place `module_caller(call_entire_module : True);` in a source file to make its stdlib imports load complete modules.
- Place `module(disabled : True);` in a module source to make the resolver reject imports of that module.
- Place `global namespace Name { ... }` in a stdlib module to expose the namespace's functions as unqualified calls through the language's internal symbol table. The symbol table is not accessible from Klang source.
- Use `run { ... }` or `run FunctionName();` to execute initialization code before ordinary statements in the same runtime block. A `run` action cannot return, break, or continue.
- Use `(* ... *)` for multiline comments. Multiline comments are ignored by the lexer before parsing.
- `Int.child(8)`, `UInt.child(16)`, `Float.child(32)`, and `Complex.child(128)` restrict values to the requested parent type width. The aliases in the builtin `types` namespace are available without imports.
- Integer literals are decimal by default and may be written as hexadecimal (`0x2A`), octal (`0o52`), or binary (`0b101010`). Numeric literals may use `_` separators between digits, for example `1_000_000`, `0xFF_FF`, `0b1010_0101`, and `12_345.67_89`; separators cannot appear at the beginning or end of a digit sequence, directly after a base prefix, or consecutively. Signed integer and float literals may use a leading `-`; exponentiation keeps unary-minus precedence, so `-2 ** 3` is parsed as `-(2 ** 3)`.
- Variable names, function names, and function parameter names may contain Unicode identifier characters, Sinhala letters and marks, and emoji symbols. Identifiers cannot begin with a digit.
- Use `TypeName.get_runtime_type_info()` to obtain a `Type` object. Its fields describe automated serialization hooks, introspection data such as field tables, and layout values such as byte size, alignment, and footprint.
- Use `assert condition;` for runtime invariants. The checker requires the condition to be `Bool`.
- Test functions are ordinary functions whose names start with `Test`. They must take no arguments when run by the CLI. They may return nothing, `Bool`, or `Int`; other return values fail the test. Golden output files compare the printed output from all discovered test functions.
- Use `report value;` or `report FunctionCall();` to emit a live diagnostic report containing the evaluated value, runtime type, and stack frames. `report` accepts any expression, respects ordinary expression errors, and does not mutate the reported value.
- Use `debug_state()` to inspect runtime state transitions for bindings and returns. Each entry contains fields such as `phase`, `event`, `kind`, `name`, `type`, `runtime`, `function`, `mutable`, and `moved`.
- Use `format("Hello %, score %% %", ["kLang", 42])` to build a formatted string and `printf(pattern, values)` to print one. Formatting values use the same display conversion as `print`.
- Alias functions may contain trait and impl declarations in addition to hooks and extension methods.
- CLI `run` prints runtime OS, architecture, CPU count, Go runtime version, and elapsed execution time.
- CLI `package` checks a program and writes a compact source bundle with `klang-build.json`.
- CLI `doc --sourcefile=["file.klang"] --out=docs.html` parses the provided source files and writes a standalone HTML documentation page listing imports, modules, namespaces, functions, aliases, enums, globals, parse diagnostics, and a source-code chapter for every file. Passing a folder project expands to every `.klang` file in that project, using the same file order as normal project loading.
- CLI `fmt file.klang` prints formatted source to stdout. `fmt --write` rewrites one file or every `.klang` file in a folder, skipping `dist`; `fmt --check` exits with an error listing files that are not already formatted.
- `BuildSystem` backend is restricted to `WASM`, `JS`, or `Standalone`; `Standalone` means the packaged program runs through the interpreter engine.
- `WASM` packaging compiles the Go interpreter/runtime to browser WebAssembly, writes `klang.wasm`, `wasm_exec.js`, `klang_browser.js`, and loads resolved Klang source files from the package manifest.
- CLI `serve` and package `--serve` start a built-in static web server for the generated WASM runtime bundle so users can run projects in a browser without manually shipping files first.
- The stdlib `html` module renders escaped text, attributes, fragments, documents, and named HTML tags as strings for browser/WASM-oriented programs.
- JavaScript FFI can load and describe local `.js` files, expose discovered exports, and create call descriptors without executing JavaScript inside the interpreter.
- Shared builtin protocols are statically checked and runtime-backed. `.count` is available on `String`, `List`, `Set`, `Map`, `Table`, `SIMD`, and `Iterator`; `.uppercase()` and `.lowercase()` are available on `String` and `Char`; `.times(callback)` is available on `Int` and `UInt` and invokes the callback with indexes from `0` to `receiver - 1`.
- Enum variants are selected with `EnumName.Variant`, have the enum name as their static type, and can be used in pattern matches. Enum values expose `.ordinal : Int`, `.name : String`, and `.variant : String`; values from different enum types are not assignable to each other.
- Ordinary assignment of aggregate collection values such as `List`, `Set`, `Map`, `Table`, and `SIMD` may share storage until one binding is mutated. Indexed mutation detaches the mutated binding first, preserving referential transparency for the other binding. Explicit `copy` and `clone` still request an eager cloned value.
- Table mutation respects mutable bindings. `None()` and `Null` are stored as ordinary values and do not delete keys. Invalid key types such as `Table`, `List`, functions, refs, allocator values, and runtime objects produce diagnostics.
- `Context` tracks the program name, entry point, selected backend, source files, and collected diagnostics. `ErrorContext` includes the failing phase (`SOURCE`, `MODULE`, `PARSE`, `TYPE`, `RUNTIME`, `BACKEND`, or `WASM`), location, source line, source span, rule, message, and fix hint. Type diagnostics may include did-you-mean suggestions, import hints, and expected/found type trees. CLI `check`, `run`, `package`, and WASM packaging must report through this structure.
- Runtime errors raised inside function calls include a stack trace before the error leaves the active call stack. The Go implementation parses loaded source files concurrently, reuses that parsed program through semantic checks, persists a best-effort `.klang-cache` for repeated unchanged program startups, collects runtime symbols per source concurrently, and merges compiler/runtime setup results in source order before sequential program execution.
