1. Function first language
2. Has Support for first class functions
3. Has small standard library containing important modules
4. Simple Module System
5. All Important Data Types are built into the language
6. Language Operates as file-based system (Meaning each file can execute as a script unless defined as a entry point to a project via the first.klang file)
7. Alias functions can define constructor-like custom data types and extension methods.
8. Arrays and slices can be attached to user-defined memory regions and always index from 0.
9. Builtin allocator/value wrappers include Box, Ref, RefMut, RefCell, HeapAllocator, RegionAllocator, BumpAllocator, and ArenaAllocator.

Rules
- Variables have scopes (either via the global or local keyword)
- Variables are immutable by default unless specified mutable via (mut keyword)
- Extension methods declared inside an alias function use `this` as their receiver.
- Region-backed array types use the `ElementType[RegionName]` form.
