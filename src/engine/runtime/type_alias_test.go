package runtime

import "testing"

func TestRuntimeTreatsTypeAliasesAsCompileTimeSynonyms(t *testing.T) {
	result := runSource(t, `
type names = string_list;
type string_list = List[String];

function Main() : Int {
    local names values = ["Ada", "Lin"];
    local names copied = values as names;
    return len(copied);
}
`)
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected aliased list length 2, got %#v", result.Value)
	}
}
