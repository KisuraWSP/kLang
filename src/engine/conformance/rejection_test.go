package conformance_test

import (
	"strings"
	"testing"

	"kLang/src/engine/backend"
	jsbackend "kLang/src/engine/backend/js"
	wasmbackend "kLang/src/engine/backend/wasm"
	"kLang/src/engine/conformance"
	"kLang/src/engine/file"
	"kLang/src/parser"
)

func TestRejectedCoreFeaturesProduceStableBackendDiagnostics(t *testing.T) {
	fixtures := []struct {
		name    string
		feature conformance.FeatureID
		source  string
	}{
		{
			name: "async", feature: conformance.FeatureAsyncFunctions,
			source: `async function Work() : Int { return 1; }
function Main() : Int { return 0; }`,
		},
		{
			name: "threads", feature: conformance.FeatureThreads,
			source: `function Work() : Int { return 1; }
function Main() : Int { local Thread[Int] worker = spawn(Work, []); return 0; }`,
		},
		{
			name: "atomic", feature: conformance.FeatureAtomic,
			source: `function Main() : Int { local Atomic[Int] value = Atomic(1); return 0; }`,
		},
		{
			name: "transactions", feature: conformance.FeatureTransactions,
			source: `function Main() : Int { transaction { local Int value = 1; } return 0; }`,
		},
		{
			name: "files", feature: conformance.FeatureFiles,
			source: `function Main() : Int { local File value = File("input.txt"); return 0; }`,
		},
		{
			name: "os", feature: conformance.FeatureOS,
			source: `function Main() : Int { local OS host = OS(); return 0; }`,
		},
		{
			name: "javascript interop", feature: conformance.FeatureJavaScriptInterop,
			source: `function Main() : Int { local JSModule module = js_import("module.js"); return 0; }`,
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			program := file.Program{
				Name: fixture.name, Root: ".", EntryPoint: "main.klang",
				Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(fixture.source, "\n")}},
			}
			parsed := parser.ParseLoadedProgram(program)
			if !parsed.Passed() {
				t.Fatalf("fixture parse failed: %#v", parsed.Errors())
			}
			request := backend.Request{Program: program, Parsed: parsed, Backend: "JS"}
			jsDiagnostics := jsbackend.New().Check(request)
			assertFeatureDiagnostic(t, jsDiagnostics, fixture.feature)

			request.Backend = "WASM"
			wasmDiagnostics := wasmbackend.New().Check(request)
			assertFeatureDiagnostic(t, wasmDiagnostics, fixture.feature)
		})
	}
}

func assertFeatureDiagnostic(t *testing.T, diagnostics []backend.Diagnostic, feature conformance.FeatureID) {
	t.Helper()
	for _, diagnostic := range diagnostics {
		if diagnostic.FeatureID == string(feature) {
			return
		}
	}
	t.Fatalf("expected diagnostic feature %s, got %#v", feature, diagnostics)
}
