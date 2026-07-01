package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kLang/src/diagnostic"
	langcontext "kLang/src/engine/context"
	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
)

func TestParseDiagnosticFormat(t *testing.T) {
	format, err := parseDiagnosticFormat([]string{"check", "main.klang", "--diagnostic-format=json"})
	if err != nil || format != "json" {
		t.Fatalf("expected JSON diagnostic format, got %q, %v", format, err)
	}
	if _, err := parseDiagnosticFormat([]string{"check", "main.klang", "--diagnostic-format=xml"}); err == nil {
		t.Fatal("expected unsupported diagnostic format to fail")
	}
}

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

func TestPrintDiagnosticUsesRedWhenColorIsForced(t *testing.T) {
	originalNoColor, hadNoColor := os.LookupEnv("NO_COLOR")
	if err := os.Unsetenv("NO_COLOR"); err != nil {
		t.Fatalf("unset NO_COLOR: %v", err)
	}
	defer func() {
		if hadNoColor {
			_ = os.Setenv("NO_COLOR", originalNoColor)
		} else {
			_ = os.Unsetenv("NO_COLOR")
		}
	}()
	t.Setenv("KLANG_COLOR", "always")

	output := captureDiagnostic(t, langcontext.ErrorContext{
		Code:     diagnostic.CodeTypeMismatch,
		Severity: diagnostic.SeverityError,
		Phase:    langcontext.PhaseType,
		File:     "main.klang",
		Line:     2,
		Column:   5,
		Message:  "bad type",
	})
	if !strings.Contains(output, "\x1b[31m") || !strings.Contains(output, "\x1b[0m") {
		t.Fatalf("expected red ANSI rendering, got %q", output)
	}
	if !strings.Contains(output, "[K2101]") {
		t.Fatalf("expected stable diagnostic code, got %q", output)
	}
}

func TestPrintDiagnosticHonorsNoColor(t *testing.T) {
	t.Setenv("KLANG_COLOR", "always")
	t.Setenv("NO_COLOR", "1")

	output := captureDiagnostic(t, langcontext.ErrorContext{
		Code:     diagnostic.CodeSyntax,
		Severity: diagnostic.SeverityError,
		Phase:    langcontext.PhaseParse,
		File:     "main.klang",
		Message:  "bad syntax",
	})
	if strings.Contains(output, "\x1b[") {
		t.Fatalf("expected NO_COLOR output without ANSI escapes, got %q", output)
	}
}

func captureDiagnostic(t *testing.T, value langcontext.ErrorContext) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create diagnostic pipe: %v", err)
	}
	printDiagnostic(writer, value)
	if err := writer.Close(); err != nil {
		t.Fatalf("close diagnostic writer: %v", err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read diagnostic output: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close diagnostic reader: %v", err)
	}
	return string(content)
}

func TestMascotRendersFriendlySuccessAndErrorMessages(t *testing.T) {
	t.Setenv("KLANG_MASCOT", "always")

	success := captureMascot(t, mascotSuccess)
	if !strings.Contains(success, "[^_^]") ||
		!strings.Contains(success, "Kibi: Nice work! Your program finished safely.") {
		t.Fatalf("unexpected success mascot: %q", success)
	}

	failure := captureMascot(t, mascotError)
	if !strings.Contains(failure, "[o_o]") ||
		!strings.Contains(failure, "the diagnostic above points the way") {
		t.Fatalf("unexpected error mascot: %q", failure)
	}
}

func TestMascotCanBeDisabled(t *testing.T) {
	t.Setenv("KLANG_MASCOT", "never")
	if output := captureMascot(t, mascotHelp); output != "" {
		t.Fatalf("expected disabled mascot to stay silent, got %q", output)
	}
}

func captureMascot(t *testing.T, mood mascotMood) string {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create mascot pipe: %v", err)
	}
	printMascot(writer, mood)
	if err := writer.Close(); err != nil {
		t.Fatalf("close mascot writer: %v", err)
	}
	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read mascot output: %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("close mascot reader: %v", err)
	}
	return string(content)
}

