package parser

import "testing"

func TestParsesJSONTagsOnAliasStructFields(t *testing.T) {
	program, errors := Parse(`
alias function User(id : String, displayName : String) : type = struct {
    this.id ` + "`json:\"user_id\"`" + `;
    this.displayName ` + "`json:\"display_name\"`" + `;
}
`)
	if len(errors) != 0 {
		t.Fatalf("parse errors: %#v", errors)
	}
	alias := program.Statements[0].(AliasFunctionStatement)
	if len(alias.FieldTags) != 2 {
		t.Fatalf("expected two field tags, got %#v", alias.FieldTags)
	}
	if alias.FieldTags[0].Field != "id" || alias.FieldTags[0].Kind != "json" || alias.FieldTags[0].Name != "user_id" {
		t.Fatalf("unexpected first field tag: %#v", alias.FieldTags[0])
	}
}

func TestRejectsMalformedAliasStructTag(t *testing.T) {
	_, errors := Parse("alias function User(id : String) : type = struct { this.id `xml:\"id\"`; }")
	if len(errors) == 0 {
		t.Fatal("expected malformed struct tag error")
	}
}
