package formatter

import (
	"strings"
	"testing"
)

func TestFormatStabilizesWhitespaceIndentationAndComments(t *testing.T) {
	input := `-- module comment
function Main():Int{
local Int value=1+2;-- inline math
if value>1{
return value-1;
}
return 0;
}
`
	expected := `-- module comment
function Main() : Int {
    local Int value = 1 + 2; -- inline math
    if value > 1 {
        return value - 1;
    }
    return 0;
}
`

	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if formatted != expected {
		t.Fatalf("unexpected formatting:\n%s", formatted)
	}
}

func TestFormatPreservesBlockComments(t *testing.T) {
	input := `function Main() : Int {
(* keep this
   comment shape *)
return 1;
}
`
	expected := `function Main() : Int {
    (* keep this
    comment shape *)
    return 1;
}
`

	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if formatted != expected {
		t.Fatalf("unexpected formatting:\n%s", formatted)
	}
}

func TestFormatRejectsParseErrors(t *testing.T) {
	_, err := Format(`function Main( {`)
	if err == nil {
		t.Fatal("expected parse error")
	}
	if !strings.Contains(err.Error(), "line") {
		t.Fatalf("expected source location in error, got %v", err)
	}
}
