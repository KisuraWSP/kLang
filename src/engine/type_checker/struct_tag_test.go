package typechecker

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckProgramAcceptsJSONTaggedAliasStruct(t *testing.T) {
	program := programFromSource("" +
		"alias function User(id : String, name : String) : type = struct {\n" +
		"    this.id `json:\"user_id\"`;\n" +
		"}\n" +
		"function Main() : String {\n" +
		"    let user = User(\"42\", \"Ada\");\n" +
		"    local JSON document = JSON(user);\n" +
		"    return json_stringify(user) + json_stringify(document);\n" +
		"}\n")
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected JSON tagged struct to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidJSONStructTags(t *testing.T) {
	program := programFromSource("" +
		"alias function User(id : String) : type = struct {\n" +
		"    this.missing `json:\"id\"`;\n" +
		"    this.id `json:\"id\"`;\n" +
		"}\n")
	report := CheckProgram(program)
	if report.Passed() {
		t.Fatal("expected invalid JSON struct tags to fail")
	}
	if !strings.Contains(fmt.Sprint(report.Errors), "unknown struct field") || !strings.Contains(fmt.Sprint(report.Errors), "already used") {
		t.Fatalf("expected field and duplicate-name diagnostics, got: %v", report.Errors)
	}
}
