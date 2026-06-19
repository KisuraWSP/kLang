package context

import (
	"errors"
	"strings"
	"testing"

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

func TestTypeErrorsAddHumanContext(t *testing.T) {
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
	if len(errors) != 1 || !strings.Contains(errors[0].Message, "This value does not have the type declared") {
		t.Fatalf("expected human type context, got %#v", errors)
	}
}

func TestTypeErrorsAddDidYouMeanAndSourceSpan(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path: "main.klang",
			Lines: []string{
				"function Main() : Int {",
				"    local Int count = 1;",
				"    return cout;",
				"}",
			},
		}},
	}
	report := typechecker.Report{Errors: []typechecker.Error{{
		File:    "main.klang",
		Line:    3,
		Message: `unknown identifier "cout"`,
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 {
		t.Fatalf("expected one error, got %#v", errors)
	}
	if errors[0].Column != 12 || errors[0].EndColumn != 15 {
		t.Fatalf("expected span over cout, got %#v", errors[0])
	}
	if !strings.Contains(errors[0].Hint, `Did you mean "count"`) {
		t.Fatalf("expected did-you-mean hint, got %#v", errors[0])
	}
	if errors[0].Rule != "name resolution" {
		t.Fatalf("expected name resolution rule, got %#v", errors[0])
	}
}

func TestTypeErrorsAddFunctionImportHint(t *testing.T) {
	program := file.Program{
		Name:       "demo",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path: "main.klang",
			Lines: []string{
				"function Main() {",
				"    Pirnt(\"hello\");",
				"}",
			},
		}},
	}
	report := typechecker.Report{Errors: []typechecker.Error{{
		File:    "main.klang",
		Line:    2,
		Message: `unknown function "Pirnt"`,
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 || !strings.Contains(errors[0].Hint, `Did you mean "print"`) ||
		!strings.Contains(errors[0].Hint, "import that module") {
		t.Fatalf("expected function suggestion with import hint, got %#v", errors)
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
		File:    "main.klang",
		Line:    1,
		Message: "cannot assign List[String] to local List[Int] values",
	}}}

	errors := TypeErrors(program, report)
	if len(errors) != 1 || !strings.Contains(errors[0].Message, "Expected type:") ||
		!strings.Contains(errors[0].Message, "  List\n    Int") ||
		!strings.Contains(errors[0].Message, "Found type:") ||
		!strings.Contains(errors[0].Message, "  List\n    String") {
		t.Fatalf("expected type tree message, got %#v", errors)
	}
}

func TestRuntimeErrorContextParsesLineColumn(t *testing.T) {
	program := file.Program{Name: "demo", EntryPoint: "main.klang"}

	err := RuntimeError(program, errors.New("runtime failed: line 3:9: cannot index value"))

	if err.Phase != PhaseRuntime || err.Line != 3 || err.Column != 9 || err.Message != "cannot index value" {
		t.Fatalf("unexpected runtime context: %#v", err)
	}
}
