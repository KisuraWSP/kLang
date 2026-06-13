# TODO
- add neovim plugin

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
- built in debugger in the language so u can debug code issues
- foreign function interface for javascript apis/libraries (via a js filesystem only)


- multi threaded interpretter runtime <done first >
- make the runtime be able to work on users browser by compiling to wasm code <done second>
- add a flag to start a simple web server that is built into the language that will package our entire languages runtime if the user doesnt want to build wasm and ship it <done third>

- start work on the standard library once all of the above are truely done

# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds
- Make this langauge be able to run on web runtime with no issues