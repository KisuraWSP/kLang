# TODO
- add a message polling system in the languages system to be able to do metaprogramming like things to the system
- revisit the modules in the standard library when more languages features are there or the language runtime becomes very powerful


- Core Table Semantics

Keep Table as the only fully dynamic container.

Support mixed primitive keys: String, Int, UInt, Float, Bool, Char.

Support mixed values, including typed kLang values.

Define whether Table keys compare by value, identity, or normalized representation.

Reject or clearly define unsafe keys like Table keys, function keys, allocator values, refs, etc.

Preserve copy-on-write behavior for ordinary assignment, as your current runtime spec requires.

Make indexed mutation detach shared storage before writing.

Define stable behavior for missing keys.
Field Access

Keep table.name as sugar for table["name"].

Decide whether table.name = value is allowed.

Make field access fail clearly if the key is missing, or return Option[Any].

Avoid silently creating fields from reads.

Keep selector protocols like .count separate from user table fields, or define precedence.
This one matters a lot. If table.count can mean either “builtin count property” or string key "count", you need a rule.
For example:
data.count      -- table field?
data["count"]   -- definitely table field
data.count()    -- method?
I would recommend:
data["count"]   -- user field
data.count      -- builtin protocol only, if available
data.name       -- sugar only when no builtin selector conflicts
Or simpler: allow data.name, but reserve builtin protocol names like count.
Deletion

Add an explicit deletion operation.

Do not make None() or Null automatically delete unless you really want Lua-style behavior.

Prefer something clear:
delete data["name"];
or:
table_delete(data, "name");
Lua’s nil deletion is elegant, but in kLang it could conflict with Option, Null, and typed absence.
Iteration

Define iter(table) behavior.

Decide whether iteration yields keys, values, or key/value pairs.

Prefer a typed pair result:
Iterator[(Any, Any)]
or a builtin table-entry type.

Add explicit helpers:
table_keys(data)
table_values(data)
table_entries(data)

Do not promise deterministic order unless you are willing to maintain it.

If deterministic order matters, specify insertion-order iteration.
Array-Like Behavior

Decide whether Table has a Lua-style array part.

Decide whether array-like table indexes are zero-based or one-based.

Since kLang already uses zero-based indexing, keep Table zero-based too.

Define whether data.count counts all keys or only sequential numeric indexes.

Avoid Lua’s #table ambiguity.
I’d strongly recommend:
data.count              -- total number of entries
table_sequence_count(t) -- optional contiguous numeric length
Metatable-Like Behavior

Add this only after the basic Table is solid.

Do not copy Lua metatables exactly at first.

Start with a controlled kLang version, maybe TableShape or TableMeta.

Support missing-key fallback first.

Add custom string/debug representation second.

Add operator hooks much later, if ever.
Minimal useful version:
local mut Table parent = {"name": "base"};
local mut Table child = {};
table_set_fallback(child, parent);
Then:
child.name -- reads from child, then parent
Avoid starting with full __index, __newindex, __call, __add, etc. That much dynamism can punch holes through your type checker and runtime invariants.
Runtime Safety

Table operations should report errors through ErrorContext.

Missing key, invalid key type, invalid mutation, and selector conflict should have clear diagnostics.

Table mutation should respect immutable vs mutable bindings.

Table mutation through aliases should obey copy-on-write.

Table values stored inside Ref, RefMut, RefCell, or allocator-backed objects need explicit behavior.

Threaded workers should not mutate shared Table without synchronization.

If shared mutation is allowed, require Atomic-like wrappers or a synchronized table type later.
What I Would Implement First
Solid Table get/set/delete.
table.has(key) or has_key(table, key).
table.keys, table.values, table.entries or equivalent functions.
Clear .count behavior.
Copy-on-write-safe mutation.
Deterministic diagnostics through ErrorContext.
Optional fallback table/prototype lookup.
Only later: metatable/operator/call hooks.
My design instinct: keep Table as a safe dynamic record/map first, then grow Lua-like behavior behind explicit APIs. That gives you Lua’s flexibility without letting dynamic table magic leak into every part of the runtime.

    
# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project under 10 seconds
- Make this langauge be able to run on web runtime with no issues
- Make the language runtime more powerful and be able to execute any user code without issue but if there is an issue then render the error and stop it either via runtime/checking stage


# REFACTOR
(**little by little polish every existing feature in this system until we can do like 
complex programs that we will be able to run with whatever user created code and it must works, also make it have good helpers and make the programming interface good for all users**)

- Improve Variables [partial done] <why because we might revisit this later>
- Improve Loops <broken right now>
- Improve Functions, Function Aliases, Structs <broken right now>
- Improve Data Types <broken right now>
    -   Zero-Cost Iterators and Functional Pipelines
        Data types like arrays, lists, and dictionaries have been upgraded to support functional programming concepts out of the box.
        Chaining: Instead of writing complex for loops with temporary variables, you can chain operations directly on the list: users.filter(...).map(...).sort(...).
        Lazy Evaluation: In languages like Rust or C# (LINQ), chaining these methods together doesn't actually process the array immediately. The language builds a highly optimized execution plan behind the scenes and processes the data in one ultra-fast pass, meaning you get the readability of functional programming without sacrificing the performance of a raw C-style for loop.

    -   Extension Methods
        Historically, if the creators of a programming language didn't include a specific method on a String, you had to write a clunky helper function like reverseString(myString).
        Bolting on Functionality: Modern languages like Kotlin, C#, and Swift feature "Extension Methods." These allow developers to "open up" built-in data types and bolt their own custom methods directly onto them as if they were there from the factory.
        Ergonomics: This allows for incredibly readable code. Instead of calculateDate(10, "days", "ago"), you can extend the integer data type so you can simply write 10.days.ago().
- Improve Error Reporting, Error Handling, Errors System <broken right now>
(**improve errors system and error handling, error reporting should be proper and helpful not vague
**)
- Improve Runtime <broken right now>

# TARGET MILESTONE
- Jan 1st 2027 - Language should be able to understand user code and have powerful interface to use with