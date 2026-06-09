1. Variables
- In this language variables are immutable by default must be explicitly defined as mutable if it were to change state
- Variables must be explicitly defined as either global or local by default
```lua
-- local variable
local Int x = 10;

-- zero initialized variables
-- If no initializer is given, the declared type receives its zero value.
local Int zeroCount;
local String emptyText;
local List[Int] emptyItems;

-- local mutable variable
local mut String xz = "string";
xz = "updated";

-- global variable
global Bool isTrue = False;

-- global mutable variable
global mut List[Int] itemsList = [10, 20, 30, 40];
itemsList[0] = 99;

-- lvalues and rvalues
-- Only variables and indexed mutable variables can be assigned to.
-- Computed expressions, literals, function calls, and string indexes are rvalues.
local mut Int count = 1;
count += 1;
itemsList[1] = count;

-- referential transparency
-- Immutable bindings snapshot aggregate rvalues, so later mutations through another
-- mutable binding do not change the immutable value.
local List[Int] savedItems = itemsList;
itemsList[0] = 100;

-- exported variable
-- export makes the variable accessible through the global scope even when declared inside a block or function.
export local Int sharedValue = 10;

-- global declarations are valid in any scope.
global mut Int sharedCounter = 0;

-- type casts
-- cast any expression with "as Type"; this works in variables, returns, calls, and loop headers.
local Float ratio = count as Float;
local String label = ratio as String;

-- indexing
-- String and List indexes use Int positions. String indexing returns Char.
local Char firstChar = "hello"[0];
local Int firstItem = itemsList[0];

-- Map indexes use the map key type.
local Int total = scores["total"];

-- list comprehension
-- Build a List by mapping each item from a List, String, or range count.
local List[Int] doubled = [value * 2 for value in itemsList];
local List[Int] evens = [value for value in itemsList if value % 2 == 0];
local List[Char] letters = [letter for letter in "hello"];
local List[Int] indexes = [index for index in range(5)];

-- operator precedence
-- From strongest to weakest: call/index/selector, cast, exponent, unary, multiply/divide/modulo, add/subtract, comparison, equality, and, or.
local Int expression = 1 + 2 * 3;
local Int powered = -2 ** 3 ** 2;
local Int grouped = (1 + 2) * 3;

-- null safety
-- The postfix ? operator returns True when an expression is not Null, otherwise False.
local Bool hasValue = MaybeValue()?;

-- boolean operators
-- not binds tighter than and, and binds tighter than xor, and xor binds tighter than or.
local Bool shouldRun = not failed and active xor retrying or fallback;

-- conditional expression
-- "return" after then is accepted as expression sugar.
local Bool ready = if Init() > 0 then return False : True;

-- restricted generic variable
-- T restrict[...] only accepts values assignable to one of the listed types.
local mut T restrict[UInt, Int, Float] numeric = 69420;

-- pipe operator
-- The left value is passed as the first argument to the function on the right.
local Int piped = 2 |> Add(3) |> Double;

-- Option and Result types
-- Some(value) creates Option[T]. None() creates an empty Option[T].
local Option[Int] maybeCount = Some(10);
local Option[Int] missingCount = None();

-- Ok(value) and Err(value) create Result[T, E]. Result(value) is a convenience Ok result.
local Result[Int, String] parsedCount = Ok(10);
local Result[Int, String] failedParse = Err("invalid number");
local Result[Int, String] wrappedCount = Result(20);

-- Complex and SIMD data
-- Complex(real, imaginary) accepts Int or Float parts.
local Complex z = Complex(2, 3);
local Complex moved = z + Complex(1, -1);

-- SIMD(list) creates a vector-like value with numeric lanes.
local SIMD[Int] lanes = SIMD([1, 2, 3, 4]);
local SIMD[Int] doubledLanes = lanes * 2;
```

2. Functions
- Basically we want user to be able to write powerful functions like this no matter the functions signature
```typescript
function Print(formatString : String, value : List[T]) : Int {
    while info:= len(formatString) > 0 {
        local List[String] splitStringIntoBytes = make([], getBytesFromString(formatString));
        if splitStringIntoBytes[info] == getEncodedStringInformation(formatString, "%s") {
            return 1;
        } else if splitStringIntoBytes[info] == getEncodedStringInformation(formatString, "%d"){
            return 2;
        } else if splitStringIntoBytes[info] == getEncodedStringInformation(formatString, "%f") {
            return 3;
        } splitStringIntoBytes[info] == getEncodedStringInformation(formatString, "%b") {
            return 4;
        }
    }
    
    return 0;
}

function ToNumber(value : String) : Int {
    return value as Int;
}

function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Double(value : Int) : Int {
    return value * 2;
}

-- default parameters and restricted generic types
-- T restrict[UInt, Int, Float] accepts only values compatible with the listed types.
function IdentityNumber[T restrict[UInt, Int, Float]](value : T = 1) : T {
    return value;
}

function AddDefault(left : Int = 1, right : Int = 2) : Int {
    return left + right;
}

-- deprecation marker tag
-- Calling a deprecated function is allowed, but the checker reports a warning.
@deprecated
function OldToNumber(value : String) : Int {
    return value as Int;
}

@deprecated("use ToNumber")
function ParseNumber(value : String) : Int {
    return value as Int;
}
```

3. Condition Handling
- Used to do basic boolean based condition operations
```lua
-- if .. else if .. else statement
if x > y {
    print(x);
} else if y < x {
    print(y);
} else {
    print("this is interesting");
}

-- boolean operations in conditions
if not failed and active xor retrying or fallback {
    print("running");
}

-- Some and Ok are truthy. None and Err are falsey.
if maybeCount and parsedCount {
    print("values are present");
}

-- unless .. else
-- If the Boolean expression evaluates to false, then the block of code inside the unless statement will be executed. If the Boolean expression evaluates to true, then the code after the else keyword of the given unless statement will be executed.
unless x > y {
    print(y);
} else {
    print(x);
}
```

4. Loops
- These are basic repetition constructs used by programming languages
```lua
-- basic for loop (like in C)
for i:=0; i > 10; i+=1 {
    print(i);
}

-- for range loop
-- iterates from 0..N-1
for i := range(10) {
    print(i);
}

-- while loop
while i:= 1 {
    print(True);
    i += 1;
    if i == 3 break;
}

-- cast in a loop header binding
while active := i as Bool {
    break;
}

-- null safety in a loop header binding
while active := MaybeValue()? {
    break;
}

-- boolean operations in loop conditions
while active := ready and not failed xor retrying {
    break;
}

-- do_while loop
do_while i := range(10) {
    print(i);
    if i == 2 break;
}
```
