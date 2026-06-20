package runtime

import "testing"

func TestRuntimeSerializesAliasStructUsingJSONTags(t *testing.T) {
	result := runSource(t, ""+
		"alias function User(id : String, name : String) : type = struct {\n"+
		"    this.id `json:\"user_id\"`;\n"+
		"}\n"+
		"function Main() : String {\n"+
		"    let user = User(\"42\", \"Ada\");\n"+
		"    return json_stringify(user);\n"+
		"}\n")
	if result.Value.Kind != ValueString || result.Value.Data.(string) != `{"name":"Ada","user_id":"42"}` {
		t.Fatalf("unexpected tagged JSON serialization: %#v", result.Value)
	}
}

func TestRuntimeExposesAliasJSONTagsThroughTypeMetadata(t *testing.T) {
	result := runSource(t, ""+
		"alias function User(id : String) : type = struct {\n"+
		"    this.id `json:\"user_id\"`;\n"+
		"}\n"+
		"function Main() : Type {\n"+
		"    return User.get_runtime_type_info();\n"+
		"}\n")
	info := result.Value.Data.(ObjectData)
	serialization := info.Fields["serialization"].Data.(TableData)
	tagsValue, ok := tableGet(serialization, TableKey{Kind: ValueString, Repr: "json_tags"})
	if !ok {
		t.Fatalf("missing JSON tag metadata: %#v", serialization)
	}
	tags := tagsValue.Data.(TableData)
	name, ok := tableGet(tags, TableKey{Kind: ValueString, Repr: "id"})
	if !ok || name.Data.(string) != "user_id" {
		t.Fatalf("unexpected JSON tag metadata: %#v", tags)
	}
}
