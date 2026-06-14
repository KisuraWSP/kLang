# TODO
- add neovim plugin

- make the runtime be able to work on users browser by compiling to wasm code <done second>
- add a flag to start a simple web server that is built into the language that will package our entire languages runtime if the user doesnt want to build wasm and ship it <done third>

- start work on the standard library once all of the above are truely done
    - TODO MODULES
    1. builtin
    2. core
    3. datetime
    4. encoding
    5. enum
    6. ffi
    7. io
    8. mathg
    9. runtime
    10. path
    11. random
    12. reflect
    13. sort
    14. strings
    15. interface
    16. unicode

- little by little polish every existing feature in this system until we can do like 
complex programs that we will be able to run with whatever user created code and it must works, also make it have good helpers and make the programming interface good for all users

# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds
- Make this langauge be able to run on web runtime with no issues