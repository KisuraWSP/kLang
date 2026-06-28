package file

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckForExistingFile(t *testing.T) {
	path := writeTestFile(t, t.TempDir(), "example.klang", "load_as_script;\nfunction Main() : Int { return 0; }")

	if !FileExists(path) {
		t.Error("This file exists")
	}
}

func TestCheckForNonExistingFile(t *testing.T) {
	if FileExists(filepath.Join(t.TempDir(), "var.klang")) {
		t.Error("This file Doesn't Exist, but FileExists returned true")
	}
}

func TestLoadProgramReadsStandaloneScript(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "script.klang", "load_as_script;\nfunction One() : Int { return 1; }")

	program, err := LoadProgram(path)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}

	if program.Name != "script" {
		t.Fatalf("expected script program name, got %q", program.Name)
	}
	if program.Root != root {
		t.Fatalf("expected script root %q, got %q", root, program.Root)
	}
	if program.EntryPoint != path {
		t.Fatalf("expected script entry point %q, got %q", path, program.EntryPoint)
	}
	if len(program.Files) != 1 {
		t.Fatalf("expected script to load one file, got %d", len(program.Files))
	}
}

func TestLoadProgramReadsStandaloneGruaProgram(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "simple.grua", `function Main() : Int {
    local data = {}
    switch data {
        case {}: return 0
        case: return 1
    }
}`)

	program, err := LoadProgram(path)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}
	if program.Name != "simple" || len(program.Files) != 1 {
		t.Fatalf("unexpected Grua program: %#v", program)
	}
	source := program.Files[0]
	if source.Language != LanguageGrua || len(source.OriginalLines) == 0 {
		t.Fatalf("expected original Grua source metadata: %#v", source)
	}
	if !strings.Contains(strings.Join(source.Lines, "\n"), "if data == {") {
		t.Fatalf("expected switch lowering:\n%s", strings.Join(source.Lines, "\n"))
	}
}

func TestLoadProgramRejectsGruaStaticVariableType(t *testing.T) {
	path := writeTestFile(t, t.TempDir(), "invalid.grua", `function Main() : Int {
    local Int value = 0
    return value
}`)

	_, err := LoadProgram(path)
	if err == nil || !strings.Contains(err.Error(), "inferred syntax") {
		t.Fatalf("expected Grua subset error, got %v", err)
	}
}

func TestLoadProgramReadsGruaProjectManifest(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, KlangProjectFile, `name = "grua-project"
entry = "main.grua"
language_version = 1
sources = ["main.grua", "helpers.grua"]
`)
	writeTestFile(t, root, "main.grua", "function Main() : Int {\n    return Helper()\n}")
	writeTestFile(t, root, "helpers.grua", "function Helper() : Int {\n    return 0\n}")

	program, err := LoadProgram(root)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}
	if len(program.Files) != 2 || filepath.Ext(program.EntryPoint) != ".grua" {
		t.Fatalf("unexpected Grua project: %#v", program)
	}
}

func TestLoadProgramRejectsStandaloneScriptWithoutLoadAsScript(t *testing.T) {
	root := t.TempDir()
	path := writeTestFile(t, root, "script.klang", "function One() : Int { return 1; }")

	if _, err := LoadProgram(path); err == nil {
		t.Fatal("expected standalone script without load_as_script to fail")
	}
}

func TestLoadProgramReadsProjectManifest(t *testing.T) {
	root := t.TempDir()
	programDir := filepath.Join(root, "test3")
	writeTestFile(t, programDir, KlangProjectFile, `name = "custom"
entry = "first.klang"
language_version = 1
sources = ["first.klang", "math.klang"]
`)
	writeTestFile(t, programDir, "first.klang", "function Main() : Int { return 0; }")
	writeTestFile(t, programDir, "math.klang", "namespace math {}")

	program, err := LoadProgram(programDir)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}

	if program.Name != "custom" {
		t.Fatalf("expected manifest program name, got %q", program.Name)
	}
	if program.Root != programDir {
		t.Fatalf("expected directory root %q, got %q", programDir, program.Root)
	}
	if filepath.Base(program.EntryPoint) != KlangEntryPoint {
		t.Fatalf("expected first.klang entry point, got %q", program.EntryPoint)
	}
	if len(program.Files) != 2 {
		t.Fatalf("expected directory program to include imported sibling files, got %d", len(program.Files))
	}
	if filepath.Base(program.Files[0].Path) != KlangEntryPoint {
		t.Fatalf("expected entry point to be read first, got %q", program.Files[0].Path)
	}
}

