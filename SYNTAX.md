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

```