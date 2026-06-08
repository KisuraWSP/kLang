package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestParseFunctionWithTypedParametersAndReturn(t *testing.T) {
	program, errors := Parse(`
function Add(left : Int, right : Int) : Int {
    local Int total = left + right;
    return total;
}
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if fn.Name != "Add" || fn.ReturnType != "Int" {
		t.Fatalf("unexpected function signature: %#v", fn)
	}
	if len(fn.Params) != 2 || fn.Params[0].Name != "left" || fn.Params[0].Type != "Int" ||
		fn.Params[1].Name != "right" || fn.Params[1].Type != "Int" {
		t.Fatalf("unexpected function params: %#v", fn.Params)
	}
	if len(fn.Body) != 2 {
		t.Fatalf("expected 2 body statements, got %d", len(fn.Body))
	}
	if _, ok := fn.Body[0].(VariableStatement); !ok {
		t.Fatalf("expected first body statement to be variable declaration, got %T", fn.Body[0])
	}
	if _, ok := fn.Body[1].(ReturnStatement); !ok {
		t.Fatalf("expected second body statement to be return, got %T", fn.Body[1])
	}
}

func TestParseGlobalGenericVariableDeclaration(t *testing.T) {
	program, errors := Parse(`global mut Map[String, List[Int]] table = {};`)
	assertNoParseErrors(t, errors)

	decl, ok := program.Statements[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable statement, got %T", program.Statements[0])
	}
	if decl.Scope != "global" || !decl.Mutable || decl.Type != "Map[String,List[Int]]" || decl.Name != "table" {
		t.Fatalf("unexpected declaration: %#v", decl)
	}
	if decl.Expression.Literal() != "{ }" {
		t.Fatalf("unexpected declaration expression: %q", decl.Expression.Literal())
	}
}

func TestParseExportedVariableDeclaration(t *testing.T) {
	program, errors := Parse(`export local mut Int shared = 1;`)
	assertNoParseErrors(t, errors)

	decl, ok := program.Statements[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable statement, got %T", program.Statements[0])
	}
	if decl.Scope != "local" || !decl.Exported || !decl.Mutable || decl.Type != "Int" || decl.Name != "shared" {
		t.Fatalf("unexpected exported declaration: %#v", decl)
	}
}

func TestParseNamespaceImportAndCallExpression(t *testing.T) {
	program, errors := Parse(`
import "math.klang";

namespace random {
    function RandomRange(min : Int, max : Int) : Int {
        return min + max;
    }
}

call random.RandomRange(1, 2);
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 3 {
		t.Fatalf("expected 3 top-level statements, got %d", len(program.Statements))
	}
	if stmt, ok := program.Statements[0].(ImportStatement); !ok || stmt.Path != "math.klang" {
		t.Fatalf("unexpected import statement: %#v", program.Statements[0])
	}
	namespace, ok := program.Statements[1].(NamespaceStatement)
	if !ok || namespace.Name != "random" {
		t.Fatalf("unexpected namespace statement: %#v", program.Statements[1])
	}
	if len(namespace.Body) != 1 {
		t.Fatalf("expected namespace body to contain one statement, got %d", len(namespace.Body))
	}
	call, ok := program.Statements[2].(ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[2])
	}
	if call.Expression.Literal() != "call random . RandomRange ( 1 , 2 )" {
		t.Fatalf("unexpected call expression: %q", call.Expression.Literal())
	}
}

func TestParseConditionalsAndLoops(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local mut Int i = 0;
    while i < 10 {
        if i == 3 {
            break;
        } else {
            i += 1;
        }
    }
    return i;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	if len(fn.Body) != 3 {
		t.Fatalf("expected 3 function body statements, got %d", len(fn.Body))
	}
	loop, ok := fn.Body[1].(LoopStatement)
	if !ok || loop.Kind != "while" {
		t.Fatalf("expected while loop, got %#v", fn.Body[1])
	}
	if loop.Header.Literal() != "i < 10" {
		t.Fatalf("unexpected loop header: %q", loop.Header.Literal())
	}
	if len(loop.Body) != 1 {
		t.Fatalf("expected one loop body statement, got %d", len(loop.Body))
	}
	condition, ok := loop.Body[0].(IfStatement)
	if !ok || condition.Kind != "if" {
		t.Fatalf("expected if statement, got %#v", loop.Body[0])
	}
	if len(condition.Consequence) != 1 || len(condition.Alternative) != 1 {
		t.Fatalf("unexpected conditional branches: %#v", condition)
	}
}

func TestParseCompactConditionStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local mut Int i = 0;
    if i == 3 break;
    unless i > 0 return 1;
    return i;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	if len(fn.Body) != 4 {
		t.Fatalf("expected 4 function body statements, got %d", len(fn.Body))
	}

	firstCondition, ok := fn.Body[1].(IfStatement)
	if !ok || firstCondition.Kind != "if" {
		t.Fatalf("expected compact if statement, got %#v", fn.Body[1])
	}
	if firstCondition.Condition.Literal() != "i == 3" {
		t.Fatalf("unexpected compact if condition: %q", firstCondition.Condition.Literal())
	}
	if len(firstCondition.Consequence) != 1 {
		t.Fatalf("expected compact if consequence, got %#v", firstCondition.Consequence)
	}
	if _, ok := firstCondition.Consequence[0].(BreakStatement); !ok {
		t.Fatalf("expected compact if consequence to be break, got %T", firstCondition.Consequence[0])
	}

	secondCondition, ok := fn.Body[2].(IfStatement)
	if !ok || secondCondition.Kind != "unless" {
		t.Fatalf("expected compact unless statement, got %#v", fn.Body[2])
	}
	if secondCondition.Condition.Literal() != "i > 0" {
		t.Fatalf("unexpected compact unless condition: %q", secondCondition.Condition.Literal())
	}
	if len(secondCondition.Consequence) != 1 {
		t.Fatalf("expected compact unless consequence, got %#v", secondCondition.Consequence)
	}
	if _, ok := secondCondition.Consequence[0].(ReturnStatement); !ok {
		t.Fatalf("expected compact unless consequence to be return, got %T", secondCondition.Consequence[0])
	}
}

