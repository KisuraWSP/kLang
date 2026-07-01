package wasmbackend

import (
	"strings"
	"testing"

	"kLang/src/engine/backend"
	"kLang/src/engine/bytecode"
	"kLang/src/engine/conformance"
	"kLang/src/engine/file"
	"kLang/src/parser"
)

func TestCompilerEmitsExecutableBytecode(t *testing.T) {
	source := `load_as_script;

function Sum(limit : Int) : Int {
    local mut Int total = 0;
    local mut Int index = 0;
    while index < limit {
        total += index;
        index += 1;
    }
    return total;
}

function Changed(values : List[Int]) : List[Int] {
    local mut List[Int] changed = values;
    changed[0] = 9;
    return changed;
}

function Even(value : Int) : Bool {
    return value % 2 == 0;
}

function TimesTen(value : Int) : Int {
    return value * 10;
}

function Add(total : Int, value : Int) : Int {
    return total + value;
}

function OnlyFirst(value : Int) : Int {
    assert value == 1;
    return value;
}

function Main() : Int {
    local mut List[Int] values = [1, 2, 3];
    local List[Int] original = values;
    local List[Int] changed = Changed(values);
    assert values[0] == 1;
    assert changed[0] == 9;
    values[1] += 3;
    values[3] = 4;
    assert original[1] == 2;
    local Iterator[Int] pipeline = values.filter(Even).map(TimesTen);
    local List[Int] transformed = pipeline.sort();
    assert transformed[0] == 40;
    assert values.fold(0, Add) == 13;
    assert values.filter(Even).count == 1;
    assert [1, 2].map(OnlyFirst).limit(1).count == 1;

    local mut Int result = Sum(5);
    for index := range(len(values)) {
        result += values[index];
    }
    for_each value in values {
        result += value;
    }
    local mut Int characters = 0;
    for_each character in "a😀" {
        characters += 1;
    }
    assert characters == 2;
    local mut Int integerIterations = 0;
    for_each index in 3 {
        integerIterations += index;
    }
    assert integerIterations == 3;
    print("sum", result, values.count);
    return result;
}
`
	program := file.Program{
		Name: "bytecode", Root: ".", EntryPoint: "main.klang",
		Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(source, "\n")}},
	}
	request := backend.Request{Program: program, Parsed: parser.ParseLoadedProgram(program)}
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit bytecode: %v", err)
	}
	if output.Entry != "program.kbc" || len(output.Artifacts) != 1 {
		t.Fatalf("unexpected output: %#v", output)
	}
	decoded, err := bytecode.Decode(output.Artifacts[0].Content)
	if err != nil {
		t.Fatalf("decode emitted bytecode: %v", err)
	}
	result, err := bytecode.NewVM().Execute(decoded)
	if err != nil {
		t.Fatalf("execute emitted bytecode: %v", err)
	}
	if result.Value.Int != 36 || len(result.Output) != 1 || result.Output[0] != "sum 36 4" {
		t.Fatalf("unexpected bytecode result: %#v", result)
	}
}

func TestCompilerReportsUnsupportedTablesForFallback(t *testing.T) {
	source := `load_as_script;

function Main() : Int {
    local Table values = {"name": "kLang"};
    return values.count;
}
`
	program := file.Program{
		Name: "fallback", Root: ".", EntryPoint: "main.klang",
		Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(source, "\n")}},
	}
	request := backend.Request{Program: program, Parsed: parser.ParseLoadedProgram(program)}
	diagnostics := New().Check(request)
	if len(diagnostics) == 0 || diagnostics[0].Rule != "WASM_BYTECODE_UNSUPPORTED" {
		t.Fatalf("expected bytecode fallback diagnostics, got %#v", diagnostics)
	}
	if diagnostics[0].FeatureID != string(conformance.FeatureValuesTable) {
		t.Fatalf("expected stable Table feature id, got %#v", diagnostics[0])
	}
	if status, ok := conformance.Lookup(conformance.FeatureValuesTable, conformance.BackendWASM); !ok || status != conformance.StatusFallback {
		t.Fatalf("matrix does not record WASM Table fallback: %q, %v", status, ok)
	}
}
