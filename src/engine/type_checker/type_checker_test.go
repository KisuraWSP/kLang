package typechecker

import (
	"fmt"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestCheckProgramAcceptsTypedVariablesFunctionsAndCalls(t *testing.T) {
	program := programFromSource(`
global mut Int counter = 0;

function Add(left : Int, right : Int) : Int {
    local Int total = left + right;
    counter += 1;
    return total;
}

function Main() : Int {
    local Int value = Add(1, 2);
    print(value);
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsNativeFileOperations(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local File target = File("notes.txt");
    local Result[File, String] created = target.create();
    local Result[Int, String] written = target.write("hello");
    local Result[Int, String] appended = file_append(target, " world");
    local Result[String, String] content = target.read();
    local Result[List[String], String] lines = file_read_lines(target);
    local Result[Bool, String] exists = target.exists();
    local Result[Int, String] size = file_size(target);
    local Result[Bool, String] removed = target.remove();
    local File castTarget = "cast.txt" as File;
    local String castPath = castTarget as String;
    print(target.path, target.name, target.extension);
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected File program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidNativeFileOperations(t *testing.T) {
	badConstructor := CheckProgram(programFromSource(`
function Main() : Int {
    local File target = File(42);
    return 0;
}
`))
	assertTypeError(t, badConstructor, "File path expects String, got Int")

	badWrite := CheckProgram(programFromSource(`
function Main() : Int {
    local File target = File("notes.txt");
    local Result[Int, String] written = target.write(42);
    return 0;
}
	`))
	assertTypeError(t, badWrite, "expects String, got Int")
}

func TestCheckProgramAcceptsNativeOSOperations(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local OS host = OS();
    local Result[String, String] current = host.current_dir();
    local Result[Bool, String] changed = os_change_dir(host, ".");
    local String temporary = host.temp_dir();
    local Result[String, String] home = os_home_dir(host);
    local Result[String, String] hostname = host.hostname();
    local Int pid = os_process_id(host);
    local Option[String] path = host.get_env("PATH");
    local Result[Bool, String] set = os_set_env(host, "KLANG_TEST", "yes");
    local Result[Bool, String] unset = host.unset_env("KLANG_TEST");
    local Map[String, String] environment = host.environment();
    local Result[Table, String] execution = host.execute("command", ["argument"]);
    print(host.name, host.arch, host.cpu_count, host.path_separator);
    return pid;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected OS program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidNativeOSOperations(t *testing.T) {
	badConstructor := CheckProgram(programFromSource(`
function Main() : Int {
    local OS host = OS(1);
    return 0;
}
`))
	assertTypeError(t, badConstructor, "OS expects 0 arguments")

	badExecute := CheckProgram(programFromSource(`
function Main() : Int {
    local OS host = OS();
    local Result[Table, String] execution = host.execute("command", "argument");
    return 0;
}
`))
	assertTypeError(t, badExecute, "expects List[String], got String")
}

func TestCheckProgramAcceptsParsableMetadataAndRestrictedKeywordMacro(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Render(value : String) : String;
}

impl Printable for String {
    function Render(value : String) : String {
        return value;
    }
}

alias printer = Parsable[T Printable].keyword_macro {
    print(get_args_from_parsable()[0]);
}

function Main() : Int {
    let mut Parsable[T Printable] empty;
    let source = //
function Parsed() : Int { return 1; }
//;
    let parsed = Parsable(source, ["source"]);
    print(len(parsable_ast(parsed)), parsable_workspace(parsed));
    let polling = parsable_begin_polling(parsed);
    let response = parsable_poll_message(polling, {"kind": "REQUEST_AST"});
    print(len(response["ast"] as List[T]), response["intercepted"] as Bool);
    printer "hallo";
    return 0;
}
`)
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected Parsable program to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsKeywordMacroArgumentOutsideTraitRestriction(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Render(value : String) : String;
}

alias printer = Parsable[T Printable].keyword_macro {
    print(get_args_from_parsable());
}

function Main() : Int {
    printer 10;
    return 0;
}
`)
	report := CheckProgram(program)
	if report.Passed() {
		t.Fatal("expected Printable restriction failure")
	}
	if !strings.Contains(fmt.Sprint(report.Errors), "requires Printable") {
		t.Fatalf("expected restriction diagnostic, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsKeywordMacroExpansionHelpers(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected keyword macro expansion helpers to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramTracksCompileTimeState(t *testing.T) {
	program := programFromSource(`
global mut Int counter = 0;

function Add(left : Int, mut right : Int) : Int {
    local Int total = left + right;
    return total;
}

function Main() : Int {
    return Add(counter, 1);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected state tracking type check to pass, got: %v", report.Errors)
	}
	assertState(t, report.States, "global", "counter", "Int")
	assertState(t, report.States, "parameter", "left", "Int")
	assertState(t, report.States, "parameter", "right", "Int")
	assertState(t, report.States, "local", "total", "Int")
	assertState(t, report.States, "return", "Add", "Int")
	assertState(t, report.States, "builtin", "Args", "List[String]")
}

func TestCheckProgramTracksTemporaryVariables(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    temp local Int scratch = 40;
    temp let answer = scratch + 2;
    temp local Int unusedScratch = 99;
    return answer;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected temporary variable program to type check, got: %v", report.Errors)
	}
	assertState(t, report.States, "temporary", "scratch", "Int")
	assertState(t, report.States, "temporary", "answer", "Int")
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, "unusedScratch") {
			t.Fatalf("did not expect unused warning for temporary variable, got %#v", report.Warnings)
		}
	}
}

func TestCheckProgramTreatsHereStringsAsTypedStrings(t *testing.T) {
	program := programFromSource(`
function Render() : String {
    return //
<main>
    <h1>Hello from kLang!</h1>
</main>
//;
}

function Accept(value : String) : Int {
    return len(value);
}

function Main() : Int {
    local String explicit = //
<p>typed</p>
//;
    let inferred = //
<p>inferred</p>
//;
    let mut mutable = //
<p>mutable</p>
//;
    mutable = Render();
    return Accept(explicit) + Accept(inferred) + Accept(mutable);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected here string type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsJSONBuiltinsAndHereStrings(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local JSON document = JSON(//
{"name":"kLang","items":[1,2]}
//);
    local JSON name = document.name;
    local JSON first = document["items"][0];
    local Option[String] text = json_string(name);
    local Option[Int] number = json_int(first);
    local Option[JSON] missing = json_get(document, "missing");
    local Result[JSON, String] parsed = json_parse("null");
    local String encoded = json_stringify(document);
    local JSON numberJSON = JSON(42);
    local String encodedString = json_stringify("native");
    local Result[String, String] safeEncoded = json_encode([1, 2, 3]);
    local Result[T, String] native = json_decode("{\"ok\":true}");
    return document.count + len(encoded);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected JSON program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidJSONUse(t *testing.T) {
	badConstructor := programFromSource(`
function Main() : Int {
    local JSON value = JSON(Set(["bad"]));
    return 0;
}
`)
	assertTypeError(t, CheckProgram(badConstructor), "JSON expects a serializable value, got Set[String]")

	badMutation := programFromSource(`
function Main() : Int {
    local mut JSON value = JSON("{}");
    value["name"] = JSON("null");
    return 0;
}
`)
	assertTypeError(t, CheckProgram(badMutation), "JSON indexes cannot be assigned")

	badHelper := programFromSource(`
function Main() : Int {
    local String value = json_stringify(Set(["not json"]));
    return len(value);
}
`)
	assertTypeError(t, CheckProgram(badHelper), "json_stringify expects a serializable value, got Set[String]")
}

func TestCheckProgramAcceptsAssertAndRuntimeTypeInfo(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Type info = Int.get_runtime_type_info();
    assert info.name == "Int";
    assert info.size == Int.sizeof;
    local Table layout = info.layout;
    return layout.size;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected assert and Type metadata program to type check, got: %v", report.Errors)
	}

	badAssert := programFromSource(`
function Main() : Int {
    assert 1;
    return 0;
}
`)
	assertTypeError(t, CheckProgram(badAssert), "assert expects Bool, got Int")
}

func TestCheckProgramAcceptsReportStatement(t *testing.T) {
	program := programFromSource(`
function BuildValue() : Int {
    return 7;
}

function Main() : Int {
    local Int value = 3;
    report value;
    report BuildValue();
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected report program to type check, got: %v", report.Errors)
	}

	badReport := programFromSource(`
function Main() : Int {
    report missing;
    return 0;
}
`)
	assertTypeError(t, CheckProgram(badReport), `unknown identifier "missing"`)
}

func TestCheckProgramAcceptsUnicodeIdentifiersAndNumericLiteralBases(t *testing.T) {
	program := programFromSource(`
function එකතු(අගය : Int, 😀 : Int) : Int {
    local Int hex = 0x2A;
    local Int octal = 0o10;
    local Int binary = 0b101;
    local Int negative = -5;
    return අගය + 😀 + hex + octal + binary + negative;
}

function Main() : Int {
    local Int මුළු = එකතු(1, 2);
    return මුළු;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected unicode and numeric literal program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsNumberSeparators(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int big = 1_000_000;
    local Int hex = 0xFF_FF;
    local Int binary = 0b1010_0101;
    local Int mode = 0o7_55;
    local Float ratio = 12_345.67_89;
    local i16 small = 32_000;
    return big + hex + binary + mode + ratio as Int + small;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected number separator program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsVariableTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String value = 10;
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign Int to local String value")
}

func TestCheckProgramRejectsReturnTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    return "wrong";
}
`)

	assertTypeError(t, CheckProgram(program), "returns Int but return expression is String")
}

func TestCheckProgramRejectsImmutableMutation(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    value = 2;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot mutate immutable variable")
}

func TestCheckProgramChecksConstInitializers(t *testing.T) {
	accept := programFromSource(`
const intSize = Int.sizeof;
const folded = (1 + 2) * 3;

function Main() : Int {
    const localSize = String.sizeof + 1;
    return intSize + folded + localSize;
}
`)
	if report := CheckProgram(accept); !report.Passed() {
		t.Fatalf("expected compile-time const initializers to pass, got %#v", report.Errors)
	}

	rejectCall := programFromSource(`
function MakeValue() : Int {
    return 1;
}

const bad = MakeValue();

function Main() : Int {
    return 0;
}
`)
	assertTypeError(t, CheckProgram(rejectCall), "const bad requires a compile-time constant initializer")

	rejectIdentifier := programFromSource(`
function Main() : Int {
    local Int value = 1;
    const bad = value;
    return bad;
}
`)
	assertTypeError(t, CheckProgram(rejectIdentifier), "const bad requires a compile-time constant initializer")
}

func TestCheckProgramRejectsRValueAssignmentTarget(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int value = 1;
    (value + 1) = 3;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "assignment target must be an lvalue")
}

func TestCheckProgramRejectsFunctionArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Main() : Int {
    return Add("nope", 1);
}
`)

	assertTypeError(t, CheckProgram(program), "argument 1 expects Int, got String")
}

func TestCheckProgramInfersGenericFunctionReturnWithUnification(t *testing.T) {
	program := programFromSource(`
function Identity(value : T) : T {
    return value;
}

function Main() : Int {
    local Int value = Identity(10);
    local List[Int] values = Identity([1, 2, 3]);
    return value + values[0];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected generic unification type check to pass, got: %v", report.Errors)
	}

	reject := programFromSource(`
function Identity(value : T) : T {
    return value;
}

function Main() : Int {
    local String value = Identity(10);
    return 0;
}
`)
	assertTypeError(t, CheckProgram(reject), "cannot assign Int to local String value")
}

func TestCheckProgramTracksFlowSensitiveTAssignments(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut T value = 1;
    value = "ready";
    local String text = value;
    return len(text);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected flow-sensitive T assignment to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsInferredVariableKeywordsAndSizeof(t *testing.T) {
	program := programFromSource(`
val maybeGlobal = Some(10);
var globalCount = 1;
const globalSize = Int.sizeof;

function Main() : Int {
    let maybeLocal = Some(69420);
    let mut localCount = 1;
    const localSize = Int.sizeof;
    let size intSize = Int.sizeof;
    localCount += 1;
    globalCount += 1;
    return maybeLocal.value + localCount + globalCount + localSize + intSize + globalSize;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected inferred variable keyword type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsChildNumericTypes(t *testing.T) {
	program := file.Program{Files: []file.SourceFile{{
		Path: "main.klang",
		Lines: strings.Split(`
global namespace types {
    alias i8 = Int.child(8);
}

function Main() : Int {
    local x : Int.child(8) = 127;
    local i16 y = x;
    local types.u8 z = 255;
    const byteSize = Int.child(8).sizeof;
    const complexSize = complex128.sizeof;
    return x + y + z + byteSize + complexSize;
}
`, "\n"),
	}}}
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected child numeric types to pass, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsOutOfRangeChildIntegerLiteral(t *testing.T) {
	program := file.Program{Files: []file.SourceFile{{
		Path: "main.klang",
		Lines: strings.Split(`
function Main() : Int {
    local i8 value = 128;
    return value;
}
`, "\n"),
	}}}
	assertTypeError(t, CheckProgram(program), "literal 128 does not fit in Int.child(8)")
}

func TestCheckProgramWarnsAboutUnusedVariablesAndParameters(t *testing.T) {
	program := programFromSource(`
function Add(left : Int, right : Int, unusedParam : Int) : Int {
    local Int unusedLocal = 10;
    local mut Int assignedOnly = 1;
    assignedOnly = 2;
    local _ = unusedLocal;
    return left + right;
}

function Main() : Int {
    return Add(1, 2, 3);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected program to pass with warnings, got %#v", report.Errors)
	}
	assertTypeWarning(t, report, `unused parameter "unusedParam"`)
	assertTypeWarning(t, report, `unused variable "assignedOnly"`)
}

func TestCheckProgramDoesNotWarnForReadVariablesOrDiscard(t *testing.T) {
	program := programFromSource(`
function Add(value : Int, _ : Int) : Int {
    local Int used = value + 1;
    local _ = 99;
    return used;
}

function Main() : Int {
    return Add(1, 2);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected program to pass, got %#v", report.Errors)
	}
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, "unused") {
			t.Fatalf("did not expect unused warning, got %#v", report.Warnings)
		}
	}
}

func TestCheckProgramAcceptsLocalTypeInference(t *testing.T) {
	program := programFromSource(`
function MakeName() : String {
    return "klang";
}

function Main() : Int {
    local count = 1;
    local mut values = [1, 2, 3];
    local name = MakeName();
    values[0] = count + len(name);
    return values[0];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected local type inference program to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsLocalInferredMutationMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut count = 1;
    count = "bad";
    return count;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign String to Int")
}

func TestCheckProgramAcceptsVariableDestructuring(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local [first, second] = [1, 2];
    let mut [left, right] = [3, 4];
    right = first + second + left;
    return right;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected destructuring type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsDestructuredMutationMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut [count, other] = [1, 2];
    count = "bad";
    return other;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign String to Int")
}

func TestCheckProgramAcceptsDiscardIdentifierReuse(t *testing.T) {
	program := programFromSource(`
function Value() : Int {
    return 1;
}

function Main() : Int {
    _ = Value();
    _ = Value();
    local _ = Value();
    let _ = Value();
    local [_, kept, _] = [1, 2, 3];
    return kept;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected discard identifier type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsDiscardIdentifierRead(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local _ = 1;
    return _;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "_"`)
}

func TestCheckProgramRejectsDiscardCompoundAssignment(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    _ += 1;
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "discard assignment only supports =")
}

func TestCheckProgramAcceptsLazyVariableInitialization(t *testing.T) {
	program := programFromSource(`
function BuildCount() : Int {
    return 10;
}

function Main() : Int {
    lazy local Int count = BuildCount();
    lazy let inferred = count + 1;
    return inferred;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected lazy variable type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsLazyVariableTypeMismatch(t *testing.T) {
	program := programFromSource(`
function BuildCount() : Int {
    return 10;
}

function Main() : String {
    lazy local String count = BuildCount();
    return count;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign Int to local String count")
}

func TestCheckProgramAcceptsMultipleReturnsAnyAndPrivateInline(t *testing.T) {
	program := programFromSource(`
private inline function Pair() : (name : String, value : Int) {
    return "ready", 7;
}

function Echo(value : Any) : Any {
    return value;
}

function Main() : Int {
    local Any anyValue = Echo("ok");
    let pair = Pair();
    return len(anyValue as String) + 1;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected multiple return/Any/private inline type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsMultiVariableDeclarationFromMultipleReturn(t *testing.T) {
	program := programFromSource(`
function Multi() : (table : Table, count : Int) {
    return {"name": "klang"}, 7;
}

function Main() : Int {
    local Table x, Int y = Multi();
    return y + x.count;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected multi-variable declaration to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsMultiVariableDeclarationTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Multi() : (table : Table, count : Int) {
    return {"name": "klang"}, 7;
}

function Main() : Int {
    local Int x, String y = Multi();
    return x;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign return value 1 (Table) to local Int x")
}

func TestCheckProgramRejectsMultipleReturnMismatch(t *testing.T) {
	program := programFromSource(`
function Pair() : (String, Int) {
    return 1, "bad";
}

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "return value 1 expects String but got Int")
}

func TestCheckProgramRejectsConstMutation(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    const value = 1;
    value = 2;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot mutate immutable variable")
}

func TestCheckProgramRejectsInferredDeclarationWithoutUsableValue(t *testing.T) {
	program := programFromSource(`
const value = Missing();

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "unknown function \"Missing\"")
}

func TestCheckProgramAcceptsConditionalExpressionsDefaultsAndZeroValues(t *testing.T) {
	program := programFromSource(`
function Init() : Int {
    return 1;
}

function Flag(value : Int = 1) : Bool {
    local Bool result = if Init() > 0 then return False : True;
    return result;
}

function Main() : Int {
    local Int zeroInt;
    local String zeroString;
    local Bool flag = Flag();
    if flag or zeroString == "" {
        return zeroInt + 1;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected conditional/default/zero type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsRestrictedGenericTypes(t *testing.T) {
	program := programFromSource(`
function IdentityNumber[T restrict[UInt, Int, Float]](value : T = 1) : T {
    return value;
}

function Main() : Int {
    local Int value = IdentityNumber();
    local Float other = IdentityNumber(1.5);
    return value + other as Int;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected restricted generic type check to pass, got: %v", report.Errors)
	}

	reject := programFromSource(`
function IdentityNumber[T restrict[UInt, Int, Float]](value : T) : T {
    return value;
}

function Main() : Int {
    local String value = IdentityNumber("bad");
    return 0;
}
`)
	assertTypeError(t, CheckProgram(reject), "argument 1 expects T:UInt|Int|Float, got String")
}

func TestCheckProgramAcceptsAnyConstrainedGenericFunctions(t *testing.T) {
	program := programFromSource(`
function Transform[T Any, U Any](value : T, mapper : Function[T, U]) : U {
    return mapper(value);
}

function Length(value : String) : Int { return len(value); }

function Main() : Int {
    return Transform("kLang", Length);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected Any-constrained generic function to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsRestrictedGenericVariableMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut T restrict[UInt, Int, Float] value = "bad";
    return 0;
}
`)
	assertTypeError(t, CheckProgram(program), "cannot assign String to local T:UInt|Int|Float value")
}

func TestCheckProgramAcceptsNamedGenericConstraints(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
    function Show(value : Int) : String {
        return value as String;
    }
}

function Add[T numeric](left : T, right : T) : T {
    return left + right;
}

function First[T iterable](values : T) : Int {
    return len(values);
}

function HashKey[T hashable](value : T) : T {
    return value;
}

function KeepKeys[T hashable](values : List[T]) : List[T] {
    return values;
}

function WithAllocator[T allocator_like](allocator : T) : T {
    return allocator;
}

function NeedsPrintable[T Printable](value : T) : T {
    return value;
}

function Main() : Int {
    local Int total = Add(1, 2);
    local Int count = First([1, 2, 3]);
    local String key = HashKey("id");
    local List[String] keys = KeepKeys(["id", "name"]);
    local HeapAllocator allocator = WithAllocator(HeapAllocator());
    local Int printable = NeedsPrintable(1);
    return total + count + printable + len(key) + len(keys);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected named generic constraints to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsNamedGenericConstraintMismatch(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Show(value : Int) : String;
}

function Add[T numeric](left : T, right : T) : T {
    return left + right;
}

function NeedsPrintable[T Printable](value : T) : T {
    return value;
}

function Main() : Int {
    local Int badNumber = Add("left", "right");
    local String badTrait = NeedsPrintable("no impl");
    return 0;
}
`)

	report := CheckProgram(program)
	assertTypeError(t, report, "function Add argument 1 expects T:numeric, got String")
	assertTypeError(t, report, "function NeedsPrintable argument 1 expects T:Printable, got String")
}

func TestCheckProgramWarnsOnDeprecatedFunctionCall(t *testing.T) {
	program := programFromSource(`
@deprecated("use NewValue")
function OldValue() : Int {
    return 1;
}

function Main() : Int {
    return OldValue();
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected deprecated call to pass type check, got: %v", report.Errors)
	}
	assertTypeWarning(t, report, "function OldValue is deprecated: use NewValue")
}

func TestCheckProgramWarnsOnDeprecatedMethodCall(t *testing.T) {
	program := programFromSource(`
#extend String {
    @deprecated("use readable_name")
    function OLD_NAME() : String {
        return this;
    }
}

function Main() : Int {
    local String value = "legacy".OLD_NAME();
    return len(value);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected deprecated method call to pass type check, got: %v", report.Errors)
	}
	assertTypeWarning(t, report, "method String.OLD_NAME is deprecated: use readable_name")
}

func TestCheckProgramChecksNestedCallsThroughOperatorPrecedence(t *testing.T) {
	program := programFromSource(`
@deprecated("use NewFlag")
function OldValue() : Int {
    return 1;
}

function Main() : Int {
    return OldValue() + 2 * 3;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected precedence expression to pass type check, got: %v", report.Errors)
	}
	assertTypeWarning(t, report, "function OldValue is deprecated: use NewFlag")
}

func TestCheckProgramAcceptsMutableMapAndListIndexAssignments(t *testing.T) {
	if !isKnownType("Map[String,Int]") {
		t.Fatal("expected Map[String,Int] to be a known type")
	}
	typeName, name, ok := splitTypeAndName("Map[String, Int] rowResults")
	if !ok || typeName != "Map[String,Int]" || name != "rowResults" {
		t.Fatalf("expected split type and name to handle map types, got %q, %q, %v", typeName, name, ok)
	}
	decl, ok := parseVariableDeclaration(`local mut Map[String, Int] rowResults = {}`, "local")
	if !ok || decl.Type != "Map[String,Int]" || decl.Name != "rowResults" {
		t.Fatalf("expected map declaration to parse, got %#v, %v", decl, ok)
	}

	program := programFromSource(`
global mut Map[String, Int] memoryStore = {};

function Main() : Int {
    local mut List[Int] values = [];
    local mut Map[String, Int] rowResults = {};
    values[0] = 1;
    rowResults["sum"] = values[0];
    memoryStore["sum"] = rowResults["sum"];
    return memoryStore["sum"];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsStringListAndMapIndexing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String text = "hey";
    local Char first = text[0];
    local List[Int] values = [10, 20];
    local Int value = values[1];
    local mut Map[String, Int] totals = {};
    totals["two"] = 2;
    local Int mapValue = totals["two"];
    if first == 'h' {
        return value + mapValue;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected indexing type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsSetBuiltin(t *testing.T) {
	if !isKnownType("Set[String]") {
		t.Fatal("expected Set[String] to be a known type")
	}

	program := programFromSource(`
function Main() : Int {
    local Set[String] imports = Set(["lexer", "parser", "lexer"]);
    local Bool hasParser = set_has(imports, "parser");
    local Iterator[String] iterator = iter(imports);
    local Option[String] first = next(iterator);
    if hasParser and first.some {
        return imports.count + len(imports);
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected set builtin program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsFormatBuiltins(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String message = format("Hello %, %", ["kLang", 42]);
    local Int printed = printf("Ready %% %", [message]);
    return len(message) + printed;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected format builtin program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidFormatArguments(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String message = format(123, "bad");
    return len(message);
}
`)

	report := CheckProgram(program)
	assertTypeError(t, report, "format pattern expects String, got Int")
	assertTypeError(t, report, "format values expect List[T], got String")
}

func TestCheckProgramRejectsInvalidSetMembershipValue(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Set[String] imports = Set(["lexer"]);
    if set_has(imports, 1) {
        return 1;
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "set_has value expects String, got Int")
}

func TestCheckProgramAcceptsOptionAndResultBuiltins(t *testing.T) {
	if !isKnownType("Option[Int]") {
		t.Fatal("expected Option[Int] to be a known type")
	}
	if !isKnownType("Result[Int,String]") {
		t.Fatal("expected Result[Int,String] to be a known type")
	}

	program := programFromSource(`
function Main() : Int {
    local Option[Int] some = Some(10);
    local Option[Int] none = None();
    local Result[Int, String] ok = Ok(1);
    local Result[Int, String] err = Err("bad");
    local Result[Int, String] wrapped = Result(2);
    if some and not none and ok and not err {
        return 1;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected option/result type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsOptionAndResultHelpers(t *testing.T) {
	program := programFromSource(`
function Double(value : Int) : Int {
    return value * 2;
}

function KeepPositive(value : Int) : Option[Int] {
    if value > 0 {
        return Some(value);
    }
    return None();
}

function ToText(value : String) : String {
    return "error:" + value;
}

function ParseMore(value : Int) : Result[String, String] {
    return Ok("value");
}

function Main() : Int {
    local Option[Int] maybe = Some(10);
    local Option[Int] doubled = option_map(maybe, Double);
    local Int unwrapped = option_unwrap_or(doubled, 0);
    local Option[Int] chained = option_and_then(maybe, KeepPositive);

    local Result[Int, String] parsed = Ok(5);
    local Result[Int, String] mapped = result_map(parsed, Double);
    local Result[Int, String] mappedErr = result_map_err(mapped, ToText);
    local Int recovered = result_unwrap_or(mappedErr, 0);
    local Result[String, String] chainedResult = result_and_then(mappedErr, ParseMore);

    return unwrapped + recovered + option_unwrap_or(chained, 0) + len(result_unwrap_or(chainedResult, ""));
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected option/result helpers to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsOptionHelperMismatch(t *testing.T) {
	program := programFromSource(`
function NeedsString(value : String) : Int {
    return len(value);
}

function Main() : Int {
    local Option[Int] maybe = Some(10);
    local Option[Int] mapped = option_map(maybe, NeedsString);
    return option_unwrap_or(mapped, 0);
}
`)

	assertTypeError(t, CheckProgram(program), "option_map callback argument expects Int, got String")
}

func TestCheckProgramRejectsOptionAndResultTypeMismatch(t *testing.T) {
	optionProgram := programFromSource(`
function Main() : Int {
    local Option[Int] value = Some("bad");
    return 0;
}
`)
	assertTypeError(t, CheckProgram(optionProgram), "cannot assign Option[String] to local Option[Int] value")

	resultProgram := programFromSource(`
function Main() : Int {
    local Result[Int, String] value = Err(404);
    return 0;
}
`)
	assertTypeError(t, CheckProgram(resultProgram), "cannot assign Result[T,Int] to local Result[Int,String] value")
}

func TestCheckProgramAcceptsComplexAndSIMDBuiltins(t *testing.T) {
	if !isKnownType("Complex") {
		t.Fatal("expected Complex to be known")
	}
	if !isKnownType("SIMD[Int]") {
		t.Fatal("expected SIMD[Int] to be known")
	}

	program := programFromSource(`
function Main() : Int {
    local Complex z = Complex(1, 2) + Complex(3, 4);
    local SIMD[Int] lanes = SIMD([1, 2, 3, 4]);
    local SIMD[Int] moved = lanes + SIMD([4, 3, 2, 1]);
    print(z);
    print(moved);
    return len(moved);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected complex/simd type check to pass, got: %v", report.Errors)
	}

	bad := programFromSource(`
function Main() : Int {
    local SIMD[Int] lanes = SIMD(["bad"]);
    return 0;
}
`)
	assertTypeError(t, CheckProgram(bad), "cannot assign SIMD[String] to local SIMD[Int] lanes")
}

func TestCheckProgramAcceptsListComprehensions(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local List[Int] values = [1, 2, 3, 4];
    local List[Int] doubled = [value * 2 for value in values];
    local List[Int] evens = [value for value in values if value % 2 == 0];
    local List[Char] letters = [letter for letter in "hey"];
    local List[Int] indexes = [index for index in range(3)];
    return doubled[1] + evens[0] + indexes[2];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected list comprehension type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsListComprehensionTypeErrors(t *testing.T) {
	typeMismatch := programFromSource(`
function Main() : Int {
    local List[Int] values = [1, 2];
    local List[String] bad = [value for value in values];
    return 0;
}
`)
	assertTypeError(t, CheckProgram(typeMismatch), "cannot assign List[Int] to local List[String] bad")

	notIterable := programFromSource(`
function Main() : Int {
    local Bool flag = True;
    local List[Int] bad = [value for value in flag];
    return 0;
}
`)
	assertTypeError(t, CheckProgram(notIterable), "list comprehension cannot iterate over Bool")
}

func TestCheckProgramRejectsInvalidIndexing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String text = "hey";
    local Char first = text["bad"];
    return 0;
}
`)
	assertTypeError(t, CheckProgram(program), "String index must be Int, got String")

	program = programFromSource(`
function Main() : Int {
    local Int number = 1;
    local Int value = number[0];
    return value;
}
`)
	assertTypeError(t, CheckProgram(program), "Int is not indexable")

	program = programFromSource(`
function Main() : Int {
    local mut String text = "hey";
    text[0] = 'H';
    return 0;
}
`)
	assertTypeError(t, CheckProgram(program), "String indexes cannot be assigned")
}

func TestCheckProgramAcceptsBlockShadowing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int value = 1;
    if True {
        local mut Int value = 2;
        value += 1;
    }
    value += 1;
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramDoesNotLeakShadowedMoveFromInnerBlock(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String value = "outer";
    if True {
        local String value = "inner";
        local String moved = move value;
        print(moved);
    }
    return len(value);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected shadowed move not to affect outer binding, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsLocalLeakFromIfBlock(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    if True {
        local Int hidden = 1;
    }
    return hidden;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "hidden"`)
}

func TestCheckProgramAcceptsUserDefinedScopeShadowing(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    scope Calculation {
        local Int value = 2;
        print(value);
    }
    return value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected user-defined scope shadowing to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsUserDefinedScopeLeak(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    scope Calculation {
        local Int hidden = 1;
    }
    return hidden;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "hidden"`)
}

func TestCheckProgramRejectsLoopVariableLeak(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    for i := range(3) {
        print(i);
    }
    return i;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "i"`)
}

func TestCheckProgramAcceptsLoopHeaderScopeInsideLoop(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int total = 0;
    for i := range(3) {
        total += i;
    }
    while active := total < 10 {
        if active {
            total += 10;
        }
        break;
    }
    return total;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected loop header scope to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsCStyleLoopEvaluationInitializer(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int total = 0;
    for index := 0; index < 3; index += 1 {
        total += index;
    }
    return total;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected C-style loop initializer to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsForEachLoopAndRejectsLeak(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Int total = 0;
    for_each value in [1, 2, 3] {
        total += value;
    }
    return total;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected for_each loop to pass, got: %v", report.Errors)
	}

	program = programFromSource(`
function Main() : Int {
    for_each value in [1, 2, 3] {
        print(value);
    }
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), `unknown identifier "value"`)
}

func TestCheckProgramRejectsForEachOverNonIterable(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    for_each value in True {
        print(value);
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "for_each expects List, String, Table, Set, Iterator, or Int")
}

func TestCheckProgramRejectsDuplicateLocalInSameScope(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    local Int value = 2;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), `variable "value" is already defined in this scope`)
}

func TestCheckProgramAcceptsNestedGlobalAndExportedDeclarations(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected nested global/exported declarations to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsTypeCastsInExpressions(t *testing.T) {
	program := programFromSource(`
function Echo(value : String) : String {
    return value as String;
}

function Main() : Int {
    local Float f = 10 as Float;
    local Int i = f as Int;
    while active := i as Bool {
        return (Echo("42") as Int) + i;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected cast type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsNullSafetyOperator(t *testing.T) {
	program := programFromSource(`
function MaybeValue() : T {
}

function Main() : Int {
    local Bool exists = MaybeValue()?;
    while active := MaybeValue()? {
        return 1;
    }
    if exists == False {
        return 0;
    }
    return 1;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected null safety type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsBooleanOperatorsInExpressions(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Bool ready = True;
    local Bool active = False;
    local Bool failed = False;
    local Bool fallback = False;
    local Bool result = not ready and active xor failed or fallback;
    if result == False {
        return 1;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected boolean operator type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsPipeOperator(t *testing.T) {
	program := programFromSource(`
function Add(left : Int, right : Int) : Int {
    return left + right;
}

function Double(value : Int) : Int {
    return value * 2;
}

function Main() : Int {
    local Int result = 2 |> Add(3) |> Double;
    return result;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected pipe operator type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsFunctionCallbacks(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsTraitsAndImpls(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
    function Show(value : Int) : String {
        return value as String;
    }
}

function Main() : Int {
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsIncompleteTraitImpl(t *testing.T) {
	program := programFromSource(`
trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
}

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), `impl Printable for Int is missing method "Show"`)
}

func TestCheckProgramRejectsUseAfterMove(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String first = "hello";
    local String second = move first;
    print(first);
    return len(second);
}
`)

	assertTypeError(t, CheckProgram(program), `variable "first" was moved`)
}

func TestCheckProgramRejectsCallbackSignatureMismatch(t *testing.T) {
	program := programFromSource(`
function ToString(value : Int) : String {
    return value as String;
}

function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Main() : Int {
    return Apply(5, ToString);
}
`)

	assertTypeError(t, CheckProgram(program), "function Apply argument 2 expects Function[Int,Int], got Function[Int,String]")
}

func TestCheckProgramRejectsCallbackArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function Apply(value : String, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "callback callback argument 1 expects Int, got String")
}

func TestCheckProgramRejectsPipeArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function Double(value : Int) : Int {
    return value * 2;
}

function Main() : Int {
    return "bad" |> Double;
}
`)

	assertTypeError(t, CheckProgram(program), "function Double argument 1 expects Int, got String")
}

func TestCheckProgramRejectsInvalidTypeCast(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = [1] as Int;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot cast List[Int] to Int")
}

func TestCheckProgramRejectsCustomTypeCastTarget(t *testing.T) {
	program := programFromSource(`
alias function User(id : String) : type = struct {}

function Main() : Int {
    local T raw = User("42");
    local User user = raw as User;
    return len(user.id);
}
`)

	assertTypeError(t, CheckProgram(program), "cast target User is not a builtin type")
}

func TestCheckProgramAcceptsNestedFunctionAsFirstClassValue(t *testing.T) {
	program := programFromSource(`
function NumberFactory(multiplier : Int) : Function[Int, Int] {
    function InnerGenerator(val : Int) : Int {
        return val * multiplier;
    }
    return InnerGenerator;
}

global Function[Int, Int] timesTen = NumberFactory(10);
global Int quickMath = NumberFactory(5)(10);

function Main() : Int {
    return timesTen(1) + quickMath;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsInnerFunctionSelectors(t *testing.T) {
	program := programFromSource(`
function Counter(base : Int) {
    inner function Eval() : Int {
        return base + 1;
    }
}

function Main() : Int {
    return Counter(41).Eval();
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsFirstClassFunctionCallArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function NumberFactory() : Function[Int, Int] {
    function Identity(value : Int) : Int {
        return value;
    }
    return Identity;
}

function Main() : Int {
    return NumberFactory()("bad");
}
`)

	assertTypeError(t, CheckProgram(program), "callback NumberFactory() argument 1 expects Int, got String")
}

func TestCheckProgramAcceptsLambdaFunctionValues(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsRestrictedLambdaFunctionValues(t *testing.T) {
	program := programFromSource(`
function Run(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}

function Main() : Int {
    local Function[Int, Int] update = fun[T restrict[Int]](value : T) : T {
        return 1;
    };
    return Run(1, update);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected restricted lambda program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsImmutableParameterMutation(t *testing.T) {
	program := programFromSource(`
function Mutate(value : Int) : Int {
    value += 1;
    return value;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot mutate immutable variable")
}

func TestCheckProgramAcceptsMutableParameterMutation(t *testing.T) {
	program := programFromSource(`
function Mutate(mut value : Int) : Int {
    value += 1;
    return value;
}

function Main() : Int {
    return Mutate(1);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected mutable parameter program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsReferenceParameterMutation(t *testing.T) {
	program := programFromSource(`
function Increment(ref value : Int) {
    value += 1;
}

function Main() : Int {
    local mut Int count = 1;
    Increment(count);
    return count;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected reference parameter program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsImmutableReferenceArgument(t *testing.T) {
	program := programFromSource(`
function Increment(ref value : Int) {
    value += 1;
}

function Main() : Int {
    local Int count = 1;
    Increment(count);
    return count;
}
`)

	assertTypeError(t, CheckProgram(program), `requires mutable variable "count"`)
}

func TestCheckProgramRejectsTemporaryReferenceArgument(t *testing.T) {
	program := programFromSource(`
function Increment(ref value : Int) {
    value += 1;
}

function Main() : Int {
    Increment(1);
    return 1;
}
`)

	assertTypeError(t, CheckProgram(program), "reference argument 1 expects a variable")
}

func TestCheckProgramRejectsReferenceParameterDefault(t *testing.T) {
	program := programFromSource(`
function Increment(ref value : Int = 1) {
    value += 1;
}
`)

	assertTypeError(t, CheckProgram(program), "reference parameter value cannot have a default value")
}

func TestCheckProgramExposesArgsGlobal(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    return len(Args);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected Args program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsFunctionGroups(t *testing.T) {
	program := programFromSource(`
function function1_name(x : Int) : Int {
    return x;
}

function function2_name(x : Int, y : String) : String {
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsTryCatchThrowAndResultPropagation(t *testing.T) {
	program := programFromSource(`
function Fallible() : Result[Int, Atom] {
    return Ok(41);
}

function Main() : Int {
    try {
        local Int value = Fallible()!;
        return value + 1;
    } catch err {
        print(err.name);
        return 0;
    }
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsNonAtomErrorPropagation(t *testing.T) {
	program := programFromSource(`
function Fallible() : Result[Int, String] {
    return Err("bad");
}

function Main() : Int {
    return Fallible()!;
}
`)

	assertTypeError(t, CheckProgram(program), "! only propagates Result[T, Atom]")
}

func TestCheckProgramRejectsNonAtomThrow(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    throw "bad";
}
`)

	assertTypeError(t, CheckProgram(program), "throw expects Atom, got String")
}

func TestCheckProgramAcceptsNamespaceLocalFunctionCalls(t *testing.T) {
	program := programFromSource(`
namespace random {
    function Random() : Int {
        return 1;
    }

    function RandomRange(min : Int, max : Int) : Int {
        local Int value = Random();
        return value + min + max;
    }
}

function Main() : Int {
    return call random.RandomRange(1, 2);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsChainedNamespacesAndAliases(t *testing.T) {
	program := programFromSource(`
namespace std {
    namespace lib {
        function LuaInit() : Int {
            return 7;
        }
    }
}

alias std_lib = std.lib;
alias std_alias = std;
alias nested_alias = std_alias.lib;

function Main() : Int {
    local Int direct = std.lib.LuaInit();
    local Int viaAlias = std_lib::LuaInit();
    local Int viaNestedAlias = nested_alias::LuaInit();
    return direct + viaAlias + viaNestedAlias;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsUnknownNamespaceAliasTarget(t *testing.T) {
	program := programFromSource(`
alias missing_alias = missing.lib;

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), `alias "missing_alias" targets unknown namespace "missing.lib"`)
}

func TestCheckProgramRejectsNamespaceAliasCycle(t *testing.T) {
	program := programFromSource(`
alias loop = loop;

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), `alias "loop" creates a namespace alias cycle`)
}

func TestCheckProgramAcceptsAliasFunctionExtensionMethodsAndRegions(t *testing.T) {
	program := programFromSource(`
region MyRegion(T, sizeof(T) * 100, 10);

alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) : type {
    [new] {
        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));
    }

    #extend {
        function get_length() -> int {
            return this.length;
        }
    }
}

function Main() : Int {
    local T list = ArrayList("value", 3, 10);
    local mut T[MyRegion] myArray;
    myArray[0] = "String";
    return list.get_length() + len(myArray);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected alias function and region program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsTemporaryMemoryRegion(t *testing.T) {
	program := programFromSource(`
temp region Scratch(T, sizeof(T) * 16, 4);

function Main() : Int {
    local mut T[Scratch] values;
    values[0] = "value";
    return len(values);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected temporary region program to type check, got %#v", report.Errors)
	}
	assertState(t, report.States, "temporary_region", "Scratch", "T")
}

func TestCheckProgramAcceptsAliasFunctionStructBody(t *testing.T) {
	program := programFromSource(`
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
    return list.get_length() + list.with_extra(4);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected struct alias function program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsStructCastAsConversions(t *testing.T) {
	program := programFromSource(`
alias function User(id : Int, name : String) : type = struct {}
alias function PublicUser(name : String, active : Bool = True) : type = struct {}

function Main() : Int {
    local User user = User(1, "Ada");
    local Table row = user.cast_as(Table);
    local JSON document = user.cast_as(JSON);
    local String encoded = user.cast_as(String);
    local PublicUser view = user.cast_as(PublicUser);
    print(row, document, encoded, view);
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected struct cast_as conversions to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsStructCastWithMissingRequiredField(t *testing.T) {
	program := programFromSource(`
alias function User(name : String) : type = struct {}
alias function Account(name : String, id : Int) : type = struct {}

function Main() : Int {
    local User user = User("Ada");
    local Account account = user.cast_as(Account);
    return 0;
}
`)

	report := CheckProgram(program)
	if report.Passed() || !strings.Contains(fmt.Sprint(report.Errors), `required field "id" is missing`) {
		t.Fatalf("expected missing cast field diagnostic, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsAliasStructFieldsMethodsAndGenerics(t *testing.T) {
	program := programFromSource(`
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

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected alias struct fields, methods, and generics to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsAliasStructGenericMethodArgumentMismatch(t *testing.T) {
	program := programFromSource(`
alias function Boxed[T: Any](items : List[T], capacity : Int) : type = struct {
    #extend {
        function push(value : T) : Boxed {
            local mut List[T] values = clone (this.items as List[T]);
            values[len(values)] = value;
            return Boxed(values, this.capacity);
        }
    }
}

function Main() : Int {
    let mut x = Boxed([1, 2], 2);
    x = x.push("bad");
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), `callback x.push argument 1 expects Int, got String`)
}

func TestCheckProgramAcceptsInferredParameterDefaultsAndAtomicBuiltins(t *testing.T) {
	program := programFromSource(`
function UserDefinedWorkspace() : String {
    return "workspace";
}

function create_workspace(name : String, workspace := UserDefinedWorkspace()) : String {
    return workspace;
}

function Main() : Int {
    local Atomic[Int] counter = Atomic(1);
    atomic_add(counter, 2);
    local String workspace = create_workspace("demo");
    return atomic_load(counter) + len(workspace);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected inferred default and atomic program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsWorkspaceBuildDebuggerAndJSFFIAPIs(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Program program = Program(["app", "mathg"]);
    local BuildSystem build = BuildSystem("demo", 2, ["first.klang", "app.klang"], "Standalone");
    local WorkSpace workspace = WorkSpace(program, build);
    local String backend = workspace_backend(workspace);
    local List[String] files = workspace_files(workspace);
    local String manifest = workspace_manifest(workspace);
    local String valueType = debug_type(files);
    local List[String] stack = debug_stack();
    breakpoint("before js");
    local JSModule module = js_import("library.js");
    local List[String] exports = js_exports(module);
    local String source = js_source(module);
    local JSCall descriptor = js_call(module, "init", [manifest, valueType, source]);
    return len(backend) + len(files) + len(exports) + len(stack);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected workspace/debug/js ffi program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidBuildBackendLiteral(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local BuildSystem build = BuildSystem("demo", 1, ["first.klang"], "Native");
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "BuildSystem backend must be WASM, JS, or Standalone")
}

func TestCheckProgramAcceptsThreadSpawnJoinAndStatus(t *testing.T) {
	program := programFromSource(`
function Worker(counter : Atomic[Int], mut amount : Int) : Int {
    while amount > 0 {
        atomic_add(counter, 1);
        amount -= 1;
    }
    return atomic_load(counter);
}

function Main() : Int {
    local Atomic[Int] counter = Atomic(0);
    local Thread[Int] left = spawn(Worker, [counter, 10]);
    local Thread[Int] right = spawn(Worker, [counter, 20]);
    local String status = thread_status(left);
    local Int a = join(left);
    local Int b = join(right);
    return atomic_load(counter) + len(status);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected threaded program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsSpawnArgumentMismatch(t *testing.T) {
	program := programFromSource(`
function Worker(value : Int) : Int {
    return value;
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker, ["bad"]);
    return join(worker);
}
`)

	assertTypeError(t, CheckProgram(program), "spawn argument 1 expects Int, got String")
}

func TestCheckProgramRejectsNonTransferSafeSpawnTypes(t *testing.T) {
	program := programFromSource(`
function Worker(value : RefMut) : Int {
    return 0;
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker, [RefMut(1)]);
    return join(worker);
}
`)

	assertTypeError(t, CheckProgram(program), "spawn parameter 1 type RefMut is not thread-transfer-safe")
}

func TestCheckProgramRejectsAsyncSpawnTarget(t *testing.T) {
	program := programFromSource(`
async function Worker() : Int {
    return 1;
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker);
    return join(worker);
}
`)

	assertTypeError(t, CheckProgram(program), "spawn target cannot be async")
}

func TestCheckProgramRejectsMutableGlobalWorkerAccess(t *testing.T) {
	program := programFromSource(`
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

	assertTypeError(t, CheckProgram(program), "spawn worker Worker accesses mutable global shared")
}

func TestCheckProgramAcceptsImmutableGlobalAtomicWorkerAccess(t *testing.T) {
	program := programFromSource(`
global Atomic[Int] shared = Atomic(0);

function Worker() : Int {
    return atomic_add(shared, 1);
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker);
    return join(worker);
}
`)

	if report := CheckProgram(program); !report.Passed() {
		t.Fatalf("expected immutable global Atomic worker access to pass, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsNonTransferableGlobalWorkerAccess(t *testing.T) {
	program := programFromSource(`
global Table shared = {"answer": 42};

function Worker() : Int {
    return shared["answer"] as Int;
}

function Main() : Int {
    local Thread[Int] worker = spawn(Worker);
    return join(worker);
}
`)

	assertTypeError(t, CheckProgram(program), "spawn worker Worker accesses non-transferable global shared of type Table")
}

func TestCheckProgramRejectsUnsafeAtomicPayload(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Atomic[RefMut] cell = Atomic(RefMut(1));
    return 0;
}
`)

	report := CheckProgram(program)
	assertTypeError(t, report, "Atomic payload type RefMut is not thread-transfer-safe")
	assertTypeError(t, report, "Atomic value type RefMut is not thread-transfer-safe")
}

func TestCheckProgramAcceptsTransferSafeAggregateWorker(t *testing.T) {
	program := programFromSource(`
function Worker(mut values : List[Int]) : List[Int] {
    values[0] = 99;
    return values;
}

function Main() : Int {
    local Thread[List[Int]] worker = spawn(Worker, [[1, 2, 3]]);
    local List[Int] result = join(worker);
    return result[0];
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected transfer-safe aggregate worker to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramEnforcesTransferableGenericConstraint(t *testing.T) {
	accepted := programFromSource(`
function Identity[T transferable](value : T) : T {
    return value;
}

function Main() : Int {
    local List[Int] values = Identity([1, 2, 3]);
    return values[0];
}
`)
	if report := CheckProgram(accepted); !report.Passed() {
		t.Fatalf("expected transferable List[Int] to pass, got %#v", report.Errors)
	}

	rejected := programFromSource(`
function Identity[T transferable](value : T) : T {
    return value;
}

function Main() : Int {
    local Table value = Identity({"answer": 42});
    return value.answer;
}
`)
	assertTypeError(t, CheckProgram(rejected), "function Identity argument 1 expects T:transferable, got Map[T,T]")
}

func TestCheckProgramAcceptsTraitsInsideAliasFunctions(t *testing.T) {
	program := programFromSource(`
alias function Wrapped(value: int) -> type
    trait AliasPrintable {
        function Show(value : Int) : String;
    }

    impl AliasPrintable for Int {
        function Show(value : Int) : String {
            return value as String;
        }
    }
end

function Main() : Int {
    local T wrapped = Wrapped(1);
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected alias-contained trait program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsUnknownRegionArrayType(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut T[MissingRegion] myArray;
    myArray[0] = "String";
    return len(myArray);
}
`)

	assertTypeError(t, CheckProgram(program), `array type T[MissingRegion] uses unknown region "MissingRegion"`)
}

func TestCheckProgramChecksAliasExtensionMethodArguments(t *testing.T) {
	program := programFromSource(`
alias function Counter(value: int) : type {
    #extend {
        function add(amount : Int) -> int {
            return this.value + amount;
        }
    }
}

function Main() : Int {
    local T counter = Counter(2);
    return counter.add(3);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected extension method argument program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsAliasExtensionMethodArgumentMismatch(t *testing.T) {
	program := programFromSource(`
alias function Counter(value: int) : type {
    #extend {
        function add(amount : Int) -> int {
            return this.value + amount;
        }
    }
}

function Main() : Int {
    local T counter = Counter(2);
    return counter.add("bad");
}
`)

	assertTypeError(t, CheckProgram(program), "callback counter.add argument 1 expects Int, got String")
}

func TestCheckProgramAcceptsAliasOperatorOverloads(t *testing.T) {
	program := programFromSource(`
alias function Vector(x : Int, y : Int) : type = struct {
    operator +(other : Vector) : Vector {
        return Vector(this.x + other.x, this.y + other.y);
    }

    operator ==(other : Vector) : Bool {
        return this.x == other.x and this.y == other.y;
    }

    operator **(power : Int) : Vector {
        return Vector(this.x ** power, this.y ** power);
    }
}

function Main() : Int {
    local mut Vector total = Vector(1, 2) + Vector(3, 4);
    total += Vector(1, 1);
    local Vector powered = Vector(2, 3) ** 2;
    if total == Vector(5, 7) {
        return total.x + total.y + powered.x + powered.y;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected alias operator overloads to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsAliasOperatorOperandMismatch(t *testing.T) {
	program := programFromSource(`
alias function Vector(x : Int) : type = struct {
    operator +(other : Vector) : Vector {
        return Vector(this.x + other.x);
    }
}

function Main() : Int {
    local Vector invalid = Vector(1) + 2;
    return invalid.x;
}
`)

	assertTypeError(t, CheckProgram(program), "operator + on Vector expects Vector, got Int")
}

func TestCheckProgramRejectsInvalidComparisonOperatorReturn(t *testing.T) {
	program := programFromSource(`
alias function Vector(x : Int) : type = struct {
    operator ==(other : Vector) : Int {
        return this.x - other.x;
    }
}

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "comparison operator == must return Bool")
}

func TestCheckProgramAcceptsStandaloneExtensionMethods(t *testing.T) {
	program := programFromSource(`
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
    return 10.days().ago() + Duration(3).doubled() + len("x".surrounded("["));
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected standalone extension methods to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsStandaloneExtensionArgumentMismatch(t *testing.T) {
	program := programFromSource(`
#extend String {
    function surrounded(left : String, right : String) : String {
        return left + this + right;
    }
}

function Main() : Int {
    return len("core".surrounded(1, "]"));
}
`)

	assertTypeError(t, CheckProgram(program), "method surrounded argument 1 expects String, got Int")
}

func TestCheckProgramRejectsExtensionMethodCollisions(t *testing.T) {
	program := programFromSource(`
#extend String {
    function uppercase() : String {
        return this;
    }
}

function Main() : Int {
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "conflicts with a builtin method")
}

func TestCheckProgramAcceptsNestedBooleanLiteralExtensionCall(t *testing.T) {
	program := programFromSource(`
#extend Bool {
    function to_int() : Int {
        if this {
            return 1;
        }
        return 0;
    }
}

function Identity(value : Int) : Int {
    return value;
}

function Main() : Int {
    return Identity(True.to_int());
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected nested Boolean literal extension call to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramSpecializesGenericListExtensionReceiver(t *testing.T) {
	program := programFromSource(`
#extend List[T] {
    function first_or(fallback : T) : T {
        if len(this) == 0 {
            return fallback;
        }
        return this[0];
    }
}

function Main() : Int {
    local List[Int] values = [4, 5];
    local Int first = values.first_or(0);
    return first;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected generic List extension to specialize to Int, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsGenericListExtensionArgumentMismatch(t *testing.T) {
	program := programFromSource(`
#extend List[T] {
    function first_or(fallback : T) : T {
        return fallback;
    }
}

function Main() : Int {
    local List[Int] values = [4, 5];
    return values.first_or("bad");
}
`)

	assertTypeError(t, CheckProgram(program), "expects Int, got String")
}

func TestCheckProgramAcceptsAllocatorConstructors(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local T boxed = Box("value");
    local T ref = Ref(1);
    local T refMut = RefMut(2);
    local T cell = RefCell(3);
    local T heap = HeapAllocator();
    local T regionAlloc = RegionAllocator("MainRegion");
    local T bump = BumpAllocator();
    local T arena = ArenaAllocator();
    return len(boxed.kind) + len(ref.kind) + len(refMut.kind) + len(cell.kind) + len(heap.kind) + len(regionAlloc.kind) + len(bump.kind) + len(arena.kind);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected allocator program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsTableAsyncIteratorAndCoroutineBuiltins(t *testing.T) {
	program := programFromSource(`
async function LoadValue() : Int {
    return 40;
}

function BuildValue() : Int {
    return 2;
}

function Main() : Int {
    local mut Table data = {"name": "klang", 1: 5};
    data["extra"] = 7;
    local Awaitable[Int] pending = LoadValue();
    local Iterator[Int] iterator = iter([1, 2, 3]);
    local Option[Int] first = next(iterator);
    local Coroutine[Int] co = coroutine(BuildValue);
    local Option[Int] resumed = resume(co);
    if first.some and resumed.some {
        return (await pending) + first.value + resumed.value + len(data.name);
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected table/async/iterator/coroutine program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsLazyIteratorPipelines(t *testing.T) {
	program := programFromSource(`
function IsEven(value : Int) : Bool { return value % 2 == 0; }
function Double(value : Int) : Int { return value * 2; }
function Add(total : Int, value : Int) : Int { return total + value; }

function Main() : Int {
    local List[Int] values = [1, 2, 3, 4];
    local Iterator[Int] pipeline = values.filter(IsEven).map(Double).skip(1).limit(1);
    local List[Int] result = pipeline.collect();
    local List[Int] sorted = values.filter(IsEven).map(Double).sort();
    local Int total = values.fold(0, Add);
    local Option[Int] first = values.filter(IsEven).first();
    local Map[String, Int] dictionary = {"one": 1, "two": 2};
    local Iterator[Table] entries = dictionary.iter();
    print(result, sorted, total, first, entries);
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected lazy iterator pipeline to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidPipelinePredicate(t *testing.T) {
	program := programFromSource(`
function NotAPredicate(value : Int) : Int { return value; }

function Main() : Int {
    local List[Int] values = [1, 2, 3];
    local Iterator[Int] invalid = values.filter(NotAPredicate);
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "filter callback must return Bool, got Int")
}

func TestCheckProgramAcceptsCoreTableHelpers(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Table data = {"name": "klang", 1: 10, True: 20, 'x': 30};
    data[1] = 11;
    local Bool hasName = table_has(data, "name");
    local Table deleted = table_delete(data, "name");
    local List[T] keys = table_keys(deleted);
    local List[T] values = table_values(deleted);
    local List[Table] entries = table_entries(deleted);
    local Iterator[Table] iterator = iter(deleted);
    local Option[Table] first = next(iterator);
    local Int sequential = table_sequence_count(deleted);
    local Table parent = {"fallback": 1};
    local Table child = table_set_fallback(deleted, parent);
    return child.count + sequential + len(keys) + len(values) + len(entries);
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected core table helper program to type check, got %#v", report.Errors)
	}

	badKey := programFromSource(`
function Main() : Int {
    local mut Table data = {};
    data[[1]] = 1;
    return 0;
}
`)
	assertTypeError(t, CheckProgram(badKey), "Table index expects String, Int, UInt, Float, Bool, or Char key")
}

func TestCheckProgramRejectsAwaitOnNonAwaitable(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    return await 1;
}
`)

	assertTypeError(t, CheckProgram(program), "await expects Awaitable, got Int")
}

func TestCheckProgramRejectsNextOnNonIterator(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Option[Int] value = next(1);
    return value.value;
}
`)

	assertTypeError(t, CheckProgram(program), "next expects Iterator, got Int")
}

func TestCheckProgramRejectsUnsafeOptionValueAccess(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Option[Int] value = None();
    return value.value;
}
`)

	assertTypeError(t, CheckProgram(program), "Option value value must be checked with .some, pattern matched with Some(...), or unwrapped with option_unwrap_or before accessing .value")
}

func TestCheckProgramAcceptsGuardedOptionValueAccess(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Option[Int] value = None();
    if value.some {
        return value.value;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected guarded Option value access to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsUnsafeResultValueAccess(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Result[Int, String] value = Err("bad");
    return value.value;
}
`)

	assertTypeError(t, CheckProgram(program), "Result value value must be checked with .ok or pattern matched with Ok(...) before accessing .value")
}

func TestCheckProgramAcceptsGuardedResultValueAccess(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Result[Int, String] value = Ok(10);
    if value.ok {
        return value.value;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected guarded Result value access to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramTracksKnownSomeAssignments(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local mut Option[Int] value = None();
    value = Some(10);
    return value.value;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected known Some assignment to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramAcceptsPatternMatchStatement(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local String mode = "blank";
    if mode == {
        case "blank":
            print("blank");
        case:
            print("default");
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected pattern match type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsExhaustiveBoolPatternMatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Bool ready = True;
    if ready == {
        case True:
            print("yes");
        case False:
            print("no");
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected exhaustive bool pattern match to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsPartialPatternMatchWithoutDefault(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    partial if value == {
        case 1:
            print(value);
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected partial pattern match to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsNonExhaustivePatternMatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    if value == {
        case 1:
            print(value);
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "pattern match is not exhaustive")
}

func TestCheckProgramRejectsPatternMatchTypeMismatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Int value = 1;
    if value == {
        case "one":
            print(value);
        case:
            print(0);
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "case pattern type String does not match Int")
}

func TestCheckProgramRejectsUnsupportedPatternMatchValue(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Set[String] data = Set(["blank"]);
    partial if data == {
        case ["blank"]:
            print("no");
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "pattern match value must be Bool, String, Int, Float, Atom, Enum, Option, Result, List, or Table, got Set[String]")
}

func TestCheckProgramAcceptsOptionResultListAndTablePatterns(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Option[Int] maybe = Some(10);
    if maybe == {
        case Some(value):
            print(value);
        case None():
            print(0);
    }

    local Result[Int, String] parsed = Err("bad");
    if parsed == {
        case Ok(value):
            print(value);
        case Err(message):
            print(message);
    }

    local List[Int] values = [1, 2];
    partial if values == {
        case [1, 2]:
            print("pair");
    }

    local Table data = {"kind": "count", "value": 4};
    partial if data == {
        case {"kind": "count", "value": amount}:
            print(amount);
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected extended pattern match type check to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramAcceptsResultPatternForInferredLocal(t *testing.T) {
	program := programFromSource(`
function Parse() : Result[Int, String] {
    return Ok(10);
}

function Main() : Int {
    local parsed = Parse();
    if parsed == {
        case Ok(value):
            return value;
        case Err(message):
            print(message);
            return 0;
    }
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected inferred Result pattern match to pass, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsNonExhaustiveResultPatternMatch(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    local Result[Int, String] parsed = Ok(1);
    if parsed == {
        case Ok(value):
            print(value);
    }
    return 0;
}
`)

	assertTypeError(t, CheckProgram(program), "pattern match is not exhaustive")
}

func TestCheckProgramAcceptsBuiltinProtocolMembers(t *testing.T) {
	program := programFromSource(`
function Remember(index : Int) : Int {
    return index;
}

function Main() : Int {
    local Int textCount = "hallo".count;
    local Int listCount = [1, 2, 3].count;
    local String upper = "hallo".uppercase();
    local String lower = upper.lowercase();
    local Int last = 3.times(Remember);
    return textCount + listCount + len(upper) + len(lower) + last;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected builtin protocol members to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidBuiltinProtocolMethodArgument(t *testing.T) {
	program := programFromSource(`
function Main() : Int {
    return 3.times("bad");
}
`)

	assertTypeError(t, CheckProgram(program), "callback 3.times argument 1 expects Function[Int,T], got String")
}

func TestCheckProgramAcceptsEnumIotaStyleDeclarations(t *testing.T) {
	program := programFromSource(`
enum Color {
    Red;
    Blue = 4;
    Green;
}

function Main() : Int {
    local Color color = Color.Green;
    if color == {
        case Color.Red:
            return 1;
        case Color.Blue:
            return 2;
        case Color.Green:
            return color.ordinal;
    }
    return 0;
}
`)

	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected enum program to type check, got %#v", report.Errors)
	}
}

func TestCheckProgramRejectsEnumAssignmentMismatch(t *testing.T) {
	program := programFromSource(`
enum Color { Red; }
enum Status { Red; }

function Main() : Int {
    local Color color = Status.Red;
    return color.ordinal;
}
`)

	assertTypeError(t, CheckProgram(program), "cannot assign Status to local Color color")
}

func programFromSource(source string) file.Program {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	return file.Program{
		Name:       "test",
		Root:       "tests",
		EntryPoint: "tests/test.klang",
		Files: []file.SourceFile{
			{
				Path:  "tests/test.klang",
				Lines: lines,
			},
		},
	}
}

func assertTypeError(t *testing.T, report Report, expected string) {
	t.Helper()

	for _, err := range report.Errors {
		if strings.Contains(err.Message, expected) {
			return
		}
	}

	t.Fatalf("expected type error containing %q, got %#v", expected, report.Errors)
}

func assertTypeWarning(t *testing.T, report Report, expected string) {
	t.Helper()

	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, expected) {
			return
		}
	}

	t.Fatalf("expected type warning containing %q, got %#v", expected, report.Warnings)
}

func assertState(t *testing.T, states []State, kind string, name string, typeName string) {
	t.Helper()

	for _, state := range states {
		if state.Kind == kind && state.Name == name && state.Type == typeName {
			return
		}
	}

	t.Fatalf("expected state kind=%q name=%q type=%q, got %#v", kind, name, typeName, states)
}

func TestCheckProgramAcceptsAtomsAsErrorCodesAndHashKeys(t *testing.T) {
	program := programFromSource(`
function Fail() {
    throw :not_found;
}

function Main() : Int {
    const missing = :not_found;
    local Atom dynamic = Atom("permission_denied");
    local Atom cast = "timeout" as Atom;
    local String name = dynamic.name;
    local Table codes = {:not_found: 404, :permission_denied: 403};
    local Set[Atom] retryable = Set([cast, :busy]);
    assert codes[missing] == 404;
    assert set_has(retryable, :busy);
    try {
        Fail();
    } catch error {
        assert error == missing;
    }
    return len(name);
}
`)
	report := CheckProgram(program)
	if !report.Passed() {
		t.Fatalf("expected Atom program to type check, got: %v", report.Errors)
	}
}

func TestCheckProgramRejectsInvalidAtomConstructorType(t *testing.T) {
	report := CheckProgram(programFromSource(`
function Main() : Int {
    local Atom code = Atom(404);
    return 0;
}
`))
	assertTypeError(t, report, "Atom name expects String, got Int")
}
