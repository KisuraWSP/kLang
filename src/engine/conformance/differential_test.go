package conformance_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"kLang/src/engine/backend"
	jsbackend "kLang/src/engine/backend/js"
	wasmbackend "kLang/src/engine/backend/wasm"
	"kLang/src/engine/bytecode"
	"kLang/src/engine/file"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

func TestTypedCoreDifferentialAcrossSupportedBackends(t *testing.T) {
	source := `
function Sum(values : List[Int]) : Int {
    local mut Int total = 0;
    for index := range(len(values)) {
        if values[index] > 1 {
            total += values[index];
        }
    }
    return total;
}

function Main() : Int {
    local Int result = Sum([1, 2, 3, 4]);
    print(result);
    return result;
}
`
	program := file.Program{
		Name: "differential", Root: ".", EntryPoint: "main.klang",
		Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(strings.TrimSpace(source), "\n")}},
	}
	for _, target := range []string{"Standalone", "JS", "WASM"} {
		if report := typechecker.CheckProgramForBackend(program, target); !report.Passed() {
			t.Fatalf("%s type check failed: %#v", target, report.Errors)
		}
	}
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		t.Fatalf("parse failed: %#v", parsed.Errors())
	}

	interpreted, err := runtime.New().Run(parsed)
	if err != nil {
		t.Fatalf("Standalone execution failed: %v", err)
	}
	if interpreted.Value.Kind != runtime.ValueInt || interpreted.Value.Data.(int) != 9 {
		t.Fatalf("unexpected Standalone value: %#v", interpreted.Value)
	}
	if !reflect.DeepEqual(interpreted.Output, []string{"9"}) {
		t.Fatalf("unexpected Standalone output: %#v", interpreted.Output)
	}

	wasmRequest := backend.Request{Program: program, Parsed: parsed, Backend: "WASM"}
	wasmOutput, err := wasmbackend.New().Emit(wasmRequest)
	if err != nil {
		t.Fatalf("WASM bytecode emission failed: %v", err)
	}
	decoded, err := bytecode.Decode(wasmOutput.Artifacts[0].Content)
	if err != nil {
		t.Fatalf("decode WASM bytecode: %v", err)
	}
	vmResult, err := bytecode.NewVM().Execute(decoded)
	if err != nil {
		t.Fatalf("bytecode execution failed: %v", err)
	}
	if vmResult.Value.Kind != bytecode.ValueInt || vmResult.Value.Int != 9 ||
		!reflect.DeepEqual(vmResult.Output, []string{"9"}) {
		t.Fatalf("bytecode drifted from Standalone: %#v", vmResult)
	}

	node, nodeErr := exec.LookPath("node")
	if nodeErr != nil {
		t.Log("Node is unavailable; JS emission remains checked but execution comparison is skipped")
		if diagnostics := jsbackend.New().Check(backend.Request{Program: program, Parsed: parsed, Backend: "JS"}); len(diagnostics) != 0 {
			t.Fatalf("JS check failed: %#v", diagnostics)
		}
		return
	}
	jsOutput, err := jsbackend.New().Emit(backend.Request{Program: program, Parsed: parsed, Backend: "JS"})
	if err != nil {
		t.Fatalf("JS emission failed: %v", err)
	}
	bundle := t.TempDir()
	if err := jsbackend.New().Package(jsOutput, bundle); err != nil {
		t.Fatalf("JS packaging failed: %v", err)
	}
	command := exec.Command(node, filepath.Join(bundle, "program.js"))
	command.Env = append(os.Environ(), "NO_COLOR=1")
	printed, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("JS execution failed: %v\n%s", err, printed)
	}
	if strings.TrimSpace(string(printed)) != interpreted.Output[0] {
		t.Fatalf("JS output drifted: want %q, got %q", interpreted.Output[0], strings.TrimSpace(string(printed)))
	}
}
