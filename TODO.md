# TODO
- add a message polling system in the languages system to be able to do metaprogramming like things to the system
- revisit the modules in the standard library when more languages features are there or the language runtime becomes very powerful

- in the doc toolchain command
   - make it that it will show the code
- make progrramming language api data oriented
- add pass by reference and pass by value sementaics in the langauge
- add a new flag to allow user to add there own functions to the stdlib via new command like —new_std_lib=["test”, “processor”]
- Allow support of assigning values to variables if a function supports multiple return types
- allow support for hot/cold programs
   - this is the basis for a hot reloading code runtime environment/system
- implement a new keyword "report" and langauage wide stack tracing for errors
   ```lua
      -- the report keyword can be used with either variables, functions, functions parameters/arguments
      -- what this keyword does is that during the execution of the program it will give a live stack trace in which the data values of said item which u mentioned to report will show in a nice stack trace interface like the below and it displays it live
      report t;
      report test();
   ```
- add lsp plugin for vscode
- In the programming language i want you to do this
So whenever we start a program tell the go code to always run a .klang file called default_program.klang this is the main entry point program that is given first priority to users (do these once enough features are there to make it powerful)
   - This code will behave similar as the main.go and same as the cmd/klang-wasm/main.go
   - we want this behaviour on this file as it is the main file we intend to implement and will be used as the programming language becomes powerful over time and once we have a stable runtime and a powerful standard library
- First workspace in the language is the default_program.klang
The language will generate the user porgram as second workspace
Workspace and meta programming types like Program, BuildSystem, Workspace, etc… allows for parallel code wide execution so for this to happen we need to use threads/channels in the golang code

- add call site to the langauge for relating to functions, variables, structs, function aliases, etc...

- HashSet / Set builtin
   You have Map and List, but a Set[T] is commonly needed. Especially useful for compiler work, graph traversal, deduping imports, and static analysis.
- Richer pattern destructuring
   Destructuring already exists. It could expand into function parameters, match cases, loops, and records.
- Better string formatting
   The docs show a custom Printf example. This probably deserves official language or stdlib support:
- Compile-time evaluation / macros, carefully limited
   You already have Program, BuildSystem, WorkSpace, .sizeof, and diagnostics. A small compile-time feature could be powerful:
   generate code, validate constants, create specialized functions, or define DSLs.
- Testing built into the CLI
   A first-class kLang test command would help the ecosystem a lot.
   Even simple conventions like test files, test functions, assertions, and golden-output tests would make the language feel more complete.
- Cancellation/context support
Async, Awaitable, threads, and WASM would benefit from a builtin Context-like runtime cancellation model:
cancellation, timeout, deadline, propagation.
- Channel/message-passing concurrency
Threads and Atomics exist, but message passing would give the language a cleaner high-level concurrency story.
Go-style channels would fit beautifully here, especially given your Go influence.
- Resource safety syntax
Since kLang has allocators, regions, defer, refs, boxes, and arenas, a structured resource feature could fit well:
something like scoped ownership or using-style blocks that guarantee cleanup at scope end.
- Generic constraints beyond restrict[...]
T restrict[Int, Float] is good, but the language could grow richer constraints:
numeric, comparable, hashable, iterable, allocator-like, trait-bound.
This would make generic functions safer and more expressive.
- Exhaustive pattern matching
Since enums and Result/Option already exist, the checker could warn when a match forgets a case. This would make kLang feel safer without adding much syntax burden.
    
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