func TestLoadProgramRejectsFutureLanguageVersion(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, KlangProjectFile, `name = "future"
entry = "first.klang"
language_version = 99
`)
	writeTestFile(t, root, KlangEntryPoint, "function Main() : Int { return 0; }")

	_, err := LoadProgram(root)
	if err == nil || !strings.Contains(err.Error(), "supports up to") {
		t.Fatalf("expected unsupported language version error, got %v", err)
	}
}

func TestLoadProgramIgnoresGeneratedDistFolder(t *testing.T) {
	root := t.TempDir()
	programDir := filepath.Join(root, "web")
	writeTestFile(t, programDir, KlangProjectFile, `name = "web"
entry = "first.klang"
`)
	writeTestFile(t, programDir, "first.klang", "function Main() : Int { return 0; }")
	writeTestFile(t, programDir, "app.klang", "namespace App {}")
	writeTestFile(t, programDir, filepath.Join("dist", "web-wasm", "src", "first.klang"), "function Main() : Int { return 1; }")
	writeTestFile(t, programDir, filepath.Join("dist", "web-wasm", "src", "app.klang"), "namespace App { function Start() : Int { return 1; } }")

	program, err := LoadProgram(programDir)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}
	if len(program.Files) != 2 {
		t.Fatalf("expected generated dist sources to be ignored, got %d file(s): %#v", len(program.Files), program.Files)
	}
}

func TestLoadProgramRejectsFolderWithoutProjectManifest(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, filepath.Join("not-a-program", "helper.klang"), "function Helper() : Int { return 0; }")

	_, err := LoadProgram(filepath.Join(root, "not-a-program"))
	if err == nil {
		t.Fatal("expected LoadProgram to reject a folder without klang.project")
	}
}

func TestDiscoverProgramsReadsExactFileAndFolderStructure(t *testing.T) {
	testsDir := t.TempDir()
	writeTestFile(t, testsDir, "test1.klang", "load_as_script;\nfunction One() : Int { return 1; }")
	writeTestFile(t, testsDir, "test2.klang", "load_as_script;\nfunction Two() : Int { return 2; }")
	writeTestFile(t, testsDir, filepath.Join("test3", KlangProjectFile), `name = "test3"
entry = "first.klang"
sources = ["first.klang", "math.klang"]
`)
	writeTestFile(t, testsDir, filepath.Join("test3", "first.klang"), "function Main() : Int { return 0; }")
	writeTestFile(t, testsDir, filepath.Join("test3", "math.klang"), "namespace math {}")
	writeTestFile(t, testsDir, filepath.Join("not-a-program", "helper.klang"), "function Helper() : Int { return 0; }")
	writeTestFile(t, testsDir, "notes.txt", "ignored")

	programs, err := DiscoverPrograms(testsDir)
	if err != nil {
		t.Fatalf("DiscoverPrograms returned an error: %v", err)
	}

	if len(programs) != 3 {
		t.Fatalf("expected 3 programs, got %d", len(programs))
	}

	if programs[0].Name != "test1" || len(programs[0].Files) != 1 {
		t.Fatalf("expected first standalone program, got %#v", programs[0])
	}

	directoryProgram := programs[2]
	if directoryProgram.Name != "test3" {
		t.Fatalf("expected directory program test3, got %q", directoryProgram.Name)
	}
	if filepath.Base(directoryProgram.EntryPoint) != KlangEntryPoint {
		t.Fatalf("expected first.klang entry point, got %q", directoryProgram.EntryPoint)
	}
	if len(directoryProgram.Files) != 2 {
		t.Fatalf("expected directory program to include imported sibling files, got %d", len(directoryProgram.Files))
	}
	if filepath.Base(directoryProgram.Files[0].Path) != KlangEntryPoint {
		t.Fatalf("expected entry point to be read first, got %q", directoryProgram.Files[0].Path)
	}
}

func writeTestFile(t *testing.T, root string, name string, content string) string {
	t.Helper()

	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create test directory: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	return path
}