func TestParseEntryFlagIsDeprecatedNoop(t *testing.T) {
	entry, err := parseEntryFlag([]string{`--entry=["Process", "String"]`})
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}
	if entry.Name != "" || entry.Type != "" {
		t.Fatalf("deprecated entry flag should be ignored, got: %#v", entry)
	}
}

func TestDeprecatedEntryFlagDetected(t *testing.T) {
	if !hasDeprecatedEntryFlag([]string{`--entry=["Process", "Int"]`}) {
		t.Fatalf("expected --entry= form to be detected")
	}
	if !hasDeprecatedEntryFlag([]string{"--entry", `["Process"]`}) {
		t.Fatalf("expected --entry separated form to be detected")
	}
}

func TestCreateProjectIgnoresCustomEntryPoint(t *testing.T) {
	projectPath := filepath.Join(t.TempDir(), "custom-entry")
	if err := createProject(projectPath, entrySpec{Name: "Process"}); err != nil {
		t.Fatalf("create project failed: %v", err)
	}

	source, err := os.ReadFile(filepath.Join(projectPath, "app.klang"))
	if err != nil {
		t.Fatalf("read generated app failed: %v", err)
	}
	text := string(source)
	if strings.Contains(text, "#set_entry_point_to_here") || strings.Contains(text, "function Process()") {
		t.Fatalf("deprecated custom entry point should not affect generated app, got:\n%s", text)
	}
	if !strings.Contains(text, "function Start() : Int") {
		t.Fatalf("expected fixed App.Start implementation hook, got:\n%s", text)
	}

	entrySource, err := os.ReadFile(filepath.Join(projectPath, file.KlangEntryPoint))
	if err != nil {
		t.Fatalf("read generated entry failed: %v", err)
	}
	entryText := string(entrySource)
	if !strings.Contains(entryText, "function Main() : Int") || !strings.Contains(entryText, "return App.Start();") {
		t.Fatalf("expected fixed Main wrapper, got:\n%s", entryText)
	}

	manifest, err := os.ReadFile(filepath.Join(projectPath, file.KlangProjectFile))
	if err != nil {
		t.Fatalf("read generated manifest failed: %v", err)
	}
	if !strings.Contains(string(manifest), "language_version = 1") {
		t.Fatalf("expected generated project language version, got:\n%s", manifest)
	}
}

func TestRunCLIUpdatesAndChecksLegacyProject(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, file.KlangEntryPoint),
		[]byte("function Main() : Int { return 0; }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := runCLI([]string{"update", root}); err != nil {
		t.Fatalf("update command failed: %v", err)
	}
	manifest, err := os.ReadFile(filepath.Join(root, file.KlangProjectFile))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), "language_version = 1") {
		t.Fatalf("expected migrated language version:\n%s", manifest)
	}
}

func TestRunCLIUpdateReportsBreakingEntrypoint(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, file.KlangEntryPoint),
		[]byte("function Start() : Int { return 0; }\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	err := runCLI([]string{"update", root})
	if err == nil || !strings.Contains(err.Error(), "breaking changes remain") {
		t.Fatalf("expected breaking-change report, got %v", err)
	}
}

func TestProgramLineThroughputMetrics(t *testing.T) {
	program := file.Program{
		Files: []file.SourceFile{
			{Path: "first.klang", Lines: []string{"function Main() : Int {", "    return App.Start();", "}"}},
			{Path: "app.klang", Lines: []string{"namespace App {", "    function Start() : Int { return 0; }", "}"}},
		},
	}

	if got := countSourceLines(program); got != 6 {
		t.Fatalf("expected 6 source lines, got %d", got)
	}
	if got := linesPerSecond(6, 2*time.Second); got != 3 {
		t.Fatalf("expected 3 lines per second, got %f", got)
	}
	if got := linesPerSecond(6, 0); got != 0 {
		t.Fatalf("expected zero elapsed throughput to be 0, got %f", got)
	}

	metrics := sourceProcessingMetrics{
		Load:      10 * time.Millisecond,
		Cache:     5 * time.Millisecond,
		Resolve:   20 * time.Millisecond,
		TypeCheck: 30 * time.Millisecond,
		Parse:     15 * time.Millisecond,
	}
	if got := metrics.Elapsed(); got != 80*time.Millisecond {
		t.Fatalf("expected measured source phases to total 80ms, got %s", got)
	}
}

