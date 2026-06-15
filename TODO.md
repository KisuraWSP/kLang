# TODO
- add **"Context"** system so that the programming langauge is able to understand user level source code and that whatever source code that the user provides is correct by the langauges rules/semantics/runtime/ WASM backend
    - build this feature into the entire language source code be it parser, lexer, or whatever it must be fully built in and integrated so that the users source code is understood by the programming langauge and it knows what to do and will give an expected output to what the user wants and if it didnt give an expected output then tell the user what is wrong (so for that we will provide something called an **"ErrorContext"**, so that the programming langauge will know that there is an error and it will know what to do and will tell user on how to fix it)
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
- Improve Functions, Function Aliases, Structs √
- Improve Data Types <broken right now>
- Improve Error Reporting, Error Handling, Errors System <broken right now>
(**improve errors system and error handling, error reporting should be proper and helpful not vague
**)
- Improve Runtime <broken right now>

# TARGET MILESTONE
- Jan 1st 2027 - Language should be able to understand user code and have powerful interface to use with