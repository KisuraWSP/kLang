Todo
- fix scoping in the langauge, like the language doesnt understand how to create either variable(for local and global), function, loop scopes
- global variable declarations must work within any scope (add a new operator called) "export" to make this variable be accessible no matter which scope it is in
- improve the standard library more by adding more functions
- add error handling (error by values / exceptions)
- add rusts' Option(), Some(), None, Err(), Result()
- add first class functions
- make the runtime be able to work on users native os
- add type casting 
- add list comprehension from python
- lazy evaluated functions
- async await
- iterators
- Complex Number Data Types to be built in the language
- SIMD Data Types to be built in the langauge
- better type checking via Unification-Based Constraint Solving (Hindley-Milner / Algorithm W) and Dataflow-Based (Flow-Sensitive) Type Checking
- lambda functions
- more support boolean operations in condition handling and loops
- pipe operator
- tail call optimization for recursive functions
- referential transparency
- support for polymorphic functions
- Type restriction on generics via typed lists (like the below)
```lua
local mut T restrict[UInt, Int, Float] = 69420;

function X[T restrict[UInt, Int, Float]](x : T) : Int {
    return x;
}
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
- Null Safety Operator
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
- String & Array/List/Slice Indexing
- coroutines
- function deprecation marker tag
- default values on functions and restricted generic types 