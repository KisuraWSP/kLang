1. Variables
- In this language variables are immutable by default must be explicitly defined as mutable if it were to change state
- Variables must be explicitly defined as either global or local by default

2. Formatter
- The CLI formatter accepts parse-valid Klang files and normalizes them to the canonical style used by examples, docs, and stdlib code.
- `kLang fmt file.klang` prints formatted source to stdout.
- `kLang fmt file.klang --write` rewrites one file, and `kLang fmt src --write` rewrites every `.klang` file in a folder.
- `kLang fmt src --check` fails when any source file would change, which is suitable for CI.

3. JSON
- `JSON` parses a String or here string into an immutable builtin JSON value.
- Use `json_parse` when invalid input should be handled as `Result[JSON, String]` instead of a runtime error.

```klang
local JSON config = JSON(//
{
    "name": "kLang",
    "ports": [8080, 8081],
    "debug": true,
    "metadata": null
}
//);

local String name = option_unwrap_or(json_string(config.name), "unknown");
local Int firstPort = option_unwrap_or(json_int(config.ports[0]), 0);
local Bool debug = option_unwrap_or(json_bool(config.debug), False);
local Bool missingMetadata = json_is_null(config.metadata);
local String encoded = json_stringify(config);
local Result[JSON, String] checked = json_parse(userInput);
local Result[String, String] encodedNative = json_encode({"items": [1, 2], "ok": True});
local Result[T, String] decodedNative = json_decode(encodedNative.value);

-- The json stdlib names the native-value operations serialize/deserialize.
local Result[String, String] serialized = json.serialize({"name": "kLang"});
local Result[T, String] deserialized = json.deserialize(serialized.value);
```

Disable the program cache for a workspace when fresh module resolution and type checking are required:
```lua
no_cache;
```

Folder projects are loaded through `klang.project`, a TOML manifest:
```toml
name = "demo"
entry = "first.klang"
sources = ["first.klang", "app.klang"]
```

Projects created with `kLang new` always use `first.klang` as the manifest
entry and generate a stable `Main() : Int` wrapper that calls `App.Start()`.
The old `new --entry` flag is deprecated and ignored.

Loose `.klang` files and legacy `first.klang` folders must opt in explicitly:
```lua
load_as_script;
```

Struct-style aliases can rename constructor fields during JSON serialization:
```lua
alias function User(id : String, displayName : String) : type = struct {
    this.id `json:"user_id"`;
    this.displayName `json:"display_name"`;
}

let user = User("42", "Ada");
local JSON document = JSON(user);
local String encoded = json_stringify(user);
local Type metadata = User.get_runtime_type_info();
print(metadata.serialization.json_tags);
```

Type aliases provide shorter compile-time names without creating a new runtime type:
```lua
type string_list = List[String];
type names = string_list;
type optional_names = Option[names];

function Echo(values : names) : names {
    local names copied = values as names;
    return copied;
}
```

Cast expressions are restricted to builtin target types or aliases that resolve to builtin target types:
```lua
local Float ratio = count as Float;
local string_list copied = values as string_list;

-- Invalid: User is a user-defined alias struct, not a builtin cast target.
-- local User user = raw as User;
```

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

-- temporary variables
-- Temporary variables are local short-lived bindings for intermediate values.
-- They are tracked as temporary state and do not produce unused-variable warnings.
temp local Int scratchTotal = 40 + 2;
temp let scratchLabel = "temporary";
lazy temp local String lazyScratch = BuildName();

-- destructuring declarations
-- List patterns use zero-based indexing. Object patterns read fields by selector.
local [first, second] = [1, 2];
let {name, count: total} = data;
local mut [head, [left, right]] = pairs;

-- typed multi-variable declarations unpack multiple return values by position.
function Multi() : (table : Table, count : Int) {
    return {"name": "klang"}, 7;
}
local Table x, Int y = Multi();
local Table kept, Int _ = Multi();

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

