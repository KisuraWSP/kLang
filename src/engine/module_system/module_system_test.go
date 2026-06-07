package modulesystem

import (
	"os"
	"path/filepath"
	"testing"

	"kLang/src/engine/file"
)

func TestResolveProgramLoadsStdlibImportWithoutExtension(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "mathg.klang", `function Sqrt(value : Int) : Int { return value; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "mathg";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected stdlib import to add one source file, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 1 || report.Modules[0].Kind != ImportStdlib {
		t.Fatalf("expected stdlib module report, got %#v", report.Modules)
	}
	if filepath.Base(resolved.Files[1].Path) != "mathg.klang" {
		t.Fatalf("expected mathg.klang import, got %q", resolved.Files[1].Path)
	}
}

func TestResolveProgramLoadsLocalSiblingImport(t *testing.T) {
	root := t.TempDir()
	writeModuleTestFile(t, root, "math.klang", `function Add(a : Int, b : Int) : Int { return a + b; }`)
	programPath := writeModuleTestFile(t, root, "main.klang", `import "math.klang";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected local import to add one source file, got %d", len(resolved.Files))
	}
	if report.Modules[0].Kind != ImportLocal {
		t.Fatalf("expected local import, got %#v", report.Modules[0])
	}
}

func TestResolveProgramPrefersLocalImportOverStdlib(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "mathg.klang", `function FromStdlib() : Int { return 1; }`)
	writeModuleTestFile(t, filepath.Join(root, "app"), "mathg.klang", `function FromLocal() : Int { return 2; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "mathg";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	if report.Modules[0].Kind != ImportLocal {
		t.Fatalf("expected local import to win, got %#v", report.Modules[0])
	}
	if filepath.Dir(resolved.Files[1].Path) != filepath.Join(root, "app") {
		t.Fatalf("expected local module path, got %q", resolved.Files[1].Path)
	}
}

func TestResolveProgramReportsMissingImport(t *testing.T) {
	root := t.TempDir()
	programPath := writeModuleTestFile(t, root, "main.klang", `import "missing";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	_, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if report.Passed() {
		t.Fatal("expected missing import to fail module resolution")
	}
}

func TestResolveProgramKeepsAlreadyLoadedProjectImportDeduped(t *testing.T) {
	program, err := file.LoadProgram(filepath.Join("..", "..", "..", "tests", "test21"))
	if err != nil {
		t.Fatalf("failed to load test21: %v", err)
	}

	resolved, report := NewResolver(filepath.Join("..", "..", "..", "stdlib")).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected test21 imports to resolve, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected already-loaded local import to stay deduped, got %d files", len(resolved.Files))
	}
	if len(report.Modules) != 1 || report.Modules[0].Kind != ImportLocal {
		t.Fatalf("expected one local module report, got %#v", report.Modules)
	}
}

func writeModuleTestFile(t *testing.T, root string, name string, content string) string {
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
