package parser

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestResolveEntryPointRequiresStrictDefaultMain(t *testing.T) {
	parsed := parsedEntryPointProgram(t, `function Main() : Int { return 0; }`)
	entry, diagnostics := ResolveEntryPoint(parsed)
	if entry != "Main" || len(diagnostics) != 0 {
		t.Fatalf("expected strict Main entry point, got %q and %#v", entry, diagnostics)
	}
}

func TestResolveEntryPointAcceptsDirectiveDesignatedFunction(t *testing.T) {
	parsed := parsedEntryPointProgram(t, `
namespace App {
    #set_entry_point_to_here
    function Start() : Int {
        return 0;
    }
}
`)
	entry, diagnostics := ResolveEntryPoint(parsed)
	if entry != "App.Start" || len(diagnostics) != 0 {
		t.Fatalf("expected custom entry point, got %q and %#v", entry, diagnostics)
	}
}

func TestResolveEntryPointRejectsMissingAndInvalidEntries(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		expected string
	}{
		{"missing", `function Helper() : Int { return 0; }`, "program must define function Main() : Int"},
		{"parameter", `function Main(value : Int) : Int { return value; }`, "entry point Main must have signature"},
		{"return", `function Main() : String { return ""; }`, "entry point Main must have signature"},
		{"async", `async function Main() : Int { return 0; }`, "entry point Main must have signature"},
		{"generic", `function Main[T Any]() : Int { return 0; }`, "entry point Main must have signature"},
		{"custom return", `#set_entry_point_to_here function Start() : String { return ""; }`, "entry point Start must have signature"},
		{"dangling", `#set_entry_point_to_here local Int value = 1; function Main() : Int { return 0; }`, "must be immediately followed by a function"},
	}
	for _, current := range tests {
		t.Run(current.name, func(t *testing.T) {
			_, diagnostics := ResolveEntryPoint(parsedEntryPointProgram(t, current.source))
			if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, current.expected) {
				t.Fatalf("expected diagnostic containing %q, got %#v", current.expected, diagnostics)
			}
		})
	}
}

func TestResolveEntryPointRejectsMultipleDirectives(t *testing.T) {
	parsed := parsedEntryPointProgram(t, `
#set_entry_point_to_here
function First() : Int { return 0; }

#set_entry_point_to_here
function Second() : Int { return 0; }
`)
	entry, diagnostics := ResolveEntryPoint(parsed)
	if entry != "First" {
		t.Fatalf("expected first custom entry for recovery, got %q", entry)
	}
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "only one") {
		t.Fatalf("expected duplicate directive diagnostic, got %#v", diagnostics)
	}
}

func parsedEntryPointProgram(t *testing.T, source string) ParsedProgram {
	t.Helper()
	return ParseLoadedProgram(file.Program{
		Name:       "entry",
		EntryPoint: "main.klang",
		Files: []file.SourceFile{{
			Path:  "main.klang",
			Lines: strings.Split(strings.TrimSpace(source), "\n"),
		}},
	})
}
