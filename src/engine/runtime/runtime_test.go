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
