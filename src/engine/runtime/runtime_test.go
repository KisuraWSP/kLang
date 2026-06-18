package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

func TestRuntimeRunsMainFunction(t *testing.T) {
	result := runSource(t, `
function Add(a : Int, b : Int) : Int {
    return a + b;
}

function Main() : Int {
    return Add(20, 22);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected Main to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesMutableVariablesAndWhileLoop(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local mut Int total = 0;
    while total < 5 {
        total += 1;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected loop to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExposesCommandLineArgs(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
function Main() : Int {
    return len(Args) + len(Args[0]);
}
`)
	if len(errors) != 0 {
		t.Fatalf("parse failed: %#v", errors)
	}

	result, err := NewWithArgs([]string{"abc", "def"}).Run(parser.ParsedProgram{
		Name: "args",
		Sources: []parser.ParsedSource{
			{Path: "args.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected Args program to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExecutesCopyAndClone(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut List[Int] source = [1, 2];
    local List[Int] copied = copy source;
    local List[Int] cloned = clone source;
    source[0] = 10;
    return copied[0] + cloned[1] + source[0];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected copy and clone program to return 13, got %#v", result.Value)
	}
}

func TestRuntimeRejectsAssignmentAfterMove(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut String first = "hello";
    local String second = move first;
    first = "again";
    return len(second);
}
`)

	assertRuntimeErrorContains(t, err, `variable "first" was moved`)
}

func TestRuntimeRejectsIndexedAssignmentAfterMove(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [1];
    local List[Int] moved = move values;
    values[0] = 2;
    return moved[0];
}
`)

	assertRuntimeErrorContains(t, err, `variable "values" was moved`)
}

func TestRuntimeExecutesInferredVariablesConstAndSizeof(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    let maybe = Some(69420);
    let mut count = 1;
    const intSize = Int.sizeof;
    count += 2;
    return maybe.value + count + intSize;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 69431 {
		t.Fatalf("expected inferred variable program to return 69431, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLocalTypeInference(t *testing.T) {
	result := runParsedSource(t, `
function MakeName() : String {
    return "klang";
}

function Main() : Int {
    local count = 2;
    local mut values = [1, 2, 3];
    local name = MakeName();
    values[1] = count + len(name);
    return values[1];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected local type inference program to return 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesVariableDestructuring(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local [first, [second, third]] = [1, [2, 3]];
    local Table data = {"name": "klang", "count": 4};
    local {name, count: total} = data;
    return first + second + third + total + len(name);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected destructuring program to return 15, got %#v", result.Value)
	}
}

func TestRuntimeExecutesDiscardIdentifier(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return calls;
}

function Main() : Int {
    _ = Mark();
    _ = Mark();
    local _ = Mark();
    local [_, kept, _] = [10, 20, 30];
    return calls + kept;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 23 {
		t.Fatalf("expected discard identifier program to return 23, got %#v", result.Value)
	}
}

func TestRuntimeRejectsMissingIndexedCompoundAssignmentTargets(t *testing.T) {
	_, listErr := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [];
    values[0] += 1;
    return values[0];
}
`)
	assertRuntimeErrorContains(t, listErr, "compound assignment requires existing list index 0")

	_, mapErr := runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values["missing"] += 1;
    return 0;
}
`)
	assertRuntimeErrorContains(t, mapErr, `compound assignment requires existing map key "missing"`)
}

func TestRuntimeExecutesMultipleReturnsHereStringAndDefer(t *testing.T) {
	result := runParsedSource(t, `
function Pair() : (name : String, value : Int) {
    let html = //
<h1>Hello</h1>
//;
    defer print("done");
    return html, 7;
}

function Main() : Int {
    let pair = Pair();
    return len(pair[0]) + pair[1];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 21 {
		t.Fatalf("expected multiple return program to return 21, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "done" {
		t.Fatalf("expected deferred output, got %#v", result.Output)
	}
}

func TestRuntimeExecutesRunStatementsBeforeNormalStatementsAndMain(t *testing.T) {
	result := runSource(t, `
function Boot() {
    print("boot");
}

run {
    print("block");
}

print("normal");
run Boot();

function Main() : Int {
    print("main");
    return 7;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected return 7, got %#v", result.Value)
	}
	expectedOutput := []string{"block", "boot", "normal", "main"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsImmutableParameterMutation(t *testing.T) {
	_, err := runSourceWithError(`
function Mutate(value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    return Mutate(1);
}
`)

	assertRuntimeErrorContains(t, err, "cannot mutate immutable variable")
}

func TestRuntimeAllowsMutableParameterMutation(t *testing.T) {
	result := runParsedSource(t, `
function Mutate(mut value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    return Mutate(1);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected mutable parameter program to return 2, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRangeLoopAndBreak(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    for i := range(10) {
        if i == 4 break;
        total += i;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 6 {
		t.Fatalf("expected range loop to return 6, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPatternMatchWithDefaultBreak(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String mode = "blank";
    local mut Int score = 0;
    if mode == {
        case "blank":
            score += 10;
        case:
            score += 100;
    }
    return score;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected pattern match default break to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPatternMatchFallthroughWithContinue(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int value = 1;
    local mut Int score = 0;
    if value == {
        case 1:
            score += 10;
            continue;
        case 2:
            score += 20;
        case:
            score += 100;
    }
    return score;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 30 {
		t.Fatalf("expected pattern match fallthrough to return 30, got %#v", result.Value)
	}
}

func TestRuntimeExecutesCStyleForLoop(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    for i := 0; i < 5; i += 1 {
        total += i;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected C-style for loop to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesWhileHeaderScope(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    while active := total < 1 {
        if active {
            total += 1;
        }
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected while header scope to return 1, got %#v", result.Value)
	}
}

func TestRuntimeExportsNestedVariablesToGlobalScope(t *testing.T) {
	result := runParsedSource(t, `
function Configure() : Int {
    if True {
        global mut Int nestedGlobal = 10;
        export local Int exportedLocal = nestedGlobal + 2;
    }
    return exportedLocal;
}

function Main() : Int {
    Configure();
    nestedGlobal += 1;
    return exportedLocal + nestedGlobal;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 23 {
		t.Fatalf("expected exported nested variables to return 23, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTypeCasts(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Float f = 3 as Float;
    local Int i = f as Int;
    while active := i as Bool {
        return ("40" as Int) + (True as Int) + i;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 44 {
		t.Fatalf("expected cast program to return 44, got %#v", result.Value)
	}
}

func TestRuntimeExecutesNullSafetyOperator(t *testing.T) {
	result := runParsedSource(t, `
function MissingValue() : T {
}

function PresentValue() : T {
    return 7;
}

function Main() : Int {
    local Bool missing = MissingValue()?;
    local Bool present = PresentValue()?;
    if missing == False and present {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected null safety program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeExecutesConditionalExpressionsDefaultsAndZeroValues(t *testing.T) {
	result := runParsedSource(t, `
function Init() : Int {
    return 1;
}

function AddDefault(value : Int = 5, extra : Int = 2) : Int {
    return value + extra;
}

function Main() : Int {
    local Int zeroInt;
    local String zeroString;
    local Bool flag = if Init() > 0 then return False : True;
    local List[Int] values;
    local Option[Int] maybe;
    if not flag and zeroString == "" and not maybe {
        return zeroInt + AddDefault() + len(values);
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected defaults/zero/conditional program to return 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRestrictedGenericParameters(t *testing.T) {
	result := runParsedSource(t, `
function IdentityNumber[T restrict[UInt, Int, Float]](value : T = 3) : T {
    return value;
}

function Main() : Int {
    local Int first = IdentityNumber();
    local Float second = IdentityNumber(2.5);
    return first + second as Int;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected restricted generic program to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRestrictedGenericVariable(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut T restrict[UInt, Int, Float] value = 69420;
    value = 10;
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected restricted generic variable to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesInnerFunctionSelector(t *testing.T) {
	result := runSource(t, `
function Test() {
    inner function Eval() {
        print("This is called");
    }
}

function Main() : Int {
    Test().Eval();
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected Main to return 0, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != "This is called" {
		t.Fatalf("expected inner function output, got %v", result.Output)
	}
}

func TestRuntimeInnerFunctionCapturesOuterScope(t *testing.T) {
	result := runSource(t, `
function Counter(base : Int) {
    inner function Eval() : Int {
        return base + 1;
    }
}

function Main() : Int {
    return Counter(41).Eval();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected captured inner function to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesOptionAndResultBuiltins(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Option[Int] some = Some(10);
    local Option[Int] none = None();
    local Result[Int, String] ok = Ok(5);
    local Result[Int, String] err = Err("bad");
    local Result[Int, String] wrapped = Result(7);
    print(some);
    print(none);
    print(ok);
    print(err);
    print(wrapped);
    if some and not none and ok and not err and wrapped {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected option/result program to return 1, got %#v", result.Value)
	}
	expectedOutput := []string{"Some(10)", "None", "Ok(5)", "Err(bad)", "Ok(7)"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsOptionInnerTypeMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Option[Int] value = Some("bad");
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, `cannot assign Option to Option[Int] variable "value"`)
}

func TestRuntimeExecutesListComprehensions(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local List[Int] values = [1, 2, 3, 4];
    local List[Int] doubled = [value * 2 for value in values];
    local List[Int] evens = [value for value in values if value % 2 == 0];
    local List[Char] letters = [letter for letter in "hey"];
    local List[Int] indexes = [index for index in range(4)];
    print(doubled[2]);
    print(evens[1]);
    print(letters[0]);
    print(indexes[3]);
    return doubled[2] + evens[1] + indexes[3];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected list comprehension program to return 13, got %#v", result.Value)
	}
	expectedOutput := []string{"6", "4", "h", "3"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsInvalidListComprehensionIterable(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Bool flag = True;
    local List[Int] values = [value for value in flag];
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "list comprehension cannot iterate over Bool")
}

func TestRuntimeExecutesComplexAndSIMDValues(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Complex z = Complex(1, 2) + Complex(3, -1);
    local Complex product = z * Complex(2, 0);
    local SIMD[Int] lanes = SIMD([1, 2, 3, 4]);
    local SIMD[Int] moved = lanes + SIMD([4, 3, 2, 1]);
    local SIMD[Int] doubled = moved * 2;
    print(z);
    print(product);
    print(doubled);
    return len(doubled);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected complex/SIMD program to return 4, got %#v", result.Value)
	}
	expectedOutput := []string{"4+1i", "8+2i", "SIMD[10, 10, 10, 10]"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsSIMDLaneMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local SIMD[Int] left = SIMD([1, 2]);
    local SIMD[Int] right = SIMD([1]);
    local SIMD[Int] bad = left + right;
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "SIMD lane counts must match")
}

func TestRuntimeRejectsInvalidTypeCast(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return "abc" as Int;
}
`)
	assertRuntimeErrorContains(t, err, `cannot cast String "abc" to Int`)
}

func TestRuntimeExecutesListsMapsAndPrint(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local mut List[Int] values = [];
    local mut Map[String, Int] totals = {};
    values[0] = 7;
    totals["answer"] = values[0] * 6;
    print(totals["answer"]);
    return totals["answer"];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected map/list program to return 42, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "42" {
		t.Fatalf("expected print output 42, got %#v", result.Output)
	}
}

func TestRuntimePrintIsVariadic(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    print("count", 2, True);
    return 0;
}
`)

	if len(result.Output) != 1 || result.Output[0] != "count 2 True" {
		t.Fatalf("expected variadic print output, got %#v", result.Output)
	}
}

func TestRuntimeInputReadsLine(t *testing.T) {
	previousStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = previousStdin
		reader.Close()
	}()
	if _, err := writer.WriteString("Klang\n"); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}
	writer.Close()

	result := runSource(t, `
function Main() : Int {
    local String name = input("name: ");
    print(name);
    return len(name);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected input length 5, got %#v", result.Value)
	}
	if strings.Join(result.Output, "\n") != "name: \nKlang" {
		t.Fatalf("unexpected input output: %#v", result.Output)
	}
}

func TestRuntimeRejectsUseAfterMove(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local String first = "hello";
    local String second = move first;
    print(first);
    return len(second);
}
`)
	assertRuntimeErrorContains(t, err, `variable "first" was moved`)
}

func TestRuntimeExecutesStringIndexing(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String text = "hey";
    if text[0] == 'h' and text[1] == 'e' and len(text) == 3 {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected string indexing program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeRejectsImmutableMutation(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local Int value = 1;
    value = 2;
    return value;
}
`)
	if err == nil || !strings.Contains(err.Error(), "type check failed") {
		t.Fatalf("expected immutable mutation to fail before runtime, got %v", err)
	}
}

func TestRuntimeKeepsImmutableAggregateSnapshotsReferentiallyTransparent(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut List[Int] values = [1, 2];
    local List[Int] snapshot = values;
    values[0] = 9;

    local mut Map[String, Int] scores = {"answer": 42};
    local Map[String, Int] savedScores = scores;
    scores["answer"] = 7;

    return snapshot[0] + savedScores["answer"];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 43 {
		t.Fatalf("expected immutable snapshots to return 43, got %#v", result.Value)
	}
}

func TestRuntimeUsesCopyOnWriteForSharedListBindings(t *testing.T) {
	runtime := New()
	original := Value{Kind: ValueList, Data: []Value{IntValue(1), IntValue(2)}}
	if err := runtime.defineValue(runtime.global, "a", true, "List[Int]", original); err != nil {
		t.Fatalf("failed to define a: %v", err)
	}
	aBinding, _ := runtime.global.Get("a")
	if err := runtime.defineValue(runtime.global, "b", true, "List[Int]", aBinding.Value); err != nil {
		t.Fatalf("failed to define b: %v", err)
	}
	bBinding, _ := runtime.global.Get("b")

	aItems := aBinding.Value.Data.([]Value)
	bItems := bBinding.Value.Data.([]Value)
	if len(aItems) == 0 || len(bItems) == 0 || &aItems[0] != &bItems[0] {
		t.Fatalf("expected bindings to share backing storage before mutation")
	}

	err := runtime.assignIndex(parser.IndexExpression{
		Target: parser.IdentifierExpression{Name: "b"},
		Index:  parser.LiteralExpression{Kind: "Int", Value: "0"},
	}, "=", IntValue(9), runtime.global)
	if err != nil {
		t.Fatalf("indexed assignment failed: %v", err)
	}

	aItems = aBinding.Value.Data.([]Value)
	bItems = bBinding.Value.Data.([]Value)
	if aItems[0].Data.(int) != 1 || bItems[0].Data.(int) != 9 {
		t.Fatalf("expected mutation to detach b from a, got a=%#v b=%#v", aItems, bItems)
	}
	if &aItems[0] == &bItems[0] {
		t.Fatalf("expected bindings to stop sharing backing storage after mutation")
	}
}

func TestRuntimeRejectsRValueAssignmentTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut Int value = 1;
    (value + 1) = 3;
    return value;
}
`)
	assertRuntimeErrorContains(t, err, "assignment target must be an lvalue")
}

func TestRuntimeRejectsDivisionByZero(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return 10 / 0;
}
`)
	assertRuntimeErrorContains(t, err, "division by zero")
}

func TestRuntimeRejectsUnsupportedLenTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return len(True);
}
`)
	assertRuntimeErrorContains(t, err, "len does not support Bool")
}

func TestRuntimeRejectsInvalidIndexes(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local List[Int] values = [1];
    return values[2];
}
`)
	assertRuntimeErrorContains(t, err, "list index 2 is out of bounds")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local Map[String, Int] values = {"ok": 1};
    return values["missing"];
}
`)
	assertRuntimeErrorContains(t, err, `map key "missing" does not exist`)

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local String text = "hey";
    return text[3] as Int;
}
`)
	assertRuntimeErrorContains(t, err, "string index 3 is out of bounds")
}

func TestRuntimeRejectsStringIndexAssignment(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut String text = "hey";
    text[0] = 'H';
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "String is not index-assignable")
}

func TestRuntimeRejectsBreakOutsideLoop(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    break;
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "break is only allowed inside a loop")
}

func TestRuntimeRejectsDuplicateVariablesInSameScope(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Int value = 1;
    local Int value = 2;
    return value;
}
`)
	assertRuntimeErrorContains(t, err, `variable "value" is already defined`)
}

func TestRuntimeExecutesBlockShadowingWithoutLeaking(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int value = 1;
    if True {
        local Int value = 20;
        print(value);
    }
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected block shadowing to preserve outer value 1, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "20" {
		t.Fatalf("expected inner shadow output 20, got %#v", result.Output)
	}
}

func TestRuntimeRejectsReturnTypeMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return "bad";
}
`)
	assertRuntimeErrorContains(t, err, "function Main returns Int, got String")
}

func TestRuntimeRejectsMissingReturnValue(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Int value = 1;
}
`)
	assertRuntimeErrorContains(t, err, "function Main returns Int, got Null")
}

func TestRuntimeRejectsTypeChangingAssignment(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut Int value = 1;
    value = "bad";
    return value;
}
`)
	assertRuntimeErrorContains(t, err, `cannot assign String to Int variable "value"`)
}

func TestRuntimeRejectsTypedListAndMapMutationMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [];
    values[0] = "bad";
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot assign String to list element type Int")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values[1] = 10;
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot use Int as map key type String")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values["bad"] = "value";
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot assign String to map value type Int")
}

func TestRuntimeShortCircuitsLogicalOperators(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    if False and missingFunction() {
        return 1;
    }
    if True or missingFunction() {
        return 2;
    }
    return 3;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected short-circuit program to return 2, got %#v", result.Value)
	}
}

func TestRuntimeExecutesBooleanOperatorsInConditionsAndLoops(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    local Bool ready = True;
    local Bool active = True;
    local Bool failed = False;
    local Bool fallback = False;

    if ready and active xor failed or fallback {
        total += 1;
    }
    unless not ready or failed {
        total += 2;
    }
    while keepGoing := total == 3 and not failed {
        total += 3;
        break;
    }
    do_while firstPass := failed xor True {
        total += 4;
        break;
    }
    for i := 0; i < 3 and total < 11; i += 1 {
        total += 1;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 11 {
		t.Fatalf("expected boolean operator program to return 11, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPipeOperator(t *testing.T) {
	result := runParsedSource(t, `
function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Double(value : Int) : Int {
    return value * 2;
}

function Main() : Int {
    return 2 |> Add(3) |> Double;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected pipe operator program to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesFunctionCallbacks(t *testing.T) {
	result := runSource(t, `
function Double(value : Int) : Int {
    return value * 2;
}

function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Combine(left : Int, right : Int, callback : Function[Int, Int, Int]) : Int {
    return callback(left, right);
}

function Main() : Int {
    local Function[Int, Int] callback = Double;
    return Apply(5, callback) + Combine(2, 3, Add);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected callback program to return 15, got %#v", result.Value)
	}
}

func TestRuntimeExecutesFirstClassFunctionClosures(t *testing.T) {
	result := runSource(t, `
function NumberFactory(multiplier : Int) : Function[Int, Int] {
    function InnerGenerator(value : Int) : Int {
        return value * multiplier;
    }
    return InnerGenerator;
}

function Main() : Int {
    local Function[Int, Int] timesTen = NumberFactory(10);
    return timesTen(42) + NumberFactory(5)(10);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 470 {
		t.Fatalf("expected first-class function program to return 470, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLazyFunctionArgumentsOnDemand(t *testing.T) {
	result := runSource(t, `
function Boom() : Int {
    return 1 / 0;
}

lazy function Choose(useFirst : Bool, first : Int, second : Int) : Int {
    if useFirst {
        return first;
    }
    return second;
}

function Main() : Int {
    return Choose(True, 42, Boom());
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected lazy function to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLazyVariableInitializationOnDemand(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return 40 + calls;
}

function Main() : Int {
    lazy local Int value = Mark();
    local Int before = calls;
    local Int first = value;
    local Int second = value;
    return before * 100 + first + second + calls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 83 {
		t.Fatalf("expected lazy variable program to return 83, got %#v", result.Value)
	}
}

func TestRuntimeDoesNotEvaluateUnusedLazyVariable(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return calls;
}

function Main() : Int {
    lazy let value = Mark();
    return calls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected unused lazy variable program to return 0, got %#v", result.Value)
	}
}

func TestRuntimeMovesLazyVariableInitializerWhenForced(t *testing.T) {
	result := runParsedSource(t, `
function Main() : String {
    local String value = "ready";
    lazy local String moved = move value;
    local String before = value;
    return before + moved;
}
`)

	if result.Value.Kind != ValueString || result.Value.Data.(string) != "readyready" {
		t.Fatalf("expected lazy move program to return readyready, got %#v", result.Value)
	}
}

func TestRuntimeTailCallOptimizesSelfRecursion(t *testing.T) {
	runtime := New()
	runtime.maxDepth = 8

	parsedProgram, errors := parser.Parse(`
function CountDown(value : Int, total : Int) : Int {
    if value == 0 {
        return total;
    }
    return CountDown(value - 1, total + 1);
}

function Main() : Int {
    return CountDown(128, 0);
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := runtime.Run(parser.ParsedProgram{
		Name: "tailcall",
		Sources: []parser.ParsedSource{
			{Path: "tailcall.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("expected tail-call optimized program to pass, got: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 128 {
		t.Fatalf("expected tail-call program to return 128, got %#v", result.Value)
	}
}

func TestRuntimeComparesCharsAndStrings(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    if '7' >= '0' and '7' <= '9' {
        if "beta" > "alpha" {
            return 1;
        }
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected char/string comparison program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeAppliesOperatorPrecedenceEverywhere(t *testing.T) {
	result := runParsedSource(t, `
function Twice(value : Int) : Int {
    return value * 2 + 1;
}

function Main() : Int {
    local mut Int total = 1 + 2 * 3;
    local Int powered = -2 ** 3 ** 2;
    local Int grouped = (1 + 2) * 3;
    total += Twice(2 + 3 * 4) // 5;
    while active := total > 10 and grouped == 9 or False {
        return total + grouped + powered;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != -491 {
		t.Fatalf("expected precedence program to return -491, got %#v", result.Value)
	}
}

func TestRuntimeRejectsDuplicateAndAmbiguousFunctions(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return 1;
}

function Main() : Int {
    return 2;
}
`)
	assertRuntimeErrorContains(t, err, `function "Main" is already defined`)

	_, err = runParsedSourceWithError(`
namespace A {
    function Pick() : Int { return 1; }
}

namespace B {
    function Pick() : Int { return 2; }
}

function Main() : Int {
    return Pick();
}
`)
	assertRuntimeErrorContains(t, err, `ambiguous function "Pick"`)
}

func TestRuntimeExecutesChainedNamespaceAndAliasCalls(t *testing.T) {
	result, err := runSourceWithError(`
namespace std {
    namespace lib {
        function LuaInit() {
            print("std.lib.LuaInit(); is called");
        }

        function Number() : Int {
            return 7;
        }
    }
}

alias std_lib = std.lib;

function Main() : Int {
    std.lib.LuaInit();
    std_lib::LuaInit();
    return std.lib.Number() + std_lib::Number();
}
`)
	if err != nil {
		t.Fatalf("expected runtime to pass, got: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 14 {
		t.Fatalf("expected namespace program to return 14, got %#v", result.Value)
	}
	expectedOutput := []string{
		"std.lib.LuaInit(); is called",
		"std.lib.LuaInit(); is called",
	}
	if strings.Join(result.Output, "\n") != strings.Join(expectedOutput, "\n") {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
}

func TestRuntimeRejectsRunawayRecursion(t *testing.T) {
	runtime := New()
	runtime.maxDepth = 8

	parsedProgram, errors := parser.Parse(`
function Loop() : Int {
    return Loop();
}

function Main() : Int {
    return Loop();
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	_, err := runtime.Run(parser.ParsedProgram{
		Name: "recursion",
		Sources: []parser.ParsedSource{
			{Path: "recursion.klang", Program: parsedProgram},
		},
	})
	assertRuntimeErrorContains(t, err, "maximum call depth 8 exceeded")
}

func TestRuntimeBorrowCheckerRejectsConflictingMutableBorrow(t *testing.T) {
	memory := NewMemory()
	objectID := memory.Allocate(IntValue(10), MemoryStack)

	if err := memory.BorrowImmutable(objectID); err != nil {
		t.Fatalf("unexpected immutable borrow error: %v", err)
	}
	if err := memory.BorrowMutable(objectID); err == nil {
		t.Fatal("expected mutable borrow to fail while immutable borrow is active")
	}
	memory.ReleaseImmutable(objectID)
	if err := memory.BorrowMutable(objectID); err != nil {
		t.Fatalf("expected mutable borrow after release to pass, got %v", err)
	}
}

func TestRuntimeTracksStackAndHeapMemory(t *testing.T) {
	result := runParsedSource(t, `
global Int shared = 10;

function Main() : Int {
    local Int localValue = shared + 1;
    return localValue;
}
`)

	if result.Memory.HeapObjects == 0 {
		t.Fatalf("expected heap allocations for global values, got %#v", result.Memory)
	}
	if result.Memory.StackObjects == 0 {
		t.Fatalf("expected stack allocations for local values, got %#v", result.Memory)
	}
	if result.Memory.TotalObjects != result.Memory.StackObjects+result.Memory.HeapObjects {
		t.Fatalf("unexpected memory totals: %#v", result.Memory)
	}
}

func TestRuntimeCanRunParsedProgramDirectly(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
function Main() : Int {
    return 9 * 9;
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := New().Run(parser.ParsedProgram{
		Name: "direct",
		Sources: []parser.ParsedSource{
			{Path: "direct.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 81 {
		t.Fatalf("expected direct parsed program to return 81, got %#v", result.Value)
	}
}

func TestRuntimeUsesEntryPointDirective(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
namespace App {
    #set_entry_point_to_here
    function Process() : Int {
        return 7;
    }
}

function Main() : Int {
    return 0;
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := New().Run(parser.ParsedProgram{
		Name:       "entry",
		EntryPoint: "App.Process",
		Sources: []parser.ParsedSource{
			{Path: "entry.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected entry point to return 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAtomicBuiltins(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Atomic[Int] counter = Atomic(1);
    atomic_add(counter, 2);
    atomic_store(counter, atomic_load(counter) + 1);
    return atomic_load(counter);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected atomic program to return 4, got %#v", result.Value)
	}
}

func TestRuntimeExecutesWorkspaceBuildAndDebuggerAPIs(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Program program = Program(["app", "mathg"]);
    local BuildSystem build = BuildSystem("demo", 2, ["first.klang", "app.klang"], "Standalone");
    local WorkSpace workspace = WorkSpace(program, build);
    local List[String] files = workspace_files(workspace);
    debug(workspace_manifest(workspace));
    breakpoint("workspace-ready");
    return len(workspace_backend(workspace)) + len(files) + len(debug_stack());
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected workspace program to return 13, got %#v", result.Value)
	}
	if len(result.Output) != 2 || !strings.Contains(result.Output[0], "[debug]") || !strings.Contains(result.Output[1], "[breakpoint]") {
		t.Fatalf("expected debug and breakpoint output, got %#v", result.Output)
	}
}

func TestRuntimeExecutesStdlibArrayModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "array";

function Main() : Int {
    local List[Int] values = [1, 2, 3];
    local List[Int] pushed = array.push(values, 4);
    local List[Int] inserted = array.insert(pushed, 1, 9);
    local List[Int] updated = array.set(inserted, 0, 8);
    local List[Int] sliced = array.slice(updated, 1, 4);
    local List[Int] removed = array.remove(sliced, 2);
    local List[Int] reversed = array.reverse(removed);
    local Option[Int] front = array.front(reversed);
    local Option[Int] missing = array.fetch(reversed, 20);
    if not front.some or missing.some {
        return 0;
    }
    return array.len(reversed) + array.get(reversed, 0) + array.index_of(reversed, 9) + array.count(updated, 9) + array.capacity(values);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected stdlib array program to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibDSAModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "dsa";

function Main() : Int {
    local mut T stack = dsa.StackEmpty();
    stack = dsa.StackPush(stack, 3);
    stack = dsa.StackPush(stack, 5);
    local Option[Int] top = dsa.StackPeek(stack);
    local Table popped = dsa.StackPop(stack);
    local T nextStack = popped.stack;
    local Option[Int] poppedValue = popped.value as Option[Int];

    local mut T queue = dsa.QueueEmpty();
    queue = dsa.QueuePush(queue, 7);
    queue = dsa.QueuePush(queue, 11);
    local Table shifted = dsa.QueuePop(queue);
    local T nextQueue = shifted.queue;
    local Option[Int] shiftedValue = shifted.value as Option[Int];

    local mut T ordered = dsa.OrderedMapEmpty();
    ordered = dsa.OrderedMapPut(ordered, "a", 10);
    ordered = dsa.OrderedMapPut(ordered, "b", 20);
    ordered = dsa.OrderedMapPut(ordered, "a", 12);
    local Option[T] found = dsa.OrderedMapGet(ordered, "a");
    local T removed = dsa.OrderedMapRemove(ordered, "b");
    local mut T compat = dsa.arrayhashmap.New(["x"], [4]);
    compat = dsa.arrayhashmap.Put(compat, "y", 6);
    local Option[T] compatFound = dsa.arrayhashmap.Get(compat, "y");

    if top.some {
        if poppedValue.some {
            if shiftedValue.some {
                if found.some {
                    if compatFound.some {
                        local Int foundValue = found.value as Int;
                        local Int compatValue = compatFound.value as Int;
                        return top.value + poppedValue.value + dsa.StackCount(nextStack) +
                            shiftedValue.value + dsa.QueueCount(nextQueue) +
                            foundValue + dsa.OrderedMapCount(removed) +
                            compatValue + dsa.OrderedMapCount(compat);
                    }
                }
            }
        }
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 40 {
		t.Fatalf("expected stdlib dsa program to return 40, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibMetasystemASTHelpers(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "metasystem";

function Main() : Int {
    local Program program = Program(["app", "mathg"]);
    local Table programAst = metasystem.build.GetASTFromSourceCode(Some(program));
    local Table defaultProgramAst = metasystem.build.GetASTFromSourceCode(None());
    local WorkSpace workspace = metasystem.workspace.UserDefinedWorkspace("demo", ["app"], ["first.klang", "app.klang"], "Standalone");
    local Table projectAst = metasystem.build.GetAstFromEntireProject(Some(workspace));
    local Table fallbackProjectAst = metasystem.build.GetAstFromEntireProject(None());

    local Bool programAvailable = programAst["available"] as Bool;
    local Bool defaultProgramAvailable = defaultProgramAst["available"] as Bool;
    local Int projectFileCount = projectAst["file_count"] as Int;
    local Int fallbackFileCount = fallbackProjectAst["file_count"] as Int;
    if not programAvailable or defaultProgramAvailable {
        return 0;
    }
    if projectFileCount != 2 or fallbackFileCount != 1 {
        return 0;
    }

    local List[String] programNodes = programAst["nodes"] as List[String];
    local List[String] projectNodes = projectAst["nodes"] as List[String];
    return len(programNodes) + len(projectNodes) + projectFileCount + fallbackFileCount;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 9 {
		t.Fatalf("expected stdlib metasystem AST helpers to return 9, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibRuntimeDebugHelpers(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "runtime";

function Main() : Int {
    local Int line = runtime.debug.__LINE__();
    local Table pos = runtime.debug.__POS__();
    local Table lineOf = runtime.debug.__LINE_OF__(5);
    local String fn = runtime.debug.__FUNCTION__();
    local String module = runtime.debug.__MODULE__();
    local String file = runtime.debug.__FILE__();
    local Int wrapperLine = runtime.debug.Line();

    if line <= 0 {
        return 0;
    }
    if wrapperLine <= 0 {
        return 0;
    }
    if (pos["line"] as Int) <= 0 {
        return 0;
    }
    if (lineOf["line"] as Int) <= 0 {
        return 0;
    }
    if (lineOf["value"] as Int) != 5 {
        return 0;
    }
    return len(fn) + len(module) + len(file) + (lineOf["value"] as Int);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 29 {
		t.Fatalf("expected stdlib runtime debug helpers to return 29, got %#v", result.Value)
	}
}

func TestRuntimeAutoLoadsStdlibGlobalNamespaceFunctions(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	appRoot := filepath.Join(root, "app")
	if err := os.MkdirAll(stdlibRoot, 0o755); err != nil {
		t.Fatalf("create stdlib dir failed: %v", err)
	}
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatalf("create app dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stdlibRoot, "alloc.klang"), []byte(`
global namespace alloc {
    function New() : Int { return 7; }
    function Add(left : Int, right : Int) : Int { return left + right; }
}
namespace hidden {
    function Secret() : Int { return 99; }
}
`), 0o644); err != nil {
		t.Fatalf("write stdlib fixture failed: %v", err)
	}
	programPath := filepath.Join(appRoot, "main.klang")
	if err := os.WriteFile(programPath, []byte(`
function Main() : Int {
    return Add(New(), 5);
}
`), 0o644); err != nil {
		t.Fatalf("write app fixture failed: %v", err)
	}
	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("load program failed: %v", err)
	}
	resolved, moduleReport := modulesystem.NewResolver(stdlibRoot).ResolveProgram(program)
	if !moduleReport.Passed() {
		t.Fatalf("module resolution failed: %#v", moduleReport.Errors)
	}
	typeReport := typechecker.CheckProgram(resolved)
	if !typeReport.Passed() {
		t.Fatalf("type check failed: %#v", typeReport.Errors)
	}
	parsed := parser.ParseLoadedProgram(resolved)
	if !parsed.Passed() {
		t.Fatalf("parse failed: %#v", parsed.Errors())
	}
	result, err := New().Run(parsed)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 12 {
		t.Fatalf("expected global namespace function program to return 12, got %#v", result.Value)
	}
}

func TestRuntimeLoadsJSFilesystemModuleDescriptor(t *testing.T) {
	jsPath := filepath.Join(t.TempDir(), "library.js")
	if err := os.WriteFile(jsPath, []byte("export function init() {}\nexport const version = 1;\n"), 0644); err != nil {
		t.Fatalf("write js fixture failed: %v", err)
	}
	source := strings.ReplaceAll(jsPath, `\`, `\\`)
	result := runParsedSource(t, `
function Main() : Int {
    local JSModule module = js_import("`+source+`");
    local List[String] exports = js_exports(module);
    local String body = js_source(module);
    local JSCall descriptor = js_call(module, "init", [body]);
    return len(exports);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected two js exports, got %#v", result.Value)
	}
}

func TestRuntimeExecutesSpawnJoinWithSharedAtomic(t *testing.T) {
	result := runParsedSource(t, `
function Worker(counter : Atomic[Int], mut amount : Int) : Int {
    while amount > 0 {
        atomic_add(counter, 1);
        amount -= 1;
    }
    return atomic_load(counter);
}

function Main() : Int {
    local Atomic[Int] counter = Atomic(0);
    local Thread[Int] left = spawn(Worker, [counter, 25]);
    local Thread[Int] right = spawn(Worker, [counter, 17]);
    local String status = thread_status(left);
    join(left);
    join(right);
    return atomic_load(counter) + len(status);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) < 46 || result.Value.Data.(int) > 49 {
		t.Fatalf("expected atomic threaded program to include 42 plus status length, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLambdaFunction(t *testing.T) {
	result := runSource(t, `
function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Main() : Int {
    local Int offset = 1;
    return Apply(41, fun(value : Int) : Int {
        return value + offset;
    });
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected lambda program to return 42, got %#v", result.Value)
	}
}

func TestRuntimeDispatchesFunctionGroup(t *testing.T) {
	result := runSource(t, `
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

function Main() : Int {
    local String y = "1";
    local mut T x = if Poly(1) == Poly(1, y) then return y : "no";
    return len(x);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected polymorphic program to return 1, got %#v", result.Value)
	}
	expectedOutput := []string{"1", "1 1"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeCatchesThrownValues(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    try {
        throw "bad";
    } catch err {
        print(err);
        return 7;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected caught throw to return 7, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != "bad" {
		t.Fatalf("expected caught error output, got %v", result.Output)
	}
}

func TestRuntimePropagatesResultErrorsWithBang(t *testing.T) {
	result := runSource(t, `
function Fallible() : Result[Int, String] {
    return Err("nope");
}

function Main() : Int {
    try {
        local Int value = Fallible()!;
        return value;
    } catch err {
        print(err);
        return 42;
    }
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected propagated error to return 42, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != "nope" {
		t.Fatalf("expected propagated error output, got %v", result.Output)
	}
}

func TestRuntimeReportsUncaughtException(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    throw "boom";
}
`)
	assertRuntimeErrorContains(t, err, "uncaught exception: boom")
}

func TestRuntimeExecutesAliasFunctionExtensionMethod(t *testing.T) {
	result := runParsedSource(t, `
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) -> type
    #extend do
        function get_length() -> int
            return this.length;
        end
    end
end

function Main() : Int {
    local T list = ArrayList("value", 3, 10);
    return list.get_length();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected extension method to return 3, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAliasFunctionStructBody(t *testing.T) {
	result := runParsedSource(t, `
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) : type = struct {
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

function Main() : Int {
    local T list = ArrayList("value", 3, 10);
    return list.get_length() + list.with_extra(4) + list.__hooks + list.__methods + list.__traits + list.__impls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 17 {
		t.Fatalf("expected struct alias runtime metadata and methods to return 17, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAliasExtensionMethodArguments(t *testing.T) {
	result := runParsedSource(t, `
alias function Counter(value: int) -> type
    #extend do
        function add(amount : Int) -> int
            return this.value + amount;
        end
    end
end

function Main() : Int {
    local T counter = Counter(2);
    return counter.add(3);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected extension method argument call to return 5, got %#v", result.Value)
	}
}

func TestRuntimeRejectsAliasExtensionMethodArgumentMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
alias function Counter(value: int) -> type
    #extend do
        function add(amount : Int) -> int
            return this.value + amount;
        end
    end
end

function Main() : Int {
    local T counter = Counter(2);
    return counter.add("bad");
}
`)

	assertRuntimeErrorContains(t, err, "method Counter.add argument 1 expects Int")
}

func TestRuntimeExecutesRegionArraySyntax(t *testing.T) {
	result := runParsedSource(t, `
region MyRegion(T, sizeof(T) * 100, 10);

function Main() : Int {
    local mut T[MyRegion] myArray;
    myArray[0] = "String";
    return len(myArray);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected region array length 1, got %#v", result.Value)
	}
}

func TestRuntimeRejectsRegionArrayCapacityOverflow(t *testing.T) {
	_, err := runParsedSourceWithError(`
region Tiny(T, 1, 1);

function Main() : Int {
    local mut T[Tiny] myArray;
    myArray[1] = "too far";
    return len(myArray);
}
`)

	assertRuntimeErrorContains(t, err, "array index 1 exceeds region Tiny capacity 1")
}

func TestRuntimeExecutesAllocatorConstructors(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local T boxed = Box("value");
    local T arena = ArenaAllocator();
    return len(boxed.kind) + len(arena.kind);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 17 {
		t.Fatalf("expected allocator kind lengths to total 17, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTableAsyncIteratorAndCoroutineBuiltins(t *testing.T) {
	result := runParsedSource(t, `
async function LoadValue() : Int {
    return 40;
}

function BuildValue() : Int {
    return 2;
}

function Main() : Int {
    local mut Table data = {"name": "klang", 1: 5};
    data["extra"] = 7;
    local Iterator[T] iterator = iter([1, 2, 3]);
    local Option[T] first = next(iterator);
    local Option[T] second = next(iterator);
    local Coroutine[Int] co = coroutine(BuildValue);
    local Option[Int] resumed = resume(co);
    return await LoadValue() + data.extra + len(data.name) + first.value + second.value + resumed.value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 57 {
		t.Fatalf("expected table/async/iterator/coroutine program to return 57, got %#v", result.Value)
	}
}

func TestRuntimeRejectsAwaitOnNonAwaitable(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return await 1;
}
`)

	assertRuntimeErrorContains(t, err, "await expects Awaitable")
}

func TestRuntimeAllocatesAliasAndAllocatorObjectsOnHeap(t *testing.T) {
	result := runParsedSource(t, `
alias function Boxed(value: int) -> type
end

function Main() : Int {
    local T boxed = Box(1);
    local T custom = Boxed(2);
    return boxed.value + custom.value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected object values to return 3, got %#v", result.Value)
	}
	if result.Memory.HeapObjects < 2 {
		t.Fatalf("expected allocator/custom objects on heap, got %#v", result.Memory)
	}
}

func TestRuntimeSupportsBuiltinProtocolMembers(t *testing.T) {
	result := runParsedSource(t, `
function Remember(index : Int) : Int {
    return index + 10;
}

function Main() : Int {
    local String upper = "hallo".uppercase();
    local String lower = upper.lowercase();
    local Char letter = 'k'.uppercase();
    local Int last = 3.times(Remember);
    if letter != 'K' {
        return 0;
    }
    return upper.count + lower.count + [1, 2, 3].count + last;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 25 {
		t.Fatalf("expected builtin protocol members to return 25, got %#v", result.Value)
	}
}

func TestRuntimeSupportsEnumIotaStyleValues(t *testing.T) {
	result := runParsedSource(t, `
enum Status {
    Pending;
    Running = 10;
    Done;
}

function Main() : Int {
    local Status status = Status.Done;
    if status == {
        case Status.Pending:
            return 1;
        case Status.Running:
            return 2;
        case Status.Done:
            return status.ordinal + len(status.name);
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected enum program to return 15, got %#v", result.Value)
	}
}

func runSource(t *testing.T, source string) Result {
	t.Helper()

	result, err := runSourceWithError(source)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	return result
}

func runParsedSource(t *testing.T, source string) Result {
	t.Helper()

	result, err := runParsedSourceWithError(source)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	return result
}

func runParsedSourceWithError(source string) (Result, error) {
	parsedProgram, errors := parser.Parse(source)
	if len(errors) != 0 {
		return Result{}, Error{Message: "parse failed"}
	}

	return New().Run(parser.ParsedProgram{
		Name: "parsed",
		Sources: []parser.ParsedSource{
			{Path: "parsed.klang", Program: parsedProgram},
		},
	})
}

func runSourceWithError(source string) (Result, error) {
	program := file.Program{
		Name:       "test",
		Root:       "tests",
		EntryPoint: "tests/test.klang",
		Files: []file.SourceFile{
			{
				Path:  "tests/test.klang",
				Lines: strings.Split(strings.TrimSpace(source), "\n"),
			},
		},
	}
	return RunProgram(program)
}

func assertRuntimeErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected runtime error containing %q, got nil", expected)
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected runtime error containing %q, got %v", expected, err)
	}
}
