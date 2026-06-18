package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	langcontext "kLang/src/engine/context"
	"kLang/src/engine/file"
)

func TestRuntimeErrorPartsExtractsLineColumnAndMessage(t *testing.T) {
	line, column, message := langcontext.RuntimeErrorParts(errors.New("runtime failed: line 3:9: cannot assign String to Int"))

	if line != 3 || column != 9 || message != "cannot assign String to Int" {
		t.Fatalf("unexpected runtime parts: %d %d %q", line, column, message)
	}
}

func TestHumanTypeMessageAddsHelpfulContext(t *testing.T) {
	message := langcontext.HumanTypeMessage("cannot assign String to local Int value")

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

func TestRunCLIPackagesProjectWithManifest(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "packaged")
	outPath := filepath.Join(root, "out")
	if err := createProject(projectPath, entrySpec{}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	if err := runCLI([]string{"package", projectPath, "--backend=Standalone", "--out", outPath}); err != nil {
		t.Fatalf("package command failed: %v", err)
	}

	manifestPath := filepath.Join(outPath, "packaged-standalone", "klang-build.json")
	manifest, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest failed: %v", err)
	}
	text := string(manifest)
	if !strings.Contains(text, `"backend": "Standalone"`) || !strings.Contains(text, `"number_of_files": 2`) {
		t.Fatalf("unexpected package manifest:\n%s", text)
	}
	if _, err := os.Stat(filepath.Join(outPath, "packaged-standalone", "src", "first.klang")); err != nil {
		t.Fatalf("expected bundled first.klang: %v", err)
	}
}

func TestParseDocSourceFilesAcceptsListSyntax(t *testing.T) {
	files, err := parseDocSourceFiles(`["test.klang", "file.klang"]`)
	if err != nil {
		t.Fatalf("parse source files failed: %v", err)
	}
	if strings.Join(files, ",") != "test.klang,file.klang" {
		t.Fatalf("unexpected source files: %#v", files)
	}
}

func TestRunCLIGeneratesDocumentationHTML(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "api.klang")
	outPath := filepath.Join(root, "docs.html")
	source := `
import "math";

global Int Version = 1;

namespace api {
    enum Status { Ready; Done; }

    function Add(left : Int, right : Int) : Int {
        return left + right;
    }
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	if err := runCLI([]string{"doc", "--sourcefile=[" + sourcePath + "]", "--out", outPath}); err != nil {
		t.Fatalf("doc command failed: %v", err)
	}

	html, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated docs failed: %v", err)
	}
	text := string(html)
	for _, expected := range []string{"Klang Source Documentation", "api.Add", "enum api.Status", "global Int Version"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected generated docs to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestParsePackageOptionsAcceptsServeHostAndPort(t *testing.T) {
	options, err := parsePackageOptions([]string{"--backend=WASM", "--serve", "--host", "0.0.0.0", "--port=9090"})
	if err != nil {
		t.Fatalf("parse package options failed: %v", err)
	}
	if !options.Serve || options.Host != "0.0.0.0" || options.Port != 9090 || !options.PortSet || options.Backend != "WASM" {
		t.Fatalf("unexpected package options: %#v", options)
	}
}

func TestPackageServeForcesWASMBackend(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "served")
	outPath := filepath.Join(root, "out")
	if err := createProject(projectPath, entrySpec{}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	options := packageOptions{Backend: "Standalone", Out: outPath, Serve: true, Port: -1, PortSet: true}
	err := packageProgram(mustLoadProgram(t, projectPath), options, commandOptions{})
	if err == nil || !strings.Contains(err.Error(), "port must be between") {
		t.Fatalf("expected invalid port after WASM package generation, got %v", err)
	}

	manifest, readErr := os.ReadFile(filepath.Join(outPath, "served-wasm", "klang-build.json"))
	if readErr != nil {
		t.Fatalf("read wasm manifest failed: %v", readErr)
	}
	if !strings.Contains(string(manifest), `"backend": "WASM"`) {
		t.Fatalf("expected serve package to force WASM backend, got:\n%s", manifest)
	}
}

func mustLoadProgram(t *testing.T, path string) file.Program {
	t.Helper()
	program, err := file.LoadProgram(path)
	if err != nil {
		t.Fatalf("load program failed: %v", err)
	}
	return program
}
