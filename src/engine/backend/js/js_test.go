package jsbackend

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/backend"
	"kLang/src/engine/file"
	"kLang/src/parser"
)

func TestJavaScriptBackendEmitsTypedCoreProgram(t *testing.T) {
	request := requestFromSource(`
function Sum(limit : Int) : Int {
    local mut Int total = 0;
    local mut Int index = 0;
    while index < limit {
        if index % 2 == 0 {
            total += index;
        }
        index += 1;
    }
    return total;
}

function Main() : Int {
    local Int result = Sum(6);
    print(result);
    return result;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected JS diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit failed: %v", err)
	}
	if output.Entry != "program.js" || len(output.Artifacts) != 4 {
		t.Fatalf("unexpected JS output: %#v", output)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"function k_Sum", "while (", "k_total = __klang_add(k_total, k_index)", "__klang_print(k_result)", "k_Main();"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated JS missing %q:\n%s", expected, source)
		}
	}
	bundle := t.TempDir()
	if err := compiler.Package(output, bundle); err != nil {
		t.Fatalf("package failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "program.js")); err != nil {
		t.Fatalf("missing program.js: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bundle, "program.js.map")); err != nil {
		t.Fatalf("missing program.js.map: %v", err)
	}
	if node, err := exec.LookPath("node"); err == nil {
		run := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := run.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "6" {
			t.Fatalf("generated JavaScript failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendRejectsRuntimeHeavyFeature(t *testing.T) {
	request := requestFromSource(`
function Main() : Int {
    local Set[String] values;
    return 0;
}
`)
	diagnostics := New().Check(request)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "Set[String]") {
		t.Fatalf("expected focused unsupported-type diagnostic, got %#v", diagnostics)
	}
	if diagnostics[0].File != "main.klang" || diagnostics[0].Line != 2 {
		t.Fatalf("expected source-positioned diagnostic, got %#v", diagnostics[0])
	}
	if diagnostics[0].Rule != "js-backend/unsupported-feature" || diagnostics[0].EndColumn <= diagnostics[0].Column {
		t.Fatalf("expected rich JS diagnostic metadata, got %#v", diagnostics[0])
	}
}

func TestJavaScriptBackendEmitsMapAndTableSemantics(t *testing.T) {
	request := requestFromSource(`
function InitialScores() : Map[String, Int] {
    return {"answer": 42};
}

function Main() : Int {
    local mut Map[String, Int] scores = InitialScores();
    local Map[String, Int] savedScores = scores;
    scores["answer"] = 7;
    scores["extra"] = 5;
    scores["extra"] += 2;

    local mut Table data = {"name": "klang", 0: 9, 1: 10, "1": 20, True: 30, 'x': 40, "count": 99};
    local Table snapshot = data;
    data[1] = 11;
    data = table_delete(data, "name");
    local Table parent = {"fallback": 7};
    local Table child = table_set_fallback(data, parent);
    local List[T] keys = table_keys(child);
    local List[Table] entries = table_entries(child);

    assert table_has(child, 1);
    assert not table_has(child, "name");
    assert 'x' == 'x';
    assert snapshot != data;
    local Int total = data[1] + snapshot[1] + data["1"] + data[True] + data['x'] + len(entries) + entries[0].value + scores["extra"];
    assert total == 133;
    print(savedScores["answer"], scores.count, child.count, child["count"], child.fallback, len(keys), entries[0].key);
    return total;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected Map/Table diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit Map/Table operations: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"__klang_as_map", "__klang_table_from_pairs", "__klang_collection_put", "__klang_table_set_fallback", "__klang_table_entries"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated Map/Table JS missing %q:\n%s", expected, source)
		}
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package Map/Table program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "42 2 6 99 7 6 0" {
			t.Fatalf("generated Map/Table program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendRoundTripsNativeJSONValues(t *testing.T) {
	request := requestFromSource(`
function Main() : Int {
    local Table source = {"name": "Ada", "items": [1, 2], "active": True};
    local Result[String, String] encoded = json_encode(source);
    assert encoded.ok;
    local Result[T, String] decoded = json_decode(encoded.value);
    assert decoded.ok;
    local Table object = decoded.value as Table;
    local List[T] items = object.items as List[T];
    assert object.name == "Ada";
    assert object.active;
    assert items[1] == 2;
    print(encoded.value, json_stringify("native"), json_stringify(42));
    return 0;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected native JSON diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit native JSON operations: %v", err)
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package native JSON program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		expected := "{\"active\":true,\"items\":[1,2],\"name\":\"Ada\"} \"native\" 42"
		if runErr != nil || strings.TrimSpace(string(printed)) != expected {
			t.Fatalf("generated native JSON program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendEnforcesStrictEntryPoint(t *testing.T) {
	request := requestFromSource(`function Main() : String { return "invalid"; }`)
	diagnostics := New().Check(request)
	if len(diagnostics) == 0 || diagnostics[0].Rule != "JS_ENTRY_POINT" ||
		!strings.Contains(diagnostics[0].Message, "function Main() : Int") {
		t.Fatalf("expected strict JS entry-point diagnostic, got %#v", diagnostics)
	}
}

func TestJavaScriptBackendEmitsSourceMapV3(t *testing.T) {
	request := requestFromSource(`
function Double(value : Int) : Int {
    return value * 2;
}

function Main() : Int {
    return Double(21);
}
`)
	output, err := New().Emit(request)
	if err != nil {
		t.Fatalf("emit source map program: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	if !strings.HasSuffix(source, "//# sourceMappingURL=program.js.map\n") {
		t.Fatalf("generated JavaScript is not linked to its source map:\n%s", source)
	}
	var decoded struct {
		Version        int      `json:"version"`
		File           string   `json:"file"`
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
		Mappings       string   `json:"mappings"`
	}
	if err := json.Unmarshal(output.Artifacts[1].Content, &decoded); err != nil {
		t.Fatalf("decode program.js.map: %v", err)
	}
	if decoded.Version != 3 || decoded.File != "program.js" || len(decoded.Sources) != 1 || decoded.Sources[0] != "src/main.klang" {
		t.Fatalf("unexpected source map metadata: %#v", decoded)
	}
	if len(decoded.SourcesContent) != 1 || !strings.Contains(decoded.SourcesContent[0], "function Double") || decoded.Mappings == "" {
		t.Fatalf("source map is missing source content or mappings: %#v", decoded)
	}
}

func TestJavaScriptBackendRendersSourceAwareRuntimeDiagnostic(t *testing.T) {
	request := requestFromSource(`
function Read(values : List[Int]) : Int {
    return values[5];
}

function Main() : Int {
    return Read([1]);
}
`)
	compiler := New()
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit runtime diagnostic program: %v", err)
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package runtime diagnostic program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		diagnostic := string(printed)
		if runErr == nil {
			t.Fatalf("expected generated runtime failure, got success:\n%s", diagnostic)
		}
		for _, expected := range []string{"JS RUNTIME ERROR", "RangeError: list index 5 is out of bounds", "at Read (src/main.klang:1:1)", "at Main (src/main.klang:5:1)", "1 | function Read(values : List[Int]) : Int {"} {
			if !strings.Contains(diagnostic, expected) {
				t.Fatalf("runtime diagnostic missing %q:\n%s", expected, diagnostic)
			}
		}
		mapped := exec.Command(node, "--enable-source-maps", "-e", "require(\"./program.js\"); globalThis.KlangProgram.Read([1]);")
		mapped.Dir = bundle
		mappedOutput, mappedErr := mapped.CombinedOutput()
		if mappedErr == nil || !strings.Contains(string(mappedOutput), "src/main.klang:2") {
			t.Fatalf("expected Node to consume program.js.map:\n%s", mappedOutput)
		}
	}
}

func TestJavaScriptBackendLowersNamespacesAliasesAndHelpers(t *testing.T) {
	request := requestFromSource(`
namespace math {
    function Double(value : Int) : Int {
        return value * 2;
    }

    function AddAndDouble(left : Int, right : Int) : Int {
        return Double(left + right);
    }
}

global namespace platform {
    function Bias() : Int { return 2; }
}

alias calc = math;

function Main() : Int {
    local Int result = calc::AddAndDouble(10, 10) + Bias();
    print(result);
    return result;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected namespace diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit namespaces: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"function k_math_u2e_Double", "k_math_u2e_Double(__klang_copy(__klang_add(k_left, k_right)))", "k_platform_u2e_Bias()"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated namespace JS missing %q:\n%s", expected, source)
		}
	}
}

func TestJavaScriptBackendHonorsImportedModuleFunctionFilter(t *testing.T) {
	program := file.Program{Name: "modules", EntryPoint: "main.klang", Files: []file.SourceFile{
		{Path: "main.klang", Lines: strings.Split("function Main() : Int { return tools.Used(); }", "\n")},
		{Path: "tools.klang", Lines: strings.Split(`namespace tools {
    function Used() : Int { return Helper(); }
    function Helper() : Int { return 7; }
    function Unused() : Int { return 99; }
}`, "\n"), ModuleFunctionFilter: map[string]bool{"tools.Used": true, "tools.Helper": true}},
	}}
	request := backend.Request{Program: program, Parsed: parser.ParseLoadedProgram(program)}
	output, err := New().Emit(request)
	if err != nil {
		t.Fatalf("emit filtered module: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	if !strings.Contains(source, "k_tools_u2e_Used") || !strings.Contains(source, "k_tools_u2e_Helper") || strings.Contains(source, "k_tools_u2e_Unused") {
		t.Fatalf("unexpected selective module output:\n%s", source)
	}
	var sourceMap struct {
		Sources  []string `json:"sources"`
		Mappings string   `json:"mappings"`
	}
	if err := json.Unmarshal(output.Artifacts[1].Content, &sourceMap); err != nil {
		t.Fatalf("decode selective module source map: %v", err)
	}
	mappedSources := strings.Join(sourceMap.Sources, ",")
	if !strings.Contains(mappedSources, "src/main.klang") || !strings.Contains(mappedSources, "src/tools.klang") || sourceMap.Mappings == "" {
		t.Fatalf("expected both imported sources in source map: %#v", sourceMap)
	}
}

func TestJavaScriptBackendEmitsUnicodeStringOperationsAndLen(t *testing.T) {
	request := requestFromSource(`
function Main() : Int {
    local String value = "h😀llo";
    local String upper = value.uppercase();
    local String lower = upper.lowercase();
    local Char emoji = value[1];
    local String message = "count=" + len(value) + ":" + True;
    print(message, upper, lower, emoji, value.count);
    return len(value) + value.count;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected String diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit String operations: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"__klang_len", "__klang_index", "__klang_string_uppercase", "__klang_string_lowercase", "__klang_add"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated String JS missing %q:\n%s", expected, source)
		}
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package String program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "count=5:True H😀LLO h😀llo 😀 5" {
			t.Fatalf("generated String program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendEmitsListCreationMutationAndIteration(t *testing.T) {
	request := requestFromSource(`
function DoubleLarge(values : List[Int]) : List[Int] {
    return [value * 2 for value in values if value > 1];
}

function Main() : Int {
    local mut List[Int] values = [1, 2, 3];
    local List[Int] saved = values;
    values[0] = 9;
    values[3] = 4;
    for index := range(len(values)) {
        values[index] += index;
    }
    local mut Int sum = 0;
    for_each value in values {
        sum += value;
    }
    local List[Int] doubled = DoubleLarge(values);
    print(saved, values, doubled, values.count, sum);
    return doubled[3] + len(values);
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected List diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit List operations: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"__klang_copy", "__klang_list_assign", "__klang_list_iter", "__klang_iter", "for (let k_index", "for (const k_value", "__klang_index", "__klang_len"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated List JS missing %q:\n%s", expected, source)
		}
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package List program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "[1, 2, 3] [9, 3, 5, 7] [18, 6, 10, 14] 4 24" {
			t.Fatalf("generated List program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendEmitsStructAliasesAndJSONSerialization(t *testing.T) {
	request := requestFromSource(strings.ReplaceAll(`
alias function User(id : String, name : String, roles : List[String], active : Bool = True) : type = struct {
    this.id TAGjson:"user_id"TAG;
    this.name TAGjson:"display_name"TAG;

    #extend {
        function label(prefix : String) : String {
            return prefix + this.name + ":" + this.roles.count;
        }

        function renamed(name : String) : User {
            return User(this.id, name, this.roles, this.active);
        }
    }
}

function Main() : Int {
    let user = User("42", "Ada", ["admin"]);
    let saved = user;
    let renamed = user.renamed("Grace");
    local JSON document = JSON(renamed);
    local String encoded = json_stringify(user);
    print(user.label("user="), encoded, json_stringify(document), saved.name);
    return 0;
}
`, "TAG", "`"))
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected struct/JSON diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit struct/JSON operations: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"function k_User", "function k_User_u2e_label", "__klang_struct_tags", "__klang_call_method", "__klang_json_stringify", "\"display_name\""} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated struct/JSON JS missing %q:\n%s", expected, source)
		}
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package struct/JSON program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		expected := "user=Ada:1 {\"active\":true,\"display_name\":\"Ada\",\"roles\":[\"admin\"],\"user_id\":\"42\"} {\"active\":true,\"display_name\":\"Grace\",\"roles\":[\"admin\"],\"user_id\":\"42\"} Ada"
		if runErr != nil || strings.TrimSpace(string(printed)) != expected {
			t.Fatalf("generated struct/JSON program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendExecutesStandaloneExtensionMethods(t *testing.T) {
	request := requestFromSource(`
alias function Duration(value : Int) : type = struct {
    #extend {
        function ago() : Int {
            return 0 - this.value;
        }
    }
}

#extend Int {
    function days() : Duration {
        return Duration(this);
    }
}

#extend String {
    function surrounded(left : String, right : String) : String {
        return left + this + right;
    }
}

function Main() : Int {
    print("core".surrounded("[", "]"), 10.days().ago());
    return 0;
}
`)
	compiler := New()
	if diagnostics := compiler.Check(request); len(diagnostics) != 0 {
		t.Fatalf("unexpected extension diagnostics: %#v", diagnostics)
	}
	output, err := compiler.Emit(request)
	if err != nil {
		t.Fatalf("emit extension methods: %v", err)
	}
	source := string(output.Artifacts[0].Content)
	for _, expected := range []string{"function k_Int_u2e_days", "function k_String_u2e_surrounded", "__klang_runtime_type"} {
		if !strings.Contains(source, expected) {
			t.Fatalf("generated extension JS missing %q:\n%s", expected, source)
		}
	}
	if node, lookupErr := exec.LookPath("node"); lookupErr == nil {
		bundle := t.TempDir()
		if err := compiler.Package(output, bundle); err != nil {
			t.Fatalf("package extension program: %v", err)
		}
		command := exec.Command(node, filepath.Join(bundle, "program.js"))
		printed, runErr := command.CombinedOutput()
		if runErr != nil || strings.TrimSpace(string(printed)) != "[core] -10" {
			t.Fatalf("generated extension program failed: %v\n%s", runErr, printed)
		}
	}
}

func TestJavaScriptBackendRejectsStructAliasHooks(t *testing.T) {
	request := requestFromSource(`
alias function Resource(id : Int) : type = struct {
    [new] {
        print(id);
    }
}

function Main() : Int {
    let resource = Resource(1);
    return resource.id;
}
`)
	diagnostics := New().Check(request)
	if len(diagnostics) == 0 || !strings.Contains(diagnostics[0].Message, "struct alias hooks") {
		t.Fatalf("expected focused struct hook diagnostic, got %#v", diagnostics)
	}
}

func requestFromSource(source string) backend.Request {
	program := file.Program{Name: "test", EntryPoint: "main.klang", Files: []file.SourceFile{{Path: "main.klang", Lines: strings.Split(strings.TrimSpace(source), "\n")}}}
	return backend.Request{Program: program, Parsed: parser.ParseLoadedProgram(program)}
}
