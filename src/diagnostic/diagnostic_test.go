package diagnostic

import "testing"

func TestNormalizeSynchronizesStructuredAndCompatibilitySpans(t *testing.T) {
	value := Normalize(Diagnostic{
		Code:    CodeTypeMismatch,
		Primary: Span{File: "main.klang", StartLine: 4, StartColumn: 7, EndLine: 4, EndColumn: 12},
		Message: "bad type",
	})

	if value.Severity != SeverityError {
		t.Fatalf("expected default error severity, got %q", value.Severity)
	}
	if value.File != "main.klang" || value.Line != 4 || value.Column != 7 || value.EndColumn != 12 {
		t.Fatalf("expected compatibility location fields, got %#v", value)
	}
}
