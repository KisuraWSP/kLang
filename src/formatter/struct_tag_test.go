package formatter

import "testing"

func TestFormatPreservesJSONStructTags(t *testing.T) {
	input := "alias function User(id:String):type=struct{\nthis.id `json:\"user_id\"`;\n}\n"
	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("formatted struct tag is not parse-valid: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("struct tag formatting is not idempotent:\n%s", formatted)
	}
}
