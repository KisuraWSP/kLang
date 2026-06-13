package main

import (
	"errors"
	"os"
	"path/filepath"
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

func TestParseEntrySpecAcceptsNameAndType(t *testing.T) {
	entry, err := parseEntrySpec(`["Process", "String"]`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if entry.Name != "Process" || entry.Type != "String" {
		t.Fatalf("unexpected entry spec: %#v", entry)
	}
}

func TestParseEntrySpecAcceptsNameOnly(t *testing.T) {
	entry, err := parseEntrySpec(`["Process"]`)
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if entry.Name != "Process" || entry.Type != "" {
		t.Fatalf("unexpected entry spec: %#v", entry)
	}
}

func TestCreateProjectUsesCustomEntryPoint(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "custom-entry")
	if err := createProject(projectPath, entrySpec{Name: "Process"}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(projectPath, "app.klang"))
	if err != nil {
		t.Fatalf("read generated app failed: %v", err)
	}
	text := string(source)
	if !strings.Contains(text, "#set_entry_point_to_here") || !strings.Contains(text, "function Process()") {
		t.Fatalf("expected custom entry point in generated app, got:\n%s", text)
	}
}
