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

func TestParseRejectsIllegalTokens(t *testing.T) {
	_, errors := Parse(`local Int value = @;`)
	if len(errors) == 0 {
		t.Fatal("expected parse errors for illegal token")
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

func assertNoParseErrors(t *testing.T, errors []Error) {
	t.Helper()
	if len(errors) != 0 {
		t.Fatalf("expected no parse errors, got %#v", errors)
	}
}
