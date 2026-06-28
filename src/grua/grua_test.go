package grua

import (
	"strings"
	"testing"
)

func TestTranspileLowersGruaSubsetSyntax(t *testing.T) {
	source := `import "io"

function Identity(value::Int) : Int {
    return value
}

function Main() : Int {
    local immutable = {}
    local mut mutable = {"count": 0}
    val shared = {}
    var changing = {}

    for index:=0, index<3, index+=1 {
        mutable["count"] += 1
    }
    for entry in mutable {
        io.Print(entry)
    }
    switch mutable["count"] {
        case 3: return 0
        case: return 1
    }
}
`
	transpiled, diagnostics := Transpile(source)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected Grua diagnostics: %#v", diagnostics)
	}
	for _, expected := range []string{
		`import "io";`,
		`function Identity(value:Int) : Int {`,
		`local immutable = {};`,
		`for index:=0; index<3; index+=1 {`,
		`for_each entry in mutable {`,
		`if mutable["count"] == {`,
		`case 3: return 0;`,
	} {
		if !strings.Contains(transpiled, expected) {
			t.Fatalf("transpiled source missing %q:\n%s", expected, transpiled)
		}
	}
}

func TestTranspileAddsAnyHintToUntypedParameters(t *testing.T) {
	transpiled, diagnostics := Transpile(`function Echo(value) {
    return value
}`)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected Grua diagnostics: %#v", diagnostics)
	}
	if !strings.Contains(transpiled, "function Echo(value : Any)") {
		t.Fatalf("expected inferred Any parameter hint:\n%s", transpiled)
	}
}

func TestTranspileTerminatesMultilineTableDeclaration(t *testing.T) {
	transpiled, diagnostics := Transpile(`function Main() : Int {
    local value = {
        "name": "grua",
        "nested": {"ok": True}
    }
    return 0
}`)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected Grua diagnostics: %#v", diagnostics)
	}
	if !strings.Contains(transpiled, "    };") {
		t.Fatalf("expected multiline Table declaration terminator:\n%s", transpiled)
	}
	if strings.Contains(transpiled, `"name": "grua";`) {
		t.Fatalf("Table fields must not receive statement terminators:\n%s", transpiled)
	}
}

func TestTranspileTerminatesReturnedTableLiteral(t *testing.T) {
	transpiled, diagnostics := Transpile(`function Empty() : Table {
    return {"count": 0}
}`)
	if len(diagnostics) != 0 {
		t.Fatalf("unexpected Grua diagnostics: %#v", diagnostics)
	}
	if !strings.Contains(transpiled, `return {"count": 0};`) {
		t.Fatalf("expected returned Table terminator:\n%s", transpiled)
	}
}

func TestTranspileRejectsFeaturesOutsideSubset(t *testing.T) {
	cases := []struct {
		source  string
		message string
	}{
		{`local Int value = 1`, "inferred syntax"},
		{`if True {}`, "conditions use switch"},
		{`while True {}`, "one loop keyword"},
		{`import "strings"`, "only exposes"},
		{`local values = [1, 2]`, "List literals"},
		{`local values = Set({})`, "instead of Set"},
		{`alias function Box() : type = struct {}`, "Table type"},
	}
	for _, testCase := range cases {
		_, diagnostics := Transpile(testCase.source)
		if len(diagnostics) == 0 {
			t.Fatalf("expected %q to be rejected", testCase.source)
		}
		found := false
		for _, diagnostic := range diagnostics {
			if strings.Contains(diagnostic.Message, testCase.message) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected diagnostic containing %q, got %#v", testCase.message, diagnostics)
		}
	}
}
