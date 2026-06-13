# TODO
- foreign function interface for javascript apis/libraries (via a js filesystem only)
- Improved CLI for better user experience and creation of ".klang" projects
    - Add time it takes to run and execute the project once the project is finished executing
    - Add The OS And System information each time the user does running of a project or program
- data race condition prevention mechanics and atomic data handling

# LATER <once todo is done>
- make the runtime be able to work on users browser by compiling to wasm code <done after first>
- multi threaded interpretter runtime <done first >
- add a flag to start a simple web server that is built into the language that will package our entire languages runtime if the user doesnt want to build wasm and ship it
- mutliple return values on functions & the ability to define whether a return value can be mutable or not, and also named return values (like the below)
```lua
function Print() : (name : String, value : Int) {
    return name, value;
}

function Print2() : (mut String, Int) {
    return "", 0;
}
```
- a compact build system (Like if u want to package the project u can do that with the languages source code)


# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds
- Make this langauge be able to run on web runtime with no issues