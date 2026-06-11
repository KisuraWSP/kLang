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

Rules
- Variables have scopes (either via the global or local keyword)
- Variables are immutable by default unless specified mutable via (mut keyword)
- Extension methods declared inside an alias function use `this` as their receiver.
- Region-backed array types use the `ElementType[RegionName]` form and must reference an existing `region`.
- Region-backed arrays grow through indexed assignment, but an index must be inside the region count.
- Alias-created objects and allocator wrapper objects are heap allocations for runtime memory tracking.
- Table values allow mixed primitive keys and mixed value types.
- `next(iterator)` returns Option[T], with None when the iterator is exhausted.
- `resume(coroutine)` returns Option[T], with None after the coroutine has completed.
