package runtime

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

func TestImplementedStdlibUsesAtomErrorPropagation(t *testing.T) {
	stdlibRoot := filepath.Join("..", "..", "..", "stdlib")
	err := filepath.WalkDir(stdlibRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(stdlibRoot, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if relative == "js-wasm" {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Base(path) == "raylib.klang" || (filepath.Ext(path) != ".klang" && filepath.Ext(path) != ".grua") {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for index, line := range strings.Split(string(content), "\n") {
			nativeBoundary := strings.Contains(line, "function with_code") ||
				strings.Contains(line, "function WithCode") ||
				strings.Contains(line, "value : Result[")
			if hasStringErrorResult(line) && !nativeBoundary {
				t.Errorf("%s:%d exposes a String error Result instead of Result[..., Atom]", relative, index+1)
			}
			if strings.Contains(line, `Err("`) {
				t.Errorf("%s:%d constructs a String error instead of an Atom", relative, index+1)
			}
			if strings.Contains(line, `throw "`) {
				t.Errorf("%s:%d throws a String instead of an Atom", relative, index+1)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("scan implemented stdlib: %v", err)
	}
}

func hasStringErrorResult(line string) bool {
	for offset := 0; offset < len(line); {
		start := strings.Index(line[offset:], "Result[")
		if start == -1 {
			return false
		}
		start += offset + len("Result[")
		depth := 0
		comma := -1
		for index := start; index < len(line); index++ {
			switch line[index] {
			case '[':
				depth++
			case ']':
				if depth == 0 {
					if comma != -1 && strings.TrimSpace(line[comma+1:index]) == "String" {
						return true
					}
					offset = index + 1
					index = len(line)
				} else {
					depth--
				}
			case ',':
				if depth == 0 && comma == -1 {
					comma = index
				}
			}
		}
		if offset <= start {
			return false
		}
	}
	return false
}

func TestRuntimeRunsMainFunction(t *testing.T) {
	result := runSource(t, `
function Add(a : Int, b : Int) : Int {
    return a + b;
}

function Main() : Int {
    return Add(20, 22);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected Main to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesNativeFileOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "notes.txt")
	result := runSource(t, fmt.Sprintf(`
function Main() : Int {
    local File target = File(%q);
    assert target.name == "notes.txt";
    assert target.extension == ".txt";
    local File castTarget = target.path as File;
    assert castTarget as String == target.path;

    local Result[File, String] created = target.create();
    assert created.ok;
    local String initial = //
hello
world
//;
    local Result[Int, String] written = target.write(initial);
    assert written.ok;
    assert result_unwrap_or(written, 0 - 1) == 11;
    local Result[Int, String] appended = file_append(target, "!");
    assert appended.ok;

    local Result[String, String] content = target.read();
    assert content.ok;
    assert result_unwrap_or(content, "") == initial + "!";
    local Result[List[String], String] lines = target.read_lines();
    assert lines.ok;
    local List[String] fallbackLines;
    local List[String] actualLines = result_unwrap_or(lines, fallbackLines);
    assert len(actualLines) == 2;
    assert actualLines[1] == "world!";

    local Result[Int, String] size = target.size();
    assert size.ok;
    local Int actualSize = result_unwrap_or(size, 0);
    assert actualSize == 12;
    local Result[Bool, String] exists = file_exists(target);
    assert exists.ok and result_unwrap_or(exists, False);
    local Result[Bool, String] removed = target.remove();
    assert removed.ok and result_unwrap_or(removed, False);
    local Result[Bool, String] missing = target.exists();
    assert missing.ok and not result_unwrap_or(missing, True);
    return actualSize;
}
`, path))

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 12 {
		t.Fatalf("expected File program to return 12, got %#v", result.Value)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected File.remove to delete %s, stat error: %v", path, err)
	}
}

func TestRuntimeReturnsFileErrorsAsResults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.txt")
	result := runSource(t, fmt.Sprintf(`
function Main() : Int {
    local File target = File(%q);
    local Result[String, String] content = target.read();
    assert not content.ok;
    local Result[Int, String] size = file_size(target);
    assert not size.ok;
    local Result[Bool, String] removed = file_remove(target);
    assert not removed.ok;
    return 0;
}
`, path))

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected handled File errors to return 0, got %#v", result.Value)
	}
}

func TestRuntimeExecutesNativeOSOperations(t *testing.T) {
	originalDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get current directory: %v", err)
	}
	defer func() {
		if restoreErr := os.Chdir(originalDirectory); restoreErr != nil {
			t.Errorf("restore current directory: %v", restoreErr)
		}
	}()

	tempDirectory := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	const environmentKey = "KLANG_NATIVE_OS_TEST"
	originalEnvironment, hadEnvironment := os.LookupEnv(environmentKey)
	defer func() {
		if hadEnvironment {
			_ = os.Setenv(environmentKey, originalEnvironment)
		} else {
			_ = os.Unsetenv(environmentKey)
		}
	}()

	result := runSource(t, fmt.Sprintf(`
function Main() : Int {
    local OS host = OS();
    assert len(host.name) > 0;
    assert len(host.arch) > 0;
    assert host.cpu_count > 0;
    assert len(host.path_separator) == 1;
    assert host.process_id() > 0;
    assert len(host.temp_dir()) > 0;

    local Result[String, String] home = host.home_dir();
    assert home.ok;
    local Result[String, String] hostname = os_hostname(host);
    assert hostname.ok;

    local Result[Bool, String] set = host.set_env(%q, "enabled");
    assert set.ok;
    local Option[String] environmentValue = os_get_env(host, %q);
    assert environmentValue.some;
    if environmentValue.some {
        assert environmentValue.value == "enabled";
    }
    local Map[String, String] environment = host.environment();
    assert len(environment) > 0;

    local Result[Bool, String] changed = host.change_dir(%q);
    assert changed.ok;
    local Result[String, String] current = os_current_dir(host);
    assert current.ok;
    assert len(result_unwrap_or(current, "")) > 0;

    local Result[Table, String] execution = host.execute(%q, ["-test.run=^$"]);
    assert execution.ok;
    local Table fallback;
    local Table process = result_unwrap_or(execution, fallback);
    assert process["success"] as Bool;
    assert process["exit_code"] as Int == 0;

    local Result[Bool, String] unset = os_unset_env(host, %q);
    assert unset.ok;
    assert not host.get_env(%q).some;
    return host.cpu_count;
}
`, environmentKey, environmentKey, tempDirectory, executable, environmentKey, environmentKey))

	if result.Value.Kind != ValueInt || result.Value.Data.(int) <= 0 {
		t.Fatalf("expected OS program to return a positive CPU count, got %#v", result.Value)
	}
	actualDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("get changed directory: %v", err)
	}
	expectedDirectory, err := filepath.EvalSymlinks(tempDirectory)
	if err != nil {
		t.Fatalf("resolve expected directory: %v", err)
	}
	actualDirectory, err = filepath.EvalSymlinks(actualDirectory)
	if err != nil {
		t.Fatalf("resolve changed directory: %v", err)
	}
	if actualDirectory != expectedDirectory {
		t.Fatalf("expected OS.change_dir to select %s, got %s", expectedDirectory, actualDirectory)
	}
}

func TestRuntimeReturnsOSExecutionStartupErrorAsResult(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local OS host = OS();
    local Result[Table, String] execution = os_execute(host, "__klang_missing_executable__", []);
    assert not execution.ok;
    return 0;
}
`)
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected handled OS execution error to return 0, got %#v", result.Value)
	}
}

func TestRuntimeBuildsParsableMetadataAndPreservesArgumentChannels(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
function Main() : Int {
    local Parsable[T] parsed = Parsable(//
function Parsed() : Int {
    return 1;
}
//, ["source"]);
    assert parsed.statement_count == 1;
    assert len(parsed.args) == 2;
    return parsed.statement_count;
}
`)
	if len(errors) != 0 {
		t.Fatalf("parse failed: %#v", errors)
	}
	result, err := NewWithArgs([]string{"cli"}).Run(parser.ParsedProgram{
		Name:    "parsable",
		Sources: []parser.ParsedSource{{Path: "parsable.klang", Program: parsedProgram}},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected Parsable-backed Main result 1, got %#v", result.Value)
	}

	metadataRuntime := NewWithArgs([]string{"cli"})
	parsable, err := metadataRuntime.newParsable([]Value{
		StringValue("function Parsed() : Int { return 1; }"),
		listFromStrings([]string{"source"}),
	})
	if err != nil {
		t.Fatalf("construct Parsable metadata: %v", err)
	}
	fields := parsable.Data.(ObjectData).Fields
	if fields["statement_count"].Data.(int) != 1 || len(fields["ast"].Data.([]Value)) != 1 {
		t.Fatalf("unexpected AST metadata: %#v", fields)
	}
	args, _ := stringList(fields["args"])
	if strings.Join(args, ",") != "cli,source" {
		t.Fatalf("expected combined argument channels, got %#v", args)
	}
	if !isObjectType(fields["workspace"], "WorkSpace") {
		t.Fatalf("expected workspace metadata, got %#v", fields["workspace"])
	}
	transformed, err := New().transformParsable("parsable_replace", []Value{parsable, StringValue("return 1"), StringValue("return 2")})
	if err != nil {
		t.Fatalf("source transform failed: %v", err)
	}
	resultData := transformed.Data.(ResultData)
	if !resultData.Ok || !strings.Contains(resultData.Value.Data.(ObjectData).Fields["source"].Data.(string), "return 2") {
		t.Fatalf("expected reparsed immutable source replacement, got %#v", transformed)
	}
}

func TestRuntimePollsParsableMessagesAndReturnsInterceptedAST(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    let parsed = Parsable(//
function Parsed() : Int {
    return 1;
}
//);
    let polling = parsable_begin_polling(parsed);
    let response = parsable_poll_message(polling, {"kind": "REQUEST_AST", "payload": "Parsed"});
    local List[T] ast = response["ast"] as List[T];
    if not response["intercepted"] as Bool {
        return 0;
    }
    return len(ast);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected intercepted AST count 1, got %#v", result.Value)
	}
}

func TestRuntimeExecutesParsableKeywordMacro(t *testing.T) {
	result := runParsedSource(t, `
alias printer = Parsable[T Printable].keyword_macro {
    print(get_args_from_parsable()[0]);
}

function Main() : Int {
    printer "hallo";
    return 0;
}
`)
	if strings.Join(result.Output, ",") != "hallo" {
		t.Fatalf("unexpected keyword macro output: %#v", result.Output)
	}
}

func TestRuntimeExecutesParsableKeywordMacroExpansion(t *testing.T) {
	result := runSource(t, `
alias answer = Parsable[T Any].keyword_macro {
    local Table context = macro_context();
    if context["arg_count"] as Int != 1 {
        return macro_expand("return 0;");
    }
    return macro_expand(//
local Int generated = 40 + 2;
return generated;
//);
}

function Main() : Int {
    return answer("ignored");
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected macro expansion to return 42, got %#v", result.Value)
	}
}

func TestRuntimeTracksExecutionState(t *testing.T) {
	engine := New()
	result := runParsedSourceOnRuntime(t, engine, `
function Add(left : Int, mut right : Int) : Int {
    local mut Int total = left + right;
    total += 1;
    return total;
}

function Main() : Int {
    local Int value = Add(1, 2);
    local String owned = "state";
    local String moved = move owned;
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected state program to return 4, got %#v", result.Value)
	}
	states := engine.stateRecordsValue().Data.([]Value)
	assertRuntimeState(t, states, "parameter", "left", "bind")
	assertRuntimeState(t, states, "parameter", "right", "bind")
	assertRuntimeState(t, states, "variable", "total", "assign")
	assertRuntimeState(t, states, "variable", "owned", "move")
	assertRuntimeState(t, states, "return", "Add", "return")
	assertRuntimeState(t, states, "variable", "value", "define")
}

func TestRuntimeTracksTemporaryVariables(t *testing.T) {
	engine := New()
	result := runParsedSourceOnRuntime(t, engine, `
function Main() : Int {
    temp local mut Int scratch = 40;
    scratch += 2;
    temp let answer = scratch;
    return answer;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected temporary state program to return 42, got %#v", result.Value)
	}
	states := engine.stateRecordsValue().Data.([]Value)
	assertRuntimeState(t, states, "temporary", "scratch", "define")
	assertRuntimeState(t, states, "temporary", "scratch", "assign")
	assertRuntimeState(t, states, "temporary", "answer", "define")
}

func TestRuntimeExecutesAssertAndRuntimeTypeInfo(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Type info = Int.get_runtime_type_info();
    assert info.name == "Int";
    assert info.supports_serialization;
    assert info.supports_introspection;
    assert info.supports_memory_layout;
    return info.size + info.alignment + info.layout.footprint;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 24 {
		t.Fatalf("expected runtime Type metadata program to return 24, got %#v", result.Value)
	}
}

func TestRuntimeRejectsFailedAssert(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    assert False;
    return 0;
}
`)

	assertRuntimeErrorContains(t, err, "assertion failed")
}

func TestRuntimeEvaluatesSignedAndPrefixedNumberLiteralsWithUnicodeIdentifiers(t *testing.T) {
	result := runParsedSource(t, `
function එකතු(අගය : Int, 😀 : Int) : Int {
    local Int hex = 0x2A;
    local Int octal = 0o10;
    local Int binary = 0b101;
    local Int negative = -5;
    local Int negativeHex = -0xA;
    return අගය + 😀 + hex + octal + binary + negative + negativeHex;
}

function Main() : Int {
    return එකතු(1, 2);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 43 {
		t.Fatalf("expected unicode numeric literal program to return 43, got %#v", result.Value)
	}
}

func TestRuntimeEvaluatesNumberSeparators(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int big = 1_000_000;
    local Int hex = 0xFF_FF;
    local Int binary = 0b1010_0101;
    local Int mode = 0o7_55;
    local Float ratio = 12_345.67_89;
    return big + hex + binary + mode + ratio as Int;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1078538 {
		t.Fatalf("expected number separator program to return 1078462, got %#v", result.Value)
	}
}

func TestRuntimeExecutesMutableVariablesAndWhileLoop(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local mut Int total = 0;
    while total < 5 {
        total += 1;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected loop to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExposesCommandLineArgs(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
function Main() : Int {
    return len(Args) + len(Args[0]);
}
`)
	if len(errors) != 0 {
		t.Fatalf("parse failed: %#v", errors)
	}

	result, err := NewWithArgs([]string{"abc", "def"}).Run(parser.ParsedProgram{
		Name: "args",
		Sources: []parser.ParsedSource{
			{Path: "args.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected Args program to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExecutesCopyAndClone(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut List[Int] source = [1, 2];
    local List[Int] copied = copy source;
    local List[Int] cloned = clone source;
    source[0] = 10;
    return copied[0] + cloned[1] + source[0];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected copy and clone program to return 13, got %#v", result.Value)
	}
}

func TestRuntimeRejectsAssignmentAfterMove(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut String first = "hello";
    local String second = move first;
    first = "again";
    return len(second);
}
`)

	assertRuntimeErrorContains(t, err, `variable "first" was moved`)
}

func TestRuntimeRejectsIndexedAssignmentAfterMove(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [1];
    local List[Int] moved = move values;
    values[0] = 2;
    return moved[0];
}
`)

	assertRuntimeErrorContains(t, err, `variable "values" was moved`)
}

func TestRuntimeExecutesInferredVariablesConstAndSizeof(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    let maybe = Some(69420);
    let mut count = 1;
    const intSize = Int.sizeof;
    count += 2;
    return maybe.value + count + intSize;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 69431 {
		t.Fatalf("expected inferred variable program to return 69431, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLocalTypeInference(t *testing.T) {
	result := runParsedSource(t, `
function MakeName() : String {
    return "klang";
}

function Main() : Int {
    local count = 2;
    local mut values = [1, 2, 3];
    local name = MakeName();
    values[1] = count + len(name);
    return values[1];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected local type inference program to return 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesVariableDestructuring(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local [first, [second, third]] = [1, [2, 3]];
    local Table data = {"name": "klang", "total": 4};
    local {name, total} = data;
    return first + second + third + total + len(name);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected destructuring program to return 15, got %#v", result.Value)
	}
}

func TestRuntimeExecutesDiscardIdentifier(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return calls;
}

function Main() : Int {
    _ = Mark();
    _ = Mark();
    local _ = Mark();
    local [_, kept, _] = [10, 20, 30];
    return calls + kept;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 23 {
		t.Fatalf("expected discard identifier program to return 23, got %#v", result.Value)
	}
}

func TestRuntimeRejectsMissingIndexedCompoundAssignmentTargets(t *testing.T) {
	_, listErr := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [];
    values[0] += 1;
    return values[0];
}
`)
	assertRuntimeErrorContains(t, listErr, "compound assignment requires existing list index 0")

	_, mapErr := runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values["missing"] += 1;
    return 0;
}
`)
	assertRuntimeErrorContains(t, mapErr, `compound assignment requires existing map key "missing"`)
}

func TestRuntimeExecutesMultipleReturnsHereStringAndDefer(t *testing.T) {
	result := runParsedSource(t, `
function Pair() : (name : String, value : Int) {
    let html = //
<h1>Hello</h1>
//;
    defer print("done");
    return html, 7;
}

function Main() : Int {
    let pair = Pair();
    return len(pair[0]) + pair[1];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 21 {
		t.Fatalf("expected multiple return program to return 21, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "done" {
		t.Fatalf("expected deferred output, got %#v", result.Output)
	}
}

func TestRuntimeAssignsMultiVariableDeclarationFromMultipleReturn(t *testing.T) {
	result := runParsedSource(t, `
function Multi() : (table : Table, count : Int) {
    return {"name": "klang"}, 7;
}

function Main() : Int {
    local Table x, Int y = Multi();
    return y + x.count;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 8 {
		t.Fatalf("expected multi-variable program to return 8, got %#v", result.Value)
	}
}

func TestRuntimeStoresHereStringInMutableInferredVariable(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    let mut here_string = //
<!DOCTYPE html>
<html lang="en">
<body>
    <h1>Hello from kLang!</h1>
</body>
</html>
//;

    return len(here_string);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 86 {
		t.Fatalf("expected here string length 86, got %#v", result.Value)
	}
}

func TestRuntimeReturnsAndPassesHereStringsAsStrings(t *testing.T) {
	result := runParsedSource(t, `
function Render() : String {
    return //
abc
//;
}

function Count(value : String) : Int {
    return len(value);
}

function Main() : Int {
    local String explicit = //
xy
//;
    return Count(Render()) + Count(explicit);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected here string program to return 5, got %#v", result.Value)
	}
}

func TestRuntimeParsesJSONFromHereString(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local JSON document = JSON(//
{
    "name": "kLang",
    "age": 42,
    "active": true,
    "scores": [7, 9],
    "missing": null
}
//);
    local JSON scores = document["scores"];
    local Int age = option_unwrap_or(json_int(document.age), 0);
    local Bool active = option_unwrap_or(json_bool(document.active), False);
    assert document.kind == "object";
	assert scores.kind == "array";
	assert active;
	assert json_is_null(document.missing);
	return age + document.count + scores.count;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 49 {
		t.Fatalf("expected JSON program to return 49, got %#v", result.Value)
	}
}

func TestRuntimeJSONParseReturnsResultError(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Result[JSON, String] parsed = json_parse(//
{
    "broken":
}
//);
    if parsed.ok {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected invalid JSON to return Err, got %#v", result.Value)
	}
}

func TestRuntimeJSONConstructorReportsSourcePosition(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local JSON parsed = JSON(//
{
    "broken":
}
//);
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "invalid JSON at 3:1")
}

func TestRuntimeExecutesRunStatementsBeforeNormalStatementsAndMain(t *testing.T) {
	result := runSource(t, `
function Boot() {
    print("boot");
}

run {
    print("block");
}

print("normal");
run Boot();

function Main() : Int {
    print("main");
    return 7;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected return 7, got %#v", result.Value)
	}
	expectedOutput := []string{"block", "boot", "normal", "main"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeExecutesChildNumericTypes(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local x : Int.child(8) = 127;
    local i16 y = x;
    local types.u8 z = 255;
    return x + y + z + Int.child(8).sizeof + complex128.sizeof;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 526 {
		t.Fatalf("expected child numeric program to return 526, got %#v", result.Value)
	}
}

func TestRuntimeRejectsOutOfRangeChildInteger(t *testing.T) {
	_, err := runSourceWithError(`
function Build() : Int {
    return 128;
}

function Main() : Int {
    local i8 value = Build();
    return value;
}
`)

	assertRuntimeErrorContains(t, err, `cannot assign Int to Int.child(8) variable "value"`)
}

func TestRuntimeRejectsImmutableParameterMutation(t *testing.T) {
	_, err := runSourceWithError(`
function Mutate(value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    return Mutate(1);
}
`)

	assertRuntimeErrorContains(t, err, "cannot mutate immutable variable")
}

func TestRuntimeAllowsMutableParameterMutation(t *testing.T) {
	result := runParsedSource(t, `
function Mutate(mut value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    return Mutate(1);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected mutable parameter program to return 2, got %#v", result.Value)
	}
}

func TestRuntimePassesMutableParameterByValue(t *testing.T) {
	result := runParsedSource(t, `
function Mutate(mut value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    local mut Int count = 1;
    Mutate(count);
    return count;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected pass-by-value call to leave caller unchanged, got %#v", result.Value)
	}
}

func TestRuntimePassesReferenceParameterByReference(t *testing.T) {
	result := runParsedSource(t, `
function Increment(ref value : Int) {
    value += 1;
}

function Main() : Int {
    local mut Int count = 1;
    Increment(count);
    return count;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected reference call to update caller value, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRangeLoopAndBreak(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    for i := range(10) {
        if i == 4 break;
        total += i;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 6 {
		t.Fatalf("expected range loop to return 6, got %#v", result.Value)
	}
}

func TestRuntimeExecutesForEachLoop(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local mut Int total = 0;
    for_each value in [1, 2, 3] {
        total += value;
    }
    for_each letter in "ab" {
        print(letter);
    }
    for_each index in 3 {
        total += index;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 9 {
		t.Fatalf("expected for_each total 9, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPatternMatchWithDefaultBreak(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String mode = "blank";
    local mut Int score = 0;
    if mode == {
        case "blank":
            score += 10;
        case:
            score += 100;
    }
    return score;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected pattern match default break to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPatternMatchFallthroughWithContinue(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int value = 1;
    local mut Int score = 0;
    if value == {
        case 1:
            score += 10;
            continue;
        case 2:
            score += 20;
        case:
            score += 100;
    }
    return score;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 30 {
		t.Fatalf("expected pattern match fallthrough to return 30, got %#v", result.Value)
	}
}

func TestRuntimeExecutesOptionResultListAndTablePatterns(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Option[Int] maybe = Some(10);
    if maybe == {
        case Some(value):
            return value;
        case None():
            return 0;
    }
    return -1;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected Option pattern to return 10, got %#v", result.Value)
	}

	result = runParsedSource(t, `
function Main() : Int {
    local Result[Int, String] parsed = Err("bad");
    if parsed == {
        case Ok(value):
            return value;
        case Err(message):
            return len(message);
    }
    return -1;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected Result pattern to return 3, got %#v", result.Value)
	}

	result = runParsedSource(t, `
function Main() : Int {
    local List[Int] values = [1, 2];
    partial if values == {
        case [1, 2]:
            return 12;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 12 {
		t.Fatalf("expected List pattern to return 12, got %#v", result.Value)
	}

	result = runParsedSource(t, `
function Main() : Int {
    local Table data = {"kind": "count", "value": 4};
    partial if data == {
        case {"kind": "count", "value": amount}:
            return amount;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected Table pattern to return 4, got %#v", result.Value)
	}
}

func TestRuntimeExecutesCStyleForLoop(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    for i := 0; i < 5; i += 1 {
        total += i;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected C-style for loop to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesWhileHeaderScope(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    while active := total < 1 {
        if active {
            total += 1;
        }
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected while header scope to return 1, got %#v", result.Value)
	}
}

func TestRuntimeExportsNestedVariablesToGlobalScope(t *testing.T) {
	result := runParsedSource(t, `
function Configure() : Int {
    if True {
        global mut Int nestedGlobal = 10;
        export local Int exportedLocal = nestedGlobal + 2;
    }
    return exportedLocal;
}

function Main() : Int {
    Configure();
    nestedGlobal += 1;
    return exportedLocal + nestedGlobal;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 23 {
		t.Fatalf("expected exported nested variables to return 23, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTypeCasts(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Float f = 3 as Float;
    local Int i = f as Int;
    while active := i as Bool {
        return ("40" as Int) + (True as Int) + i;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 44 {
		t.Fatalf("expected cast program to return 44, got %#v", result.Value)
	}
}

func TestRuntimeRejectsCustomTypeCastTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
alias function User(id : String) : type = struct {}

function Main() : Int {
    local T raw = User("42");
    local User user = raw as User;
    return len(user.id);
}
`)

	assertRuntimeErrorContains(t, err, "cast target User is not a builtin type")
}

func TestRuntimeExecutesNullSafetyOperator(t *testing.T) {
	result := runParsedSource(t, `
function MissingValue() : T {
}

function PresentValue() : T {
    return 7;
}

function Main() : Int {
    local Bool missing = MissingValue()?;
    local Bool present = PresentValue()?;
    if missing == False and present {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected null safety program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeExecutesConditionalExpressionsDefaultsAndZeroValues(t *testing.T) {
	result := runParsedSource(t, `
function Init() : Int {
    return 1;
}

function AddDefault(value : Int = 5, extra : Int = 2) : Int {
    return value + extra;
}

function Main() : Int {
    local Int zeroInt;
    local String zeroString;
    local Bool flag = if Init() > 0 then return False : True;
    local List[Int] values;
    local Option[Int] maybe;
    if not flag and zeroString == "" and not maybe {
        return zeroInt + AddDefault() + len(values);
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected defaults/zero/conditional program to return 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRestrictedGenericParameters(t *testing.T) {
	result := runParsedSource(t, `
function IdentityNumber[T restrict[UInt, Int, Float]](value : T = 3) : T {
    return value;
}

function Main() : Int {
    local Int first = IdentityNumber();
    local Float second = IdentityNumber(2.5);
    return first + second as Int;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected restricted generic program to return 5, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRestrictedGenericVariable(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut T restrict[UInt, Int, Float] value = 69420;
    value = 10;
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected restricted generic variable to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesInnerFunctionSelector(t *testing.T) {
	result := runSource(t, `
function Test() {
    inner function Eval() {
        print("This is called");
    }
}

function Main() : Int {
    Test().Eval();
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected Main to return 0, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != "This is called" {
		t.Fatalf("expected inner function output, got %v", result.Output)
	}
}

func TestRuntimeInnerFunctionCapturesOuterScope(t *testing.T) {
	result := runSource(t, `
function Counter(base : Int) {
    inner function Eval() : Int {
        return base + 1;
    }
}

function Main() : Int {
    return Counter(41).Eval();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected captured inner function to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesOptionAndResultBuiltins(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Option[Int] some = Some(10);
    local Option[Int] none = None();
    local Result[Int, String] ok = Ok(5);
    local Result[Int, String] err = Err("bad");
    local Result[Int, String] wrapped = Result(7);
    print(some);
    print(none);
    print(ok);
    print(err);
    print(wrapped);
    if some and not none and ok and not err and wrapped {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected option/result program to return 1, got %#v", result.Value)
	}
	expectedOutput := []string{"Some(10)", "None", "Ok(5)", "Err(bad)", "Ok(7)"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeExecutesOptionAndResultHelpers(t *testing.T) {
	result := runParsedSource(t, `
function Double(value : Int) : Int {
    return value * 2;
}

function KeepPositive(value : Int) : Option[Int] {
    if value > 0 {
        return Some(value);
    }
    return None();
}

function ParseMore(value : Int) : Result[String, String] {
    return Ok("value");
}

function Prefix(value : String) : String {
    return "error:" + value;
}

function Main() : Int {
    local Option[Int] maybe = Some(10);
    local Option[Int] none = None();
    local Option[Int] doubled = option_map(maybe, Double);
    local Option[Int] skipped = option_map(none, Double);
    local Option[Int] chained = option_and_then(doubled, KeepPositive);

    local Result[Int, String] ok = Ok(5);
    local Result[Int, String] err = Err("bad");
    local Result[Int, String] mapped = result_map(ok, Double);
    local Result[Int, String] skippedResult = result_map(err, Double);
    local Result[Int, String] mappedErr = result_map_err(skippedResult, Prefix);
    local Result[String, String] chainedResult = result_and_then(mapped, ParseMore);

    return option_unwrap_or(doubled, 0)
        + option_unwrap_or(skipped, 3)
        + option_unwrap_or(chained, 0)
        + result_unwrap_or(mapped, 0)
        + result_unwrap_or(mappedErr, 7)
        + len(result_unwrap_or(chainedResult, ""));
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 65 {
		t.Fatalf("expected option/result helpers to return 65, got %#v", result.Value)
	}
}

func TestRuntimeQuestionChecksOptionAndResultPresence(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Option[Int] some = Some(1);
    local Option[Int] none = None();
    local Result[Int, String] ok = Ok(1);
    local Result[Int, String] err = Err("bad");
    if some? and not none? and ok? and not err? {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected ? to check Option/Result presence, got %#v", result.Value)
	}
}

func TestRuntimeRejectsOptionInnerTypeMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Option[Int] value = Some("bad");
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, `cannot assign Option to Option[Int] variable "value"`)
}

func TestRuntimeExecutesListComprehensions(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local List[Int] values = [1, 2, 3, 4];
    local List[Int] doubled = [value * 2 for value in values];
    local List[Int] evens = [value for value in values if value % 2 == 0];
    local List[Char] letters = [letter for letter in "hey"];
    local List[Int] indexes = [index for index in range(4)];
    print(doubled[2]);
    print(evens[1]);
    print(letters[0]);
    print(indexes[3]);
    return doubled[2] + evens[1] + indexes[3];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected list comprehension program to return 13, got %#v", result.Value)
	}
	expectedOutput := []string{"6", "4", "h", "3"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsInvalidListComprehensionIterable(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Bool flag = True;
    local List[Int] values = [value for value in flag];
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "list comprehension cannot iterate over Bool")
}

func TestRuntimeExecutesComplexAndSIMDValues(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Complex z = Complex(1, 2) + Complex(3, -1);
    local Complex product = z * Complex(2, 0);
    local SIMD[Int] lanes = SIMD([1, 2, 3, 4]);
    local SIMD[Int] moved = lanes + SIMD([4, 3, 2, 1]);
    local SIMD[Int] doubled = moved * 2;
    print(z);
    print(product);
    print(doubled);
    return len(doubled);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected complex/SIMD program to return 4, got %#v", result.Value)
	}
	expectedOutput := []string{"4+1i", "8+2i", "SIMD[10, 10, 10, 10]"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeRejectsSIMDLaneMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local SIMD[Int] left = SIMD([1, 2]);
    local SIMD[Int] right = SIMD([1]);
    local SIMD[Int] bad = left + right;
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "SIMD lane counts must match")
}

func TestRuntimeRejectsInvalidTypeCast(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return "abc" as Int;
}
`)
	assertRuntimeErrorContains(t, err, `cannot cast String "abc" to Int`)
}

func TestRuntimeExecutesListsMapsAndPrint(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    local mut List[Int] values = [];
    local mut Map[String, Int] totals = {};
    values[0] = 7;
    totals["answer"] = values[0] * 6;
    print(totals["answer"]);
    return totals["answer"];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected map/list program to return 42, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "42" {
		t.Fatalf("expected print output 42, got %#v", result.Output)
	}
}

func TestRuntimePrintIsVariadic(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    print("count", 2, True);
    return 0;
}
`)

	if len(result.Output) != 1 || result.Output[0] != "count 2 True" {
		t.Fatalf("expected variadic print output, got %#v", result.Output)
	}
}

func TestRuntimeInputReadsLine(t *testing.T) {
	previousStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = previousStdin
		reader.Close()
	}()
	if _, err := writer.WriteString("Klang\n"); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}
	writer.Close()

	result := runSource(t, `
function Main() : Int {
    local String name = input("name: ");
    print(name);
    return len(name);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected input length 5, got %#v", result.Value)
	}
	if strings.Join(result.Output, "\n") != "name: \nKlang" {
		t.Fatalf("unexpected input output: %#v", result.Output)
	}
}

func TestRuntimeInputPreservesBufferedLinesAcrossCalls(t *testing.T) {
	previousStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	os.Stdin = reader
	defer func() {
		os.Stdin = previousStdin
		reader.Close()
	}()
	if _, err := writer.WriteString("7\n35\n"); err != nil {
		t.Fatalf("failed to write input: %v", err)
	}
	writer.Close()

	result := runSource(t, `
function Main() : Int {
    local Int first = input() as Int;
    local Int second = input() as Int;
    print(first, second);
    return 0;
}
`)

	if strings.Join(result.Output, "\n") != "7 35" {
		t.Fatalf("unexpected input output: %#v", result.Output)
	}
}

func TestRuntimeRejectsUseAfterMove(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local String first = "hello";
    local String second = move first;
    print(first);
    return len(second);
}
`)
	assertRuntimeErrorContains(t, err, `variable "first" was moved`)
}

func TestRuntimeExecutesStringIndexing(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String text = "hey";
    if text[0] == 'h' and text[1] == 'e' and len(text) == 3 {
        return 1;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected string indexing program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeRejectsImmutableMutation(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local Int value = 1;
    value = 2;
    return value;
}
`)
	if err == nil || !strings.Contains(err.Error(), "type check failed") {
		t.Fatalf("expected immutable mutation to fail before runtime, got %v", err)
	}
}

func TestRuntimeKeepsImmutableAggregateSnapshotsReferentiallyTransparent(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut List[Int] values = [1, 2];
    local List[Int] snapshot = values;
    values[0] = 9;

    local mut Map[String, Int] scores = {"answer": 42};
    local Map[String, Int] savedScores = scores;
    scores["answer"] = 7;

    return snapshot[0] + savedScores["answer"];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 43 {
		t.Fatalf("expected immutable snapshots to return 43, got %#v", result.Value)
	}
}

func TestRuntimeExecutesSetBuiltin(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Set[String] imports = Set(["lexer", "parser", "lexer"]);
    local Iterator[String] iterator = iter(imports);
    local Option[String] first = next(iterator);
    if set_has(imports, "parser") and first.some and first.value == "lexer" {
        return imports.count + len(imports);
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected set program to return 4, got %#v", result.Value)
	}
}

func TestRuntimeExecutesFormatBuiltins(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String message = format("Hello %, score %% %", ["kLang", 42]);
    local Int printed = printf("Ready: %", [message]);
    return len(message) + printed;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 53 {
		t.Fatalf("expected format program to return 53, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "Ready: Hello kLang, score % 42" {
		t.Fatalf("unexpected printf output: %#v", result.Output)
	}
}

func TestRuntimeRejectsFormatArityMismatch(t *testing.T) {
	_, missingErr := runParsedSourceWithError(`
function Main() : Int {
    return len(format("Hello % %", ["kLang"]));
}
`)
	assertRuntimeErrorContains(t, missingErr, "format missing value for placeholder")

	_, extraErr := runParsedSourceWithError(`
function Main() : Int {
    return len(format("Hello %", ["kLang", 42]));
}
`)
	assertRuntimeErrorContains(t, extraErr, "format received more values than placeholders")
}

func TestRuntimeExecutesStdlibFmtModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "fmt";

function Main() : Int {
    local String message = fmt.Format("module %", ["fmt"]);
    local Int printed = fmt.Printf("% ready", [message]);
    return len(message) + printed;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 26 {
		t.Fatalf("expected stdlib fmt program to return 26, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "module fmt ready" {
		t.Fatalf("unexpected fmt.Printf output: %#v", result.Output)
	}
}

func TestRuntimeRejectsInvalidSetItem(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local Set[T] values = Set([[1]]);
    return len(values);
}
`)

	if err == nil || !strings.Contains(err.Error(), "Set item expects String, Int, UInt, Float, Bool, or Char, got List[Int]") {
		t.Fatalf("expected set item type check failure, got %v", err)
	}
}

func TestRuntimeUsesCopyOnWriteForSharedListBindings(t *testing.T) {
	runtime := New()
	original := Value{Kind: ValueList, Data: []Value{IntValue(1), IntValue(2)}}
	if err := runtime.defineValue(runtime.global, "a", true, "List[Int]", original); err != nil {
		t.Fatalf("failed to define a: %v", err)
	}
	aBinding, _ := runtime.global.Get("a")
	if err := runtime.defineValue(runtime.global, "b", true, "List[Int]", aBinding.Value); err != nil {
		t.Fatalf("failed to define b: %v", err)
	}
	bBinding, _ := runtime.global.Get("b")

	aItems := aBinding.Value.Data.([]Value)
	bItems := bBinding.Value.Data.([]Value)
	if len(aItems) == 0 || len(bItems) == 0 || &aItems[0] != &bItems[0] {
		t.Fatalf("expected bindings to share backing storage before mutation")
	}

	err := runtime.assignIndex(parser.IndexExpression{
		Target: parser.IdentifierExpression{Name: "b"},
		Index:  parser.LiteralExpression{Kind: "Int", Value: "0"},
	}, "=", IntValue(9), runtime.global)
	if err != nil {
		t.Fatalf("indexed assignment failed: %v", err)
	}

	aItems = aBinding.Value.Data.([]Value)
	bItems = bBinding.Value.Data.([]Value)
	if aItems[0].Data.(int) != 1 || bItems[0].Data.(int) != 9 {
		t.Fatalf("expected mutation to detach b from a, got a=%#v b=%#v", aItems, bItems)
	}
	if &aItems[0] == &bItems[0] {
		t.Fatalf("expected bindings to stop sharing backing storage after mutation")
	}
}

func TestRuntimeRejectsRValueAssignmentTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut Int value = 1;
    (value + 1) = 3;
    return value;
}
`)
	assertRuntimeErrorContains(t, err, "assignment target must be an lvalue")
}

func TestRuntimeRejectsDivisionByZero(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return 10 / 0;
}
`)
	assertRuntimeErrorContains(t, err, "division by zero")
}

func TestRuntimeRejectsUnsupportedLenTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return len(True);
}
`)
	assertRuntimeErrorContains(t, err, "len does not support Bool")
}

func TestRuntimeRejectsInvalidIndexes(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local List[Int] values = [1];
    return values[2];
}
`)
	assertRuntimeErrorContains(t, err, "list index 2 is out of bounds")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local Map[String, Int] values = {"ok": 1};
    return values["missing"];
}
`)
	assertRuntimeErrorContains(t, err, `map key "missing" does not exist`)

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local String text = "hey";
    return text[3] as Int;
}
`)
	assertRuntimeErrorContains(t, err, "string index 3 is out of bounds")
}

func TestRuntimeRejectsStringIndexAssignment(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut String text = "hey";
    text[0] = 'H';
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, "String is not index-assignable")
}

func TestRuntimeRejectsBreakOutsideLoop(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    break;
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "break is only allowed inside a loop")
}

func TestRuntimeRejectsDuplicateVariablesInSameScope(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Int value = 1;
    local Int value = 2;
    return value;
}
`)
	assertRuntimeErrorContains(t, err, `variable "value" is already defined`)
}

func TestRuntimeExecutesBlockShadowingWithoutLeaking(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int value = 1;
    if True {
        local Int value = 20;
        print(value);
    }
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected block shadowing to preserve outer value 1, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "20" {
		t.Fatalf("expected inner shadow output 20, got %#v", result.Output)
	}
}

func TestRuntimeExecutesUserDefinedScopeWithoutLeaking(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Int value = 1;
    scope Calculation {
        local Int value = 20;
        print(value);
    }
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected user-defined scope to preserve outer value 1, got %#v", result.Value)
	}
	if len(result.Output) != 1 || result.Output[0] != "20" {
		t.Fatalf("expected scoped shadow output 20, got %#v", result.Output)
	}
}

func TestRuntimeRejectsReturnTypeMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return "bad";
}
`)
	assertRuntimeErrorContains(t, err, "function Main returns Int, got String")
}

func TestRuntimeRejectsMissingReturnValue(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Int value = 1;
}
`)
	assertRuntimeErrorContains(t, err, "function Main returns Int, got Null")
}

func TestRuntimeRejectsTypeChangingAssignment(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut Int value = 1;
    value = "bad";
    return value;
}
`)
	assertRuntimeErrorContains(t, err, `cannot assign String to Int variable "value"`)
}

func TestRuntimeRejectsTypedListAndMapMutationMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local mut List[Int] values = [];
    values[0] = "bad";
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot assign String to list element type Int")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values[1] = 10;
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot use Int as map key type String")

	_, err = runParsedSourceWithError(`
function Main() : Int {
    local mut Map[String, Int] values = {};
    values["bad"] = "value";
    return 1;
}
`)
	assertRuntimeErrorContains(t, err, "cannot assign String to map value type Int")
}

func TestRuntimeShortCircuitsLogicalOperators(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    if False and missingFunction() {
        return 1;
    }
    if True or missingFunction() {
        return 2;
    }
    return 3;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected short-circuit program to return 2, got %#v", result.Value)
	}
}

func TestRuntimeExecutesBooleanOperatorsInConditionsAndLoops(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Int total = 0;
    local Bool ready = True;
    local Bool active = True;
    local Bool failed = False;
    local Bool fallback = False;

    if ready and active xor failed or fallback {
        total += 1;
    }
    unless not ready or failed {
        total += 2;
    }
    while keepGoing := total == 3 and not failed {
        total += 3;
        break;
    }
    do_while firstPass := failed xor True {
        total += 4;
        break;
    }
    for i := 0; i < 3 and total < 11; i += 1 {
        total += 1;
    }
    return total;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 11 {
		t.Fatalf("expected boolean operator program to return 11, got %#v", result.Value)
	}
}

func TestRuntimeExecutesPipeOperator(t *testing.T) {
	result := runParsedSource(t, `
function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Double(value : Int) : Int {
    return value * 2;
}

function Main() : Int {
    return 2 |> Add(3) |> Double;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected pipe operator program to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesFunctionCallbacks(t *testing.T) {
	result := runSource(t, `
function Double(value : Int) : Int {
    return value * 2;
}

function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Combine(left : Int, right : Int, callback : Function[Int, Int, Int]) : Int {
    return callback(left, right);
}

function Main() : Int {
    local Function[Int, Int] callback = Double;
    return Apply(5, callback) + Combine(2, 3, Add);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected callback program to return 15, got %#v", result.Value)
	}
}

func TestRuntimeExecutesFirstClassFunctionClosures(t *testing.T) {
	result := runSource(t, `
function NumberFactory(multiplier : Int) : Function[Int, Int] {
    function InnerGenerator(value : Int) : Int {
        return value * multiplier;
    }
    return InnerGenerator;
}

function Main() : Int {
    local Function[Int, Int] timesTen = NumberFactory(10);
    return timesTen(42) + NumberFactory(5)(10);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 470 {
		t.Fatalf("expected first-class function program to return 470, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLazyFunctionArgumentsOnDemand(t *testing.T) {
	result := runSource(t, `
function Boom() : Int {
    return 1 / 0;
}

lazy function Choose(useFirst : Bool, first : Int, second : Int) : Int {
    if useFirst {
        return first;
    }
    return second;
}

function Main() : Int {
    return Choose(True, 42, Boom());
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected lazy function to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLazyVariableInitializationOnDemand(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return 40 + calls;
}

function Main() : Int {
    lazy local Int value = Mark();
    local Int before = calls;
    local Int first = value;
    local Int second = value;
    return before * 100 + first + second + calls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 83 {
		t.Fatalf("expected lazy variable program to return 83, got %#v", result.Value)
	}
}

func TestRuntimeDoesNotEvaluateUnusedLazyVariable(t *testing.T) {
	result := runParsedSource(t, `
global mut Int calls = 0;

function Mark() : Int {
    calls += 1;
    return calls;
}

function Main() : Int {
    lazy let value = Mark();
    return calls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected unused lazy variable program to return 0, got %#v", result.Value)
	}
}

func TestRuntimeMovesLazyVariableInitializerWhenForced(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local String value = "ready";
    lazy local String moved = move value;
    local String before = value;
    assert before + moved == "readyready";
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected lazy move program to return readyready, got %#v", result.Value)
	}
}

func TestRuntimeTailCallOptimizesSelfRecursion(t *testing.T) {
	runtime := New()
	runtime.maxDepth = 8

	parsedProgram, errors := parser.Parse(`
function CountDown(value : Int, total : Int) : Int {
    if value == 0 {
        return total;
    }
    return CountDown(value - 1, total + 1);
}

function Main() : Int {
    return CountDown(128, 0);
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := runtime.Run(parser.ParsedProgram{
		Name: "tailcall",
		Sources: []parser.ParsedSource{
			{Path: "tailcall.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("expected tail-call optimized program to pass, got: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 128 {
		t.Fatalf("expected tail-call program to return 128, got %#v", result.Value)
	}
}

func TestRuntimeComparesCharsAndStrings(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    if '7' >= '0' and '7' <= '9' {
        if "beta" > "alpha" {
            return 1;
        }
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected char/string comparison program to return 1, got %#v", result.Value)
	}
}

func TestRuntimeAppliesOperatorPrecedenceEverywhere(t *testing.T) {
	result := runParsedSource(t, `
function Twice(value : Int) : Int {
    return value * 2 + 1;
}

function Main() : Int {
    local mut Int total = 1 + 2 * 3;
    local Int powered = -2 ** 3 ** 2;
    local Int grouped = (1 + 2) * 3;
    total += Twice(2 + 3 * 4) // 5;
    while active := total > 10 and grouped == 9 or False {
        return total + grouped + powered;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != -491 {
		t.Fatalf("expected precedence program to return -491, got %#v", result.Value)
	}
}

func TestRuntimeRejectsDuplicateAndAmbiguousFunctions(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return 1;
}

function Main() : Int {
    return 2;
}
`)
	assertRuntimeErrorContains(t, err, "program can define only one top-level Main entry function")

	_, err = runParsedSourceWithError(`
namespace A {
    function Pick() : Int { return 1; }
}

namespace B {
    function Pick() : Int { return 2; }
}

function Main() : Int {
    return Pick();
}
`)
	assertRuntimeErrorContains(t, err, `ambiguous function "Pick"`)
}

func TestRuntimeExecutesChainedNamespaceAndAliasCalls(t *testing.T) {
	result, err := runSourceWithError(`
namespace std {
    namespace lib {
        function LuaInit() {
            print("std.lib.LuaInit(); is called");
        }

        function Number() : Int {
            return 7;
        }
    }
}

alias std_lib = std.lib;
alias std_alias = std;
alias lib_alias = std_alias.lib;

function Main() : Int {
    std.lib.LuaInit();
    std_lib::LuaInit();
    return std.lib.Number() + std_lib::Number() + lib_alias::Number();
}
`)
	if err != nil {
		t.Fatalf("expected runtime to pass, got: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 21 {
		t.Fatalf("expected namespace program to return 21, got %#v", result.Value)
	}
	expectedOutput := []string{
		"std.lib.LuaInit(); is called",
		"std.lib.LuaInit(); is called",
	}
	if strings.Join(result.Output, "\n") != strings.Join(expectedOutput, "\n") {
		t.Fatalf("unexpected output: %#v", result.Output)
	}
}

func TestRuntimeRejectsRunawayRecursion(t *testing.T) {
	runtime := New()
	runtime.maxDepth = 8

	parsedProgram, errors := parser.Parse(`
function Loop() : Int {
    return Loop();
}

function Main() : Int {
    return Loop();
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	_, err := runtime.Run(parser.ParsedProgram{
		Name: "recursion",
		Sources: []parser.ParsedSource{
			{Path: "recursion.klang", Program: parsedProgram},
		},
	})
	assertRuntimeErrorContains(t, err, "maximum call depth 8 exceeded")
}

func TestRuntimeBorrowCheckerRejectsConflictingMutableBorrow(t *testing.T) {
	memory := NewMemory()
	objectID := memory.Allocate(IntValue(10), MemoryStack)

	if err := memory.BorrowImmutable(objectID); err != nil {
		t.Fatalf("unexpected immutable borrow error: %v", err)
	}
	if err := memory.BorrowMutable(objectID); err == nil {
		t.Fatal("expected mutable borrow to fail while immutable borrow is active")
	}
	memory.ReleaseImmutable(objectID)
	if err := memory.BorrowMutable(objectID); err != nil {
		t.Fatalf("expected mutable borrow after release to pass, got %v", err)
	}
}

func TestRuntimeTracksStackAndHeapMemory(t *testing.T) {
	result := runParsedSource(t, `
global Int shared = 10;

function Main() : Int {
    local Int localValue = shared + 1;
    return localValue;
}
`)

	if result.Memory.HeapObjects == 0 {
		t.Fatalf("expected heap allocations for global values, got %#v", result.Memory)
	}
	if result.Memory.StackObjects == 0 {
		t.Fatalf("expected stack allocations for local values, got %#v", result.Memory)
	}
	if result.Memory.TotalObjects != result.Memory.StackObjects+result.Memory.HeapObjects {
		t.Fatalf("unexpected memory totals: %#v", result.Memory)
	}
}

func TestRuntimeCanRunParsedProgramDirectly(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
function Main() : Int {
    return 9 * 9;
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := New().Run(parser.ParsedProgram{
		Name: "direct",
		Sources: []parser.ParsedSource{
			{Path: "direct.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 81 {
		t.Fatalf("expected direct parsed program to return 81, got %#v", result.Value)
	}
}

func TestRuntimeUsesEntryPointDirective(t *testing.T) {
	parsedProgram, errors := parser.Parse(`
namespace App {
    #set_entry_point_to_here
    function Process() : Int {
        return 7;
    }
}

function Main() : Int {
    return 0;
}
`)
	if len(errors) != 0 {
		t.Fatalf("unexpected parse errors: %#v", errors)
	}

	result, err := New().Run(parser.ParsedProgram{
		Name:       "entry",
		EntryPoint: "App.Process",
		Sources: []parser.ParsedSource{
			{Path: "entry.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected entry point to return 7, got %#v", result.Value)
	}
}

func TestRuntimeRejectsMissingAndInvalidEntryPoints(t *testing.T) {
	_, missingErr := runParsedSourceWithError(`function Helper() : Int { return 0; }`)
	assertRuntimeErrorContains(t, missingErr, "program must define function Main() : Int")

	_, invalidErr := runParsedSourceWithError(`function Main() : String { return ""; }`)
	assertRuntimeErrorContains(t, invalidErr, "entry point Main must have signature function Main() : Int")
}

func TestRuntimeExecutesAtomicBuiltins(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Atomic[Int] counter = Atomic(1);
    atomic_add(counter, 2);
    atomic_store(counter, atomic_load(counter) + 1);
    return atomic_load(counter);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 4 {
		t.Fatalf("expected atomic program to return 4, got %#v", result.Value)
	}
}

func TestRuntimeExecutesWorkspaceBuildAndDebuggerAPIs(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local Program program = Program(["app", "mathg"]);
    local BuildSystem build = BuildSystem("demo", 2, ["first.klang", "app.klang"], "Standalone");
    local WorkSpace workspace = WorkSpace(program, build);
    local List[String] files = workspace_files(workspace);
    debug(workspace_manifest(workspace));
    breakpoint("workspace-ready");
    return len(workspace_backend(workspace)) + len(files) + len(debug_stack());
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 13 {
		t.Fatalf("expected workspace program to return 13, got %#v", result.Value)
	}
	if len(result.Output) != 2 || !strings.Contains(result.Output[0], "[debug]") || !strings.Contains(result.Output[1], "[breakpoint]") {
		t.Fatalf("expected debug and breakpoint output, got %#v", result.Output)
	}
}

func TestRuntimeExecutesStdlibArrayModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "array";

function Main() : Int {
    local List[Int] values = [1, 2, 3];
    local List[Int] pushed = array.push(values, 4);
    local List[Int] inserted = array.insert(pushed, 1, 9);
    local List[Int] updated = array.set(inserted, 0, 8);
    local List[Int] sliced = array.slice(updated, 1, 4);
    local List[Int] removed = array.remove(sliced, 2);
    local List[Int] reversed = array.reverse(removed);
    local Option[Int] front = array.front(reversed);
    local Option[Int] missing = array.fetch(reversed, 20);
    if not front.some or missing.some {
        return 0;
    }
    return array.len(reversed) + array.get(reversed, 0) + array.index_of(reversed, 9) + array.count(updated, 9) + array.capacity(values);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected stdlib array program to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibArrayAlias(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "array";

function AliasEven(value : Int) : Bool { return value % 2 == 0; }
function AliasDouble(value : Int) : Int { return value * 2; }

function Main() : Int {
    local List[Int] values = [1, 2, 3];
    local T created = Array(values, len(values), 3);
    local Array pushed = created.push(4);
    local Array inserted = pushed.insert(1, 9);
    local Array grown = inserted.set(0, 8);
    local Array sliced = grown.slice(1, 4);
    local Array removed = sliced.remove(2);
    local Array window = removed.reverse();
    local Option[Int] front = window.front();
    local Option[Int] missing = window.fetch(20);
    local List[Int] pipelineValues = created.filter(AliasEven).map(AliasDouble).sort();
    if not front.some or missing.some {
        return 0;
    }
    assert pipelineValues[0] == 4;
    return window.count() + window.get(0) + window.index_of(9) + grown.occurrences(9) + created.capacity_value();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected stdlib Array alias program to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibTableCompatibilityHelpers(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "basic";
import "builtin";
import "reflect";
import "errors";
import "datetime";
import "json";
import "exceptions";
import "metasystem";

function Main() : Int {
    local mut Table data = {"name": "klang", "count": 99};
    if not basic.TableHas(data, "name") or basic.TableHas(data, "missing") {
        return 0;
    }
    local Option[T] found = basic.TableGet(data, "name");
    local Option[Any] reflected = reflect.Field(data, "name");
    data = builtin.delete(data, "name");
    if basic.TableHas(data, "name") or not found.some or not reflected.some {
        return 0;
    }

    local String message = errors.Message(:ok);
    local String kind = errors.Name(errors.Error("error"));
    local Atom cause = errors.NotFound();
    local Bool stopped = datetime.TimerStop({});
    local Table pair = {"key": "a", "value": "b"};
    local Result[String, Atom] encoded = json.encode_map_checked(pair, json.encode_binary);
    local Result[String, String] nativeEncoded = json_encode(pair);
    local Result[String, Atom] coded = errors.WithCode(nativeEncoded, :json_encode_failed);
    local Table exceptionOptions = exceptions.format_options();
    local String formatted = exceptions.format_exception(:E, :why, [], exceptionOptions);
    local mut Bool atomCaught = False;
    try {
        local Result[Int, Atom] failure = errors.ErrResult(:not_found);
        _ = exceptions.ResultToException(failure);
    } catch reason {
        atomCaught = errors.Is(reason, errors.NotFound());
    }
    local WorkSpace workspace = metasystem.workspace.UserDefinedWorkspace("demo", ["app"], ["first.klang"], "Standalone");
    local Table loop = metasystem.build.message_loop(workspace, [{"kind": "COMPLETE"}]);

    if message != "ok" or kind != "error" or debug_type(cause) != "Atom" or stopped or not atomCaught {
        return 0;
    }
    if not encoded.ok or not coded.ok or not loop["complete"] as Bool {
        return 0;
    }

	return len(encoded!) + len(formatted) + data.count + data["count"] as Int;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 131 {
		t.Fatalf("expected stdlib table compatibility helpers to return 131, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAtomErrorStdlibFacades(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "api";
import "basic";
import "calender";
import "core";
import "datetime";
import "encoding";
import "file";
import "hash";
import "interface";
import "io";
import "json";
import "language";
import "metasystem";
import "repl";

function Main() : Int {
    local Result[Int, Atom] basicValue = basic.OkValue(1);
    local Result[Table, Atom] args = core.argparse.Parse([], {});
    local Result[Parsable[Any], Atom] line = repl.ParseLine("function Value() : Int { return 1; }");
    local Parsable[Any] parsed = Parsable("function Value() : Int { return 1; }", []);
    local Result[Parsable[Any], Atom] changed = language.WithSource(parsed, "function Value() : Int { return 2; }");
    local Result[Parsable[Any], Atom] replaced = api.ReplaceSource("api", "Reflective", "Reflective");

    assert basicValue.ok;
    assert args.ok;
    assert line.ok;
    assert changed.ok;
    assert replaced.ok;
    assert not api.Describe("__missing__").ok;
    assert not calender.LocalTime().ok;
    assert not datetime.ParseDuration("").ok;
    assert not encoding.base64.DecodeString("").ok;
    assert not file.Read("__klang_missing_file__").ok;
    assert not hash.New("").ok;
    assert not interface.Access("", 0).ok;
    assert not io.Unsupported("ffi").ok;
    assert not json.Parse("{").ok;
    assert not metasystem.workspace.Validate(WorkSpace(Program([]), BuildSystem("empty", 0, [], "Standalone"))).ok;
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 0 {
		t.Fatalf("expected Atom-error stdlib facades to return 0, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibNamespaceAliasCalls(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "array";
alias arr = array;

function Main() : Int {
    local List[Int] list = [1, 2, 3, 4];
    if arr::empty(list) {
        return 0;
    }
    local Int length = arr::len(list);
    local List[Int] cleared = arr::clear(list);
    let mut copied = arr::copy(list);
    return length + copied.count + cleared.count;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 8 {
		t.Fatalf("expected stdlib namespace alias calls to return 8, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTypedJSONStdlibFacade(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "json";

function Main() : Int {
    local JSON document = json.must_parse(//
{"name":"kLang","ports":[8080,8081],"metadata":null}
//);
    local String name = option_unwrap_or(json.as_string(document.name), "missing");
    local JSON first = option_unwrap_or(json.get_index(document.ports, 0), json.null_json());
    local Int port = option_unwrap_or(json.as_int(first), 0);
    local Result[JSON, Atom] reparsed = json.parse(json.stringify(document));
    local Table native = {"enabled": True, "ports": [1, 2]};
    local Result[String, Atom] serialized = json.serialize(native);
    if name != "kLang" or not reparsed.ok or not serialized.ok or not json.is_null(document.metadata) {
        return 0;
    }
    local String serializedText = result_unwrap_or(serialized, "");
    local Result[T, Atom] restored = json.deserialize(serializedText);
    if not restored.ok {
        return 0;
    }
    local T restoredValue = result_unwrap_or(restored, {});
    local Table restoredTable = restoredValue as Table;
    local List[T] restoredPorts = restoredTable.ports as List[T];
    return port + document.count + len(restoredPorts);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 8085 {
		t.Fatalf("expected typed JSON stdlib facade to return 8085, got %#v", result.Value)
	}
}

func TestRuntimeExecutesDeepStdlibCoreFacades(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "array";
import "list";
import "table";
import "strings";
import "mathg";
import "io";
import "json";
import "option";
import "result";
import "test";

function Double(value : Int) : Int { return value * 2; }
function Positive(value : Int) : Bool { return value > 0; }
function Add(total : Int, value : Int) : Int { return total + value; }
function KeepEntry(key : String, value : Int) : Bool { return key != "drop" and value > 0; }
function DoubleEntry(key : String, value : Int) : Int { _ = key; return value * 2; }

function Main() : Int {
    local List[Int] values = [-1, 2, 3];
    local List[Int] positive = list.filter(values, Positive);
    local List[Int] doubled = array.map(positive, Double);
    local Int total = list.fold(doubled, 0, Add);
    _ = test.equal(total, 10);
    _ = test.some(list.first(values));
    _ = test.none(list.get(values, 20));

    local Table source = {"keep": 2, "drop": 4};
    local Table kept = table.filter(source, KeepEntry);
    local Table mapped = table.map_values(kept, DoubleEntry);
    local Option[Int] found = table.get(mapped, "keep");
    _ = test.equal(option.expect(found, "missing keep"), 4);
    _ = test.equal(table.len(mapped), 1);

    local Result[Int, String] okValue = result.ok(5);
    local Result[Int, String] mappedResult = result.map(okValue, Double);
    local Result[Int, String] badValue = result.err("bad");
    _ = test.equal(result.expect(mappedResult, "expected mapped result"), 10);
    _ = test.err(badValue);

    local Table reader = io.Reader("abcd");
    local Table read = io.Read(reader, 2);
    local Table writer = io.WriteLine(io.Writer(), read.value as String);
    _ = test.equal(io.BufferString(writer), "ab\n");
    _ = test.equal(io.Remaining(read), 2);

    _ = test.equal(strings.LastIndex("one two one", "one"), 8);
    _ = test.equal(strings.Slice("abc", 0 - 2, 2), "ab");
    _ = test.equal(mathg.GCD(54, 24), 6);
    _ = test.equal(mathg.LCM(6, 8), 24);
    _ = test.assert_true(json.valid(//
{"ok":true}
//));
    _ = test.assert_false(json.valid("{"));
    return 42;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected deep stdlib facade program to return 42, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibDSAModule(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "dsa";

function Main() : Int {
    local mut T stack = dsa.StackEmpty();
    stack = dsa.StackPush(stack, 3);
    stack = dsa.StackPush(stack, 5);
    local Option[Int] top = dsa.StackPeek(stack);
    local Table popped = dsa.StackPop(stack);
    local T nextStack = popped.stack;
    local Option[Int] poppedValue = popped.value as Option[Int];

    local mut T queue = dsa.QueueEmpty();
    queue = dsa.QueuePush(queue, 7);
    queue = dsa.QueuePush(queue, 11);
    local Table shifted = dsa.QueuePop(queue);
    local T nextQueue = shifted.queue;
    local Option[Int] shiftedValue = shifted.value as Option[Int];

    local mut T ordered = dsa.OrderedMapEmpty();
    ordered = dsa.OrderedMapPut(ordered, "a", 10);
    ordered = dsa.OrderedMapPut(ordered, "b", 20);
    ordered = dsa.OrderedMapPut(ordered, "a", 12);
    local Option[T] found = dsa.OrderedMapGet(ordered, "a");
    local T removed = dsa.OrderedMapRemove(ordered, "b");
    local mut T compat = dsa.arrayhashmap.New(["x"], [4]);
    compat = dsa.arrayhashmap.Put(compat, "y", 6);
    local Option[T] compatFound = dsa.arrayhashmap.Get(compat, "y");

    if top.some {
        if poppedValue.some {
            if shiftedValue.some {
                if found.some {
                    if compatFound.some {
                        local Int foundValue = found.value as Int;
                        local Int compatValue = compatFound.value as Int;
                        return top.value + poppedValue.value + dsa.StackCount(nextStack) +
                            shiftedValue.value + dsa.QueueCount(nextQueue) +
                            foundValue + dsa.OrderedMapCount(removed) +
                            compatValue + dsa.OrderedMapCount(compat);
                    }
                }
            }
        }
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 40 {
		t.Fatalf("expected stdlib dsa program to return 40, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibMetasystemASTHelpers(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "metasystem";

function Main() : Int {
    local Program program = Program(["app", "mathg"]);
    local Table programAst = metasystem.build.GetASTFromSourceCode(Some(program));
    local Table defaultProgramAst = metasystem.build.GetASTFromSourceCode(None());
    local WorkSpace workspace = metasystem.workspace.UserDefinedWorkspace("demo", ["app"], ["first.klang", "app.klang"], "Standalone");
    local Table projectAst = metasystem.build.GetAstFromEntireProject(Some(workspace));
    local Table fallbackProjectAst = metasystem.build.GetAstFromEntireProject(None());

    local Bool programAvailable = programAst["available"] as Bool;
    local Bool defaultProgramAvailable = defaultProgramAst["available"] as Bool;
    local Int projectFileCount = projectAst["file_count"] as Int;
    local Int fallbackFileCount = fallbackProjectAst["file_count"] as Int;
    if not programAvailable or defaultProgramAvailable {
        return 0;
    }
    if projectFileCount != 2 or fallbackFileCount != 1 {
        return 0;
    }

    local List[String] programNodes = programAst["nodes"] as List[String];
    local List[String] projectNodes = projectAst["nodes"] as List[String];
    return len(programNodes) + len(projectNodes) + projectFileCount + fallbackFileCount;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 9 {
		t.Fatalf("expected stdlib metasystem AST helpers to return 9, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStdlibRuntimeDebugHelpers(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	}()

	result := runSource(t, `
import "runtime";

function Main() : Int {
    local Int line = runtime.debug.__LINE__();
    local Table pos = runtime.debug.__POS__();
    local Table lineOf = runtime.debug.__LINE_OF__(5);
    local String fn = runtime.debug.__FUNCTION__();
    local String module = runtime.debug.__MODULE__();
    local String file = runtime.debug.__FILE__();
    local Int wrapperLine = runtime.debug.Line();

    if line <= 0 {
        return 0;
    }
    if wrapperLine <= 0 {
        return 0;
    }
    if (pos["line"] as Int) <= 0 {
        return 0;
    }
    if (lineOf["line"] as Int) <= 0 {
        return 0;
    }
    if (lineOf["value"] as Int) != 5 {
        return 0;
    }
    return len(fn) + len(module) + len(file) + (lineOf["value"] as Int);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 29 {
		t.Fatalf("expected stdlib runtime debug helpers to return 29, got %#v", result.Value)
	}
}

func TestRuntimeAutoLoadsStdlibGlobalNamespaceFunctions(t *testing.T) {
	root := t.TempDir()
	stdlibRoot := filepath.Join(root, "stdlib")
	appRoot := filepath.Join(root, "app")
	if err := os.MkdirAll(stdlibRoot, 0o755); err != nil {
		t.Fatalf("create stdlib dir failed: %v", err)
	}
	if err := os.MkdirAll(appRoot, 0o755); err != nil {
		t.Fatalf("create app dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stdlibRoot, "alloc.klang"), []byte(`
global namespace alloc {
    function New() : Int { return 7; }
    function Add(left : Int, right : Int) : Int { return left + right; }
}
namespace hidden {
    function Secret() : Int { return 99; }
}
`), 0o644); err != nil {
		t.Fatalf("write stdlib fixture failed: %v", err)
	}
	programPath := filepath.Join(appRoot, "main.klang")
	if err := os.WriteFile(programPath, []byte(`
load_as_script;
function Main() : Int {
    return Add(New(), 5);
}
`), 0o644); err != nil {
		t.Fatalf("write app fixture failed: %v", err)
	}
	program, err := file.LoadProgram(programPath)
	if err != nil {
		t.Fatalf("load program failed: %v", err)
	}
	resolved, moduleReport := modulesystem.NewResolver(stdlibRoot).ResolveProgram(program)
	if !moduleReport.Passed() {
		t.Fatalf("module resolution failed: %#v", moduleReport.Errors)
	}
	typeReport := typechecker.CheckProgram(resolved)
	if !typeReport.Passed() {
		t.Fatalf("type check failed: %#v", typeReport.Errors)
	}
	parsed := parser.ParseLoadedProgram(resolved)
	if !parsed.Passed() {
		t.Fatalf("parse failed: %#v", parsed.Errors())
	}
	result, err := New().Run(parsed)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 12 {
		t.Fatalf("expected global namespace function program to return 12, got %#v", result.Value)
	}
}

func TestRuntimeLoadsJSFilesystemModuleDescriptor(t *testing.T) {
	jsPath := filepath.Join(t.TempDir(), "library.js")
	if err := os.WriteFile(jsPath, []byte("export function init() {}\nexport const version = 1;\n"), 0644); err != nil {
		t.Fatalf("write js fixture failed: %v", err)
	}
	source := strings.ReplaceAll(jsPath, `\`, `\\`)
	result := runParsedSource(t, `
function Main() : Int {
    local JSModule module = js_import("`+source+`");
    local List[String] exports = js_exports(module);
    local String body = js_source(module);
    local JSCall descriptor = js_call(module, "init", [body]);
    return len(exports);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected two js exports, got %#v", result.Value)
	}
}

func TestRuntimeExecutesSpawnJoinWithSharedAtomic(t *testing.T) {
	result := runParsedSource(t, `
function Worker(counter : Atomic[Int], mut amount : Int) : Int {
    while amount > 0 {
        atomic_add(counter, 1);
        amount -= 1;
    }
    return atomic_load(counter);
}

function Main() : Int {
    local Atomic[Int] counter = Atomic(0);
    local Thread[Int] left = spawn(Worker, [counter, 25]);
    local Thread[Int] right = spawn(Worker, [counter, 17]);
    local String status = thread_status(left);
    join(left);
    join(right);
    return atomic_load(counter) + len(status);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) < 46 || result.Value.Data.(int) > 49 {
		t.Fatalf("expected atomic threaded program to include 42 plus status length, got %#v", result.Value)
	}
}

func TestRuntimeSnapshotsAggregateSpawnArguments(t *testing.T) {
	result := runParsedSource(t, `
function Worker(mut values : List[Int]) : Int {
    values[0] = 99;
    return values[0];
}

function Main() : Int {
    local List[Int] source = [1, 2, 3];
    local Thread[Int] worker = spawn(Worker, [source]);
    local Int changed = join(worker);
    return changed * 10 + source[0];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 991 {
		t.Fatalf("expected worker snapshot isolation, got %#v", result.Value)
	}
}

func TestRuntimeRejectsMutableGlobalAccessFromWorker(t *testing.T) {
	_, err := runParsedSourceWithError(`
global mut Int shared = 0;

function Worker() : Int {
    shared += 1;
    return shared;
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker);
    return join(worker);
}
`)

	if err == nil || !strings.Contains(err.Error(), `thread worker cannot mutate global "shared"`) {
		t.Fatalf("expected mutable-global worker error, got %v", err)
	}
}

func TestRuntimeSharesImmutableGlobalAtomicWithWorkers(t *testing.T) {
	result := runParsedSource(t, `
global Atomic[Int] shared = Atomic(0);

function Worker() : Int {
    return atomic_add(shared, 1);
}

function Main() : Int {
    local Thread[Int] left = spawn(Worker);
    local Thread[Int] right = spawn(Worker);
    join(left);
    join(right);
    return atomic_load(shared);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected shared global Atomic value 2, got %#v", result.Value)
	}
}

func TestRuntimeRejectsUnsafeSpawnArgument(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Worker(value : RefMut) : Int {
    return 0;
}

function Main() : Int {
    local T reference = RefMut(1);
    local Thread[Int] worker = spawn(Worker, [reference]);
    return join(worker);
}
`)

	if err == nil || !strings.Contains(err.Error(), "spawn argument 1 is not thread-transfer-safe") {
		t.Fatalf("expected unsafe spawn argument error, got %v", err)
	}
}

func TestRuntimeRejectsCapturedSpawnTarget(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    local Int captured = 41;
    function Worker() : Int {
        return captured + 1;
    }
    local Thread[Int] worker = spawn(Worker);
    return join(worker);
}
`)

	if err == nil || !strings.Contains(err.Error(), "spawn target must be a named workspace function") {
		t.Fatalf("expected captured worker error, got %v", err)
	}
}

func TestRuntimeExecutesLambdaFunction(t *testing.T) {
	result := runSource(t, `
function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Main() : Int {
    local Int offset = 1;
    return Apply(41, fun(value : Int) : Int {
        return value + offset;
    });
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected lambda program to return 42, got %#v", result.Value)
	}
}

func TestRuntimeDispatchesFunctionGroup(t *testing.T) {
	result := runSource(t, `
function function1_name(x : Int) : Int {
    print(x);
    return x;
}

function function2_name(x : Int, y : String) : String {
    print(x, y);
    return y;
}

function_group Poly {
    set_function_as_part_of[{ .name = "Poly" }, "function1_name", "function2_name"];
}

function Main() : Int {
    local String y = "1";
    local mut T x = if Poly(1) == Poly(1, y) then return y : "no";
    return len(x);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected polymorphic program to return 1, got %#v", result.Value)
	}
	expectedOutput := []string{"1", "1 1"}
	if strings.Join(result.Output, ",") != strings.Join(expectedOutput, ",") {
		t.Fatalf("expected output %v, got %v", expectedOutput, result.Output)
	}
}

func TestRuntimeCatchesThrownValues(t *testing.T) {
	result := runSource(t, `
function Main() : Int {
    try {
        throw :bad;
    } catch err {
        print(err);
        return 7;
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected caught throw to return 7, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != ":bad" {
		t.Fatalf("expected caught error output, got %v", result.Output)
	}
}

func TestRuntimePropagatesResultErrorsWithBang(t *testing.T) {
	result := runSource(t, `
function Fallible() : Result[Int, Atom] {
    return Err(:nope);
}

function Main() : Int {
    try {
        local Int value = Fallible()!;
        return value;
    } catch err {
        print(err);
        return 42;
    }
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 42 {
		t.Fatalf("expected propagated error to return 42, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != ":nope" {
		t.Fatalf("expected propagated error output, got %v", result.Output)
	}
}

func TestRuntimeReportsUncaughtException(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    throw :boom;
}
`)
	assertRuntimeErrorContains(t, err, "uncaught exception: :boom")
}

func TestRuntimeRejectsNonAtomThrowWhenTypeCheckingIsBypassed(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    throw "boom";
}
`)
	assertRuntimeErrorContains(t, err, "throw expects Atom, got String")
}

func TestRuntimeRejectsNonAtomResultPropagationWhenTypeCheckingIsBypassed(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return Err("boom")!;
}
`)
	assertRuntimeErrorContains(t, err, "! only propagates Result[T, Atom], got Err(String)")
}

func TestRuntimeExecutesReportStatement(t *testing.T) {
	result := runParsedSource(t, `
function BuildValue() : Int {
    local Int innerValue = 7;
    report innerValue;
    return innerValue;
}

function Main() : Int {
    local Int value = 3;
    report value;
    report BuildValue();
    return value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected report program to return 3, got %#v", result.Value)
	}
	output := strings.Join(result.Output, "\n")
	if !strings.Contains(output, "[report] value = 3 :: Int") {
		t.Fatalf("expected report output for value, got %#v", result.Output)
	}
	if !strings.Contains(output, "[report] innerValue = 7 :: Int") || !strings.Contains(output, "BuildValue") {
		t.Fatalf("expected report output with BuildValue stack frame, got %#v", result.Output)
	}
	if !strings.Contains(output, "[report] BuildValue ( ) = 7 :: Int") {
		t.Fatalf("expected report output for function call, got %#v", result.Output)
	}
}

func TestRuntimeAddsStackTraceToErrors(t *testing.T) {
	_, err := runSourceWithError(`
function Boom() : Int {
    local List[Int] values = [1];
    return values[4];
}

function Main() : Int {
    return Boom();
}
`)
	assertRuntimeErrorContains(t, err, "Stack trace:")
	assertRuntimeErrorContains(t, err, "Boom")
	assertRuntimeErrorContains(t, err, "Main")
}

func TestRuntimeExecutesAliasFunctionExtensionMethod(t *testing.T) {
	result := runParsedSource(t, `
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) -> type
    #extend do
        function get_length() -> int
            return this.length;
        end
    end
end

function Main() : Int {
    local T list = ArrayList("value", 3, 10);
    return list.get_length();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected extension method to return 3, got %#v", result.Value)
	}
}

func TestRuntimeCastsStructAliasesByFieldName(t *testing.T) {
	result := runParsedSource(t, `
alias function User(id : Int, name : String) : type = struct {}
alias function PublicUser(name : String, active : Bool = True) : type = struct {}

function Main() : Int {
    local User user = User(7, "Ada");
    local Table row = user.cast_as(Table);
    local JSON document = user.cast_as(JSON);
    local String encoded = user.cast_as(String);
    local PublicUser view = user.cast_as(PublicUser);
    assert row["id"] == 7;
    assert row["name"] == "Ada";
    assert document.id == 7;
    assert encoded == json_stringify(document);
    assert view.name == "Ada";
    assert view.active;
    return row["id"];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 7 {
		t.Fatalf("expected converted table id 7, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAliasFunctionStructBody(t *testing.T) {
	result := runParsedSource(t, `
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) : type = struct {
    trait LengthTracked {
        function Size(value : Int) : Int;
    }

    impl LengthTracked for Int {
        function Size(value : Int) : Int {
            return value;
        }
    }

    [new] {
        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));
    }

    [delete] {
        allocator.free = free_all_allocator(.{});
    }

    [side_effects] {
        local T call_site = #call_site;
        allocator.free = free_all_allocator(.{});
    }

    #extend {
        function get_length() -> int {
            return this.length;
        }

        function with_extra(extra : Int) -> int {
            return this.length + extra;
        }
    }
}

function Main() : Int {
    local T list = ArrayList("value", 3, 10);
    return list.get_length() + list.with_extra(4) + list.__hooks + list.__methods + list.__traits + list.__impls;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 17 {
		t.Fatalf("expected struct alias runtime metadata and methods to return 17, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAliasStructFieldsMethodsAndGenerics(t *testing.T) {
	result := runParsedSource(t, `
alias function Boxed[T: Any](items : List[T], capacity : Int) : type = struct {
    #extend {
        function count() : Int {
            return len(this.items);
        }

        function get(index : Int) : T {
            local List[T] values = this.items as List[T];
            return values[index];
        }

        function push(value : T) : Boxed {
            local mut List[T] values = clone (this.items as List[T]);
            values[len(values)] = value;
            return Boxed(values, this.capacity);
        }
    }
}

function Main() : Int {
    let mut x = Boxed([1, 2], 2);
    local Int first = x.get(0);
    local Int total = x.count();
    x = x.push(3);
    return first + total + x.get(2);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 6 {
		t.Fatalf("expected alias struct generic methods to return 6, got %#v", result.Value)
	}
}

func TestRuntimeExecutesAliasExtensionMethodArguments(t *testing.T) {
	result := runParsedSource(t, `
alias function Counter(value: int) -> type
    #extend do
        function add(amount : Int) -> int
            return this.value + amount;
        end
    end
end

function Main() : Int {
    local T counter = Counter(2);
    return counter.add(3);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 5 {
		t.Fatalf("expected extension method argument call to return 5, got %#v", result.Value)
	}
}

func TestRuntimeRejectsAliasExtensionMethodArgumentMismatch(t *testing.T) {
	_, err := runParsedSourceWithError(`
alias function Counter(value: int) -> type
    #extend do
        function add(amount : Int) -> int
            return this.value + amount;
        end
    end
end

function Main() : Int {
    local T counter = Counter(2);
    return counter.add("bad");
}
`)

	assertRuntimeErrorContains(t, err, "method Counter.add argument 1 expects Int")
}

func TestRuntimeExecutesAliasOperatorOverloads(t *testing.T) {
	result := runParsedSource(t, `
alias function Vector(x : Int, y : Int) : type = struct {
    operator +(other : Vector) : Vector {
        return Vector(this.x + other.x, this.y + other.y);
    }

    operator *(factor : Int) : Vector {
        return Vector(this.x * factor, this.y * factor);
    }

    operator **(power : Int) : Vector {
        return Vector(this.x ** power, this.y ** power);
    }

    operator ==(other : Vector) : Bool {
        return this.x == other.x and this.y == other.y;
    }

    operator !=(other : Vector) : Bool {
        return not (this == other);
    }
}

function Main() : Int {
    local mut Vector total = Vector(1, 2) + Vector(3, 4);
    total += Vector(1, 1);
    local Vector scaled = total * 2;
    local Vector powered = Vector(2, 3) ** 2;
    assert total == Vector(5, 7);
    assert scaled != Vector(1, 1);
    return scaled.x + scaled.y + powered.x + powered.y;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 37 {
		t.Fatalf("expected overloaded vector operators to return 37, got %#v", result.Value)
	}
}

func TestRuntimeExecutesStandaloneExtensionMethods(t *testing.T) {
	result := runParsedSource(t, `
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

#extend Duration {
    function doubled() : Int {
        return this.value * 2;
    }
}

#extend String {
    function surrounded(left : String, right : String = "]") : String {
        return left + this + right;
    }
}

function Main() : Int {
    local String label = "core".surrounded("[");
    return 10.days().ago() + Duration(len(label)).doubled();
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected chained standalone extensions to return 2, got %#v", result.Value)
	}
}

func TestRuntimeExecutesGenericListExtensionMethods(t *testing.T) {
	result := runParsedSource(t, `
#extend List[T] {
    function first_or(fallback : T) : T {
        if len(this) == 0 {
            return fallback;
        }
        return this[0];
    }

    function reversed() : List[T] {
        local mut List[T] result;
        local mut Int index = len(this) - 1;
        while index >= 0 {
            result[len(result)] = this[index];
            index -= 1;
        }
        return result;
    }
}

function Main() : Int {
    local List[Int] values = [4, 5, 6];
    local List[Int] reversed = values.reversed();
    return values.first_or(0) + reversed[0];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 10 {
		t.Fatalf("expected generic List extensions to return 10, got %#v", result.Value)
	}
}

func TestRuntimeExecutesRegionArraySyntax(t *testing.T) {
	result := runParsedSource(t, `
region MyRegion(T, sizeof(T) * 100, 10);

function Main() : Int {
    local mut T[MyRegion] myArray;
    myArray[0] = "String";
    return len(myArray);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 1 {
		t.Fatalf("expected region array length 1, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTemporaryRegionArraySyntax(t *testing.T) {
	engine := New()
	result := runParsedSourceOnRuntime(t, engine, `
temp region Scratch(T, sizeof(T) * 16, 4);

function Main() : Int {
    local mut T[Scratch] values;
    values[0] = "value";
    values[1] = "next";
    return len(values);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 2 {
		t.Fatalf("expected temporary region program to return 2, got %#v", result.Value)
	}
	assertRuntimeState(t, engine.stateRecordsValue().Data.([]Value), "temporary_region", "Scratch", "define")
	if result.Memory.TempObjects == 0 {
		t.Fatalf("expected temporary region allocation to use temporary memory bucket, got %#v", result.Memory)
	}
}

func TestRuntimeRejectsRegionArrayCapacityOverflow(t *testing.T) {
	_, err := runParsedSourceWithError(`
region Tiny(T, 1, 1);

function Main() : Int {
    local mut T[Tiny] myArray;
    myArray[1] = "too far";
    return len(myArray);
}
`)

	assertRuntimeErrorContains(t, err, "array index 1 exceeds region Tiny capacity 1")
}

func TestRuntimeExecutesAllocatorConstructors(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local T boxed = Box("value");
    local T arena = ArenaAllocator();
    return len(boxed.kind) + len(arena.kind);
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 17 {
		t.Fatalf("expected allocator kind lengths to total 17, got %#v", result.Value)
	}
}

func TestRuntimeExecutesTableAsyncIteratorAndCoroutineBuiltins(t *testing.T) {
	result := runParsedSource(t, `
async function LoadValue() : Int {
    return 40;
}

function BuildValue() : Int {
    return 2;
}

function Main() : Int {
    local mut Table data = {"name": "klang", 1: 5};
    data["extra"] = 7;
    local Iterator[T] iterator = iter([1, 2, 3]);
    local Option[T] first = next(iterator);
    local Option[T] second = next(iterator);
    local Coroutine[Int] co = coroutine(BuildValue);
    local Option[Int] resumed = resume(co);
    return await LoadValue() + data.extra + len(data.name) + first.value + second.value + resumed.value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 57 {
		t.Fatalf("expected table/async/iterator/coroutine program to return 57, got %#v", result.Value)
	}
}

func TestRuntimeExecutesLazyFusedIteratorPipelines(t *testing.T) {
	result := runParsedSource(t, `
global mut Int PipelineCalls = 0;

function IsEven(value : Int) : Bool {
    return value % 2 == 0;
}

function Track(value : Int) : Int {
    PipelineCalls += 1;
    return value * 10;
}

function Add(total : Int, value : Int) : Int {
    return total + value;
}

function GreaterThanFive(value : Int) : Bool {
    return value > 5;
}

function OnlyFirst(value : Int) : Int {
    assert value == 1;
    return value;
}

function PositiveEntry(entry : Table) : Bool {
    return (entry.value as Int) > 0;
}

function EntryValue(entry : Table) : Int {
    return entry.value as Int;
}

function Main() : Int {
    local List[Int] values = [6, 1, 4, 3, 2];
    local Iterator[Int] pipeline = values.filter(IsEven).limit(3).map(Track);
    assert PipelineCalls == 0;

    local List[Int] sorted = pipeline.sort();
    assert PipelineCalls == 3;
    assert sorted[0] == 20;
    assert sorted[1] == 40;
    assert sorted[2] == 60;

    local Int total = values.filter(IsEven).fold(0, Add);
    local Option[Int] first = values.filter(IsEven).first();
    local Bool any = values.map(Track).any(GreaterThanFive);
    local Int evenCount = values.filter(IsEven).count;
    local Int limitedCount = [1, 2].map(OnlyFirst).limit(1).count;
    local Table dictionary = {"six": 6, "two": 2, "four": 4};
    local List[Int] dictionaryValues = dictionary.filter(PositiveEntry).map(EntryValue).sort();
    local Map[String, Int] typedDictionary = {"eight": 8, "one": 1};
    local List[Int] typedValues = typedDictionary.map(EntryValue).sort();
    assert total == 12;
    assert evenCount == 3;
    assert limitedCount == 1;
    assert first.some;
    assert first.value == 6;
    assert any;
    assert dictionaryValues[0] == 2;
    assert dictionaryValues[2] == 6;
    assert typedValues[0] == 1;
    assert typedValues[1] == 8;
    return sorted[2];
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 60 {
		t.Fatalf("expected fused pipeline result 60, got %#v", result.Value)
	}
}

func TestRuntimeExecutesCoreTableSemantics(t *testing.T) {
	result := runParsedSource(t, `
function Main() : Int {
    local mut Table data = {"name": "klang", 0: 9, 1: 10, "1": 20, True: 30, 'x': 40, "count": 99};
    local Table snapshot = data;
    data[1] = 11;
    data = table_delete(data, "name");

    local Table parent = {"fallback": 7};
    local Table child = table_set_fallback(data, parent);
    local List[Table] entries = table_entries(child);
    local Iterator[Table] iterator = iter(child);
    local Option[Table] first = next(iterator);

    assert table_has(child, 1);
    assert not table_has(child, "name");
    assert child.count == 6;
    assert child["count"] == 99;
    assert table_sequence_count(child) == 2;
    assert child.fallback == 7;

    return data[1] + snapshot[1] + data["1"] + data[True] + data['x'] + len(entries) + first.value.value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 126 {
		t.Fatalf("expected core table semantics program to return 126, got %#v", result.Value)
	}
}

func TestRuntimeRejectsInvalidAndMissingTableKeys(t *testing.T) {
	_, invalidErr := runParsedSourceWithError(`
function Main() : Int {
    local mut Table data = {};
    data[[1]] = 1;
    return 0;
}
`)
	assertRuntimeErrorContains(t, invalidErr, "List cannot be used as a table key")

	_, missingErr := runParsedSourceWithError(`
function Main() : Int {
    local Table data = {};
    return data["missing"];
}
`)
	assertRuntimeErrorContains(t, missingErr, "table key missing does not exist")
}

func TestRuntimeRejectsAwaitOnNonAwaitable(t *testing.T) {
	_, err := runParsedSourceWithError(`
function Main() : Int {
    return await 1;
}
`)

	assertRuntimeErrorContains(t, err, "await expects Awaitable")
}

func TestRuntimeAllocatesAliasAndAllocatorObjectsOnHeap(t *testing.T) {
	result := runParsedSource(t, `
alias function Boxed(value: int) -> type
end

function Main() : Int {
    local T boxed = Box(1);
    local T custom = Boxed(2);
    return boxed.value + custom.value;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 3 {
		t.Fatalf("expected object values to return 3, got %#v", result.Value)
	}
	if result.Memory.HeapObjects < 2 {
		t.Fatalf("expected allocator/custom objects on heap, got %#v", result.Memory)
	}
}

func TestRuntimeSupportsBuiltinProtocolMembers(t *testing.T) {
	result := runParsedSource(t, `
function Remember(index : Int) : Int {
    return index + 10;
}

function Main() : Int {
    local String upper = "hallo".uppercase();
    local String lower = upper.lowercase();
    local Char letter = 'k'.uppercase();
    local Int last = 3.times(Remember);
    if letter != 'K' {
        return 0;
    }
    return upper.count + lower.count + [1, 2, 3].count + last;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 25 {
		t.Fatalf("expected builtin protocol members to return 25, got %#v", result.Value)
	}
}

func TestRuntimeSupportsEnumIotaStyleValues(t *testing.T) {
	result := runParsedSource(t, `
enum Status {
    Pending;
    Running = 10;
    Done;
}

function Main() : Int {
    local Status status = Status.Done;
    if status == {
        case Status.Pending:
            return 1;
        case Status.Running:
            return 2;
        case Status.Done:
            return status.ordinal + len(status.name);
    }
    return 0;
}
`)

	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 15 {
		t.Fatalf("expected enum program to return 15, got %#v", result.Value)
	}
}

func runSource(t *testing.T, source string) Result {
	t.Helper()

	result, err := runSourceWithError(source)
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	return result
}

func runParsedSource(t *testing.T, source string) Result {
	t.Helper()

	return runParsedSourceOnRuntime(t, New(), source)
}

func runParsedSourceOnRuntime(t *testing.T, runtime *Runtime, source string) Result {
	t.Helper()

	parsedProgram, errors := parser.Parse(source)
	if len(errors) != 0 {
		t.Fatalf("parse failed: %#v", errors)
	}
	result, err := runtime.Run(parser.ParsedProgram{
		Name: "parsed",
		Sources: []parser.ParsedSource{
			{Path: "parsed.klang", Program: parsedProgram},
		},
	})
	if err != nil {
		t.Fatalf("runtime failed: %v", err)
	}
	return result
}

func runParsedSourceWithError(source string) (Result, error) {
	parsedProgram, errors := parser.Parse(source)
	if len(errors) != 0 {
		return Result{}, Error{Message: fmt.Sprintf("parse failed: %#v", errors)}
	}

	return New().Run(parser.ParsedProgram{
		Name: "parsed",
		Sources: []parser.ParsedSource{
			{Path: "parsed.klang", Program: parsedProgram},
		},
	})
}

func runSourceWithError(source string) (Result, error) {
	program := file.Program{
		Name:       "test",
		Root:       "tests",
		EntryPoint: "tests/test.klang",
		Files: []file.SourceFile{
			{
				Path:  "tests/test.klang",
				Lines: strings.Split(strings.TrimSpace(source), "\n"),
			},
		},
	}
	return RunProgram(program)
}

func assertRuntimeState(t *testing.T, states []Value, kind string, name string, event string) {
	t.Helper()

	for _, state := range states {
		if state.Kind != ValueTable {
			continue
		}
		table := state.Data.(TableData)
		if tableString(table, "kind") == kind && tableString(table, "name") == name && tableString(table, "event") == event {
			return
		}
	}

	t.Fatalf("expected runtime state kind=%q name=%q event=%q, got %#v", kind, name, event, states)
}

func tableString(table TableData, key string) string {
	value, ok := tableGet(table, TableKey{Kind: ValueString, Repr: key})
	if !ok || value.Kind != ValueString {
		return ""
	}
	return value.Data.(string)
}

func assertRuntimeErrorContains(t *testing.T, err error, expected string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected runtime error containing %q, got nil", expected)
	}
	if !strings.Contains(err.Error(), expected) {
		t.Fatalf("expected runtime error containing %q, got %v", expected, err)
	}
}

func TestRuntimeUsesAtomsForThrownErrorCodes(t *testing.T) {
	result := runSource(t, `
function Fail() {
    throw :not_found;
}

function Main() : Int {
    local Atom missing = :not_found;
    local Atom dynamic = Atom("permission_denied");
    assert dynamic.name == "permission_denied";
    assert dynamic as String == "permission_denied";
    assert "not_found" as Atom == missing;

    local Table codes = {:not_found: 404, :permission_denied: 403};
    local Set[Atom] known = Set([missing, dynamic, :not_found]);
    assert len(known) == 2;
    assert codes[missing] == 404;
    local mut Int matched = 0;
    if missing == {
        case :not_found:
            matched = 1;
        case:
            matched = 2;
    }
    assert matched == 1;

    try {
        Fail();
    } catch error {
        assert error == missing;
        print(error);
        return codes[error];
    }
    return 0;
}
`)
	if result.Value.Kind != ValueInt || result.Value.Data.(int) != 404 {
		t.Fatalf("expected caught Atom code to return 404, got %#v", result.Value)
	}
	if strings.Join(result.Output, ",") != ":not_found" {
		t.Fatalf("expected Atom display output, got %v", result.Output)
	}
}

func TestRuntimeRejectsInvalidDynamicAtomNames(t *testing.T) {
	_, err := runSourceWithError(`
function Main() : Int {
    local Atom invalid = Atom("not found");
    return 0;
}
`)
	assertRuntimeErrorContains(t, err, `invalid Atom name "not found"`)
}

func TestRuntimeExecutesOSStdlibFacade(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get cwd failed: %v", err)
	}
	repoRoot := filepath.Join(cwd, "..", "..", "..")
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("chdir repo root failed: %v", err)
	}
	defer func() {
		if restoreErr := os.Chdir(cwd); restoreErr != nil {
			t.Errorf("restore cwd failed: %v", restoreErr)
		}
	}()

	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("find test executable: %v", err)
	}
	const environmentKey = "KLANG_OS_STDLIB_TEST"
	originalEnvironment, hadEnvironment := os.LookupEnv(environmentKey)
	defer func() {
		if hadEnvironment {
			_ = os.Setenv(environmentKey, originalEnvironment)
		} else {
			_ = os.Unsetenv(environmentKey)
		}
	}()

	result := runSource(t, fmt.Sprintf(`
import "os";

function Main() : Int {
    local Table platform = os.PLATFORM();
    assert len(platform["name"] as String) > 0;
    assert len(platform["arch"] as String) > 0;
    assert platform["cpu_count"] as Int > 0;
    assert os.PROCESS_ID() > 0;
    assert len(os.TEMP_DIR()) > 0;

    local Result[String, Atom] current = os.CURRENT_DIR();
    local Result[String, Atom] home = os.HOME_DIR();
    local Result[String, Atom] hostname = os.HOSTNAME();
    assert current.ok and home.ok and hostname.ok;

    local Result[Bool, Atom] set = os.SET_ENV(%q, "facade");
    assert set.ok;
    assert os.HAS_ENV(%q);
    assert os.GET_ENV_OR(%q, "missing") == "facade";
    local Map[String, String] environment = os.ENVIRONMENT();
    assert len(environment) > 0;

    local Result[Table, Atom] execution = os.EXECUTE(%q, ["-test.run=^$"]);
    assert execution.ok;
    local Table fallback;
    local Table process = result_unwrap_or(execution, fallback);
    assert os.SUCCEEDED(process);
    assert os.EXIT_CODE(process) == 0;
    _ = os.STDOUT(process);
    _ = os.STDERR(process);

    local Result[Bool, Atom] unset = os.UNSET_ENV(%q);
    assert unset.ok;
    assert not os.HAS_ENV(%q);
    return platform["cpu_count"] as Int;
}
`, environmentKey, environmentKey, environmentKey, executable, environmentKey, environmentKey))

	if result.Value.Kind != ValueInt || result.Value.Data.(int) <= 0 {
		t.Fatalf("expected os stdlib facade to return a positive CPU count, got %#v", result.Value)
	}
}
