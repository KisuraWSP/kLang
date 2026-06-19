package parser

import (
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestParseFunctionWithTypedParametersAndReturn(t *testing.T) {
	program, errors := Parse(`
function Add(left : Int, right : Int) : Int {
    local Int total = left + right;
    return total;
}
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(program.Statements))
	}

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if fn.Name != "Add" || fn.ReturnType != "Int" {
		t.Fatalf("unexpected function signature: %#v", fn)
	}
	if len(fn.Params) != 2 || fn.Params[0].Name != "left" || fn.Params[0].Type != "Int" ||
		fn.Params[1].Name != "right" || fn.Params[1].Type != "Int" {
		t.Fatalf("unexpected function params: %#v", fn.Params)
	}
	if len(fn.Body) != 2 {
		t.Fatalf("expected 2 body statements, got %d", len(fn.Body))
	}
	if _, ok := fn.Body[0].(VariableStatement); !ok {
		t.Fatalf("expected first body statement to be variable declaration, got %T", fn.Body[0])
	}
	if _, ok := fn.Body[1].(ReturnStatement); !ok {
		t.Fatalf("expected second body statement to be return, got %T", fn.Body[1])
	}
}

func TestParseFunctionWithReferenceParameter(t *testing.T) {
	program, errors := Parse(`
function Increment(ref value : Int) {
    value += 1;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if len(fn.Params) != 1 || !fn.Params[0].ByRef || !fn.Params[0].Mutable || fn.Params[0].Name != "value" {
		t.Fatalf("unexpected reference parameter: %#v", fn.Params)
	}
}

func TestParseDefaultParametersRestrictedGenericsAndConditionalExpression(t *testing.T) {
	program, errors := Parse(`
function Pick[T restrict[UInt, Int, Float]](value : T = 1) : T {
    local Bool ok = if value > 0 then return False : True;
    return value;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if len(fn.TypeParams) != 1 || fn.TypeParams[0].Type != "T:UInt|Int|Float" {
		t.Fatalf("unexpected type params: %#v", fn.TypeParams)
	}
	if len(fn.Params) != 1 || fn.Params[0].Type != "T:UInt|Int|Float" || fn.Params[0].Default.Node == nil {
		t.Fatalf("unexpected parameter: %#v", fn.Params)
	}
	decl, ok := fn.Body[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable declaration, got %T", fn.Body[0])
	}
	if _, ok := decl.Expression.Node.(ConditionalExpression); !ok {
		t.Fatalf("expected conditional expression, got %T", decl.Expression.Node)
	}
}

func TestParseInferredVariableDeclarations(t *testing.T) {
	program, errors := Parse(`
let x = Some(1);
let mut y = Some(2);
val z = Some(3);
var w = Some(4);
const sizeValue = Int.sizeof;
let size intSize = Int.sizeof;
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 6 {
		t.Fatalf("expected 6 statements, got %d", len(program.Statements))
	}

	cases := []struct {
		index    int
		scope    string
		name     string
		typeName string
		mutable  bool
	}{
		{0, "local", "x", "T", false},
		{1, "local", "y", "T", true},
		{2, "global", "z", "T", false},
		{3, "global", "w", "T", true},
		{4, "const", "sizeValue", "T", false},
		{5, "local", "intSize", "Int", false},
	}

	for _, expected := range cases {
		decl, ok := program.Statements[expected.index].(VariableStatement)
		if !ok {
			t.Fatalf("expected variable declaration at %d, got %T", expected.index, program.Statements[expected.index])
		}
		if !decl.Inferred || decl.Scope != expected.scope || decl.Name != expected.name || decl.Type != expected.typeName || decl.Mutable != expected.mutable {
			t.Fatalf("unexpected inferred declaration at %d: %#v", expected.index, decl)
		}
	}
}

func TestParseUnicodeIdentifiersAndPrefixedNumberLiterals(t *testing.T) {
	program, errors := Parse(`
function එකතු(අගය : Int, 😀 : Int) : Int {
    local Int hex = 0x2A;
    local Int negative = -42;
    return අගය + 😀 + hex + negative;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if fn.Name != "එකතු" || len(fn.Params) != 2 || fn.Params[0].Name != "අගය" || fn.Params[1].Name != "😀" {
		t.Fatalf("unexpected unicode function signature: %#v", fn)
	}

	hexDecl, ok := fn.Body[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected hex variable declaration, got %T", fn.Body[0])
	}
	if literal, ok := hexDecl.Expression.Node.(LiteralExpression); !ok || literal.Kind != "Int" || literal.Value != "0x2A" {
		t.Fatalf("expected hex int literal, got %#v", hexDecl.Expression.Node)
	}

	negativeDecl, ok := fn.Body[1].(VariableStatement)
	if !ok {
		t.Fatalf("expected negative variable declaration, got %T", fn.Body[1])
	}
	if literal, ok := negativeDecl.Expression.Node.(LiteralExpression); !ok || literal.Kind != "Int" || literal.Value != "-42" {
		t.Fatalf("expected negative int literal, got %#v", negativeDecl.Expression.Node)
	}
}

func TestParseLocalInferredVariableDeclaration(t *testing.T) {
	program, errors := Parse(`
local count = 1;
local mut values = [1, 2, 3];
export local exported = "shared";
`)
	assertNoParseErrors(t, errors)

	cases := []struct {
		index    int
		name     string
		mutable  bool
		exported bool
	}{
		{0, "count", false, false},
		{1, "values", true, false},
		{2, "exported", false, true},
	}

	for _, expected := range cases {
		decl, ok := program.Statements[expected.index].(VariableStatement)
		if !ok {
			t.Fatalf("expected variable declaration at %d, got %T", expected.index, program.Statements[expected.index])
		}
		if !decl.Inferred || decl.Scope != "local" || decl.Type != "T" || decl.Name != expected.name ||
			decl.Mutable != expected.mutable || decl.Exported != expected.exported || decl.Expression.Node == nil {
			t.Fatalf("unexpected inferred local declaration at %d: %#v", expected.index, decl)
		}
	}
}

func TestParseDestructuringDeclarationsLowerToVariables(t *testing.T) {
	program, errors := Parse(`
local [first, second] = [1, 2];
let {name: label, size} = data;
local mut [head, [left, right]] = pairs;
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 11 {
		t.Fatalf("expected 11 lowered statements, got %d", len(program.Statements))
	}

	temp, ok := program.Statements[0].(VariableStatement)
	if !ok || temp.Name != "__klang_destructure_0" || !temp.Inferred || temp.Scope != "local" {
		t.Fatalf("expected first lowered temp declaration, got %#v", program.Statements[0])
	}

	first, ok := program.Statements[1].(VariableStatement)
	if !ok || first.Name != "first" || !first.Inferred || first.Expression.Literal() != "__klang_destructure_0 [ 0 ]" {
		t.Fatalf("expected lowered first binding, got %#v", program.Statements[1])
	}

	label, ok := program.Statements[4].(VariableStatement)
	if !ok || label.Name != "label" || label.Expression.Literal() != "__klang_destructure_1 . name" {
		t.Fatalf("expected lowered object alias binding, got %#v", program.Statements[4])
	}

	nestedTemp, ok := program.Statements[8].(VariableStatement)
	if !ok || nestedTemp.Name != "__klang_destructure_3" || nestedTemp.Mutable {
		t.Fatalf("expected nested temp declaration, got %#v", program.Statements[8])
	}
	right, ok := program.Statements[10].(VariableStatement)
	if !ok || right.Name != "right" || !right.Mutable {
		t.Fatalf("expected mutable nested right binding, got %#v", program.Statements[10])
	}
}

func TestParsePrivateInlineDeferHereStringAndMultipleReturns(t *testing.T) {
	program, errors := Parse(`
private inline function Print() : (name : String, value : Int) {
    defer print("done");
    let mut here_string = //
<h1>Hello</h1>
//;
    return here_string, 1;
}

private {
    local Int hidden = 1;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function, got %T", program.Statements[0])
	}
	if !fn.Private || !fn.Inline || fn.ReturnType != "(String,Int)" || len(fn.ReturnValues) != 2 {
		t.Fatalf("unexpected private inline function: %#v", fn)
	}
	if fn.ReturnValues[0].Name != "name" || fn.ReturnValues[0].Type != "String" || fn.ReturnValues[1].Name != "value" || fn.ReturnValues[1].Type != "Int" {
		t.Fatalf("unexpected return values: %#v", fn.ReturnValues)
	}
	if _, ok := fn.Body[0].(DeferStatement); !ok {
		t.Fatalf("expected defer statement, got %T", fn.Body[0])
	}
	decl, ok := fn.Body[1].(VariableStatement)
	if !ok {
		t.Fatalf("expected here string declaration, got %T", fn.Body[1])
	}
	if literal, ok := decl.Expression.Node.(LiteralExpression); !ok || literal.Kind != "String" || !strings.Contains(literal.Value, "<h1>Hello</h1>") {
		t.Fatalf("expected here string literal, got %#v", decl.Expression.Node)
	}
	ret, ok := fn.Body[2].(ReturnStatement)
	if !ok || len(ret.Values) != 2 {
		t.Fatalf("expected multi return, got %#v", fn.Body[2])
	}
	if _, ok := program.Statements[1].(PrivateBlockStatement); !ok {
		t.Fatalf("expected private block, got %T", program.Statements[1])
	}
}

func TestParseRunStatements(t *testing.T) {
	program, errors := Parse(`
run {
    print("first");
}
run Boot();
function Boot() {}
`)
	assertNoParseErrors(t, errors)

	blockRun, ok := program.Statements[0].(RunStatement)
	if !ok || len(blockRun.Body) != 1 {
		t.Fatalf("expected block run statement, got %#v", program.Statements[0])
	}
	callRun, ok := program.Statements[1].(RunStatement)
	if !ok || callRun.Stmt == nil {
		t.Fatalf("expected call run statement, got %#v", program.Statements[1])
	}
	if _, ok := callRun.Stmt.(ExpressionStatement); !ok {
		t.Fatalf("expected run call expression statement, got %T", callRun.Stmt)
	}
}

func TestParseSkipsMultilineComments(t *testing.T) {
	program, errors := Parse(`
local Int before = 1;
(*
    This is a multi line comment
    (* nested comment *)
*)
local Int after = 2;
`)
	assertNoParseErrors(t, errors)
	if len(program.Statements) != 2 {
		t.Fatalf("expected two statements, got %d", len(program.Statements))
	}
	if first, ok := program.Statements[0].(VariableStatement); !ok || first.Name != "before" {
		t.Fatalf("expected before variable, got %#v", program.Statements[0])
	}
	if second, ok := program.Statements[1].(VariableStatement); !ok || second.Name != "after" {
		t.Fatalf("expected after variable, got %#v", program.Statements[1])
	}
}

func TestParseChildTypeDeclarations(t *testing.T) {
	program, errors := Parse(`
local x : Int.child(8) = 1;
local i16 y = 2;
alias i8 = Int.child(8);
`)
	assertNoParseErrors(t, errors)
	first, ok := program.Statements[0].(VariableStatement)
	if !ok || first.Name != "x" || first.Type != "Int.child(8)" {
		t.Fatalf("expected colon child type declaration, got %#v", program.Statements[0])
	}
	second, ok := program.Statements[1].(VariableStatement)
	if !ok || second.Name != "y" || second.Type != "i16" {
		t.Fatalf("expected alias type declaration, got %#v", program.Statements[1])
	}
	alias, ok := program.Statements[2].(AliasStatement)
	if !ok || alias.Name != "i8" || alias.Target != "Int.child(8)" {
		t.Fatalf("expected child type alias, got %#v", program.Statements[2])
	}
}

func TestParseFunctionCallbackParameterType(t *testing.T) {
	program, errors := Parse(`
function Apply(value : Int, callback : Function[Int, Int]) : Int {
    return callback(value);
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if len(fn.Params) != 2 || fn.Params[1].Type != "Function[Int,Int]" {
		t.Fatalf("unexpected callback parameter: %#v", fn.Params)
	}
	ret, ok := fn.Body[0].(ReturnStatement)
	if !ok {
		t.Fatalf("expected return statement, got %T", fn.Body[0])
	}
	if call, ok := ret.Expression.Node.(CallExpression); !ok {
		t.Fatalf("expected callback call expression, got %#v", ret.Expression.Node)
	} else if callee, ok := call.Callee.(IdentifierExpression); !ok || callee.Name != "callback" {
		t.Fatalf("unexpected callback callee: %#v", call.Callee)
	}
}

func TestParsePatternMatchStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local String mode = "blank";
    if mode == {
        case "blank":
            print("hallo");
            continue;
        case:
            print(10);
    }
    return 0;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	match, ok := fn.Body[1].(MatchStatement)
	if !ok {
		t.Fatalf("expected match statement, got %T", fn.Body[1])
	}
	if match.Partial || match.Value.Literal() != "mode" || len(match.Cases) != 2 {
		t.Fatalf("unexpected match statement: %#v", match)
	}
	if match.Cases[0].Pattern.Literal() != "blank" || match.Cases[0].Default {
		t.Fatalf("unexpected first match case: %#v", match.Cases[0])
	}
	if _, ok := match.Cases[0].Body[1].(ContinueStatement); !ok {
		t.Fatalf("expected continue statement in first case, got %#v", match.Cases[0].Body)
	}
	if !match.Cases[1].Default {
		t.Fatalf("expected second case to be default: %#v", match.Cases[1])
	}
}

func TestParsePartialPatternMatchStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local Int value = 1;
    partial if value == {
        case 1:
            return 1;
    }
    return 0;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	match, ok := fn.Body[1].(MatchStatement)
	if !ok || !match.Partial {
		t.Fatalf("expected partial match statement, got %#v", fn.Body[1])
	}
}

func TestParseLazyFunction(t *testing.T) {
	program, errors := Parse(`
lazy function Choose(useFirst : Bool, first : Int, second : Int) : Int {
    if useFirst {
        return first;
    }
    return second;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if !fn.Lazy || fn.Name != "Choose" || fn.ReturnType != "Int" {
		t.Fatalf("unexpected lazy function: %#v", fn)
	}
}

func TestParseLazyVariableDeclarations(t *testing.T) {
	program, errors := Parse(`
lazy local Int count = BuildCount();
lazy let name = BuildName();
lazy var shared = BuildShared();
`)
	assertNoParseErrors(t, errors)

	cases := []struct {
		index    int
		name     string
		scope    string
		mutable  bool
		inferred bool
	}{
		{0, "count", "local", false, false},
		{1, "name", "local", false, true},
		{2, "shared", "global", true, true},
	}
	for _, expected := range cases {
		decl, ok := program.Statements[expected.index].(VariableStatement)
		if !ok {
			t.Fatalf("expected variable declaration at %d, got %T", expected.index, program.Statements[expected.index])
		}
		if !decl.Lazy || decl.Name != expected.name || decl.Scope != expected.scope ||
			decl.Mutable != expected.mutable || decl.Inferred != expected.inferred || decl.Expression.Node == nil {
			t.Fatalf("unexpected lazy variable declaration at %d: %#v", expected.index, decl)
		}
	}
}

func TestParseAsyncFunctionAndAwait(t *testing.T) {
	program, errors := Parse(`
async function LoadValue() : Int {
    return 41;
}

function Main() : Int {
    return await LoadValue() + 1;
}
`)
	assertNoParseErrors(t, errors)

	asyncFn, ok := program.Statements[0].(FunctionStatement)
	if !ok || !asyncFn.Async || asyncFn.Name != "LoadValue" {
		t.Fatalf("expected async function statement, got %#v", program.Statements[0])
	}
	mainFn := program.Statements[1].(FunctionStatement)
	ret := mainFn.Body[0].(ReturnStatement)
	binary, ok := ret.Expression.Node.(BinaryExpression)
	if !ok || binary.Operator != "+" {
		t.Fatalf("expected await expression inside binary return, got %#v", ret.Expression.Node)
	}
	awaitExpr, ok := binary.Left.(UnaryExpression)
	if !ok || awaitExpr.Operator != "await" {
		t.Fatalf("expected await unary expression, got %#v", binary.Left)
	}
}

func TestParseInnerFunctionAndCallSelector(t *testing.T) {
	program, errors := Parse(`
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
	assertNoParseErrors(t, errors)

	outer, ok := program.Statements[0].(FunctionStatement)
	if !ok || outer.Name != "Test" {
		t.Fatalf("expected Test function, got %#v", program.Statements[0])
	}
	inner, ok := outer.Body[0].(FunctionStatement)
	if !ok || !inner.Inner || inner.Name != "Eval" {
		t.Fatalf("expected inner Eval function, got %#v", outer.Body[0])
	}

	mainFn, ok := program.Statements[1].(FunctionStatement)
	if !ok {
		t.Fatalf("expected Main function, got %#v", program.Statements[1])
	}
	exprStmt, ok := mainFn.Body[0].(ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %#v", mainFn.Body[0])
	}
	call, ok := exprStmt.Expression.Node.(CallExpression)
	if !ok {
		t.Fatalf("expected call expression, got %#v", exprStmt.Expression.Node)
	}
	selector, ok := call.Callee.(SelectorExpression)
	if !ok || selector.Field != "Eval" {
		t.Fatalf("expected selector callee Eval, got %#v", call.Callee)
	}
	if _, ok := selector.Target.(CallExpression); !ok {
		t.Fatalf("expected selector target to be Test() call, got %#v", selector.Target)
	}
}

func TestParseTraitAndImpl(t *testing.T) {
	program, errors := Parse(`
trait Printable {
    function Show(value : Int) : String;
}

impl Printable for Int {
    function Show(value : Int) : String {
        return value as String;
    }
}
`)
	assertNoParseErrors(t, errors)

	trait, ok := program.Statements[0].(TraitStatement)
	if !ok || trait.Name != "Printable" || len(trait.Methods) != 1 {
		t.Fatalf("unexpected trait statement: %#v", program.Statements[0])
	}
	if trait.Methods[0].Name != "Show" || trait.Methods[0].ReturnType != "String" {
		t.Fatalf("unexpected trait method: %#v", trait.Methods[0])
	}
	impl, ok := program.Statements[1].(ImplStatement)
	if !ok || impl.Trait != "Printable" || impl.Type != "Int" || len(impl.Methods) != 1 {
		t.Fatalf("unexpected impl statement: %#v", program.Statements[1])
	}
}

func TestParseDeprecatedFunctionMarkerTag(t *testing.T) {
	program, errors := Parse(`
@deprecated("use Add")
function OldAdd(left : Int, right : Int) : Int {
    return left + right;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if !fn.Deprecated || fn.DeprecationMessage != "use Add" {
		t.Fatalf("expected deprecated function marker, got %#v", fn)
	}
}

func TestParseGlobalGenericVariableDeclaration(t *testing.T) {
	program, errors := Parse(`global mut Map[String, List[Int]] table = {};`)
	assertNoParseErrors(t, errors)

	decl, ok := program.Statements[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable statement, got %T", program.Statements[0])
	}
	if decl.Scope != "global" || !decl.Mutable || decl.Type != "Map[String,List[Int]]" || decl.Name != "table" {
		t.Fatalf("unexpected declaration: %#v", decl)
	}
	if decl.Expression.Literal() != "{ }" {
		t.Fatalf("unexpected declaration expression: %q", decl.Expression.Literal())
	}
}

func TestParseOptionAndResultVariableDeclarations(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local Option[Int] maybe = Some(10);
    local Result[Int, String] parsed = Err("bad");
    return 0;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	if len(fn.Body) != 3 {
		t.Fatalf("expected 3 body statements, got %d", len(fn.Body))
	}

	optionDecl, ok := fn.Body[0].(VariableStatement)
	if !ok || optionDecl.Type != "Option[Int]" {
		t.Fatalf("unexpected option declaration: %#v", fn.Body[0])
	}
	optionCall, ok := optionDecl.Expression.Node.(CallExpression)
	if !ok {
		t.Fatalf("expected option expression to be a call, got %T", optionDecl.Expression.Node)
	}
	optionCallee, ok := optionCall.Callee.(IdentifierExpression)
	if !ok || optionCallee.Name != "Some" {
		t.Fatalf("unexpected option callee: %#v", optionCall.Callee)
	}

	resultDecl, ok := fn.Body[1].(VariableStatement)
	if !ok || resultDecl.Type != "Result[Int,String]" {
		t.Fatalf("unexpected result declaration: %#v", fn.Body[1])
	}
	resultCall, ok := resultDecl.Expression.Node.(CallExpression)
	if !ok {
		t.Fatalf("expected result expression to be a call, got %T", resultDecl.Expression.Node)
	}
	resultCallee, ok := resultCall.Callee.(IdentifierExpression)
	if !ok || resultCallee.Name != "Err" {
		t.Fatalf("unexpected result callee: %#v", resultCall.Callee)
	}
}

func TestParseListComprehensionExpression(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local List[Int] doubled = [value * 2 for value in values if value > 1];
    return 0;
}
`)
	assertNoParseErrors(t, errors)

	fn, ok := program.Statements[0].(FunctionStatement)
	if !ok {
		t.Fatalf("expected function statement, got %T", program.Statements[0])
	}
	decl, ok := fn.Body[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable declaration, got %T", fn.Body[0])
	}
	comprehension, ok := decl.Expression.Node.(ListComprehensionExpression)
	if !ok {
		t.Fatalf("expected list comprehension, got %T", decl.Expression.Node)
	}
	if comprehension.Iterator != "value" {
		t.Fatalf("expected iterator value, got %q", comprehension.Iterator)
	}
	if _, ok := comprehension.Value.(BinaryExpression); !ok {
		t.Fatalf("expected mapped value to be binary expression, got %T", comprehension.Value)
	}
	if identifier, ok := comprehension.Iterable.(IdentifierExpression); !ok || identifier.Name != "values" {
		t.Fatalf("unexpected iterable expression: %#v", comprehension.Iterable)
	}
	if _, ok := comprehension.Condition.(BinaryExpression); !ok {
		t.Fatalf("expected condition to be binary expression, got %T", comprehension.Condition)
	}
}

func TestParseExportedVariableDeclaration(t *testing.T) {
	program, errors := Parse(`export local mut Int shared = 1;`)
	assertNoParseErrors(t, errors)

	decl, ok := program.Statements[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable statement, got %T", program.Statements[0])
	}
	if decl.Scope != "local" || !decl.Exported || !decl.Mutable || decl.Type != "Int" || decl.Name != "shared" {
		t.Fatalf("unexpected exported declaration: %#v", decl)
	}
}

func TestParseNamespaceImportAndCallExpression(t *testing.T) {
	program, errors := Parse(`
import "math.klang";

namespace random {
    function RandomRange(min : Int, max : Int) : Int {
        return min + max;
    }
}

call random.RandomRange(1, 2);
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 3 {
		t.Fatalf("expected 3 top-level statements, got %d", len(program.Statements))
	}
	if stmt, ok := program.Statements[0].(ImportStatement); !ok || stmt.Path != "math.klang" {
		t.Fatalf("unexpected import statement: %#v", program.Statements[0])
	}
	namespace, ok := program.Statements[1].(NamespaceStatement)
	if !ok || namespace.Name != "random" {
		t.Fatalf("unexpected namespace statement: %#v", program.Statements[1])
	}
	if len(namespace.Body) != 1 {
		t.Fatalf("expected namespace body to contain one statement, got %d", len(namespace.Body))
	}
	call, ok := program.Statements[2].(ExpressionStatement)
	if !ok {
		t.Fatalf("expected expression statement, got %T", program.Statements[2])
	}
	if call.Expression.Literal() != "call random . RandomRange ( 1 , 2 )" {
		t.Fatalf("unexpected call expression: %q", call.Expression.Literal())
	}
}

func TestParseModuleDirectives(t *testing.T) {
	program, errors := Parse(`
module(disabled : True);
module_caller(call_entire_module : True);
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 2 {
		t.Fatalf("expected 2 module directive statements, got %d", len(program.Statements))
	}
	module, ok := program.Statements[0].(ModuleDirectiveStatement)
	if !ok || module.Name != "module" || !module.Options["disabled"] {
		t.Fatalf("unexpected module directive: %#v", program.Statements[0])
	}
	caller, ok := program.Statements[1].(ModuleDirectiveStatement)
	if !ok || caller.Name != "module_caller" || !caller.Options["call_entire_module"] {
		t.Fatalf("unexpected module caller directive: %#v", program.Statements[1])
	}
}

func TestParseAssertStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    assert 1 < 2;
    return 1;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	assertStmt, ok := fn.Body[0].(AssertStatement)
	if !ok {
		t.Fatalf("expected assert statement, got %T", fn.Body[0])
	}
	if assertStmt.Expression.Literal() != "1 < 2" {
		t.Fatalf("unexpected assert expression: %q", assertStmt.Expression.Literal())
	}
}

func TestParseReportStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    report value;
    report BuildValue();
    return 1;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	first, ok := fn.Body[0].(ReportStatement)
	if !ok {
		t.Fatalf("expected report statement, got %T", fn.Body[0])
	}
	if first.Expression.Literal() != "value" {
		t.Fatalf("unexpected report expression: %q", first.Expression.Literal())
	}
	second, ok := fn.Body[1].(ReportStatement)
	if !ok {
		t.Fatalf("expected second report statement, got %T", fn.Body[1])
	}
	if second.Expression.Literal() != "BuildValue ( )" {
		t.Fatalf("unexpected second report expression: %q", second.Expression.Literal())
	}
}

func TestParseChainedNamespaceAliasAndNamespaceAccess(t *testing.T) {
	program, errors := Parse(`
namespace std {
    namespace lib {
        function LuaInit() {
            print("std.lib.LuaInit(); is called");
        }
    }
}

alias std_lib = std.lib;
std_lib::LuaInit();
`)
	assertNoParseErrors(t, errors)

	if len(program.Statements) != 3 {
		t.Fatalf("expected 3 top-level statements, got %d", len(program.Statements))
	}
	namespace, ok := program.Statements[0].(NamespaceStatement)
	if !ok || namespace.Name != "std" {
		t.Fatalf("unexpected namespace statement: %#v", program.Statements[0])
	}
	nested, ok := namespace.Body[0].(NamespaceStatement)
	if !ok || nested.Name != "lib" {
		t.Fatalf("expected nested namespace lib, got %#v", namespace.Body[0])
	}
	fn, ok := nested.Body[0].(FunctionStatement)
	if !ok || fn.Name != "LuaInit" || fn.ReturnType != "T" {
		t.Fatalf("unexpected nested function: %#v", nested.Body[0])
	}
	alias, ok := program.Statements[1].(AliasStatement)
	if !ok || alias.Name != "std_lib" || alias.Target != "std.lib" {
		t.Fatalf("unexpected alias statement: %#v", program.Statements[1])
	}
	stmt, ok := program.Statements[2].(ExpressionStatement)
	if !ok {
		t.Fatalf("expected alias call expression, got %T", program.Statements[2])
	}
	call, ok := stmt.Expression.Node.(CallExpression)
	if !ok {
		t.Fatalf("expected call expression, got %#v", stmt.Expression.Node)
	}
	selector, ok := call.Callee.(SelectorExpression)
	if !ok || selector.Field != "LuaInit" {
		t.Fatalf("expected selector callee, got %#v", call.Callee)
	}
	if identifier, ok := selector.Target.(IdentifierExpression); !ok || identifier.Name != "std_lib" {
		t.Fatalf("expected alias selector target, got %#v", selector.Target)
	}
}

func TestParseGlobalNamespace(t *testing.T) {
	program, errors := Parse(`
global namespace alloc {
    function New() : Int {
        return 1;
    }
}
`)
	assertNoParseErrors(t, errors)

	namespace, ok := program.Statements[0].(NamespaceStatement)
	if !ok || namespace.Name != "alloc" || !namespace.Global {
		t.Fatalf("expected global namespace alloc, got %#v", program.Statements[0])
	}
	fn, ok := namespace.Body[0].(FunctionStatement)
	if !ok || fn.Name != "New" {
		t.Fatalf("expected function New in global namespace, got %#v", namespace.Body[0])
	}
}

func TestParseConditionalsAndLoops(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local mut Int i = 0;
    while i < 10 {
        if i == 3 {
            break;
        } else {
            i += 1;
        }
    }
    return i;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	if len(fn.Body) != 3 {
		t.Fatalf("expected 3 function body statements, got %d", len(fn.Body))
	}
	loop, ok := fn.Body[1].(LoopStatement)
	if !ok || loop.Kind != "while" {
		t.Fatalf("expected while loop, got %#v", fn.Body[1])
	}
	if loop.Header.Literal() != "i < 10" {
		t.Fatalf("unexpected loop header: %q", loop.Header.Literal())
	}
	if len(loop.Body) != 1 {
		t.Fatalf("expected one loop body statement, got %d", len(loop.Body))
	}
	condition, ok := loop.Body[0].(IfStatement)
	if !ok || condition.Kind != "if" {
		t.Fatalf("expected if statement, got %#v", loop.Body[0])
	}
	if len(condition.Consequence) != 1 || len(condition.Alternative) != 1 {
		t.Fatalf("unexpected conditional branches: %#v", condition)
	}
}

func TestParseCompactConditionStatement(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local mut Int i = 0;
    if i == 3 break;
    unless i > 0 return 1;
    return i;
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	if len(fn.Body) != 4 {
		t.Fatalf("expected 4 function body statements, got %d", len(fn.Body))
	}

	firstCondition, ok := fn.Body[1].(IfStatement)
	if !ok || firstCondition.Kind != "if" {
		t.Fatalf("expected compact if statement, got %#v", fn.Body[1])
	}
	if firstCondition.Condition.Literal() != "i == 3" {
		t.Fatalf("unexpected compact if condition: %q", firstCondition.Condition.Literal())
	}
	if len(firstCondition.Consequence) != 1 {
		t.Fatalf("expected compact if consequence, got %#v", firstCondition.Consequence)
	}
	if _, ok := firstCondition.Consequence[0].(BreakStatement); !ok {
		t.Fatalf("expected compact if consequence to be break, got %T", firstCondition.Consequence[0])
	}

	secondCondition, ok := fn.Body[2].(IfStatement)
	if !ok || secondCondition.Kind != "unless" {
		t.Fatalf("expected compact unless statement, got %#v", fn.Body[2])
	}
	if secondCondition.Condition.Literal() != "i > 0" {
		t.Fatalf("unexpected compact unless condition: %q", secondCondition.Condition.Literal())
	}
	if len(secondCondition.Consequence) != 1 {
		t.Fatalf("expected compact unless consequence, got %#v", secondCondition.Consequence)
	}
	if _, ok := secondCondition.Consequence[0].(ReturnStatement); !ok {
		t.Fatalf("expected compact unless consequence to be return, got %T", secondCondition.Consequence[0])
	}
}

func TestParseExpressionTreeForBinaryPrecedence(t *testing.T) {
	program, errors := Parse(`local Int result = 1 + 2 * 3;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	root, ok := decl.Expression.Node.(BinaryExpression)
	if !ok || root.Operator != "+" {
		t.Fatalf("expected root + binary expression, got %#v", decl.Expression.Node)
	}
	if _, ok := root.Left.(LiteralExpression); !ok {
		t.Fatalf("expected left literal, got %#v", root.Left)
	}
	right, ok := root.Right.(BinaryExpression)
	if !ok || right.Operator != "*" {
		t.Fatalf("expected right * binary expression, got %#v", root.Right)
	}
}

func TestParseExpressionTreeForPowerPrecedenceAndAssociativity(t *testing.T) {
	program, errors := Parse(`local Int result = -2 ** 3 ** 2;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	unary, ok := decl.Expression.Node.(UnaryExpression)
	if !ok || unary.Operator != "-" {
		t.Fatalf("expected root unary -, got %#v", decl.Expression.Node)
	}
	power, ok := unary.Right.(BinaryExpression)
	if !ok || power.Operator != "**" {
		t.Fatalf("expected unary to contain power expression, got %#v", unary.Right)
	}
	rightPower, ok := power.Right.(BinaryExpression)
	if !ok || rightPower.Operator != "**" {
		t.Fatalf("expected power to be right-associative, got %#v", power.Right)
	}
}

func TestParseExpressionTreeForBooleanPrecedence(t *testing.T) {
	program, errors := Parse(`local Bool result = not ready and active xor failed or fallback;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	root, ok := decl.Expression.Node.(BinaryExpression)
	if !ok || root.Operator != "or" {
		t.Fatalf("expected root or expression, got %#v", decl.Expression.Node)
	}
	xorExpr, ok := root.Left.(BinaryExpression)
	if !ok || xorExpr.Operator != "xor" {
		t.Fatalf("expected xor below or, got %#v", root.Left)
	}
	andExpr, ok := xorExpr.Left.(BinaryExpression)
	if !ok || andExpr.Operator != "and" {
		t.Fatalf("expected and below xor, got %#v", xorExpr.Left)
	}
	if unary, ok := andExpr.Left.(UnaryExpression); !ok || unary.Operator != "not" {
		t.Fatalf("expected not to bind before and, got %#v", andExpr.Left)
	}
}

func TestParseExpressionTreeForPipeOperator(t *testing.T) {
	program, errors := Parse(`local Int result = 2 |> Add(3) |> Double;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	root, ok := decl.Expression.Node.(BinaryExpression)
	if !ok || root.Operator != "|>" {
		t.Fatalf("expected root pipe expression, got %#v", decl.Expression.Node)
	}
	if _, ok := root.Right.(IdentifierExpression); !ok {
		t.Fatalf("expected bare function target on right pipe, got %#v", root.Right)
	}
	leftPipe, ok := root.Left.(BinaryExpression)
	if !ok || leftPipe.Operator != "|>" {
		t.Fatalf("expected pipe to be left-associative, got %#v", root.Left)
	}
	if _, ok := leftPipe.Right.(CallExpression); !ok {
		t.Fatalf("expected function call target on left pipe, got %#v", leftPipe.Right)
	}
}

func TestParseExpressionTreeForTypeCast(t *testing.T) {
	program, errors := Parse(`local Float value = 10 as Float;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	cast, ok := decl.Expression.Node.(CastExpression)
	if !ok {
		t.Fatalf("expected cast expression, got %#v", decl.Expression.Node)
	}
	if cast.Type != "Float" {
		t.Fatalf("expected cast target Float, got %q", cast.Type)
	}
	if literal, ok := cast.Value.(LiteralExpression); !ok || literal.Kind != "Int" || literal.Value != "10" {
		t.Fatalf("unexpected cast value: %#v", cast.Value)
	}
}

func TestParseExpressionTreeForNullSafetyOperator(t *testing.T) {
	program, errors := Parse(`local Bool exists = MaybeValue()?;`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	nullCheck, ok := decl.Expression.Node.(NullCheckExpression)
	if !ok {
		t.Fatalf("expected null check expression, got %#v", decl.Expression.Node)
	}
	if _, ok := nullCheck.Value.(CallExpression); !ok {
		t.Fatalf("expected null check value to be a call, got %#v", nullCheck.Value)
	}
}

func TestParseExpressionTreeForCallsSelectorsAndIndexes(t *testing.T) {
	program, errors := Parse(`local Int value = call random.RandomRange(items[0], 10);`)
	assertNoParseErrors(t, errors)

	decl := program.Statements[0].(VariableStatement)
	callPrefix, ok := decl.Expression.Node.(UnaryExpression)
	if !ok || callPrefix.Operator != "call" {
		t.Fatalf("expected call unary expression, got %#v", decl.Expression.Node)
	}
	call, ok := callPrefix.Right.(CallExpression)
	if !ok {
		t.Fatalf("expected call expression, got %#v", callPrefix.Right)
	}
	selector, ok := call.Callee.(SelectorExpression)
	if !ok || selector.Field != "RandomRange" {
		t.Fatalf("expected selector callee, got %#v", call.Callee)
	}
	if len(call.Arguments) != 2 {
		t.Fatalf("expected two call args, got %d", len(call.Arguments))
	}
	if _, ok := call.Arguments[0].(IndexExpression); !ok {
		t.Fatalf("expected first arg index expression, got %#v", call.Arguments[0])
	}
}

func TestParseFunctionAndSelectorNamedCopy(t *testing.T) {
	program, errors := Parse(`
namespace list {
    function copy(values : List[T]) : List[T] {
        return copy values;
    }
}

function Main() : Int {
    local List[Int] values = [1, 2, 3];
    local List[Int] copied = list.copy(values);
    return len(copied);
}
`)
	assertNoParseErrors(t, errors)

	namespace, ok := program.Statements[0].(NamespaceStatement)
	if !ok || namespace.Name != "list" {
		t.Fatalf("expected list namespace, got %#v", program.Statements[0])
	}
	fn, ok := namespace.Body[0].(FunctionStatement)
	if !ok || fn.Name != "copy" {
		t.Fatalf("expected copy function, got %#v", namespace.Body[0])
	}

	mainFn := program.Statements[1].(FunctionStatement)
	decl := mainFn.Body[1].(VariableStatement)
	call, ok := decl.Expression.Node.(CallExpression)
	if !ok {
		t.Fatalf("expected call expression, got %#v", decl.Expression.Node)
	}
	selector, ok := call.Callee.(SelectorExpression)
	if !ok || selector.Field != "copy" {
		t.Fatalf("expected list.copy selector, got %#v", call.Callee)
	}
}

func TestParseLambdaExpression(t *testing.T) {
	program, errors := Parse(`
local Function[Int, Int] increment = fun(value : Int) : Int {
    return value + 1;
};
`)
	assertNoParseErrors(t, errors)

	decl, ok := program.Statements[0].(VariableStatement)
	if !ok {
		t.Fatalf("expected variable statement, got %#v", program.Statements[0])
	}
	lambda, ok := decl.Expression.Node.(LambdaExpression)
	if !ok {
		t.Fatalf("expected lambda expression, got %#v", decl.Expression.Node)
	}
	if len(lambda.Params) != 1 || lambda.Params[0].Name != "value" || lambda.ReturnType != "Int" {
		t.Fatalf("unexpected lambda signature: %#v", lambda)
	}
	if len(lambda.Body) != 1 {
		t.Fatalf("expected lambda body with one statement, got %d", len(lambda.Body))
	}
}

func TestParseRestrictedLambdaAndMutableParameter(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    local Function[Int, Int] update = fun[T restrict[Int]](mut value : T) : T {
        value += 1;
        return value;
    };
    return update(1);
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	decl := fn.Body[0].(VariableStatement)
	lambda, ok := decl.Expression.Node.(LambdaExpression)
	if !ok {
		t.Fatalf("expected lambda expression, got %#v", decl.Expression.Node)
	}
	if len(lambda.TypeParams) != 1 || lambda.TypeParams[0].Type != "T:Int" {
		t.Fatalf("unexpected lambda type params: %#v", lambda.TypeParams)
	}
	if len(lambda.Params) != 1 || !lambda.Params[0].Mutable || lambda.Params[0].Type != "T:Int" {
		t.Fatalf("unexpected lambda param: %#v", lambda.Params)
	}
}

func TestParseFunctionGroup(t *testing.T) {
	program, errors := Parse(`
function_group Poly {
    set_function_as_part_of[{ .name = "Poly" }, "function1_name", "function2_name"];
}
`)
	assertNoParseErrors(t, errors)

	group, ok := program.Statements[0].(FunctionGroupStatement)
	if !ok {
		t.Fatalf("expected function group statement, got %#v", program.Statements[0])
	}
	if group.Name != "Poly" || len(group.Functions) != 2 ||
		group.Functions[0] != "function1_name" || group.Functions[1] != "function2_name" {
		t.Fatalf("unexpected function group: %#v", group)
	}
}

func TestParseAliasFunctionWithExtensionMethodAndRegion(t *testing.T) {
	program, errors := Parse(`
region MyRegion(T, sizeof(T) * 100, 10);

alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) -> type
    [new] do
        allocator.region = get_default_procces_allocator(#region(100, T), #sizeof(capacity));
    end

    #extend do
        function get_length() -> int
            return this.length;
        end
    end
end
`)
	assertNoParseErrors(t, errors)

	if _, ok := program.Statements[0].(RegionStatement); !ok {
		t.Fatalf("expected region statement, got %#v", program.Statements[0])
	}
	alias, ok := program.Statements[1].(AliasFunctionStatement)
	if !ok {
		t.Fatalf("expected alias function statement, got %#v", program.Statements[1])
	}
	if alias.Name != "ArrayList" || len(alias.Params) != 4 || len(alias.Hooks) != 1 || len(alias.Methods) != 1 {
		t.Fatalf("unexpected alias function: %#v", alias)
	}
	if alias.Methods[0].Name != "get_length" || alias.Methods[0].ReturnType != "Int" {
		t.Fatalf("unexpected extension method: %#v", alias.Methods[0])
	}
}

func TestParseModernAliasFunctionSyntaxAndInferredParameterDefault(t *testing.T) {
	program, errors := Parse(`
alias function ArrayList[T: Any](data: T, length: int, capacity: int, allocator = .DEFAULT) : type {
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
    }
}

function create_workspace(name : String, workspace := UserDefinedWorkspace()) {
    print(name);
}
`)
	assertNoParseErrors(t, errors)

	alias, ok := program.Statements[0].(AliasFunctionStatement)
	if !ok {
		t.Fatalf("expected alias function, got %T", program.Statements[0])
	}
	if alias.ReturnType != "T" || len(alias.Hooks) != 1 || len(alias.Methods) != 1 || len(alias.Body) != 2 {
		t.Fatalf("unexpected modern alias parse: %#v", alias)
	}
	fn := program.Statements[1].(FunctionStatement)
	if len(fn.Params) != 2 || fn.Params[1].Name != "workspace" || fn.Params[1].Type != "T" || fn.Params[1].Default.Node == nil {
		t.Fatalf("unexpected inferred default parameter: %#v", fn.Params)
	}
}

func TestParseAliasFunctionStructBody(t *testing.T) {
	program, errors := Parse(`
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
`)
	assertNoParseErrors(t, errors)

	alias, ok := program.Statements[0].(AliasFunctionStatement)
	if !ok {
		t.Fatalf("expected alias function, got %T", program.Statements[0])
	}
	if !alias.Struct || alias.ReturnType != "T" || len(alias.Hooks) != 3 || len(alias.Methods) != 2 || len(alias.Body) != 2 {
		t.Fatalf("unexpected struct alias parse: %#v", alias)
	}
}

func TestParseEntryPointDirective(t *testing.T) {
	program, errors := Parse(`
namespace App {
    #set_entry_point_to_here
    function Process() : Int {
        return 1;
    }
}
`)
	assertNoParseErrors(t, errors)
	namespace := program.Statements[0].(NamespaceStatement)
	if _, ok := namespace.Body[0].(EntryPointStatement); !ok {
		t.Fatalf("expected entry point directive, got %T", namespace.Body[0])
	}
}

func TestParseExpressionTreeForListAndMapLiterals(t *testing.T) {
	listProgram, listErrors := Parse(`local List[Int] values = [1, 2, 3];`)
	assertNoParseErrors(t, listErrors)
	listDecl := listProgram.Statements[0].(VariableStatement)
	list, ok := listDecl.Expression.Node.(ListExpression)
	if !ok || len(list.Items) != 3 {
		t.Fatalf("expected list expression with 3 items, got %#v", listDecl.Expression.Node)
	}

	mapProgram, mapErrors := Parse(`local Map[String, Int] values = {"one": 1, "two": 2};`)
	assertNoParseErrors(t, mapErrors)
	mapDecl := mapProgram.Statements[0].(VariableStatement)
	mapExpr, ok := mapDecl.Expression.Node.(MapExpression)
	if !ok || len(mapExpr.Entries) != 2 {
		t.Fatalf("expected map expression with 2 entries, got %#v", mapDecl.Expression.Node)
	}
}

func TestParseAssignmentExpressionTree(t *testing.T) {
	program, errors := Parse(`items[index + 1] = value * 2;`)
	assertNoParseErrors(t, errors)

	assignment, ok := program.Statements[0].(AssignmentStatement)
	if !ok {
		t.Fatalf("expected assignment statement, got %T", program.Statements[0])
	}
	if _, ok := assignment.Target.Node.(IndexExpression); !ok {
		t.Fatalf("expected indexed assignment target, got %#v", assignment.Target.Node)
	}
	value, ok := assignment.Expression.Node.(BinaryExpression)
	if !ok || value.Operator != "*" {
		t.Fatalf("expected binary assignment value, got %#v", assignment.Expression.Node)
	}
}

func TestParseTryCatchThrowAndResultPropagation(t *testing.T) {
	program, errors := Parse(`
function Main() : Int {
    try {
        local Int value = Err("bad")!;
        return value;
    } catch err {
        throw err;
    }
}
`)
	assertNoParseErrors(t, errors)

	fn := program.Statements[0].(FunctionStatement)
	tryStmt, ok := fn.Body[0].(TryCatchStatement)
	if !ok {
		t.Fatalf("expected try/catch statement, got %#v", fn.Body[0])
	}
	if tryStmt.ErrorName != "err" || len(tryStmt.TryBody) != 2 || len(tryStmt.CatchBody) != 1 {
		t.Fatalf("unexpected try/catch shape: %#v", tryStmt)
	}
	decl := tryStmt.TryBody[0].(VariableStatement)
	if _, ok := decl.Expression.Node.(PropagateExpression); !ok {
		t.Fatalf("expected propagation expression, got %#v", decl.Expression.Node)
	}
	if _, ok := tryStmt.CatchBody[0].(ThrowStatement); !ok {
		t.Fatalf("expected throw statement, got %#v", tryStmt.CatchBody[0])
	}
}

func TestParseRejectsIllegalTokens(t *testing.T) {
	_, errors := Parse(`local Int value = ~;`)
	if len(errors) == 0 {
		t.Fatal("expected parse errors for illegal token")
	}
}

func TestParseRejectsMalformedGenericType(t *testing.T) {
	_, errors := Parse(`local Map[String, Int table = {};`)
	if len(errors) == 0 {
		t.Fatal("expected parse errors for malformed generic type")
	}
}

func TestParseEnumDeclaration(t *testing.T) {
	program, errors := Parse(`enum Color { Red; Blue = 4; Green; }`)
	assertNoParseErrors(t, errors)

	enumStmt, ok := program.Statements[0].(EnumStatement)
	if !ok {
		t.Fatalf("expected enum statement, got %#v", program.Statements[0])
	}
	if enumStmt.Name != "Color" || len(enumStmt.Variants) != 3 {
		t.Fatalf("unexpected enum statement: %#v", enumStmt)
	}
	if enumStmt.Variants[0].Ordinal != 0 || enumStmt.Variants[1].Ordinal != 4 || enumStmt.Variants[2].Ordinal != 5 {
		t.Fatalf("unexpected enum ordinals: %#v", enumStmt.Variants)
	}
}

func TestParseFixturePrograms(t *testing.T) {
	programs, err := file.DiscoverPrograms(filepath.Join("..", "..", "tests"))
	if err != nil {
		t.Fatalf("failed to discover fixture programs: %v", err)
	}

	for _, program := range programs {
		for _, source := range program.Files {
			_, errors := Parse(strings.Join(source.Lines, "\n"))
			if len(errors) != 0 {
				t.Fatalf("%s parse errors: %#v", source.Path, errors)
			}
		}
	}
}

func TestParseLoadedProgramParsesEverySourceFile(t *testing.T) {
	loadedProgram, err := file.LoadProgram(filepath.Join("..", "..", "tests", "test21"))
	if err != nil {
		t.Fatalf("failed to load fixture program: %v", err)
	}

	parsed := ParseLoadedProgram(loadedProgram)
	if !parsed.Passed() {
		t.Fatalf("expected loaded program to parse, got %#v", parsed.Errors())
	}
	if parsed.Name != "test21" {
		t.Fatalf("expected parsed program name test21, got %q", parsed.Name)
	}
	if len(parsed.Sources) != 2 {
		t.Fatalf("expected test21 to parse two source files, got %d", len(parsed.Sources))
	}
}

func assertNoParseErrors(t *testing.T, errors []Error) {
	t.Helper()
	if len(errors) != 0 {
		t.Fatalf("expected no parse errors, got %#v", errors)
	}
}
