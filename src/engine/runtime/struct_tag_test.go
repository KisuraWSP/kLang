package runtime

import "testing"

func TestRuntimeSerializesAliasStructUsingJSONTags(t *testing.T) {
	result := runSource(t, ""+
		"alias function User(id : String, name : String) : type = struct {\n"+
		"    this.id `json:\"user_id\"`;\n"+
		"}\n"+
		"function Main() : Int {\n"+
		"    let user = User(\"42\", \"Ada\");\n"+
		"    local JSON document = JSON(user);\n"+
		"    assert option_unwrap_or(json_string(document.user_id), \"\") == \"42\";\n"+
		"    return 0;\n"+
		"}\n")
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("unexpected tagged JSON serialization: %#v", result.Value)
	}
}

func TestRuntimeExposesAliasJSONTagsThroughTypeMetadata(t *testing.T) {
	result := runSource(t, ""+
		"alias function User(id : String) : type = struct {\n"+
		"    this.id `json:\"user_id\"`;\n"+
		"}\n"+
		"function Main() : Int {\n"+
		"    local Type info = User.get_runtime_type_info();\n"+
		"    local Table serialization = info.serialization;\n"+
		"    local Table tags = serialization.json_tags as Table;\n"+
		"    assert tags.id == \"user_id\";\n"+
		"    return 0;\n"+
		"}\n")
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("unexpected JSON tag metadata result: %#v", result.Value)
	}
}
