# TODO
- add first class functions
- lambda functions
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
- traits system from rust
- move semantics
- print function must be variadic,
- add input() function from python
- function aliases & extension functions inside aliases
```lua
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) -> type
    -- This tells the langauge to do the following code when it is trying to allocate memory for this type
    [new] do
        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));
    end

    -- This tells the langauge to do the following code when it is trying to deallocate memory for this type
    [delete] do
        allocator.free = free_all_allocator(.{}); 
    end

    -- This tells the langauge to do the following code to occur when this type is having side_efffects
    [side_effects] do
        allocator.free = free_all_allocator(.{}); 
    end

    -- This tells the langauge that we are extending the custom data type to contain such a method to be allowed the below code
    -- x = ArrayList(bool, 100, 1000);
    -- x.get_length();
    #extend do
        function get_length() -> int
            return this.length;
        end
    end
end
```


# LATER
- user defined memory regions for array types and slices <implement these array types and slices>
- arrays and slices start at index 0 always
```lua
region MyRegion(T, sizeof(T) * 100, 10);
local mut T[region] myArray;
myArray[0] = "String";
```
- add memory allocators like {Box, Ref, RefMut, RefCell} from rust, HeapAllocator, RegionAllocator, BumpAllocator, ArenaAllocator
- improve the standard library more by adding more functions
- add error handling (error by values / exceptions)
- make the runtime be able to work on users native os
- async await
- iterators
- foreign function interface
- Table Data type to be builtin data type to the language which is used by lua (this is the only dynamically typed data type)
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