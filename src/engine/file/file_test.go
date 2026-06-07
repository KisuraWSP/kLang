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

func TestDiscoverTestProgramsReadsStandaloneFilesAndEntryPointDirectories(t *testing.T) {
	testsDir := t.TempDir()
	writeTestFile(t, testsDir, "test1.klang", "function One() : Int { return 1; }")
	writeTestFile(t, testsDir, "test2.klang", "function Two() : Int { return 2; }")
	writeTestFile(t, testsDir, filepath.Join("test3", "first.klang"), "function Main() : Int { return 0; }")
	writeTestFile(t, testsDir, filepath.Join("test3", "math.klang"), "namespace math {}")
	writeTestFile(t, testsDir, filepath.Join("not-a-program", "helper.klang"), "function Helper() : Int { return 0; }")
	writeTestFile(t, testsDir, "notes.txt", "ignored")

	programs, err := DiscoverTestPrograms(testsDir)
	if err != nil {
		t.Fatalf("DiscoverTestPrograms returned an error: %v", err)
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
