1. Variables
- In this language variables are immutable by default must be explicitly defined as mutable if it were to change state
- Variables must be explicitly defined as either global or local by default
```lua
-- local variable
local Int x = 10;

-- local mutable variable
local mut String xz = "string";

-- global variable
global Bool isTrue = False;

-- global mutable variable
global mut List[Int] itemsList = [10, 20, 30, 40];

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

-- pipe operator
-- The left value is passed as the first argument to the function on the right.
local Int piped = 2 |> Add(3) |> Double;
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
