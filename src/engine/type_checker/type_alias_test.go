package typechecker

import (
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestCheckProgramAcceptsChainedTypeAliases(t *testing.T) {
	program := programFromSource(`
type names = string_list;
type string_list = List[String];
type optional_names = Option[names];

function Echo(values : names) : names {
    local names copied = values as names;
    return copied;
}

function Main() : Int {
    local names values = ["Ada", "Lin"];
    local optional_names present = Some(values);
    return len(Echo(values));
}
`)
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type aliases to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramResolvesTypeAliasesAcrossWorkspaceFiles(t *testing.T) {
	program := file.Program{Name: "aliases", Files: []file.SourceFile{
		{Path: "main.klang", Lines: strings.Split(`function Names(value : string_list) : string_list { return value; }`, "\n")},
		{Path: "types.klang", Lines: []string{`type string_list = List[String];`}},
	}}
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected workspace type alias to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsUnknownTypeAliasTarget(t *testing.T) {
	report := CheckProgram(programFromSource(`type broken = missing_type;`))
	if report.Passed() {
		t.Fatal("expected unknown type alias target failure")
	}
	if !strings.Contains(report.Errors[0].Message, "targets unknown type missing_type") {
		t.Fatalf("unexpected type alias diagnostic: %v", report.Errors)
	}
}