func TestRunCLIExecutesGruaSubsetProgram(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "program.grua")
	source := `import "basic"

function Identity(value::Int) : Int {
    return value
}

function Main() : Int {
    local immutable = {}
    local mut state = {"total": 0}
    val shared = {}
    var changing = {}

    local mut forever = 0
    for {
        forever += 1
        break
    }
    for forever < 2 {
        forever += 1
    }
    for index:=0, index<3, index+=1 {
        state["total"] += index
    }
    for entry in state {
        _ = entry
    }

    local total = state["total"]
    switch total {
        case 3:
        basic.PRINT("grua ok")
            return Identity(0)
        case:
            return 1
    }
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runCLI([]string{"run", sourcePath}); err != nil {
		t.Fatalf("Grua run failed: %v", err)
	}
}

func TestRunCLIPackagesOriginalGruaSource(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "package.grua")
	outPath := filepath.Join(root, "out")
	source := `function Main() : Int {
    switch 0 {
        case 0: return 0
        case: return 1
    }
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runCLI([]string{"package", sourcePath, "--backend=Standalone", "--out", outPath}); err != nil {
		t.Fatalf("Grua package failed: %v", err)
	}
	bundledPath := filepath.Join(outPath, "package-standalone", "src", "package.grua")
	bundled, err := os.ReadFile(bundledPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(bundled), "switch 0 {") || strings.Contains(string(bundled), "if 0 == {") {
		t.Fatalf("package leaked lowered kLang instead of original Grua:\n%s", bundled)
	}
}

func TestRunCLIChecksGruaFileStdlib(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "file_api.grua")
	source := `import "file"

function Main() : Int {
    local descriptor = file.OPEN("example.txt")
    switch descriptor.path {
        case "example.txt": return 0
        case: return 1
    }
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCLI([]string{"check", sourcePath}); err != nil {
		t.Fatalf("Grua file stdlib check failed: %v", err)
	}
}

func TestRunCLIRejectsInferredDisallowedGruaStdlib(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "restricted.grua")
	source := `function Main() : Int {
    local value = strings.Trim(" grua ")
    _ = value
    return 0
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runCLI([]string{"check", sourcePath}); err == nil {
		t.Fatal("expected inferred strings stdlib import to be rejected for Grua")
	}
}

func TestGruaResolverUsesOnlyGruaStdlibModules(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "stdlib.grua")
	source := `import "basic"
import "file"
import "io"
import "repl"

function Main() : Int {
    local data = basic.SET({}, "ready", True)
    local writer = io.WRITE(io.WRITER(), basic.STRING(data["ready"]))
    local handle = file.OPEN("example.txt")
    local session = repl.NEW_SESSION()
    _ = writer
    _ = handle
    _ = session
    return 0
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	program, err := file.LoadProgram(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	resolved, report := modulesystem.NewResolver("stdlib").ResolveProgram(program)
	if !report.Passed() {
		t.Fatalf("Grua module resolution failed: %#v", report.Errors)
	}
	if len(report.Modules) != 4 {
		t.Fatalf("expected four Grua stdlib modules, got %#v", report.Modules)
	}
	for _, module := range report.Modules {
		if filepath.Ext(module.Path) != ".grua" ||
			filepath.Base(filepath.Dir(module.Path)) != "grua" {
			t.Fatalf("Grua resolved a non-Grua stdlib module: %#v", module)
		}
	}
	for index, source := range resolved.Files {
		if strings.Contains(filepath.ToSlash(source.Path), "stdlib/grua/") &&
			source.Language != file.LanguageGrua {
			t.Fatalf("Grua stdlib source lost its dialect: %#v", source)
		}
		if strings.Contains(filepath.ToSlash(source.Path), "stdlib/grua/") {
			resolved.Files[index].ModuleFunctionFilter = nil
		}
	}
	if report := typechecker.CheckProgram(resolved); !report.Passed() {
		t.Fatalf("Grua stdlib type check failed: %#v", report.Errors)
	}
}

func TestKlangResolverKeepsKlangStdlibModules(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "stdlib.klang")
	source := `load_as_script;
