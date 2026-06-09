package runtime

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
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
function IdentityNumber(value : T:Int|Float = 3) : T {
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
	objectID := memory.Allocate(IntValue(10))

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
