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

func TestResolveProgramLoadsDirectoryModuleWithEntryPoint(t *testing.T) {
	root := t.TempDir()
	moduleRoot := filepath.Join(root, "stdlib", "crypto")
	writeModuleTestFile(t, moduleRoot, "first.klang", `function Hash(value : String) : String { return value; }`)
	writeModuleTestFile(t, moduleRoot, "helper.klang", `function Salt() : String { return "salt"; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "crypto";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected directory module resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 3 {
		t.Fatalf("expected directory module to add two source files, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 1 || report.Modules[0].Kind != ImportStdlib {
		t.Fatalf("expected one stdlib directory module report, got %#v", report.Modules)
	}
}

func TestResolveProgramRecursivelyLoadsImportedModuleImports(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "a.klang", `import "b"; function A() : Int { return B(); }`)
	writeModuleTestFile(t, stdlibRoot, "b.klang", `function B() : Int { return 1; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "a";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected recursive module resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 3 {
		t.Fatalf("expected recursive imports to add two source files, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 2 {
		t.Fatalf("expected two module reports, got %#v", report.Modules)
	}
}

func TestResolveProgramReportsDuplicateImportOnce(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "mathg.klang", `function Sqrt(value : Int) : Int { return value; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
import "mathg";
import "mathg";
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected duplicate import resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected duplicate import to add one source file, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 1 {
		t.Fatalf("expected duplicate import to be reported once, got %#v", report.Modules)
	}
}

func TestResolveProgramDetectsImportCycle(t *testing.T) {
	root := t.TempDir()
	writeModuleTestFile(t, root, "main.klang", `import "helper";`)
	writeModuleTestFile(t, root, "helper.klang", `import "main";`)

	program, err := file.LoadProgram(filepath.Join(root, "main.klang"))
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	_, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if report.Passed() {
		t.Fatal("expected import cycle to fail module resolution")
	}
}

func TestResolverCachesImportsWithoutLeakingVisitedState(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "mathg.klang", `function Sqrt(value : Int) : Int { return value; }`)
	firstProgramPath := writeModuleTestFile(t, filepath.Join(root, "first"), "main.klang", `import "mathg";`)
	secondProgramPath := writeModuleTestFile(t, filepath.Join(root, "second"), "main.klang", `import "mathg";`)

	firstProgram, err := file.LoadProgram(firstProgramPath)
	if err != nil {
		t.Fatalf("failed to load first test program: %v", err)
	}
	secondProgram, err := file.LoadProgram(secondProgramPath)
	if err != nil {
		t.Fatalf("failed to load second test program: %v", err)
	}

	resolver := NewResolver(stdlibRoot)
	firstResolved, firstReport := resolver.ResolveProgram(firstProgram)
	if !firstReport.Passed() {
		t.Fatalf("expected first module resolution to pass, got %#v", firstReport.Errors)
	}
	secondResolved, secondReport := resolver.ResolveProgram(secondProgram)
	if !secondReport.Passed() {
		t.Fatalf("expected second module resolution to pass, got %#v", secondReport.Errors)
	}

	if len(firstResolved.Files) != 2 || len(secondResolved.Files) != 2 {
		t.Fatalf("expected both programs to receive imported source, got %d and %d", len(firstResolved.Files), len(secondResolved.Files))
	}
	if len(firstReport.Modules) != 1 || len(secondReport.Modules) != 1 {
		t.Fatalf("expected both reports to include the import, got %#v and %#v", firstReport.Modules, secondReport.Modules)
	}
	stats := resolver.Stats()
	if stats.ProgramEntries != 1 {
		t.Fatalf("expected shared resolver to cache one imported program, got %d", stats.ProgramEntries)
	}
	if stats.ImportEntries != 3 {
		t.Fatalf("expected import cache for two mains and one stdlib module, got %d", stats.ImportEntries)
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