-- Type metadata is available for every language type.
local Type intInfo = Int.get_runtime_type_info();
local Type stringInfo = String.get_runtime_type_info();
local String typeName = intInfo.name;
local Int typeBytes = intInfo.size;
local Table layout = intInfo.layout;

-- Numeric parent types can be restricted to child widths.
local x : Int.child(8) = 127;
local i16 smallCount = x;
local types.u8 byteCount = 255;
local float32 sampleRatio = 1.5;
local complex128 zed = Complex(1, 2);

-- Numeric literals may be signed and may use explicit bases.
local Int debt = -42;
local Int hexMask = 0xAAAA;
local Int octalMode = 0o755;
local Int binaryFlags = 0b1010;
local Int negativeHex = -0xA;
local Int readableMillion = 1_000_000;
local Int readableMask = 0xFF_FF;
local Float readableRatio = 12_345.67_89;

-- Unicode identifiers are allowed for variables, functions, and parameters.
function එකතු(අගය : Int, 😀 : Int) : Int {
    return අගය + 😀;
}
local Int මුළු = එකතු(1, 2);

-- function arguments are pass-by-value by default. A mutable parameter changes
-- only the local copy inside the callee.
function IncrementCopy(mut value : Int) : Int {
    value += 1;
    return value;
}

-- ref parameters alias a mutable caller binding and can write back to it.
function IncrementRef(ref value : Int) {
    value += 1;
}

local mut Int referenceCount = 1;
IncrementCopy(referenceCount); -- referenceCount is still 1
IncrementRef(referenceCount);  -- referenceCount is now 2

-- assert checks runtime invariants. The condition must be Bool.
assert මුළු == 3;

-- report prints a runtime diagnostic block with value, type, and stack frames.
report මුළු;
report එකතු(1, 2);

-- tests
-- The CLI discovers and runs functions whose names start with Test.
function TestAddition() {
    assert 1 + 1 == 2;
}

function TestBoolStyle() : Bool {
    return "klang".count == 5;
}

function TestStatusStyle() : Int {
    return 0;
}

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

-- for_each loops over iterable values directly.
local mut Int directTotal = 0;
for_each value in [1, 2, 3] {
    directTotal += value;
}

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

-- user-defined lexical scope
-- scope creates a named block for grouping setup/workflow code without creating
-- a namespace. Locals declared inside do not leak outside, while export/global
-- declarations still intentionally escape.
scope Setup {
    local Int value = 10;
    print(value);
}

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

-- Set[T] stores unique primitive values and preserves insertion order when iterated.
local Set[String] imports = Set(["lexer", "parser", "lexer"]);
local Int importCount = imports.count;      -- 2
local Bool hasParser = set_has(imports, "parser");
local Iterator[String] importIterator = iter(imports);

-- Table is the Lua-style dynamic container. Keys can be primitive values and values can be mixed.
local mut Table data = {"name": "klang", 1: True};
data["count"] = 3;
local String tableName = data.name;
local Int tableEntries = data.count;      -- builtin protocol count
local Int userCount = data["count"];      -- user field named "count"
local Bool hasName = table_has(data, "name");
data = table_delete(data, "name");        -- None()/Null values do not delete

-- Table keys compare by primitive kind and value, so these are distinct.
data[1] = "numeric";
data["1"] = "string";
data['1'] = "char";

-- Table iteration yields insertion-order entry tables with key and value fields.
local Iterator[Table] tableIterator = iter(data);
local Option[Table] firstEntry = next(tableIterator);
local List[T] tableKeys = table_keys(data);
local List[T] tableValues = table_values(data);
local List[Table] tableEntriesList = table_entries(data);
local Int sequenceLength = table_sequence_count(data);

-- Fallback tables provide controlled prototype-style missing-key lookup.
local Table parent = {"kind": "base"};
local Table child = table_set_fallback({}, parent);
local String inheritedKind = child.kind;

