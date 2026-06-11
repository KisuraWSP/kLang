# TODO
- foreign function interface
- Improved CLI for better user experience and creation of ".klang" projects
- pattern matching switch statement (like the below)
    - should support boolean operations
    - should support strings
    - should support integers
    - should support floats
    - this system must be strict and type safe
    - tables are not allowed to be pattern matched
    - should have break by default
    - user can fallthrough a switch statement if "continue" keyword is provided
    - switch statements are exhaustive unless declared as partial via "partial" keyword
```lua
if x == {
    case "blank":
        print("hallo");
    -- default case
    case: print(10);
}
```
- data race condition prevention mechanics and atomic data handling
- traits and other features must be able to used in function aliases


# LATER <once todo is done>
- make the runtime be able to work on users native os <done after first>
- multi threaded interpretter runtime <done first >


# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds