1. Variables
- In this language variables are immutable by default must be explicitly defined as mutable if it were to change state
- Variables must be explicitly defined as either global or local by default
```lua
-- local variable
local Int x = 10;

(* 
   This is a multi line comment
*)

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

-- inferred local variables
-- let is immutable by default. let mut allows mutation.
let maybeCount = Some(69420);
let mut maybeMutableCount = Some(10);
maybeMutableCount = Some(11);

-- lazy variables
-- lazy declarations evaluate their initializer only when the value is first accessed.
lazy local Int expensiveCount = BuildCount();
lazy let cachedName = BuildName();
lazy var sharedConfig = LoadConfig();

-- destructuring declarations
-- List patterns use zero-based indexing. Object patterns read fields by selector.
local [first, second] = [1, 2];
let {name, count: total} = data;
local mut [head, [left, right]] = pairs;

-- discard identifier
-- _ evaluates and ignores a value without creating a binding, and can be reused.
-- Use _ when a value is intentionally ignored; ordinary unused variables and
-- function parameters are reported as checker warnings.
_ = LogEvent();
local _ = BuildTemporaryValue();
local [_, selected, _] = [1, 2, 3];

-- inferred global variables
-- val is immutable, while var is mutable.
val globalMaybeCount = Some(69420);
var sharedMaybeCount = Some(10);
sharedMaybeCount = Some(11);

-- const values are strictly immutable and must be resolved before runtime.
const intSize = Int.sizeof;
const byteSize = Int.child(8).sizeof;

-- Type.sizeof returns the size of a builtin type as Int.
-- The optional size marker maps the declaration type to Int.
let size intSizeAlias = Int.sizeof;

-- Numeric parent types can be restricted to child widths.
local x : Int.child(8) = 127;
local i16 smallCount = x;
local types.u8 byteCount = 255;
local float32 sampleRatio = 1.5;
local complex128 zed = Complex(1, 2);

-- shared builtin protocols
-- Collection-like values expose the same .count property.
local Int textCount = "hallo".count;
local Int listCount = [1, 2, 3].count;

-- Strings and chars expose case conversion methods.
local String loud = "hallo".uppercase();
local Char firstLetter = 'k'.uppercase();

-- Integers can call a callback once for each zero-based index.
function RememberIndex(index : Int) : Int {
    return index;
}

local Int lastIndex = 5.times(RememberIndex);

-- lvalues and rvalues
-- Only variables and indexed mutable variables can be assigned to.
-- Computed expressions, literals, function calls, and string indexes are rvalues.
local mut Int count = 1;
count += 1;
itemsList[1] = count;

-- referential transparency
-- Immutable bindings snapshot aggregate rvalues, so later mutations through another
-- mutable binding do not change the immutable value. The runtime implements this
-- with copy-on-write storage: assignment shares first, mutation detaches later.
local List[Int] savedItems = itemsList;
itemsList[0] = 100;

-- move semantics
-- move transfers a variable value and prevents later reads from the original variable.
local String owned = "hello";
local String transferred = move owned;

-- copy and clone semantics
-- copy and clone create eager cloned values without moving from the original binding.
local List[Int] sourceItems = [1, 2, 3];
local List[Int] copiedItems = copy sourceItems;
local List[Int] clonedItems = clone sourceItems;

-- command line arguments
-- Args is an immutable List[String] provided by the current program workspace.
local Int argCount = len(Args);
local String firstArg = Args[0];

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
-- String, List, and region-backed array indexes use Int positions and start at 0.
-- String indexing returns Char.
local Char firstChar = "hello"[0];
local Int firstItem = itemsList[0];

-- Map indexes use the map key type.
local Int total = scores["total"];

-- Table is the Lua-style dynamic container. Keys can be primitive values and values can be mixed.
local mut Table data = {"name": "klang", 1: True};
data["count"] = 3;
local String tableName = data.name;

-- user-defined memory regions for arrays and slices
-- T[RegionName] stores a zero-initialized region-backed array/slice value.
-- The final region argument is the maximum element count.
region MyRegion(T, sizeof(T) * 100, 10);
local mut T[MyRegion] myArray;
myArray[0] = "String";

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

-- Error propagation
-- The postfix ! operator unwraps Ok(value), or throws Err(value) to the nearest catch.
local Int parsedValue = parsedCount!;

-- typed ordinal enums
-- Enum variants follow Go/iota-style numbering: first implicit value is 0,
-- explicit values reset the next implicit ordinal.
enum NetworkState {
    Idle;
    Connecting = 10;
    Connected;
    Failed;
}

local NetworkState state = NetworkState.Connected;
local Int stateOrdinal = state.ordinal;
local String stateName = state.name;

if state == {
    case NetworkState.Idle:
        print("idle");
    case NetworkState.Connecting:
        print("connecting");
    case NetworkState.Connected:
        print("connected");
    case NetworkState.Failed:
        print("failed");
}

-- Complex and SIMD data
-- Complex(real, imaginary) accepts Int or Float parts.
local Complex z = Complex(2, 3);
local Complex moved = z + Complex(1, -1);

-- SIMD(list) creates a vector-like value with numeric lanes.
local SIMD[Int] lanes = SIMD([1, 2, 3, 4]);
local SIMD[Int] doubledLanes = lanes * 2;

-- Any accepts any value and cannot be restricted.
local mut Any dynamicValue = "text";
dynamicValue = 10;

-- here strings use // delimiters and produce multiline String values.
let mut here_string = //
<!DOCTYPE html>
<html lang="en">
<body>
    <h1>Hello from kLang!</h1>
</body>
</html>
//;

-- stdlib/html can render markup from typed functions, which is useful for WASM.
import "html";
local String renderedPage = html.Document(
    "kLang",
    html.Main([html.Class("page")], html.H1([], html.Text("Hello from kLang")))
);

-- stdlib imports collect only the referenced module functions by default.
-- This source collects html.Document, html.Main, html.Class, html.H1, html.Text,
-- and same-module helper functions reached from those functions.

-- opt out of selective stdlib function lookup for this source's imports
module_caller(call_entire_module : True);
import "runtime";

-- inside a module source, reject imports until the directive is removed
module(disabled : True);

-- inside a stdlib module, expose functions through the language's internal
-- global namespace symbol table. User code can call New() without importing
-- this module or writing alloc.New().
global namespace alloc {
    function New() : Int {
        return 1;
    }
}

-- run blocks and run calls execute before ordinary statements in the same block.
function Boot() {
    print("boot");
}

run {
    print("Yay this code gets executed");
}

run Boot();
```

