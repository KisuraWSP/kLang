package modulesystem

import (
	"os"
	"path/filepath"
	"strings"
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
	if len(resolved.Files) != 3 {
		t.Fatalf("expected already-loaded local import plus inferred random import, got %d files", len(resolved.Files))
	}
	if len(report.Modules) != 2 || report.Modules[0].Kind != ImportLocal || report.Modules[1].Name != "random" {
		t.Fatalf("expected local module report plus inferred random module, got %#v", report.Modules)
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

func TestResolveProgramRawLangDisablesStdlibImports(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "mathg.klang", `function Sqrt(value : Int) : Int { return value; }`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "mathg";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolver := NewResolver(stdlibRoot)
	resolver.DisableStdlib = true
	_, report := resolver.ResolveProgram(program)
	if report.Passed() {
		t.Fatal("expected raw-lang stdlib import to fail")
	}
	if !strings.Contains(report.Errors[0].Message, "raw-lang mode") {
		t.Fatalf("expected raw-lang error, got %#v", report.Errors)
	}
}

func TestResolveProgramRejectsDisabledModule(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "sealed.klang", `
module(disabled : True);
namespace sealed {
    function Open() : Int { return 1; }
}
`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `import "sealed";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	_, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if report.Passed() {
		t.Fatal("expected disabled module import to fail")
	}
	if !strings.Contains(report.Errors[0].Message, "disabled") {
		t.Fatalf("expected disabled module error, got %#v", report.Errors)
	}
}

func TestResolveProgramFiltersStdlibFunctionsByModuleCalls(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "tools.klang", `
namespace tools {
    function Used() : Int { return Helper(); }
    function Helper() : Int { return 7; }
    function Unused() : Int { return 99; }
}
`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
import "tools";
function Main() : Int {
    return tools.Used();
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	filter := resolved.Files[1].ModuleFunctionFilter
	if !filter["tools.Used"] || !filter["tools.Helper"] {
		t.Fatalf("expected used function and helper in filter, got %#v", filter)
	}
	if filter["tools.Unused"] {
		t.Fatalf("did not expect unused function in filter, got %#v", filter)
	}
}

func TestResolveProgramModuleCallerLoadsEntireStdlibModule(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "tools.klang", `
namespace tools {
    function Used() : Int { return 1; }
    function Unused() : Int { return 2; }
}
`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
module_caller(call_entire_module : True);
import "tools";
function Main() : Int {
    return tools.Used();
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	if resolved.Files[1].ModuleFunctionFilter != nil {
		t.Fatalf("expected full module import to have no function filter, got %#v", resolved.Files[1].ModuleFunctionFilter)
	}
}

func TestResolveProgramInfersStdlibImportFromModuleCall(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "tools.klang", `
namespace tools {
    function Used() : Int { return Helper(); }
    function Helper() : Int { return 7; }
    function Unused() : Int { return 99; }
}
`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
function Main() : Int {
    return tools.Used();
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected inferred stdlib import to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected inferred stdlib import to add one file, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 1 || report.Modules[0].Name != "tools" || report.Modules[0].Kind != ImportStdlib {
		t.Fatalf("expected inferred stdlib module report, got %#v", report.Modules)
	}
	filter := resolved.Files[1].ModuleFunctionFilter
	if !filter["tools.Used"] || !filter["tools.Helper"] || filter["tools.Unused"] {
		t.Fatalf("unexpected inferred stdlib function filter: %#v", filter)
	}
}

func TestResolveProgramInfersLocalImportFromModuleCall(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "app")
	writeModuleTestFile(t, appRoot, "math.klang", `
namespace math {
    function Add(left : Int, right : Int) : Int { return left + right; }
}
`)
	programPath := writeModuleTestFile(t, appRoot, "main.klang", `
function Main() : Int {
    return math.Add(1, 2);
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected inferred local import to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected inferred local import to add one file, got %d", len(resolved.Files))
	}
	if len(report.Modules) != 1 || report.Modules[0].Name != "math" || report.Modules[0].Kind != ImportLocal {
		t.Fatalf("expected inferred local module report, got %#v", report.Modules)
	}
}

func TestResolveProgramDoesNotInferMissingModuleFromOrdinarySelectorCall(t *testing.T) {
	root := t.TempDir()
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
function Main() : Int {
    local Table data = {"run": 1};
    data.run();
    return 0;
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(filepath.Join(root, "stdlib")).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected unresolved selector module inference to be ignored, got %#v", report.Errors)
	}
	if len(resolved.Files) != 1 || len(report.Modules) != 0 {
		t.Fatalf("did not expect inferred import, got files=%d modules=%#v", len(resolved.Files), report.Modules)
	}
}

func TestResolveProgramAutoLoadsStdlibGlobalNamespace(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	writeModuleTestFile(t, stdlibRoot, "alloc.klang", `
global namespace alloc {
    function New() : Int { return 7; }
    function Helper() : Int { return 1; }
}
namespace hidden {
    function Secret() : Int { return 99; }
}
`)
	programPath := writeModuleTestFile(t, filepath.Join(root, "app"), "main.klang", `
function Main() : Int {
    return New();
}
`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolved, report := NewResolver(stdlibRoot).ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected module resolution to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected global namespace stdlib file to be loaded, got %d file(s)", len(resolved.Files))
	}
	filter := resolved.Files[1].ModuleFunctionFilter
	if !filter["alloc.New"] || !filter["alloc.Helper"] {
		t.Fatalf("expected global namespace functions in filter, got %#v", filter)
	}
	if filter["hidden.Secret"] {
		t.Fatalf("did not expect non-global namespace function in filter, got %#v", filter)
	}
}

func TestResolveProgramRawLangStillAllowsLocalImports(t *testing.T) {
	root := t.TempDir()
	appRoot := filepath.Join(root, "app")
	writeModuleTestFile(t, appRoot, "helper.klang", `function Helper() : Int { return 1; }`)
	programPath := writeModuleTestFile(t, appRoot, "main.klang", `import "helper";`)

	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("failed to load test program: %v", err)
	}

	resolver := NewResolver(filepath.Join(root, "stdlib"))
	resolver.DisableStdlib = true
	resolved, report := resolver.ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("expected raw-lang local import to pass, got %#v", report.Errors)
	}
	if len(resolved.Files) != 2 {
		t.Fatalf("expected local import to be loaded, got %d file(s)", len(resolved.Files))
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
