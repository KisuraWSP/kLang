# TODO
- add a message polling system in the languages system to be able to do metaprogramming like things to the system
- revisit the modules in the standard library when more languages features are there or the language runtime becomes very powerful

    
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