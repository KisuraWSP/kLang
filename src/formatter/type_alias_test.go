package formatter

import "testing"

func TestFormatPreservesTypeAliases(t *testing.T) {
	input := "type string_list=List[String];\n"
	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if formatted != "type string_list = List[String];\n" {
		t.Fatalf("unexpected type alias formatting: %q", formatted)
	}
}
