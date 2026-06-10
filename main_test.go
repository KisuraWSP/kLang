package main

import (
	"errors"
	"strings"
	"testing"
)

func TestRuntimeErrorPartsExtractsLineColumnAndMessage(t *testing.T) {
	line, column, message := runtimeErrorParts(errors.New("runtime failed: line 3:9: cannot assign String to Int"))

	if line != 3 || column != 9 || message != "cannot assign String to Int" {
		t.Fatalf("unexpected runtime parts: %d %d %q", line, column, message)
	}
}

func TestHumanTypeMessageAddsHelpfulContext(t *testing.T) {
	message := humanTypeMessage("cannot assign String to local Int value")

	if !strings.Contains(message, "This value does not have the type declared") {
		t.Fatalf("expected helpful type context, got %q", message)
	}
}
