# TODO
- foreign function interface for javascript apis/libraries (via a js filesystem only)
- Improved CLI for better user experience and creation of ".klang" projects
    - Add time it takes to run and execute the project once the project is finished executing
    - Add The OS And System information each time the user does running of a project or program
- data race condition prevention mechanics and atomic data handling
- traits and other features must be able to used in function aliases
- new keywords "copy" & "clone" for copy semantics and cloning protocols in variables, functions or scopes of any type
- language should be able to read commandline arguments via an os Level List called Args which should be built in to the language just like how Golang does it
    - and also provide a way for the programmer to be able to access such an array
- caching system for improvement of langauge speed
- restrict on lambda functions
- immutable arguments/parameters on functions (to make a parameter/argument mutable use the "mut" keyword)
- workspaces <treat a program(if its a standalone script)/project as if its a seperate workspace>
- add "raw-lang" flag to tell langauge runtime to not allow any stdlib modules/files to be able to used

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
- introduce 3 new keywords to simplify the code which are "let, var, val"
    - these keywords do strict type checking and type inference to variables
```lua
-- infers the type Option[Int] but this variable represents the below
-- local Option[Int] x = Some(69420);
let x = Some(69420);

-- infers the type Option[Int] but this variable represents the below
-- local mut Option[Int] x = Some(69420);
let mut x = Some(69420);

-- infers the type Option[Int] but this variable represents the below
-- global Option[Int] x = Some(69420);
val x = Some(69420);

-- infers the type Option[Int] but this variable represents the below
-- global mut Option[Int] x = Some(69420);
var x = Some(69420);
```
- add a new operator to return size of a type like the below
```lua
let size intSize = Int.sizeof;
```
- add new keyword called "const"
    - this keyword will work in local or global scope
    - is strictly immutable
    - this keyword if a value propagation is not correct during like the steps to either check the type/parsing like what ever proccess if it fails before any step before running the code this will immediately stop what the language is doing and propagate what the error is occuring in this
    - this keyword must have strict semantics
```lua
const x = Int.sizeof;
```
- a compact build system (Like if u want to package the project u can do that with the languages source code)


# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds
- Make this langauge be able to run on web runtime with no issues