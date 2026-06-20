package formatter

import "testing"

func TestFormatPreservesPrefixAliasFunctionConstraints(t *testing.T) {
	input := `alias function[T restrict[List[Option[Int]]]] Box(value:T):type=struct{
}
`
	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("formatted alias is not parse-valid: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("prefix constrained alias formatting is not idempotent:\n%s", formatted)
	}
}
