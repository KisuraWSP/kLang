package parser

import "testing"

func TestParsesPrefixAliasFunctionTypeRestrictions(t *testing.T) {
	program, errors := Parse(`
alias function[T Printable] Test(value : T) : type = struct {
}

alias function[T restrict[List[Option[Int]]]] Test2(value : T) : type = struct {
}

alias function[T Printable] Empty() : type = struct {
}
`)
	if len(errors) != 0 {
		t.Fatalf("parse errors: %#v", errors)
	}
	if len(program.Statements) != 3 {
		t.Fatalf("expected three alias functions, got %d", len(program.Statements))
	}
	printable := program.Statements[0].(AliasFunctionStatement)
	if len(printable.TypeParams) != 1 || printable.TypeParams[0].Name != "T" || printable.TypeParams[0].Type != "T:Printable" {
		t.Fatalf("unexpected trait restriction: %#v", printable.TypeParams)
	}
	if printable.Params[0].Type != "T:Printable" {
		t.Fatalf("expected parameter restriction propagation, got %#v", printable.Params)
	}
	nested := program.Statements[1].(AliasFunctionStatement)
	if len(nested.TypeParams) != 1 || nested.TypeParams[0].Type != "T:List[Option[Int]]" {
		t.Fatalf("unexpected nested restriction: %#v", nested.TypeParams)
	}
	if nested.Params[0].Type != "T:List[Option[Int]]" {
		t.Fatalf("expected nested restriction propagation, got %#v", nested.Params)
	}
	empty := program.Statements[2].(AliasFunctionStatement)
	if len(empty.TypeParams) != 1 || empty.TypeParams[0].Type != "T:Printable" || len(empty.Params) != 0 {
		t.Fatalf("unexpected zero-argument constrained alias: %#v", empty)
	}
}

func TestRejectsAliasFunctionTypeParametersInBothPositions(t *testing.T) {
	_, errors := Parse(`alias function[T Printable] Test[U numeric](value : T) : type = struct {}`)
	if len(errors) == 0 {
		t.Fatal("expected duplicate generic-position diagnostic")
	}
}
