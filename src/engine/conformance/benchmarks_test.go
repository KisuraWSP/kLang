package conformance_test

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

const benchmarkSource = `
function Sum(limit : Int) : Int {
    local mut Int total = 0;
    local mut Int index = 0;
    while index < limit {
        total += index;
        index += 1;
    }
    return total;
}

function Main() : Int {
    return Sum(100);
}
`

func benchmarkProgram() file.Program {
	return file.Program{
		Name: "benchmark", Root: ".", EntryPoint: "main.klang",
		Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(strings.TrimSpace(benchmarkSource), "\n")}},
	}
}

func BenchmarkFrontendParseAndTypeCheck(b *testing.B) {
	program := benchmarkProgram()
	b.ReportAllocs()
	for b.Loop() {
		parsed := parser.ParseLoadedProgram(program)
		if !parsed.Passed() {
			b.Fatal(parsed.Errors())
		}
		if report := typechecker.CheckProgram(program); !report.Passed() {
			b.Fatal(report.Errors)
		}
	}
}

func BenchmarkInterpreterTypedCore(b *testing.B) {
	parsed := parser.ParseLoadedProgram(benchmarkProgram())
	if !parsed.Passed() {
		b.Fatal(parsed.Errors())
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := runtime.New().Run(parsed); err != nil {
			b.Fatal(err)
		}
	}
}
