package conformance_test

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

func TestClaimedCoreDefectsHaveMinimalRegressionPrograms(t *testing.T) {
	fixtures := []struct {
		name   string
		source string
		want   int
	}{
		{
			name: "loops",
			source: `function Main() : Int {
    local mut Int total = 0;
    for index := range(4) { total += index; }
    return total;
}`,
			want: 6,
		},
		{
			name: "functions",
			source: `function Double(value : Int) : Int { return value * 2; }
function Main() : Int { return Double(21); }`,
			want: 42,
		},
		{
			name: "alias structs",
			source: `alias function User(id : Int) : type = struct {}
function Main() : Int { local User user = User(7); return user.id; }`,
			want: 7,
		},
		{
			name: "typed collections",
			source: `function Main() : Int {
    local List[Int] values = [2, 3, 4];
    return values[0] + values[1] + values[2];
}`,
			want: 9,
		},
		{
			name: "Atom errors",
			source: `function Fail() { throw :expected; }
function Main() : Int {
    try { Fail(); } catch reason {
        if reason == :expected { return 1; }
    }
    return 0;
}`,
			want: 1,
		},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			program := file.Program{
				Name: fixture.name, Root: ".", EntryPoint: "main.klang",
				Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(fixture.source, "\n")}},
			}
			if report := typechecker.CheckProgram(program); !report.Passed() {
				t.Fatalf("type check failed: %#v", report.Errors)
			}
			parsed := parser.ParseLoadedProgram(program)
			if !parsed.Passed() {
				t.Fatalf("parse failed: %#v", parsed.Errors())
			}
			result, err := runtime.New().Run(parsed)
			if err != nil {
				t.Fatalf("runtime failed: %v", err)
			}
			if result.Value.Kind != runtime.ValueInt || result.Value.Data.(int) != fixture.want {
				t.Fatalf("want %d, got %#v", fixture.want, result.Value)
			}
		})
	}
}

func TestCoreCollectionBoundsFailureRemainsDeterministic(t *testing.T) {
	program := file.Program{
		Name: "bounds", Root: ".", EntryPoint: "main.klang",
		Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(`
function Main() : Int {
    local List[Int] values = [1];
    return values[1];
}
`, "\n")}},
	}
	parsed := parser.ParseLoadedProgram(program)
	_, err := runtime.New().Run(parsed)
	if err == nil || !strings.Contains(err.Error(), "out of bounds") {
		t.Fatalf("expected deterministic bounds diagnostic, got %v", err)
	}
}
