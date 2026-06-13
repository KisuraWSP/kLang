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
    - i want the user to be able to define a custom workspace since we have workspaces
    - WorkSpace should be a builtin type in the langauge
        - this should take arguments "Program : Program (might have to add this as a seperate type), BuildSystem : BuildSystem (also a seperate type)"
        - BuidSystem must take in 
            project_name : String
            number_of_files : Int
            files : List[String]
            backend : String Options are ["WASM", "JS", "Standalone"] if Standalone the entire program runs through the interpretter engine
        - Program must take in
            module : List[String]
    - this system must have its own api that the user can use to program and do meta programming like features
- private keyword/scope to make either a namespace or a function or even a scope hidden to other files (like the below)
```lua
-- function is now hidden to other modules/files
private function Add() : T{
    return "String is added" as Int;
}

-- this is a scope we can use this for many usecases we just don't know what but we 
-- will provide this
private {
    -- any code can exist here
}
```
- update function aliases to use new syntax ruleset rather than grua langs syntax XD (just tell the clanker to like make the function alias use like a actual readable syntax thats it)
-- here strings (like the below)
```lua
let mut here_string = //
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>GRUA Web Server</title>
    <style>
        body { font-family: sans-serif; background: #121212; color: #fff; }
        h1 { color: #00ffcc; }
    </style>
</head>
<body>
    <h1>Hello from Native GRUA!</h1>
    <p>This string was parsed perfectly across multiple lines.</p>
</body>
</html>
//
```
- add this #set_entry_point_to_here
    - reason if u want to set any function as entry point
    - add that as a cli flag when creating new project like as this "go run . new --entry=["Process", "<Data Type>"]" if Data Type Area is not mentioned make the below
    ```lua
    namespace App {
        function Process() {

        }
    }
    ```
- add support for this syntax in function arguments/parameters
```lua
-- workspace := UserDefinedWorkspace() 
-- this means above is to infer this type to the function argument/parameter 
function create_workspace(name : String, workspace := UserDefinedWorkspace()) {

}
```
-- add support for the below on restrict on T 
    - T must allow to be restricted to any builtin data type in the system
```lua
T restrict[Option[DefaultWorkspace], Option[UserDefinedWorkspace]]
```
- vim and emacs plugin for the langauge

# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds
- Make this langauge be able to run on web runtime with no issues