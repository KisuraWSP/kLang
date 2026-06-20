package runtime

import "testing"

func TestRuntimeConstructsPrefixConstrainedAliasStruct(t *testing.T) {
	result := runSource(t, `
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

function Main() : String {
    let box = PrintableBox("hello");
    return box.value;
}
`)
	if result.Value.Kind != ValueString || result.Value.Data.(string) != "hello" {
		t.Fatalf("expected constrained alias field value, got %#v", result.Value)
	}
}
