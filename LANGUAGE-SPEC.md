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

Rules
- Variables have scopes (either via the global or local keyword)
- Variables are immutable by default unless specified mutable via (mut keyword)
- `let` declares a local inferred immutable variable, and `let mut` declares a local inferred mutable variable.
- `val` declares a global inferred immutable variable, and `var` declares a global inferred mutable variable.
- `const` declares a strictly immutable inferred value in the current scope and requires an initializer.
- Inferred declarations must have an initializer and are checked before runtime.
- Destructuring declarations must have an initializer and lower to ordinary inferred declarations before semantic checking and runtime execution.
- `_ = expression;` evaluates and discards an expression result. Declarations and destructuring bindings named `_` also discard their values instead of entering scope.
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
- Table values allow mixed primitive keys and mixed value types.
- `next(iterator)` returns Option[T], with None when the iterator is exhausted.
- `resume(coroutine)` returns Option[T], with None after the coroutine has completed.
- `spawn(functionValue, [args...])` starts a child interpreter worker and returns `Thread[T]`; `join(thread)` waits and returns `T`.
- Threaded workers share loaded functions, globals, memory tracking, and output. Use `Atomic[T]` for shared mutable values that need safe read-modify-write behavior.
- Each standalone script or project is resolved as its own workspace. Resolver caches speed repeated imports without sharing visited-state between workspaces.
- Alias functions may contain trait and impl declarations in addition to hooks and extension methods.
- CLI `run` prints runtime OS, architecture, CPU count, Go runtime version, and elapsed execution time.
- CLI `package` checks a program and writes a compact source bundle with `klang-build.json`.
- `BuildSystem` backend is restricted to `WASM`, `JS`, or `Standalone`; `Standalone` means the packaged program runs through the interpreter engine.
- `WASM` packaging compiles the Go interpreter/runtime to browser WebAssembly, writes `klang.wasm`, `wasm_exec.js`, `klang_browser.js`, and loads resolved Klang source files from the package manifest.
- CLI `serve` and package `--serve` start a built-in static web server for the generated WASM runtime bundle so users can run projects in a browser without manually shipping files first.
- The stdlib `html` module renders escaped text, attributes, fragments, documents, and named HTML tags as strings for browser/WASM-oriented programs.
- JavaScript FFI can load and describe local `.js` files, expose discovered exports, and create call descriptors without executing JavaScript inside the interpreter.
