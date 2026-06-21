package main

import (
	"errors"
	"os"
	"os/exec"
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

func TestRunCLIPackagesRealJavaScriptBackend(t *testing.T) {
	root := t.TempDir()
	sourcePath := filepath.Join(root, "main.klang")
	outPath := filepath.Join(root, "out")
	source := `function Sum(limit : Int) : Int {
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
	source := `function Main() : Int {
    local Map[String, Int] values;
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
	source := `function Main() : Int {
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

func TestRunCLIDocumentationExpandsProjectIntoSourceChapters(t *testing.T) {
	root := t.TempDir()
	projectPath := filepath.Join(root, "project")
	outPath := filepath.Join(root, "project-docs.html")
	if err := os.MkdirAll(projectPath, 0755); err != nil {
		t.Fatalf("create project dir failed: %v", err)
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
	source := `
function TestAssertionStyle() {
    assert 1 + 1 == 2;
}

function TestBoolStyle() : Bool {
    return "klang".count == 5;
}

function TestStatusStyle() : Int {
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
	source := `
function TestOutput() {
    print("golden hello");
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
	source := `
function TestFailure() : Bool {
    return False;
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
}

func mustLoadProgram(t *testing.T, path string) file.Program {
	t.Helper()
	program, err := file.LoadProgram(path)
	if err != nil {
		t.Fatalf("load program failed: %v", err)
	}
	return program
}
