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

function Main() : Int {
    let box = PrintableBox("hello");
    assert box.value == "hello";
    return 0;
}
`)
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected constrained alias field value, got %#v", result.Value)
	}
}
