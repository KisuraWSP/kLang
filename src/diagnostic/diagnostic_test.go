package diagnostic

import (
	"encoding/json"
	"strings"
	"testing"
)

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

func TestDiagnosticJSONUsesStableMachineReadableFields(t *testing.T) {
	encoded, err := json.Marshal(Normalize(Diagnostic{
		Code: "K5001", Severity: SeverityError, Phase: PhaseJS,
		File: "main.klang", Line: 2, Column: 3, EndLine: 2, EndColumn: 7,
		Message: "unsupported", Rule: "backend feature support", FeatureID: "values.set",
	}))
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, field := range []string{
		`"code":"K5001"`, `"feature_id":"values.set"`, `"start_column":3`, `"end_column":7`,
	} {
		if !strings.Contains(text, field) {
			t.Fatalf("diagnostic JSON missing %s: %s", field, text)
		}
	}
}
