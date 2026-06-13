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
28. Args // Builtin immutable List[String] containing command line arguments passed to the program workspace
29. Any // Fully dynamic wildcard type; unlike T, it cannot be restricted and accepts any value
30. Atomic[$Item] // Runtime synchronized cell for race-safe shared numeric/value updates

All builtin type names expose a compile-time size query through `.sizeof`, which returns an `Int`.
For example, `Int.sizeof` returns the runtime size used for an `Int` value.