-- user-defined memory regions for arrays and slices
-- T[RegionName] stores a zero-initialized region-backed array/slice value.
-- The final region argument is the maximum element count.
region MyRegion(T, sizeof(T) * 100, 10);
temp region ScratchRegion(T, sizeof(T) * 16, 4);
local mut T[MyRegion] myArray;
myArray[0] = "String";
local mut T[ScratchRegion] scratchArray;
scratchArray[0] = "temporary";

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
-- The postfix ? operator returns True when an expression is present: non-Null,
-- Some(...) for Option, or Ok(...) for Result.
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

-- named generic constraints
-- T numeric, comparable, hashable, iterable, allocator_like, or a trait name
-- can be used where a generic parameter needs a shared capability.
function AddNumbers[T numeric](left : T, right : T) : T {
    return left + right;
}

trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
    function Show(value : Int) : String {
        return value as String;
    }
}

function KeepPrintable[T Printable](value : T) : T {
    return value;
}

-- Any is the universal generic constraint. Nested callback types participate
-- in inference across the complete argument list.
function Transform[T Any, U Any](value : T, mapper : Function[T, U]) : U {
    return mapper(value);
}

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
local Bool hasCount = maybeCount?;
local Bool parseSucceeded = parsedCount?;

function Double(value : Int) : Int {
    return value * 2;
}

function KeepPositive(value : Int) : Option[Int] {
    if value > 0 { return Some(value); }
    return None();
}

function ParseLabel(value : Int) : Result[String, String] {
    return Ok("count");
}

local Option[Int] doubledCount = option_map(maybeCount, Double);
local Int countOrZero = option_unwrap_or(missingCount, 0);
local Option[Int] positiveCount = option_and_then(maybeCount, KeepPositive);
local Result[Int, String] doubledParsed = result_map(parsedCount, Double);
local Int parsedOrZero = result_unwrap_or(failedParse, 0);
local Result[String, String] parsedLabel = result_and_then(parsedCount, ParseLabel);

-- Focused stdlib facades use the same builtin Option and Result values.
import "option";
import "result";
import "test";

local Int requiredCount = option.expect(maybeCount, "count is missing");
local Result[Int, String] mappedCount = result.map(parsedCount, Double);
_ = test.ok(mappedCount);

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

-- imports can appear anywhere, and a qualified call can infer a matching import.
-- The call below resolves stdlib/list without writing import "list"; first.
local List[Int] copiedNumbers = list.copy([1, 2, 3]);

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