2. Functions
- Basically we want user to be able to write powerful functions like this no matter the functions signature
```typescript
function Printf(formatString : String, value : List[T]) : Int {
    -- % is the universal placeholder. It consumes the next value from value.
    -- %% prints a literal percent sign.
    local mut Table report = {"format": formatString, "items": len(value), "placeholders": 0, "literals": 0, "valid": True};
    local Iterator[Char] chars = iter(formatString);
    local mut Option[Char] current = next(chars);
    local mut Int valueIndex = 0;

    while current.some {
        if current.value == "%" {
            local Option[Char] escaped = next(chars);
            if escaped.some and escaped.value == "%" {
                print("%");
            } else if valueIndex < len(value) {
                print(value[valueIndex]);
                valueIndex += 1;
                report["placeholders"] = report.placeholders + 1;
            } else {
                report["valid"] = False;
            }
        } else {
            print(current.value);
            report["literals"] = report.literals + 1;
        }

        current = next(chars);
    }

    if valueIndex != len(value) {
        report["valid"] = False;
    }

    print("format", report.format, "placeholders", report.placeholders, "literals", report.literals, "valid", report.valid);
    return report.placeholders;
}

function ToNumber(value : String) : Int {
    return value as Int;
}

function Add(left : Int, right : Int) : Int {
    return left + right;
}

-- multiple return values
-- Named return values are zero-initialized in the body.
function PrintPair() : (name : String, value : Int) {
    return "count", 10;
}

function PrintPair2() : (mut String, Int) {
    return "", 0;
}

-- private functions are hidden from other files.
private function HiddenAdd() : T {
    return "String is added" as Int;
}

-- private blocks create a hidden lexical scope.
private {
    local Int hiddenScratch = 1;
}

-- inline marks a function as an eager inline candidate.
inline function InlineAdd(left : Int, right : Int) : Int {
    return left + right;
}

-- defer runs at the end of the current runtime block.
function WithCleanup() : Int {
    defer print("cleanup");
    return 1;
}

-- parameters are immutable by default; add mut before the name to allow mutation.
function Increment(mut value : Int) : Int {
    value += 1;
    return value;
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

-- inferred default parameters
-- workspace := UserDefinedWorkspace() infers the parameter type from the default expression.
function UserDefinedWorkspace() : String {
    return "default-workspace";
}

function create_workspace(name : String, workspace := UserDefinedWorkspace()) : String {
    return workspace;
}

-- entry point directive
-- The following function becomes the runtime entry point for this workspace.
namespace App {
    #set_entry_point_to_here
    function Process() {
        print("custom entry");
    }
}

-- callbacks
-- Function[Return] is a no-argument callback. Function[Arg, Return] takes one argument.
-- Function[Left, Right, Return] takes two arguments.
function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Combine(left : Int, right : Int, callback : Function[Int, Int, Int]) : Int {
    return callback(left, right);
}

-- first-class functions
-- Functions can be stored in variables, returned from functions, and called later.
function NumberFactory(multiplier : Int) : Function[Int, Int] {
    function InnerGenerator(value : Int) : Int {
        return value * multiplier;
    }
    return InnerGenerator;
}

local Function[Int, Int] timesTen = NumberFactory(10);
local Int generated = NumberFactory(5)(10);

-- lambda functions
-- fun creates an anonymous first-class function that captures the current scope.
local Function[Int, Int] increment = fun(value : Int) : Int {
    return value + 1;
};

local Function[Int, Int] restrictedIncrement = fun[T restrict[Int]](mut value : T) : T {
    value += 1;
    return value;
};

local Int appliedLambda = Apply(41, fun(value : Int) : Int {
    return value + 1;
});

-- inner functions
-- inner functions are captured from the outer function call and can be selected from its result.
function Test() {
    inner function Eval() {
        print("This is called");
    }
}

Test().Eval();

-- polymorphic function groups
-- function_group creates a global vTable-style call target that dispatches by argument count and runtime types.
function function1_name(x : Int) : Int {
    print(x);
    return x;
}

function function2_name(x : Int, y : String) : String {
    print(x, y);
    return y;
}

function_group Poly {
    set_function_as_part_of[{ .name = "Poly" }, "function1_name", "function2_name"];
}

local String y = "1";
local mut T x = if Poly(1) == Poly(1, y) then return y : "no";

-- lazy evaluated functions
-- lazy function arguments are evaluated only when the function body reads them.
lazy function Choose(useFirst : Bool, first : Int, second : Int) : Int {
    if useFirst {
        return first;
    }
    return second;
}

-- tail-call optimized recursion
-- A recursive call returned directly from the same function reuses the current call frame.
function CountDown(value : Int, total : Int) : Int {
    if value == 0 {
        return total;
    }
    return CountDown(value - 1, total + 1);
}

-- async/await
-- async functions return Awaitable[T]. await unwraps the completed value.
async function LoadCount() : Int {
    return 40;
}

local Awaitable[Int] pendingCount = LoadCount();
local Int loadedCount = await pendingCount;

-- iterators
-- iter(value) creates Iterator[T]. next(iterator) returns Option[T].
local Iterator[Int] numbers = iter([1, 2, 3]);
local Option[Int] firstNumber = next(numbers);
local Option[Int] secondNumber = next(numbers);

-- coroutines
-- coroutine(functionValue) creates Coroutine[T]. resume(coroutine) returns Option[T].
function BuildCount() : Int {
    return 2;
}

local Coroutine[Int] buildCount = coroutine(BuildCount);
local Option[Int] resumedCount = resume(buildCount);

-- multi-threaded interpreter workers
-- spawn starts a child interpreter worker. join waits for its result.
function CountWorker(counter : Atomic[Int], mut amount : Int) : Int {
    while amount > 0 {
        atomic_add(counter, 1);
        amount -= 1;
    }
    return atomic_load(counter);
}

local Atomic[Int] sharedCounter = Atomic(0);
local Thread[Int] workerA = spawn(CountWorker, [sharedCounter, 25]);
local Thread[Int] workerB = spawn(CountWorker, [sharedCounter, 17]);
local String workerStatus = thread_status(workerA);
local Int workerAResult = join(workerA);
local Int workerBResult = join(workerB);
local Int threadedTotal = atomic_load(sharedCounter);

-- atomic race-safe cells
local Atomic[Int] counter = Atomic(1);
atomic_add(counter, 2);
atomic_store(counter, atomic_load(counter) + 1);
local Int counterValue = atomic_load(counter);

-- compact build/workspace meta-programming
-- BuildSystem backend must be "WASM", "JS", or "Standalone".
-- WASM packages run the interpreter/runtime in the browser as WebAssembly.
-- The CLI can also serve a WASM browser bundle through `kLang serve`.
local Program program = Program(["app", "mathg"]);
local BuildSystem build = BuildSystem("demo", 2, ["first.klang", "app.klang"], "Standalone");
local WorkSpace workspace = WorkSpace(program, build);
local String backend = workspace_backend(workspace);
local List[String] packageFiles = workspace_files(workspace);
local String manifest = workspace_manifest(workspace);

-- debugger helpers
debug(manifest);
local String manifestType = debug_type(manifest);
local List[String] stack = debug_stack();
breakpoint("after manifest");

-- source context diagnostics
-- The engine builds Context and ErrorContext descriptors while checking,
-- running, packaging, and generating WASM bundles. ErrorContext reports include
-- phase, file, line, column, source line, violated rule, message, and hint.

-- JavaScript filesystem-only FFI
-- js_import reads a local .js file and returns a descriptor.
local JSModule js = js_import("vendor/library.js");
local List[String] jsExports = js_exports(js);
local String jsSource = js_source(js);
local JSCall pendingJSCall = js_call(js, "init", [manifest]);

-- variadic print and input
print("count", 1, True);
local String name = input("name: ");

-- alias functions and extension methods
-- alias function creates a constructor-like type value. #extend adds receiver methods.
-- [T: Any] can later be replaced with stricter forms like T restrict[Int, Float].
-- .DEFAULT asks the runtime to use the default initializer for that argument.
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) : type {
    trait LengthTracked {
        function Size(value : Int) : Int;
    }

    impl LengthTracked for Int {
        function Size(value : Int) : Int {
            return value;
        }
    }

    [new] {
        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));
    }

    [delete] {
        allocator.free = free_all_allocator(.{});
    }

    [side_effects] {
        local T call_site = #call_site;
        allocator.free = free_all_allocator(.{});
    }

    #extend {
        function get_length() -> int {
            return this.length;
        }

        function with_extra(extra : Int) -> int {
            return this.length + extra;
        }
    }
}

local T arrayList = ArrayList("value", 1, 100);
local Int arrayListLength = arrayList.get_length();
local Int extendedLength = arrayList.with_extra(5);

-- generic Array alias with allocator support
-- allocator is typed as T so HeapAllocator, RegionAllocator, BumpAllocator, ArenaAllocator,
-- Box, Ref, RefMut, RefCell, or any future allocator-like value can be passed in.
alias function Array[T: Any](data: T, length: int, capacity: int, allocator: T = .DEFAULT) : type {
    [new] {
        allocator.region = get_default_procces_allocator(#region(capacity, T), #sizeof(T));
    }

    [delete] {
        allocator.free = free_all_allocator(.{});
    }

    [side_effects] {
        allocator.free = free_all_allocator(.{});
    }

    #extend {
        function get_length() -> int {
            return this.length;
        }

        function get_capacity() -> int {
            return this.capacity;
        }

        function remaining() -> int {
            return this.capacity - this.length;
        }
    }
}

local T heapArray = Array(10, 1, 32, HeapAllocator());
local T regionArray = Array("text", 1, 64, RegionAllocator("TextRegion"));
local T bumpArray = Array(True, 1, 128, BumpAllocator());
local T defaultArray = Array(0, 0, 16);
local Int arrayRemaining = heapArray.remaining();

-- allocator and pointer-like wrappers
-- These values are tracked as heap allocations by the runtime memory model.
local T boxed = Box("value");
local T ref = Ref(boxed);
local T refMut = RefMut(boxed);
local T cell = RefCell(boxed);
local T heap = HeapAllocator();
local T regionAllocator = RegionAllocator("MyRegion");
local T bump = BumpAllocator();
local T arena = ArenaAllocator();

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

3. Error Handling
- Errors can be handled as values with `Result[T, E]`, or as exceptions with `throw` and `try/catch`.
- `Result!` propagates `Err(value)` through the exception path and unwraps `Ok(value)`.
```lua
function Fallible() : Result[Int, String] {
    return Err("not ready");
}