import "basic";

function Main() : Int {
    return basic.Print("klang");
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	program, err := file.LoadProgram(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	_, report := modulesystem.NewResolver("stdlib").ResolveProgram(program)
	if !report.Passed() || len(report.Modules) != 1 {
		t.Fatalf("kLang module resolution failed: %#v", report)
	}
	module := report.Modules[0]
	if filepath.Ext(module.Path) != file.KlangExtension ||
		filepath.Base(filepath.Dir(module.Path)) == "grua" {
		t.Fatalf("kLang resolved the Grua stdlib module: %#v", module)
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

func TestRunCLIPackagesRealJavaScriptBackend(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	outPath := filepath.Join(root, "out")
	source := `load_as_script;

function Sum(limit : Int) : Int {
    local mut Int total = 0;
    local mut Int index = 0;
    while index < limit {
        total += index;
        index += 1;
    }
    return total;
}

function Main() : Int {
    local Int result = Sum(5);
    print(result);
    return result;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	if err := runCLI([]string{"package", sourcePath, "--backend=JS", "--out", outPath}); err != nil {
		t.Fatalf("JS package failed: %v", err)
	}
	bundle := filepath.Join(outPath, "main-js")
	generated, err := os.ReadFile(filepath.Join(bundle, "program.js"))
	if err != nil {
		t.Fatalf("read generated JavaScript: %v", err)
	}
	if !strings.Contains(string(generated), "function k_Sum") || strings.Contains(string(generated), "klang_browser") {
		t.Fatalf("expected native JavaScript output, got:\n%s", generated)
	}
	generatedMap, err := os.ReadFile(filepath.Join(bundle, "program.js.map"))
	if err != nil {
		t.Fatalf("read generated JavaScript source map: %v", err)
	}
	if !strings.Contains(string(generated), "sourceMappingURL=program.js.map") || !strings.Contains(string(generatedMap), "\"version\":3") {
		t.Fatalf("expected linked Source Map v3 output:\n%s\n%s", generated, generatedMap)
	}
	manifest, err := os.ReadFile(filepath.Join(bundle, "klang-build.json"))
	if err != nil {
		t.Fatalf("read JS manifest: %v", err)
	}
	manifestText := string(manifest)
	for _, expected := range []string{`"backend": "JS"`, `"backend_mode": "native-codegen"`, `"backend_status": "experimental"`, `"entry": "program.js"`, `"program.js.map"`} {
		if !strings.Contains(manifestText, expected) {
			t.Fatalf("manifest missing %s:\n%s", expected, manifestText)
		}
	}
}

func TestRunCLIRejectsUnsupportedJavaScriptBackendFeature(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "unsupported.klang")
	source := `load_as_script;

function Main() : Int {
    local Set[String] values;
    return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	err := runCLI([]string{"package", sourcePath, "--backend=JS", "--out", filepath.Join(root, "out")})
	if err == nil || !strings.Contains(err.Error(), "JS backend check failed") {
		t.Fatalf("expected JS subset rejection, got %v", err)
	}
}

func TestRunCLIPackagesJavaScriptWithImportedNamespaceModule(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "modules")
	outPath := filepath.Join(root, "out")
	if err := os.MkdirAll(project, 0755); err != nil {
		t.Fatalf("create project: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, file.KlangProjectFile), []byte(`name = "modules"
entry = "first.klang"
sources = ["first.klang", "math.klang"]
`), 0644); err != nil {
		t.Fatalf("write project manifest: %v", err)
	}
	mainSource := `import "math";

function Main() : Int {
    local Int result = math.Add(20, 22);
    print(result);
    return result;
}
`
	mathSource := `namespace math {
    function Add(left : Int, right : Int) : Int {
        return left + right;
    }
}
`
	if err := os.WriteFile(filepath.Join(project, "first.klang"), []byte(mainSource), 0644); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "math.klang"), []byte(mathSource), 0644); err != nil {
		t.Fatalf("write module: %v", err)
	}
	if err := runCLI([]string{"package", project, "--backend=JS", "--out", outPath}); err != nil {
		t.Fatalf("package imported JS module: %v", err)
	}
	bundle := filepath.Join(outPath, "modules-js")
	generated, err := os.ReadFile(filepath.Join(bundle, "program.js"))
	if err != nil {
		t.Fatalf("read generated module JavaScript: %v", err)
	}
	if !strings.Contains(string(generated), "function k_math_u2e_Add") || !strings.Contains(string(generated), "k_math_u2e_Add(__klang_copy(20), __klang_copy(22))") {
		t.Fatalf("expected flattened imported namespace:\n%s", generated)
	}
	if node, err := exec.LookPath("node"); err == nil {
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "42" {
			t.Fatalf("generated imported module failed: %v\n%s", runErr, printed)
		}
	}
}

func TestRunCLIPackagesJavaScriptStringOperations(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "strings.klang")
	outPath := filepath.Join(root, "out")
	source := `load_as_script;

function Main() : Int {
    local String value = "h😀llo";
    local String message = "len=" + len(value);
    print(message, value.uppercase(), value[1], value.count);
    return len(value);
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write String source: %v", err)
	}
	if err := runCLI([]string{"package", sourcePath, "--backend=JS", "--out", outPath}); err != nil {
		t.Fatalf("package JS String operations: %v", err)
	}
	bundle := filepath.Join(outPath, "strings-js")
	if node, err := exec.LookPath("node"); err == nil {
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "len=5 H😀LLO 😀 5" {
			t.Fatalf("generated JS String package failed: %v\n%s", runErr, printed)
		}
	}
}

func TestRunCLIFormatsSingleFileWithWriteAndCheck(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	source := `function Main():Int{
local Int value=1+2;
return value;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	err := runCLI([]string{"fmt", sourcePath, "--check"})
	if err == nil || !strings.Contains(err.Error(), "need formatting") {
		t.Fatalf("expected fmt --check to fail before formatting, got %v", err)
	}
	if err := runCLI([]string{"fmt", sourcePath, "--write"}); err != nil {
		t.Fatalf("fmt --write failed: %v", err)
	}
	if err := runCLI([]string{"fmt", sourcePath, "--check"}); err != nil {
		t.Fatalf("fmt --check failed after formatting: %v", err)
	}

	formatted, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read formatted source failed: %v", err)
	}
	expected := `function Main() : Int {
    local Int value = 1 + 2;
    return value;
}
`
	if string(formatted) != expected {
		t.Fatalf("unexpected formatted source:\n%s", formatted)
	}
}

func TestRunCLIFormatsFolderDeterministically(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first.klang")
	helperPath := filepath.Join(root, "helper.klang")
	if err := os.WriteFile(firstPath, []byte("function Main():Int{return Helper();}\n"), 0644); err != nil {
		t.Fatalf("write first source failed: %v", err)
	}
	if err := os.WriteFile(helperPath, []byte("function Helper():Int{return 1;}\n"), 0644); err != nil {
		t.Fatalf("write helper source failed: %v", err)
	}

	if err := runCLI([]string{"format", root, "--write"}); err != nil {
		t.Fatalf("format folder failed: %v", err)
	}
	first, err := os.ReadFile(firstPath)
	if err != nil {
		t.Fatalf("read first source failed: %v", err)
	}
	helper, err := os.ReadFile(helperPath)
	if err != nil {
		t.Fatalf("read helper source failed: %v", err)
	}
	if !strings.Contains(string(first), "function Main() : Int { return Helper(); }") {
		t.Fatalf("unexpected formatted first source:\n%s", first)
	}
	if !strings.Contains(string(helper), "function Helper() : Int { return 1; }") {
		t.Fatalf("unexpected formatted helper source:\n%s", helper)
	}
}

func TestRunCLIRequiresModeForSingleFileFolder(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "main.klang"), []byte("function Main() : Int { return 0; }\n"), 0644); err != nil {
		t.Fatalf("write source failed: %v", err)
	}

	err := runCLI([]string{"fmt", root})
	if err == nil || !strings.Contains(err.Error(), "requires --write or --check") {
		t.Fatalf("expected folder mode error, got %v", err)
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
	source := `load_as_script;

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

func TestRunCLIDocumentationExpandsProjectIntoSourceChapters(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	outPath := filepath.Join(root, "project-docs.html")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("create project dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, file.KlangProjectFile), []byte(`name = "project"
entry = "first.klang"
sources = ["first.klang", "app.klang"]
`), 0644); err != nil {
		t.Fatalf("write project manifest failed: %v", err)
	}
	first := `import "app";

function Main() : Int {
    return App.Start();
}
`
	app := `namespace App {
    function Start() : Int {
        let mut here_string = //
<h1>Hello from kLang!</h1>
//;
        return len(here_string);
    }
}
`
	if err := os.WriteFile(filepath.Join(projectPath, "first.klang"), []byte(first), 0644); err != nil {
		t.Fatalf("write first source failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectPath, "app.klang"), []byte(app), 0644); err != nil {
		t.Fatalf("write app source failed: %v", err)
	}

	if err := runCLI([]string{"doc", "--sourcefile=[" + projectPath + "]", "--out", outPath}); err != nil {
		t.Fatalf("doc command failed: %v", err)
	}

	html, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read generated docs failed: %v", err)
	}
	text := string(html)
	for _, expected := range []string{"Source Chapters", "Chapter 1: first.klang", "Chapter 2: app.klang", "&lt;h1&gt;Hello from kLang!&lt;/h1&gt;", "App.Start"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("expected generated docs to contain %q, got:\n%s", expected, text)
		}
	}
}

func TestRunCLIRunsKlangTestFunctions(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "math_test.klang")
	source := `load_as_script;

function TestAssertionStyle() {
    assert 1 + 1 == 2;
}

function TestBoolStyle() : Bool {
    return "klang".count == 5;
}

function TestStatusStyle() : Int {
    return 0;
}

function Main() : Int {
    return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write test source failed: %v", err)
	}

	if err := runCLI([]string{"test", sourcePath}); err != nil {
		t.Fatalf("test command failed: %v", err)
	}
}

func TestRunCLIRunsKlangTestFolderAndGoldenOutput(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "output_test.klang")
	source := `load_as_script;

function TestOutput() {
    print("golden hello");
}

function Main() : Int {
    return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write test source failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "output_test.golden"), []byte("golden hello\n"), 0644); err != nil {
		t.Fatalf("write golden file failed: %v", err)
	}

	if err := runCLI([]string{"test", root}); err != nil {
		t.Fatalf("test command with golden output failed: %v", err)
	}
}

func TestRunCLIFailsKlangBoolTest(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "failing_test.klang")
	source := `load_as_script;

function TestFailure() : Bool {
    return False;
}

function Main() : Int {
    return 0;
}
`
	if err := os.WriteFile(sourcePath, []byte(source), 0644); err != nil {
		t.Fatalf("write test source failed: %v", err)
	}

	err := runCLI([]string{"test", sourcePath})
	if err == nil || !strings.Contains(err.Error(), "one or more Klang tests failed") {
		t.Fatalf("expected failing Klang test error, got %v", err)
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
	if !strings.Contains(string(manifest), `"backend_mode": "bytecode-vm"`) ||
		!strings.Contains(string(manifest), `"bytecode_status": "compiled"`) {
		t.Fatalf("expected WASM bytecode metadata, got:\n%s", manifest)
	}
	bytecodePath := filepath.Join(outPath, "served-wasm", "program.kbc")
	if info, statErr := os.Stat(bytecodePath); statErr != nil || info.Size() == 0 {
		t.Fatalf("expected compiled program.kbc: %v", statErr)
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