4. Functions
- Basically we want user to be able to write powerful functions like this no matter the functions signature
```typescript
-- Official runtime-backed formatting is available without imports.
local String formatted = format("Hello %, score %% %", ["kLang", 42]);
local Int printedCount = printf("%", [formatted]);

-- The stdlib fmt module provides namespaced wrappers.
import "fmt";
local String moduleFormatted = fmt.Format("module %", ["fmt"]);
local Int modulePrinted = fmt.Printf("% ready", [moduleFormatted]);

-- You can still write custom formatters when you need domain-specific rules.
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
-- New projects default to Main() in first.klang.
-- Use this only when a source file intentionally chooses another runtime entry.
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
-- JS emits experimental native JavaScript for the typed core subset.
-- Standalone packages interpreter sources; WASM runs the interpreter/runtime in browser WebAssembly.
-- The CLI can also serve a WASM browser bundle through `kLang serve`.
local Program program = Program(["app", "mathg"]);
local BuildSystem build = BuildSystem("demo", 2, ["first.klang", "app.klang"], "Standalone");
local WorkSpace workspace = WorkSpace(program, build);
local String backend = workspace_backend(workspace);
local List[String] packageFiles = workspace_files(workspace);
local String manifest = workspace_manifest(workspace);

-- Native JS core subset example:
-- kLang package main.klang --backend=JS
function SumForJS(limit : Int) : Int {
    local mut Int total = 0;
    local mut Int index = 0;
    while index < limit {
        total += index;
        index += 1;
    }
    return total;
}

-- JS also lowers nested namespaces and resolved imports. Selective stdlib
-- imports emit only called functions plus resolver-expanded helpers.
import "math";
alias arithmetic = math;
function ImportedSumForJS() : Int {
    return arithmetic::Add(20, 22);
}

-- JS String operations preserve Klang's Unicode code-point behavior.
function StringsForJS() : Int {
    local String value = "h😀llo";
    local String message = "len=" + len(value) + ":" + True;
    local String upper = value.uppercase();
    local String lower = upper.lowercase();
    local Char emoji = value[1];
    print(message, upper, lower, emoji, value.count);
    return len(value);
}

-- JS List operations preserve checked indexing and collection value isolation.
function ListsForJS() : Int {
    local mut List[Int] values = [1, 2, 3];
    local List[Int] original = values;
    values[len(values)] = 4;
    for index := range(len(values)) {
        values[index] += index;
    }
    local List[Int] doubled = [value * 2 for value in values if value > 2];
    print(original, values, doubled, values.count);
    return doubled[0];
}

-- JS struct aliases include fields, defaults, extension methods, and JSON tags.
alias function User(id : String, name : String, active : Bool = True) : type = struct {
    this.id `json:"user_id"`;

    #extend {
        function label() : String {
            return this.name + ":" + this.active;
        }
    }
}

function StructJSONForJS() : String {
    let user = User("42", "Ada");
    local JSON document = JSON(user);
    print(user.label());
    return json_stringify(document);
}

-- Native JS packages include program.js.map and a sourceMappingURL.
-- npm start enables Node's source-map stack rewriting. Direct execution also
-- renders kLang-aware runtime frames with source excerpts and carets.

-- Native JS also preserves typed Map and insertion-ordered Table semantics,
-- including key-kind identity, copy isolation, fallback lookup, and helpers.

-- debugger helpers
debug(manifest);
local String manifestType = debug_type(manifest);
local List[String] stack = debug_stack();
local List[Table] states = debug_state();
breakpoint("after manifest");

-- source context diagnostics
-- The engine builds Context and ErrorContext descriptors while checking,
-- running, packaging, and generating WASM bundles. ErrorContext reports include
-- phase, file, line, column span, source line, violated rule, message, and hint.
-- Type diagnostics can include did-you-mean suggestions, import hints, and
-- expected/found type trees.

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
-- Generic restrictions may appear after function or after the alias name.
-- .DEFAULT asks the runtime to use the default initializer for that argument.
alias function[T Printable] PrintableValue(value : T) : type = struct {
}

alias function[T restrict[List[Option[Int]]]] OptionalInts(value : T) : type = struct {
}

-- The established suffix form remains equivalent:
alias function NumericValue[T numeric](value : T) : type = struct {
}

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

-- struct-style alias functions are statically understood by the checker.
-- Parameters become fields, generic arguments are inferred from constructor
-- arguments, and #extend methods can use this plus returned alias values.
alias function Boxed[T: Any](items : List[T], capacity : Int) : type = struct {
    #extend {
        function count() : Int {
            return len(this.items);
        }

        function get(index : Int) : T {
            local List[T] values = this.items as List[T];
            return values[index];
        }

        function push(value : T) : Boxed {
            local mut List[T] values = clone (this.items as List[T]);
            values[len(values)] = value;
            return Boxed(values, this.capacity);
        }
    }
}

let mut boxed = Boxed([1, 2], 2);
boxed = boxed.push(3);
local Int boxedValue = boxed.get(2);
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

5. Error Handling
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

6. Condition Handling
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
-- Matches are strict and type safe. Bool, String, Int, Float, Enum, Option,
-- Result, List, and Table values are allowed. Each case breaks by default.
-- Use continue inside a case to fall through to the next case.
-- Non-partial matches must be exhaustive for Bool, enum, Option, and Result
-- values, or include a default case.
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

local Option[Int] maybeValue = Some(10);
if maybeValue == {
    case Some(value):
        print(value);
    case None():
        print(0);
}

local Result[Int, String] parsedValue = Err("bad");
if parsedValue == {
    case Ok(value):
        print(value);
    case Err(message):
        print(message);
}

local List[Int] pair = [1, 2];
partial if pair == {
    case [1, 2]:
        print("pair");
}

local Table event = {"kind": "count", "value": 4};
partial if event == {
    case {"kind": "count", "value": amount}:
        print(amount);
}

-- unless .. else
-- If the Boolean expression evaluates to false, then the block of code inside the unless statement will be executed. If the Boolean expression evaluates to true, then the code after the else keyword of the given unless statement will be executed.
unless x > y {
    print(y);
} else {
    print(x);
}
```

