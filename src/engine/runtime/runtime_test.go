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

	parsedProgram, errors := parser.Parse(source)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := New().Run(parser.ParsedProgram{
		Name: "parsed",
		Sources: []parser.ParsedSource{
			{Path: "parsed.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	return result
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
