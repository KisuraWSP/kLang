package file

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckForExistingFile(t *testing.T) {
	path := writeTestFile(t, t.TempDir(), "example.klang", "function Main() : Int { return 0; }")

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
	path := writeTestFile(t, root, "script.klang", "function One() : Int { return 1; }")

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

func TestLoadProgramReadsFolderWithFirstEntryPoint(t *testing.T) {
	root := t.TempDir()
	programDir := filepath.Join(root, "test3")
	writeTestFile(t, programDir, "first.klang", "function Main() : Int { return 0; }")
	writeTestFile(t, programDir, "math.klang", "namespace math {}")

	program, err := LoadProgram(programDir)
	if err != nil {
		t.Fatalf("LoadProgram returned an error: %v", err)
	}

	if program.Name != "test3" {
		t.Fatalf("expected directory program name, got %q", program.Name)
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

func TestLoadProgramRejectsFolderWithoutFirstEntryPoint(t *testing.T) {
	root := t.TempDir()
	writeTestFile(t, root, filepath.Join("not-a-program", "helper.klang"), "function Helper() : Int { return 0; }")

	_, err := LoadProgram(filepath.Join(root, "not-a-program"))
	if err == nil {
		t.Fatal("expected LoadProgram to reject a folder without first.klang")
	}
}

func TestDiscoverProgramsReadsExactFileAndFolderStructure(t *testing.T) {
	testsDir := t.TempDir()
	writeTestFile(t, testsDir, "test1.klang", "function One() : Int { return 1; }")
	writeTestFile(t, testsDir, "test2.klang", "function Two() : Int { return 2; }")
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
