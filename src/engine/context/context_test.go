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

func TestRuntimeErrorContextParsesLineColumn(t *testing.T) {
	program := file.Program{Name: "demo", EntryPoint: "main.klang"}

	err := RuntimeError(program, errors.New("runtime failed: line 3:9: cannot index value"))

	if err.Phase != PhaseRuntime || err.Line != 3 || err.Column != 9 || err.Message != "cannot index value" {
		t.Fatalf("unexpected runtime context: %#v", err)
	}
}
