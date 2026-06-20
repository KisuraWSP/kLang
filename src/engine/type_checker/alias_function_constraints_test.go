package typechecker

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckProgramAcceptsPrefixAliasFunctionTraitAndNestedRestrictions(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Render(value : String) : String;
}

impl Printable for String {
    function Render(value : String) : String {
        return value;
    }
}

alias function[T Printable] PrintableBox(value : T) : type = struct {
}

alias function[T restrict[List[Option[Int]]]] OptionalInts(value : T) : type = struct {
}

function Main() : Int {
    let printable = PrintableBox("hello");
    let optional = OptionalInts([Some(1), Some(2)]);
    print(printable.value, optional.value);
    return 0;
}
`)
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected constrained alias functions to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsPrefixAliasFunctionTraitViolation(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Render(value : String) : String;
}

alias function[T Printable] PrintableBox(value : T) : type = struct {
}

function Main() : Int {
    let invalid = PrintableBox(42);
    return 0;
}
`)
	report := CheckProgram(program)
	if report.Passed() {
		t.Fatal("expected trait restriction failure")
	}
	if !strings.Contains(fmt.Sprint(report.Errors), "expects T:Printable, got Int") {
		t.Fatalf("expected trait restriction diagnostic, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsPrefixAliasFunctionNestedRestrictionViolation(t *testing.T) {
	program := programFromSource(`
alias function[T restrict[List[Option[Int]]]] OptionalInts(value : T) : type = struct {
}

function Main() : Int {
    let invalid = OptionalInts(["wrong"]);
    return 0;
}
`)
	report := CheckProgram(program)
	if report.Passed() {
		t.Fatal("expected nested generic restriction failure")
	}
	if !strings.Contains(fmt.Sprint(report.Errors), "expects T:List[Option[Int]], got List[String]") {
		t.Fatalf("expected nested restriction diagnostic, got: %v", report.Errors)
	}
}
