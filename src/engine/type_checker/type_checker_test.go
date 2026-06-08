package typechecker

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestCheckProgramAcceptsTypedVariablesFunctionsAndCalls(t *testing.T) {
	program := programFromSource(`
global mut Int counter = 0;

function Add(left : Int, right : Int) : Int {
    local Int total = left + right;
    counter += 1;
    return total;
}

function Main() : Int {
    local Int value = Add(1, 2);
    print(value);
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsVariableTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String value = 10;
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign Int to local String value")
}

func TestCheckProgramRejectsReturnTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    return "wrong";
}
`)

	assertTypeError(t, CheckProgram(program), "returns Int but return expression is String")
}

func TestCheckProgramRejectsImmutableMutation(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    value = 2;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot mutate immutable variable")
}

func TestCheckProgramRejectsFunctionArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Main() : Int {
    return Add("nope", 1);
}
`)

	assertTypeError(t, CheckProgram(program), "argument 1 expects Int, got String")
}

func TestCheckProgramWarnsOnDeprecatedFunctionCall(t *testing.T) {
	program := programFromSource(`
@deprecated("use NewValue")
function OldValue() : Int {
    return 1;
}

function Main() : Int {
    return OldValue();
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected deprecated call to pass type check, got: %v", report.Errors)
	}
	assertTypeWarning(t, report, "function OldValue is deprecated: use NewValue")
}

func TestCheckProgramChecksNestedCallsThroughOperatorPrecedence(t *testing.T) {
	program := programFromSource(`
@deprecated("use NewFlag")
function OldValue() : Int {
    return 1;
}

function Main() : Int {
    return OldValue() + 2 * 3;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected precedence expression to pass type check, got: %v", report.Errors)
	}
	assertTypeWarning(t, report, "function OldValue is deprecated: use NewFlag")
}

func TestCheckProgramAcceptsMutableMapAndListIndexAssignments(t *testing.T) {
	if !isKnownType("Map[String,Int]") {
		t.Fatal("expected Map[String,Int] to be a known type")
	}
	typeName, name, ok := splitTypeAndName("Map[String, Int] rowResults")
	if !ok || typeName != "Map[String,Int]" || name != "rowResults" {
		t.Fatalf("expected split type and name to handle map types, got %q, %q, %v", typeName, name, ok)
	}
	decl, ok := parseVariableDeclaration(`local mut Map[String, Int] rowResults = {}`, "local")
	if !ok || decl.Type != "Map[String,Int]" || decl.Name != "rowResults" {
		t.Fatalf("expected map declaration to parse, got %#v, %v", decl, ok)
	}

	program := programFromSource(`
global mut Map[String, Int] memoryStore = {};

function Main() : Int {
    local mut List[Int] values = [];
    local mut Map[String, Int] rowResults = {};
    values[0] = 1;
    rowResults["sum"] = values[0];
    memoryStore["sum"] = rowResults["sum"];
    return memoryStore["sum"];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsStringListAndMapIndexing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String text = "hey";
    local Char first = text[0];
    local List[Int] values = [10, 20];
    local Int value = values[1];
    local mut Map[String, Int] totals = {};
    totals["two"] = 2;
    local Int mapValue = totals["two"];
    if first == 'h' {
        return value + mapValue;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected indexing type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidIndexing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String text = "hey";
    local Char first = text["bad"];
    return 0;
}
`)
	assertTypeError(t, CheckProgram(program), "String index must be Int, got String")

	program = programFromSource(`
function Main() : Int {
    local Int number = 1;
    local Int value = number[0];
    return value;
}
`)
	assertTypeError(t, CheckProgram(program), "Int is not indexable")

	program = programFromSource(`
function Main() : Int {
    local mut String text = "hey";
    text[0] = 'H';
    return 0;
}
`)
	assertTypeError(t, CheckProgram(program), "String indexes cannot be assigned")
}

func TestCheckProgramAcceptsBlockShadowing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int value = 1;
    if True {
        local mut Int value = 2;
        value += 1;
    }
    value += 1;
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsLocalLeakFromIfBlock(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    if True {
        local Int hidden = 1;
    }
    return hidden;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "hidden"`)
}

func TestCheckProgramRejectsLoopVariableLeak(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    for i := range(3) {
        print(i);
    }
    return i;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "i"`)
}

func TestCheckProgramAcceptsLoopHeaderScopeInsideLoop(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int total = 0;
    for i := range(3) {
        total += i;
    }
    while active := total < 10 {
        if active {
            total += 10;
        }
        break;
    }
    return total;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected loop header scope to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsDuplicateLocalInSameScope(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    local Int value = 2;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), `variable "value" is already defined in this scope`)
}

func TestCheckProgramAcceptsNestedGlobalAndExportedDeclarations(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected nested global/exported declarations to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsTypeCastsInExpressions(t *testing.T) {
	program := programFromSource(`
function Echo(value : String) : String {
    return value as String;
}

function Main() : Int {
    local Float f = 10 as Float;
    local Int i = f as Int;
    while active := i as Bool {
        return (Echo("42") as Int) + i;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected cast type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidTypeCast(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = [1] as Int;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot cast List[Int] to Int")
}

func TestCheckProgramAcceptsNestedFunctionAsFirstClassValue(t *testing.T) {
	program := programFromSource(`
function NumberFactory(multiplier : Int) : T {
    function InnerGenerator(val : Int) : Int {
        return val * multiplier;
    }
    return InnerGenerator;
}

global T timesTen = NumberFactory(10);
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsNamespaceLocalFunctionCalls(t *testing.T) {
	program := programFromSource(`
namespace random {
    function Random() : Int {
        return 1;
    }

    function RandomRange(min : Int, max : Int) : Int {
        local Int value = Random();
        return value + min + max;
    }
}

function Main() : Int {
    return call random.RandomRange(1, 2);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func programFromSource(source string) file.Program {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	return file.Program{
		Name:       "test",
		Root:       "tests",
		EntryPoint: "tests/test.klang",
		Files: []file.SourceFile{
			{
				Path:  "tests/test.klang",
				Lines: lines,
			},
		},
	}
}

func assertTypeError(t *testing.T, report Report, expected string) {
	t.Helper()

	for _, err := range report.Errors {
		if strings.Contains(err.Message, expected) {
			return
		}
	}

	t.Fatalf("expected type error containing %q, got %#v", expected, report.Errors)
}

func assertTypeWarning(t *testing.T, report Report, expected string) {
	t.Helper()

	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, expected) {
			return
		}
	}

	t.Fatalf("expected type warning containing %q, got %#v", expected, report.Warnings)
}
