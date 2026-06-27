package typechecker

import "testing"

func TestCheckProgramEnforcesStrictMainEntryPoint(t *testing.T) {
	missing := CheckProgram(programFromSource(`function Helper() : Int { return 0; }`))
	assertTypeError(t, missing, "program must define function Main() : Int")

	invalid := CheckProgram(programFromSource(`function Main(value : Int) : Int { return value; }`))
	assertTypeError(t, invalid, "entry point Main must have signature function Main() : Int")
}

func TestCheckProgramAcceptsStrictCustomEntryPoint(t *testing.T) {
	report := CheckProgram(programFromSource(`
namespace App {
    #set_entry_point_to_here
    function Start() : Int {
        return 0;
    }
}
`))
	if !report.Passed() {
		t.Fatalf("expected strict custom entry point to pass, got %#v", report.Errors)
	}
}
