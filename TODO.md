# TODO
- add list comprehension from python <do>
- lazy evaluated functions <do>
- Complex Number Data Types to be built in the language <do>
- SIMD Data Types to be built in the langauge <do>
- better type checking via Unification-Based Constraint Solving (Hindley-Milner / Algorithm W) and Dataflow-Based (Flow-Sensitive) Type Checking <do>
- lvalues & rvalues in variables <do>
- tail call optimization for recursive functions <do>
- referential transparency <do>
- Type restriction on generics via typed lists (like the below) <do>
    - Type restrictions must be strictly type checked no matter the type in T
```lua
local mut T restrict[UInt, Int, Float] = 69420;

function X[T restrict[UInt, Int, Float]](x : T) : Int {
    return x;
}
```
- default values on functions and restricted generic types, and zero initialized variables no matter the type  <do>
- add support for boolean like expressions on variables like the below, start of ternary operator <do>
```lua
local Bool x = if Init() > 0 then return False : True; 
```
- support for chained namespaces (like the below) <do>
```lua
namespace std {
    namespace lib {
        function LuaInit() {
            print("std.lib.LuaInit(); is called");
        }
    }
}

-- so now u will be able to call the function as
std.lib.LuaInit();
```
- also support for namespace aliasing <do>
```lua
-- this is to prevent weird issues in runtime execution regarding namespaces/functions/methods/callbacks or etc
alias std_lib = std.lib;

std_lib::LuaInit();
```
- add support for callbacks for functions <do>


# LATER
- improve the standard library more by adding more functions
- add error handling (error by values / exceptions)
- add first class functions
- make the runtime be able to work on users native os
- async await
- iterators
- lambda functions
- foreign function interface
- support for polymorphic functions via the below
```lua
function function1_name(x : Int) {
    print(x);
}

function function2_name(x : Int, y : String) {
    print(x, y);
}

function_group Poly {
    -- a global vTable that stores lists containing definitions of polymorphic function
    set_function_as_part_of[{ .name = "Poly" }, "function1_name", "function2_name"];
}

-- allow support for boolean operations on variables like this
local mut T x = if Poly(x) == Poly(x, y) then return y;
```
- inner functions (like the below)
```lua
function Test() {
    inner function Eval() {
        print("This is called");
    }
}

-- can be called like
Test().Eval();
```
- Table Data type to be builtin data type to the language which is used by lua (this is the only dynamically typed data type)
- traits system from rust
- move semantics
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
- multi threaded interpretter runtime
- coroutines
- better error/exception messages like similar to elms error messages (like the message should like tell the user whats wrong in the program and actually point the line of code where the error occurred)


# TODO When All Previous todos are done (End Goal)
- Make the languages runtime be able to run a million line code project within 10 seconds