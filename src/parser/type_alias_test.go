package parser

import "testing"

func TestParsesAndResolvesTypeAliasesThroughoutSource(t *testing.T) {
	program, errors := Parse(`
function Forward(value : names) : names {
    local names copied = value as names;
    return copied;
}

type names = string_list;
type string_list = List[String];
`)
	if len(errors) != 0 {
		t.Fatalf("parse errors: %#v", errors)
	}
	function := program.Statements[0].(FunctionStatement)
	if function.Params[0].Type != "List[String]" || function.ReturnType != "List[String]" {
		t.Fatalf("expected resolved function signature, got %#v", function)
	}
	local := function.Body[0].(VariableStatement)
	if local.Type != "List[String]" {
		t.Fatalf("expected resolved local type, got %#v", local)
	}
	cast := local.Expression.Node.(CastExpression)
	if cast.Type != "List[String]" {
		t.Fatalf("expected resolved cast type, got %#v", cast)
	}
	alias := program.Statements[1].(TypeAliasStatement)
	if alias.Name != "names" || alias.Resolved != "List[String]" {
		t.Fatalf("unexpected chained type alias: %#v", alias)
	}
}

func TestRejectsCyclicTypeAliases(t *testing.T) {
	_, errors := Parse(`
type first = second;
type second = first;
`)
	if len(errors) == 0 {
		t.Fatal("expected cyclic type alias error")
	}
}
