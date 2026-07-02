package context

import (
	"errors"
	"strings"
	"testing"

	"kLang/src/diagnostic"
	"kLang/src/engine/file"
	typechecker "kLang/src/engine/type_checker"
)

func TestContextAttachesSourceLine(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path:  "main.klang",
			Lines: []string{"function Main() : Int {", "    return \"bad\";", "}"},
		}},
	}

	ctx := New(program)
	err := ctx.WithSource(ErrorContext{Phase: PhaseType, File: "main.klang", Line: 2, Message: "bad return"})

	if err.SourceLine != `    return "bad";` {
		t.Fatalf("expected source line to be attached, got %#v", err)
	}
	if err.Column != 1 {
		t.Fatalf("expected default column 1, got %d", err.Column)
	}
}

func TestTypeErrorsDoNotInferMetadataFromMessageText(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path:  "main.klang",
			Lines: []string{"local Int value = \"bad\";"},
		}},
	}
	report := typechecker.Report{Errors: []typechecker.Error{{
		File:    "main.klang",
		Line:    1,
		Message: "cannot assign String to local Int value",
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 || errors[0].Code != diagnostic.CodeStaticSemantics ||
		errors[0].Rule != "static semantics" || errors[0].Column != 1 {
		t.Fatalf("expected generic producer fallback without parsing prose, got %#v", errors)
	}
}

func TestTypeErrorsAddExpectedFoundTypeTree(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path:  "main.klang",
			Lines: []string{`local List[Int] values = ["bad"];`},
		}},
	}
	report := typechecker.Report{Errors: []typechecker.Error{{
		Code:         diagnostic.CodeTypeMismatch,
		File:         "main.klang",
		Line:         1,
		Column:       27,
		EndLine:      1,
		EndColumn:    34,
		Message:      "cannot assign List[String] to local List[Int] values",
		Rule:         "type compatibility",
		Hint:         "Use a List[Int] value.",
		ExpectedType: "List[Int]",
		FoundType:    "List[String]",
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 || !strings.Contains(errors[0].Message, "Expected type:") ||
		!strings.Contains(errors[0].Message, "  List\n    Int") ||
		!strings.Contains(errors[0].Message, "Found type:") ||
		!strings.Contains(errors[0].Message, "  List\n    String") {
		t.Fatalf("expected type tree message, got %#v", errors)
	}
}

func TestRuntimeErrorDoesNotParseLocationFromMessageText(t *testing.T) {
	program := file.Program{Name: "demo", EntryPoint: "main.klang"}

	err := RuntimeError(program, errors.New("runtime failed: line 3:9: cannot index value"))

	if err.Phase != PhaseRuntime || err.Line != 0 || err.Column != 1 ||
		err.Message != "runtime failed: line 3:9: cannot index value" {
		t.Fatalf("unexpected runtime context: %#v", err)
	}
}

func TestTypeErrorsPreserveStructuredDiagnosticData(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path:  "main.klang",
			Lines: []string{`local Int value = "bad";`},
		}},
	}
	report := typechecker.Report{Errors: []typechecker.Error{{
		Code:         diagnostic.CodeTypeMismatch,
		File:         "main.klang",
		Line:         1,
		Column:       7,
		EndColumn:    9,
		Message:      "cannot assign String to Int",
		Rule:         "type compatibility",
		Hint:         "Use an Int value.",
		ExpectedType: "Int",
		FoundType:    "String",
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 {
		t.Fatalf("expected one error, got %#v", errors)
	}
	err := errors[0]
	if err.Code != diagnostic.CodeTypeMismatch || err.Primary.StartColumn != 7 ||
		err.ExpectedType != "Int" || err.FoundType != "String" {
		t.Fatalf("structured diagnostic data was not preserved: %#v", err)
	}
}