function Main() : Int {
    try {
        local Int value = Fallible()!;
        return value;
    } catch err {
        print("handled", err);
        return 0;
    }
}

function FailFast() {
    throw "boom";
}
```

4. Condition Handling
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

-- pattern matching switch statement
-- Matches are strict and type safe. Bool, String, Int, and Float values are allowed.
-- Table values cannot be matched. Each case breaks by default.
-- Use continue inside a case to fall through to the next case.
-- Non-partial matches must include a default case, except Bool matches can cover True and False.
local String mode = "blank";
if mode == {
    case "blank":
        print("hallo");
    case "debug":
        print("debug");
        continue;
    case:
        print(10);
}

local Bool ready = True;
if ready and not False == {
    case True:
        print("ready");
    case False:
        print("not ready");
}

local Int code = 2;
partial if code == {
    case 1:
        print("one");
    case 2:
        print("two");
}

-- unless .. else
-- If the Boolean expression evaluates to false, then the block of code inside the unless statement will be executed. If the Boolean expression evaluates to true, then the code after the else keyword of the given unless statement will be executed.
unless x > y {
    print(y);
} else {
    print(x);
}
```

5. Loops
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

6. Namespaces
- Namespaces can be nested. Nested functions are called through chained dot paths.
- `alias` binds a shorter name to a namespace path, and `::` calls through that alias.
```lua
namespace std {
    namespace lib {
        function LuaInit() {
            print("std.lib.LuaInit(); is called");
        }
    }
}

std.lib.LuaInit();

alias std_lib = std.lib;
std_lib::LuaInit();
```

7. Traits
- Traits declare required method signatures. Impl blocks must define matching methods for a type.
```lua
trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
    function Show(value : Int) : String {
        return value as String;
    }
}
```

8. Workspaces and raw language mode
- Each standalone script or folder project is resolved as a separate workspace.
- Local imports are resolved inside the workspace before stdlib imports.
- Resolver caches imported files and parsed import lists to speed repeated checks.
- Stdlib imports use selective function lookup by default; only functions called through the imported module namespace and their same-module helper dependencies are collected.
- Use `module_caller(call_entire_module : True);` in the importing source to load complete stdlib modules.
- Use `module(disabled : True);` in a module source to make imports of that module fail until the directive is removed or set to false.
- Use `--raw-lang` with the CLI to disable stdlib imports for a pure language run.
```sh
kLang run examples/helloworld
kLang run examples/helloworld first second
kLang check examples/helloworld --raw-lang
```
