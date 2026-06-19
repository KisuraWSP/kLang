# TODO
- add a message polling system in the languages system to be able to do metaprogramming like things to the system
- revisit the modules in the standard library when more languages features are there or the language runtime becomes very powerful

- add support for type aliases
```lua
type string_list = List[String];
```
- Add CSV as a built in type in the language 
   - with its own parser/implementation as part of the go source code
   - make use of the here strings for this

- Add JSON as a built in type in the language 
   - with its own parser/implementation as part of the go source code
   - make use of the here strings for this

- add support for json tag annotations on structs parameters like the below
```lua
alias function Array(id : String) : type = struct {
   this.id `json:"id"`;
}
```
- Allow Type restriction, traits, etc on T in function aliases and structs
```lua
alias function[T Printable] Test(){

}


alias function[T restrict[List[Option[Int]]] Test2(){

}
```
- Add a builtin type called Parsable that is a type that reads a specific program
   - contains meta programming features
   - stores the runtime information
   - stores information of the AST of a single program
   - Works seemlessly with Workspace, BuildSystem, etc..
   - this type can also be restricted like the below to according to the users use case
   ```lua
      let mut Parsable[T Printable] parsable;
   ```
   - allows the ability to modify source code when specified
   - allows the ability to add new keywords while letting user define there own functionality
   ```lua
   alias printer = Parsable[T Printable].keyword_macro {
      print(get_args_from_parsable(), T);
   }

   printer "hallo";
   ```
   - this type has the ability to get stored arguments provided by user as if its a function
      - this counts for both cli args and runtime args, source code args

- add support for members in function aliases
```lua
alias function T(test : List[String]) {
   let member test = this;
}
```
- in the language if there is an error (this is for the error reporting)
   - provide a valid reason why the error occured
   - tell the user what they expect from them to correct the error
   - provide suggestions into helping user to fix the error
- add a special annotation for which we can mark which functions are running on which backend when in stdlib modules
   ```lua
      @backend("JS");
      function console_log(){}

      @backend("WASM");
      function console_print(){}

      @backend("Standalone");
      function os_run_dir(){}
   ```
- we need a system in which when user passes any sort of flags or arguments in the 
   command line of the go executable it should be passed to the builtin Args array in 
   the langauge and then only the respective operations can be done
- make sure that the langauge is able to understand and execute js code
   - we have made a small foundation for it for now 
   - we need to extend our existing foundation to make it powerful enough to do some
   - operations
- allow support for directory based modules like in golang
- make progrramming language api data oriented
- add a new flag to allow user to add there own functions to the stdlib via new command like —new_std_lib=["test”, “processor”]
- allow support for hot/cold programs
   - this is the basis for a hot reloading code runtime environment/system
- add lsp plugin for vscode
- In the programming language i want you to do this
So whenever we start a program tell the go code to always run a .klang file called default_program.klang this is the main entry point program that is given first priority to users (do these once enough features are there to make it powerful)
   - This code will behave similar as the main.go and same as the cmd/klang-wasm/main.go
   - we want this behaviour on this file as it is the main file we intend to implement and will be used as the programming language becomes powerful over time and once we have a stable runtime and a powerful standard library
- First workspace in the language is the default_program.klang
The language will generate the user porgram as second workspace
Workspace and meta programming types like Program, BuildSystem, Workspace, etc… allows for parallel code wide execution so for this to happen we need to use threads/channels in the golang code

- add call site to the langauge for relating to functions, variables, structs, function aliases, etc...

- if its a function alias it behaves more like a callback

- in the language add exampels for advent of code when the runtim and features are powerful
- Richer pattern destructuring
   Destructuring already exists. It could expand into function parameters, match cases, loops, and records.
- Compile-time evaluation / macros, carefully limited
   You already have Program, BuildSystem, WorkSpace, .sizeof, and diagnostics. A small compile-time feature could be powerful:
   generate code, validate constants, create specialized functions, or define DSLs.
- Cancellation/context support
Async, Awaitable, threads, and WASM would benefit from a builtin Context-like runtime cancellation model:
cancellation, timeout, deadline, propagation.
- Channel/message-passing concurrency
Threads and Atomics exist, but message passing would give the language a cleaner high-level concurrency story.
Go-style channels would fit beautifully here, especially given your Go influence.
- Resource safety syntax
Since kLang has allocators, regions, defer, refs, boxes, and arenas, a structured resource feature could fit well:
something like scoped ownership or using-style blocks that guarantee cleanup at scope end.

    
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