package runtime

import (
	"strings"
	"testing"
)

func TestJSONParserPreservesNumbersAndSerializesDeterministically(t *testing.T) {
	value, err := parseJSONValue(`{"z":9007199254740993,"a":[true,null,"text"]}`)
	if err != nil {
		t.Fatalf("parse JSON failed: %v", err)
	}

	encoded, err := stringifyJSONValue(value)
	if err != nil {
		t.Fatalf("stringify JSON failed: %v", err)
	}
	if encoded != `{"a":[true,null,"text"],"z":9007199254740993}` {
		t.Fatalf("unexpected JSON serialization: %s", encoded)
	}
}

func TestJSONParserReportsLineAndColumn(t *testing.T) {
	_, err := parseJSONValue("{\n  \"name\":\n}")
	if err == nil || !strings.Contains(err.Error(), "invalid JSON at 3:1") {
		t.Fatalf("expected positioned JSON error, got %v", err)
	}
}