func TestParseExpressionTreeForBinaryPrecedence(t *testing.T) {
	program, errors := Parse(`local Int result = 1 + 2 * 3;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	root, ok := decl.Expression.Node.(BinaryExpression)
	if !ok || root.Operator != "+" {
		t.Fatalf("expected root + binary expression, got %#v", decl.Expression.Node)
	}
	if _, ok := root.Left.(LiteralExpression); !ok {
		t.Fatalf("expected left literal, got %#v", root.Left)
	}
	right, ok := root.Right.(BinaryExpression)
	if !ok || right.Operator != "*" {
		t.Fatalf("expected right * binary expression, got %#v", root.Right)
	}
}

func TestParseExpressionTreeForCallsSelectorsAndIndexes(t *testing.T) {
	program, errors := Parse(`local Int value = call random.RandomRange(items[0], 10);`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	callPrefix, ok := decl.Expression.Node.(UnaryExpression)
	if !ok || callPrefix.Operator != "call" {
		t.Fatalf("expected call unary expression, got %#v", decl.Expression.Node)
	}
	call, ok := callPrefix.Right.(CallExpression)
	if !ok {
		t.Fatalf("expected call expression, got %#v", callPrefix.Right)
	}
	selector, ok := call.Callee.(SelectorExpression)
	if !ok || selector.Field != "RandomRange" {
		t.Fatalf("expected selector callee, got %#v", call.Callee)
	}
	if len(call.Arguments) != 2 {
		t.Fatalf("expected two call args, got %d", len(call.Arguments))
	}
	if _, ok := call.Arguments[0].(IndexExpression); !ok {
		t.Fatalf("expected first arg index expression, got %#v", call.Arguments[0])
	}
}

func TestParseExpressionTreeForListAndMapLiterals(t *testing.T) {
	listProgram, listErrors := Parse(`local List[Int] values = [1, 2, 3];`)
	assertNoParseErrors(t, listErrors)
	listDecl := listProgram.Statements[0].(VariableStatement)
	list, ok := listDecl.Expression.Node.(ListExpression)
	if !ok || len(list.Items) != 3 {
		t.Fatalf("expected list expression with 3 items, got %#v", listDecl.Expression.Node)
	}

	mapProgram, mapErrors := Parse(`local Map[String, Int] values = {"one": 1, "two": 2};`)
	assertNoParseErrors(t, mapErrors)
	mapDecl := mapProgram.Statements[0].(VariableStatement)
	mapExpr, ok := mapDecl.Expression.Node.(MapExpression)
	if !ok || len(mapExpr.Entries) != 2 {
		t.Fatalf("expected map expression with 2 entries, got %#v", mapDecl.Expression.Node)
	}
}

func TestParseAssignmentExpressionTree(t *testing.T) {
	program, errors := Parse(`items[index + 1] = value * 2;`)
	assertNoParseErrors(t, errors)

	assignment, ok := program.Statements[0].(AssignmentStatement)
	if !ok {
		t.Fatalf("expected assignment statement, got %T", program.Statements[0])
	}
	if _, ok := assignment.Target.Node.(IndexExpression); !ok {
		t.Fatalf("expected indexed assignment target, got %#v", assignment.Target.Node)
	}
	value, ok := assignment.Expression.Node.(BinaryExpression)
	if !ok || value.Operator != "*" {
		t.Fatalf("expected binary assignment value, got %#v", assignment.Expression.Node)
	}
}

func TestParseRejectsIllegalTokens(t *testing.T) {
	_, errors := Parse(`local Int value = @;`)
	if len(errors) == 0 {
		t.Fatal("expected parse errors for illegal token")
	}
}

func TestParseRejectsMalformedGenericType(t *testing.T) {
	_, errors := Parse(`local Map[String, Int table = {};`)
	if len(errors) == 0 {
		t.Fatal("expected parse errors for malformed generic type")
	}
}

func TestParseFixturePrograms(t *testing.T) {
	programs, err := file.DiscoverPrograms(filepath.Join("..", "..", "tests"))
	if err != nil {
		t.Fatalf("failed to discover fixture programs: %v", err)
	}

	for _, program := range programs {
		for _, source := range program.Files {
			_, errors := Parse(strings.Join(source.Lines, "\n"))
			if len(errors) != 0 {
				t.Fatalf("%s parse errors: %#v", source.Path, errors)
			}
		}
	}
}

func TestParseLoadedProgramParsesEverySourceFile(t *testing.T) {
	loadedProgram, err := file.LoadProgram(filepath.Join("..", "..", "tests", "test21"))
	if err != nil {
		t.Fatalf("failed to load fixture program: %v", err)
	}

	parsed := ParseLoadedProgram(loadedProgram)
	if !parsed.Passed() {
		t.Fatalf("expected loaded program to parse, got %#v", parsed.Errors())
	}
	if parsed.Name != "test21" {
		t.Fatalf("expected parsed program name test21, got %q", parsed.Name)
	}
	if len(parsed.Sources) != 2 {
		t.Fatalf("expected test21 to parse two source files, got %d", len(parsed.Sources))
	}
}

func assertNoParseErrors(t *testing.T, errors []Error) {
	t.Helper()
	if len(errors) != 0 {
		t.Fatalf("expected no parse errors, got %#v", errors)
	}
}
