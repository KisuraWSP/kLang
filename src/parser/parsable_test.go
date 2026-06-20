package parser

import "testing"

func TestParsesParsableKeywordMacroAndBareInvocation(t *testing.T) {
	program, errors := Parse(`
alias printer = Parsable[T Printable].keyword_macro {
    print(get_args_from_parsable(), T);
}
printer "hallo";
`)
	if len(errors) != 0 {
		t.Fatalf("parse errors: %#v", errors)
	}
	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(program.Statements))
	}
	macro, ok := program.Statements[0].(AliasStatement)
	if !ok || !macro.KeywordMacro || macro.Target != "Parsable[T:Printable]" || len(macro.Body) != 1 {
		t.Fatalf("unexpected macro: %#v", program.Statements[0])
	}
	invocation, ok := program.Statements[1].(ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[1])
	}
	call, ok := invocation.Expression.Node.(CallExpression)
	if !ok || len(call.Arguments) != 1 {
		t.Fatalf("expected one-argument macro call, got %#v", invocation.Expression.Node)
	}
}

func TestParsesExplicitParsableLetWithoutInitializer(t *testing.T) {
	program, errors := Parse(`let mut Parsable[T Printable] parsable;`)
	if len(errors) != 0 {
		t.Fatalf("parse errors: %#v", errors)
	}
	variable, ok := program.Statements[0].(VariableStatement)
	if !ok || variable.Type != "Parsable[T:Printable]" || !variable.Mutable || variable.Inferred {
		t.Fatalf("unexpected variable: %#v", program.Statements[0])
	}
}
