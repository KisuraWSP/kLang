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

	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("formatter is not idempotent:\n%s", formattedAgain)
	}
}

func TestFormatPreservesParsableKeywordMacros(t *testing.T) {
	input := `alias printer=Parsable[T Printable].keyword_macro{
print(get_args_from_parsable(),T);
}
printer "hallo";
`
	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("formatted macro is not parse-valid: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("Parsable keyword macro formatting is not idempotent:\n%s", formatted)
	}
}

func TestFormatUserDefinedScope(t *testing.T) {
	input := `scope Setup{
local Int value=1;
scope Inner{
print(value);
}
}
`
	expected := `scope Setup {
    local Int value = 1;
    scope Inner {
        print(value);
    }
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

func TestFormatForEachLoop(t *testing.T) {
	input := `function Main(){
for_each value in [1,2,3]{
print(value);
}
}
`
	expected := `function Main() {
    for_each value in [1, 2, 3] {
        print(value);
    }
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

func TestFormatPreservesStringEscapesIdempotently(t *testing.T) {
	input := "function Main():String{\nreturn \"\\\\n\\\\\\\"\";\n}\n"

	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("second format failed: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("escaped strings are not idempotent:\nfirst:\n%s\nsecond:\n%s", formatted, formattedAgain)
	}
	if !strings.Contains(formatted, `"\\n\\\""`) {
		t.Fatalf("formatter changed string escapes: %s", formatted)
	}
}

func TestFormatPreservesHereStringContents(t *testing.T) {
	input := `function Main():Int{
local String value=//
  first line

    indented line
//;
return len(value);
}
`
	expected := `function Main() : Int {
    local String value = //
  first line

    indented line
//;
    return len(value);
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

func TestFormatPreservesHereStringInsideJSONConstructor(t *testing.T) {
	input := `function Main():Int{
local JSON document=JSON(//
{
  "name": "kLang"
}
//);
return document.count;
}
`
	expected := `function Main() : Int {
    local JSON document = JSON(//
{
  "name": "kLang"
}
//);
    return document.count;
}
`

	formatted, err := Format(input)
	if err != nil {
		t.Fatalf("format failed: %v", err)
	}
	if formatted != expected {
		t.Fatalf("unexpected formatting:\n%s", formatted)
	}

	formattedAgain, err := Format(formatted)
	if err != nil {
		t.Fatalf("formatted JSON did not parse: %v", err)
	}
	if formattedAgain != formatted {
		t.Fatalf("JSON formatting is not idempotent:\n%s", formattedAgain)
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

func TestFormatPreservesCanonicalAtomLiterals(t *testing.T) {
	formatted, err := Format(`function Main():Atom{
local Atom code=:not_found;
return code;
}`)
	if err != nil {
		t.Fatalf("format Atom source: %v", err)
	}
	expected := "function Main() : Atom {\n    local Atom code = :not_found;\n    return code;\n}\n"
	if formatted != expected {
		t.Fatalf("unexpected Atom formatting:\n%s", formatted)
	}
}
