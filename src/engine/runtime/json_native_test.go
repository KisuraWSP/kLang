package runtime

import (
	"strings"
	"testing"
)

func TestRuntimeRoundTripsNativeValuesThroughJSON(t *testing.T) {
	result := runParsedSource(t, strings.ReplaceAll(`
alias function User(id : String, name : String) : type = struct {
    this.name TAGjson:"display_name"TAG;
}

function Main() : String {
    local Table source = {
        "name": "Ada",
        "items": [1, 2],
        "active": True,
        "missing": None()
    };
    local Result[String, String] encoded = json_encode(source);
    assert encoded.ok;
    local String text = encoded.value;
    assert text == json_stringify(source);

    local Result[T, String] decoded = json_decode(text);
    assert decoded.ok;
    local Table object = decoded.value as Table;
    local List[T] items = object.items as List[T];
    local List[T] keys = table_keys(object);
    assert object.name == "Ada";
    assert object.active;
    assert len(items) == 2;
    assert items[0] == 1;
    assert keys[0] == "active";

    let user = User("7", "Ada");
    return text + "|" + json_stringify(user);
}
`, "TAG", "`"))
	expected := "{\"active\":true,\"items\":[1,2],\"missing\":null,\"name\":\"Ada\"}|{\"display_name\":\"Ada\",\"id\":\"7\"}"
	if result.Value.Kind != ValueString || result.Value.Data.(string) != expected {
		t.Fatalf("unexpected native JSON round trip: %#v", result.Value)
	}
}

func TestRuntimeJSONEncodeReturnsErrorForUnsafeTableKey(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Bool {
    local Table source = {1: "not an object key"};
    local Result[String, String] encoded = json_encode(source);
    return not encoded.ok;
}
`)
	if result.Value.Kind != ValueBool || !result.Value.Data.(bool) {
		t.Fatalf("expected safe JSON encoder error, got %#v", result.Value)
	}
}
