package conformance_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/diagnostic"
	"kLang/src/engine/backend"
	jsbackend "kLang/src/engine/backend/js"
	wasmbackend "kLang/src/engine/backend/wasm"
	langcontext "kLang/src/engine/context"
	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

type diagnosticGolden struct {
	Code         string              `json:"code"`
	Severity     diagnostic.Severity `json:"severity"`
	Phase        diagnostic.Phase    `json:"phase"`
	Rule         string              `json:"rule"`
	FeatureID    string              `json:"feature_id,omitempty"`
	Line         int                 `json:"line"`
	Column       int                 `json:"column"`
	EndLine      int                 `json:"end_line"`
	EndColumn    int                 `json:"end_column"`
	Message      string              `json:"message"`
	Hint         string              `json:"hint"`
	ExpectedType string              `json:"expected_type,omitempty"`
	FoundType    string              `json:"found_type,omitempty"`
	LabelCount   int                 `json:"label_count"`
	FrameCount   int                 `json:"frame_count"`
}

func TestDiagnosticProducerGoldens(t *testing.T) {
	programFor := func(source string) file.Program {
		return file.Program{
			Name: "diagnostic", Root: ".", EntryPoint: "main.klang",
			Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(strings.TrimSpace(source), "\n")}},
		}
	}

	parserProgram := programFor(`function Main( : Int { return 0; }`)
	parserDiagnostic := langcontext.ParseErrors(parserProgram, parser.ParseLoadedProgram(parserProgram))[0]

	typeProgram := programFor(`
function Main() : Int {
    local Int value = "bad";
    return value;
}`)
	typeDiagnostic := langcontext.TypeErrors(typeProgram, typechecker.CheckProgram(typeProgram))[0]

	moduleProgram := programFor(`
import "definitely_missing_p1_module"
function Main() : Int { return 0; }`)
	resolver := modulesystem.NewResolver(".")
	resolver.DisableStdlib = true
	_, moduleReport := resolver.ResolveProgram(moduleProgram)
	moduleDiagnostic := langcontext.ModuleErrors(moduleProgram, moduleReport)[0]

	runtimeDiagnostic := langcontext.RuntimeError(programFor(`function Main() : Int { return 0; }`), runtime.Error{
		Code: diagnostic.CodeRuntimeFailure, Rule: "index bounds", Hint: "Use an index inside the collection.",
		File: "main.klang", Line: 2, Column: 12, EndLine: 2, EndColumn: 17,
		Message: "list index 4 is out of bounds",
		Frames:  []diagnostic.StackFrame{{Function: "Main", File: "main.klang", Line: 2, Column: 12}},
	})

	jsProgram := programFor(`
function Main() : Int {
    local Set[String] values;
    return 0;
}`)
	jsRequest := backend.Request{Program: jsProgram, Parsed: parser.ParseLoadedProgram(jsProgram), Backend: "JS"}
	jsDiagnostic := langcontext.BackendDiagnostics(jsProgram, "JS", jsbackend.New().Check(jsRequest))[0]

	wasmProgram := programFor(`
function Main() : Int {
    local Table values = {};
    return values.count;
}`)
	wasmRequest := backend.Request{Program: wasmProgram, Parsed: parser.ParseLoadedProgram(wasmProgram), Backend: "WASM"}
	wasmDiagnostic := langcontext.BackendDiagnostics(wasmProgram, "WASM", wasmbackend.New().Check(wasmRequest))[0]

	cases := map[string]diagnostic.Diagnostic{
		"parser":  parserDiagnostic,
		"type":    typeDiagnostic,
		"module":  moduleDiagnostic,
		"runtime": runtimeDiagnostic,
		"js":      jsDiagnostic,
		"wasm":    wasmDiagnostic,
	}
	for name, value := range cases {
		name, value := name, diagnostic.Normalize(value)
		t.Run(name, func(t *testing.T) {
			actual, err := json.MarshalIndent(diagnosticGolden{
				Code: value.Code, Severity: value.Severity, Phase: value.Phase, Rule: value.Rule,
				FeatureID: value.FeatureID, Line: value.Line, Column: value.Column,
				EndLine: value.EndLine, EndColumn: value.EndColumn, Message: value.Message, Hint: value.Hint,
				ExpectedType: value.ExpectedType, FoundType: value.FoundType,
				LabelCount: len(value.Labels), FrameCount: len(value.Frames),
			}, "", "  ")
			if err != nil {
				t.Fatalf("marshal diagnostic golden: %v", err)
			}
			actual = append(actual, '\n')
			path := filepath.Join("testdata", "diagnostics", name+".golden")
			expected, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read %s: %v\nactual:\n%s", path, err, actual)
			}
			if string(expected) != string(actual) {
				t.Fatalf("%s differs\nexpected:\n%s\nactual:\n%s", path, expected, actual)
			}
		})
	}
}