7. Loops
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

8. Namespaces
- Namespaces can be nested. Nested functions are called through chained dot paths.
- `alias` binds a shorter name to a local or imported namespace path, and `::` calls through that alias.
- Alias targets can name nested namespaces or use an earlier alias. Selective and inferred imports resolve the full alias chain.
```lua
import "array";

alias arr = array;
alias arr_sort = arr.sort;

local List[Int] values = [1, 2, 3, 4];
print(arr::empty(values));
print(arr::len(values));
local List[Int] copied = arr::copy(values);
local List[Int] sorted = arr_sort::sort(values);

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

9. Traits
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

10. Workspaces and raw language mode
- Each standalone script or folder project is resolved as a separate workspace.
- Local imports are resolved inside the workspace before stdlib imports.
- Resolver caches imported files and parsed import lists to speed repeated checks.
- Successful CLI checks and runs also write a `.klang-cache` entry for the workspace. Repeating the same unchanged script or project can reuse the resolved and checked source set, while source edits, entry changes, or `--raw-lang` changes force a miss.
- Stdlib imports use selective function lookup by default; only functions called through the imported module namespace and their same-module helper dependencies are collected.
- Use `module_caller(call_entire_module : True);` in the importing source to load complete stdlib modules.
- Use `module(disabled : True);` in a module source to make imports of that module fail until the directive is removed or set to false.
- Use `--raw-lang` with the CLI to disable stdlib imports for a pure language run.
```sh
kLang run examples/helloworld
kLang run examples/helloworld first second
kLang check examples/helloworld --raw-lang
kLang doc '--sourcefile=["test.klang", "file.klang"]' --out=docs.html
kLang doc '--sourcefile=[examples/helloworld]' --out=helloworld-docs.html
```

11. Parsable metaprogramming
- `Parsable[T]` parses one Klang source string and exposes its AST, runtime argument channels, and workspace metadata.
- Adjacent generic constraint syntax such as `T Printable` is canonicalized to `T:Printable`.
- A `.keyword_macro` alias introduces a contextual bare-call keyword after its declaration.
```lua
trait Printable {
    function Render(value : String) : String;
}

impl Printable for String {
    function Render(value : String) : String {
        return value;
    }
}

let source = //
function Parsed() : Int { return 1; }
//;
let parsed = Parsable(source, ["source-argument"]);
print(len(parsable_ast(parsed)));
print(parsable_workspace(parsed));

let polling = parsable_begin_polling(parsed);
let response = parsable_poll_message(polling, {"kind": "REQUEST_AST", "payload": "Parsed"});
local List[T] ast = response["ast"] as List[T];
print(len(ast), response["intercepted"] as Bool);

alias printer = Parsable[T Printable].keyword_macro {
    print(get_args_from_parsable(), T);
}

printer "hallo";

alias answer = Parsable[T Any].keyword_macro {
    local Table context = macro_context();
    if context["arg_count"] as Int != 1 {
        return macro_expand("return 0;");
    }
    return macro_expand(//
local Int generated = 40 + 2;
return generated;
//);
}

print(answer("ignored"));

let changed = parsable_replace(parsed, "return 1", "return 2");
```
