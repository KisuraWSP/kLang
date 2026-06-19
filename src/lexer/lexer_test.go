package lexer

import (
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestLexerTokenizesVariableDeclaration(t *testing.T) {
	input := `export local mut Map[String, Int] rowResults = {};`

	assertTokens(t, input, []Token{
		{Type: TokenExport, Literal: "export"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenMut, Literal: "mut"},
		{Type: TokenIdentifier, Literal: "Map"},
		{Type: TokenLeftSquareBrace, Literal: "["},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenComma, Literal: ","},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenRightSquareBrace, Literal: "]"},
		{Type: TokenIdentifier, Literal: "rowResults"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesInferredVariableKeywords(t *testing.T) {
	input := `let x = Some(1); let mut y = Some(2); val z = Some(3); var w = Some(4); const sizeValue = Int.sizeof;`

	assertTokens(t, input, []Token{
		{Type: TokenLet, Literal: "let"},
		{Type: TokenIdentifier, Literal: "x"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "Some"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLet, Literal: "let"},
		{Type: TokenMut, Literal: "mut"},
		{Type: TokenIdentifier, Literal: "y"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "Some"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "2"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenVal, Literal: "val"},
		{Type: TokenIdentifier, Literal: "z"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "Some"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "3"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenVar, Literal: "var"},
		{Type: TokenIdentifier, Literal: "w"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "Some"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "4"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenConst, Literal: "const"},
		{Type: TokenIdentifier, Literal: "sizeValue"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenDot, Literal: "."},
		{Type: TokenIdentifier, Literal: "sizeof"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesTemporaryVariableKeyword(t *testing.T) {
	input := `temp local Int scratch = 1; temp let cached = scratch + 1;`

	assertTokens(t, input, []Token{
		{Type: TokenTemp, Literal: "temp"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "scratch"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenTemp, Literal: "temp"},
		{Type: TokenLet, Literal: "let"},
		{Type: TokenIdentifier, Literal: "cached"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "scratch"},
		{Type: TokenPlus, Literal: "+"},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesNumberSelectors(t *testing.T) {
	input := `5.times(Callback);`

	assertTokens(t, input, []Token{
		{Type: TokenInt, Literal: "5"},
		{Type: TokenDot, Literal: "."},
		{Type: TokenIdentifier, Literal: "times"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenIdentifier, Literal: "Callback"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesSignedAndPrefixedNumberLiterals(t *testing.T) {
	input := `local Int negative = -42; local Int hex = 0xAAAA; local Int octal = 0o755; local Int binary = 0b1010; local Int negativeHex = -0xA; local Int delta = 1 - 2; local Int powered = -0x2 ** 3;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "negative"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "-42"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "hex"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0xAAAA"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "octal"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0o755"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "binary"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0b1010"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "negativeHex"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "-0xA"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "delta"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenMinus, Literal: "-"},
		{Type: TokenInt, Literal: "2"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "powered"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenMinus, Literal: "-"},
		{Type: TokenInt, Literal: "0x2"},
		{Type: TokenExponent, Literal: "**"},
		{Type: TokenInt, Literal: "3"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesNumberSeparators(t *testing.T) {
	input := `local Int big = 1_000_000; local Float ratio = 12_345.67_89; local Int mask = 0xFF_FF; local Int flags = 0b1010_0101; local Int mode = 0o7_55;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "big"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "1_000_000"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Float"},
		{Type: TokenIdentifier, Literal: "ratio"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenFloat, Literal: "12_345.67_89"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "mask"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0xFF_FF"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "flags"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0b1010_0101"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "mode"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "0o7_55"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesUnicodeIdentifiers(t *testing.T) {
	input := `function එකතු(අගය : Int, 😀 : Int) : Int { return අගය + 😀; }`

	tokens := New(input).Tokenize()
	for _, token := range tokens {
		if token.Type == TokenIllegal {
			t.Fatalf("unexpected illegal token %#v", token)
		}
	}

	var identifiers []string
	for _, token := range tokens {
		if token.Type == TokenIdentifier {
			identifiers = append(identifiers, token.Literal)
		}
	}
	if strings.Join(identifiers, ",") != "එකතු,අගය,Int,😀,Int,Int,අගය,😀" {
		t.Fatalf("unexpected unicode identifiers: %#v", identifiers)
	}
}

func TestLexerTokenizesEnumDeclarations(t *testing.T) {
	input := `enum Color { Red; Blue = 4; Green; }`

	tokens := New(input).Tokenize()
	if tokens[0].Type != TokenEnum || tokens[0].Literal != "enum" {
		t.Fatalf("expected enum keyword token, got %#v", tokens[0])
	}
}

func TestLexerTokenizesPrivateInlineDeferAndHereString(t *testing.T) {
	input := "private inline function Hidden() : String { defer print(\"done\"); let html = //\n<section>\n    hi\n</section>\n//;\nreturn html;\n}"

	tokens := New(input).Tokenize()
	foundPrivate := false
	foundInline := false
	foundDefer := false
	foundStruct := false
	foundHereString := false
	for _, token := range tokens {
		switch token.Type {
		case TokenPrivate:
			foundPrivate = true
		case TokenInline:
			foundInline = true
		case TokenDefer:
			foundDefer = true
		case TokenStruct:
			foundStruct = true
		case TokenString:
			if strings.Contains(token.Literal, "<section>") && strings.Contains(token.Literal, "</section>") {
				foundHereString = true
			}
		}
	}
	structTokens := New(`alias function Wrapped(value : Int) : type = struct {}`).Tokenize()
	for _, token := range structTokens {
		if token.Type == TokenStruct {
			foundStruct = true
		}
	}
	if !foundPrivate || !foundInline || !foundDefer || !foundStruct || !foundHereString {
		t.Fatalf("expected private/inline/defer/struct/here string tokens, got %#v %#v", tokens, structTokens)
	}
}

func TestLexerSkipsMultilineComments(t *testing.T) {
	input := `local Int before = 1;
(*
   This is a multi line comment
   (* nested comment *)
*)
local Int after = 2;`

	tokens := New(input).Tokenize()
	var identifiers []string
	for _, token := range tokens {
		if token.Type == TokenIdentifier {
			identifiers = append(identifiers, token.Literal)
		}
		if token.Type == TokenIllegal {
			t.Fatalf("unexpected illegal token %#v", token)
		}
	}
	if strings.Join(identifiers, ",") != "Int,before,Int,after" {
		t.Fatalf("unexpected identifiers after multiline comment skip: %#v", identifiers)
	}
}

func TestLexerTokenizesFunctionAndControlFlowSyntax(t *testing.T) {
	input := `
-- comments are skipped
function Add(a : Int, b : Int) : Int {
    while info := a < b {
        a += 1;
        if a == 3 break;
    }
    return a;
}`

	assertTokens(t, input, []Token{
		{Type: TokenFunc, Literal: "function"},
		{Type: TokenIdentifier, Literal: "Add"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenComma, Literal: ","},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenWhile, Literal: "while"},
		{Type: TokenIdentifier, Literal: "info"},
		{Type: TokenEvaluationAssign, Literal: ":="},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenLessThan, Literal: "<"},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenPlusEqual, Literal: "+="},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIf, Literal: "if"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenStrictEquality, Literal: "=="},
		{Type: TokenInt, Literal: "3"},
		{Type: TokenBreak, Literal: "break"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenReturn, Literal: "return"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesPatternMatchKeywords(t *testing.T) {
	input := `partial if value == { case "blank": continue; case: break; }`

	assertTokens(t, input, []Token{
		{Type: TokenPartial, Literal: "partial"},
		{Type: TokenIf, Literal: "if"},
		{Type: TokenIdentifier, Literal: "value"},
		{Type: TokenStrictEquality, Literal: "=="},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenCase, Literal: "case"},
		{Type: TokenString, Literal: "blank"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenContinue, Literal: "continue"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenCase, Literal: "case"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenBreak, Literal: "break"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesFunctionMarkerTags(t *testing.T) {
	input := `@deprecated("use NewAdd") function OldAdd() : Int { return 1; }`

	assertTokens(t, input, []Token{
		{Type: TokenAt, Literal: "@"},
		{Type: TokenIdentifier, Literal: "deprecated"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenString, Literal: "use NewAdd"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenFunc, Literal: "function"},
		{Type: TokenIdentifier, Literal: "OldAdd"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenReturn, Literal: "return"},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesLazyFunction(t *testing.T) {
	input := `lazy function Choose() : Int { return 1; }`

	assertTokens(t, input, []Token{
		{Type: TokenLazy, Literal: "lazy"},
		{Type: TokenFunc, Literal: "function"},
		{Type: TokenIdentifier, Literal: "Choose"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenReturn, Literal: "return"},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesInnerFunction(t *testing.T) {
	input := `inner function Eval() { print("This is called"); }`

	assertTokens(t, input, []Token{
		{Type: TokenInner, Literal: "inner"},
		{Type: TokenFunc, Literal: "function"},
		{Type: TokenIdentifier, Literal: "Eval"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenIdentifier, Literal: "print"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenString, Literal: "This is called"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesLambdaAndFunctionGroup(t *testing.T) {
	input := `function_group Poly { set_function_as_part_of[{ .name = "Poly" }, "A"]; } local Function[Int, Int] f = fun(x : Int) : Int { return x; };`

	tokens := New(input).Tokenize()
	foundGroup := false
	foundLambda := false
	for _, token := range tokens {
		if token.Type == TokenFuncGroup && token.Literal == "function_group" {
			foundGroup = true
		}
		if token.Type == TokenLambdaFunc && token.Literal == "fun" {
			foundLambda = true
		}
	}
	if !foundGroup || !foundLambda {
		t.Fatalf("expected function_group and fun tokens, got %#v", tokens)
	}
}

func TestLexerTokenizesTraitsAndMove(t *testing.T) {
	input := `trait Printable { function Show(value : Int) : String; } impl Printable for Int {} local String b = move a;`

	assertTokens(t, input, []Token{
		{Type: TokenTrait, Literal: "trait"},
		{Type: TokenIdentifier, Literal: "Printable"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenFunc, Literal: "function"},
		{Type: TokenIdentifier, Literal: "Show"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenIdentifier, Literal: "value"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenInferReturn, Literal: ":"},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenImpl, Literal: "impl"},
		{Type: TokenIdentifier, Literal: "Printable"},
		{Type: TokenFor, Literal: "for"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenMove, Literal: "move"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesNullSafetyOperator(t *testing.T) {
	input := `local Bool exists = MaybeValue()?;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Bool"},
		{Type: TokenIdentifier, Literal: "exists"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "MaybeValue"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenQuestion, Literal: "?"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesBooleanOperators(t *testing.T) {
	input := `if not ready and active xor failed or fallback { assert ready; return 1; }`

	assertTokens(t, input, []Token{
		{Type: TokenIf, Literal: "if"},
		{Type: TokenNot, Literal: "not"},
		{Type: TokenIdentifier, Literal: "ready"},
		{Type: TokenAnd, Literal: "and"},
		{Type: TokenIdentifier, Literal: "active"},
		{Type: TokenXor, Literal: "xor"},
		{Type: TokenIdentifier, Literal: "failed"},
		{Type: TokenOr, Literal: "or"},
		{Type: TokenIdentifier, Literal: "fallback"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenAssert, Literal: "assert"},
		{Type: TokenIdentifier, Literal: "ready"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenReturn, Literal: "return"},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesPipeOperator(t *testing.T) {
	input := `local Int result = 2 |> Add(3) |> Double;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "result"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "2"},
		{Type: TokenPipe, Literal: "|>"},
		{Type: TokenIdentifier, Literal: "Add"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "3"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenPipe, Literal: "|>"},
		{Type: TokenIdentifier, Literal: "Double"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesLiteralsNamespaceCallsAndOperators(t *testing.T) {
	input := `call random.RandomRange(-2, 3.5); local String text = "hello"; local Char letter = 'K'; unless True != False { return text; }`

	assertTokens(t, input, []Token{
		{Type: TokenCall, Literal: "call"},
		{Type: TokenIdentifier, Literal: "random"},
		{Type: TokenDot, Literal: "."},
		{Type: TokenIdentifier, Literal: "RandomRange"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenInt, Literal: "-2"},
		{Type: TokenComma, Literal: ","},
		{Type: TokenFloat, Literal: "3.5"},
		{Type: TokenRightBrace, Literal: ")"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenIdentifier, Literal: "text"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenString, Literal: "hello"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Char"},
		{Type: TokenIdentifier, Literal: "letter"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenChar, Literal: "K"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenUnless, Literal: "unless"},
		{Type: TokenBool, Literal: "True"},
		{Type: TokenNotEqual, Literal: "!="},
		{Type: TokenBool, Literal: "False"},
		{Type: TokenScopeBegin, Literal: "{"},
		{Type: TokenReturn, Literal: "return"},
		{Type: TokenIdentifier, Literal: "text"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenScopeEnd, Literal: "}"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerKeepsCommentMarkersInsideStringLiterals(t *testing.T) {
	input := `local String text = "--not a comment"; -- real comment
local Int value = 1;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenIdentifier, Literal: "text"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenString, Literal: "--not a comment"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "value"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerDistinguishesFloorDivisionFromComments(t *testing.T) {
	input := `local Int half = total // divisor; -- comment
local Int value = 1;`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "half"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenIdentifier, Literal: "total"},
		{Type: TokenFloorDivision, Literal: "//"},
		{Type: TokenIdentifier, Literal: "divisor"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Int"},
		{Type: TokenIdentifier, Literal: "value"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenInt, Literal: "1"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerUsesLongestOperatorMatch(t *testing.T) {
	input := `a**b; a->b; a>=b; a<=b; a/=b; a*=b; a-=b; a+=b; a:=b;`

	assertTokens(t, input, []Token{
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenExponent, Literal: "**"},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenArrow, Literal: "->"},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenGreaterThanOrEqualTo, Literal: ">="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenLessThanOrEqualTo, Literal: "<="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenDivideEqual, Literal: "/="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenMultiEqual, Literal: "*="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenMinusEqual, Literal: "-="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenPlusEqual, Literal: "+="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenIdentifier, Literal: "a"},
		{Type: TokenEvaluationAssign, Literal: ":="},
		{Type: TokenIdentifier, Literal: "b"},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerTokenizesEscapedStringAndCharLiterals(t *testing.T) {
	input := `local String text = "hello \"world\""; local Char newline = '\n';`

	assertTokens(t, input, []Token{
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "String"},
		{Type: TokenIdentifier, Literal: "text"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenString, Literal: `hello \"world\"`},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenLocal, Literal: "local"},
		{Type: TokenIdentifier, Literal: "Char"},
		{Type: TokenIdentifier, Literal: "newline"},
		{Type: TokenAssign, Literal: "="},
		{Type: TokenChar, Literal: `\n`},
		{Type: TokenSemicolon, Literal: ";"},
		{Type: TokenEOFDescriptor, Literal: ""},
	})
}

func TestLexerReportsIllegalStringEscapes(t *testing.T) {
	for _, input := range []string{`"bad \q escape"`, `"bad \0 escape"`} {
		tokens := New(input).Tokenize()
		if tokens[0].Type != TokenIllegal {
			t.Fatalf("%q: expected illegal string literal, got %#v", input, tokens[0])
		}
	}
}

func TestLexerReportsIllegalCharLiterals(t *testing.T) {
	for _, input := range []string{`''`, `'ab'`, `'unterminated`} {
		tokens := New(input).Tokenize()
		if tokens[0].Type != TokenIllegal {
			t.Fatalf("%q: expected illegal char literal, got %#v", input, tokens[0])
		}
	}
}

func TestLexerReportsIllegalCharEscapes(t *testing.T) {
	for _, input := range []string{`'\q'`, `'\0'`} {
		tokens := New(input).Tokenize()
		if tokens[0].Type != TokenIllegal {
			t.Fatalf("%q: expected illegal char literal, got %#v", input, tokens[0])
		}
	}
}

func TestLexerReportsIllegalUnterminatedString(t *testing.T) {
	tokens := New(`local String text = "unterminated`).Tokenize()
	lastRealToken := tokens[len(tokens)-2]

	if lastRealToken.Type != TokenIllegal {
		t.Fatalf("expected illegal token, got %#v", lastRealToken)
	}
}

func TestLexerReportsMalformedNumbers(t *testing.T) {
	for _, input := range []string{`123abc`, `1.2.3`, `10.`, `0x`, `0xG`, `0b102`, `0o789`, `1_`, `1__2`, `0x_FF`, `0b101_2`, `1_.2`} {
		tokens := New(input).Tokenize()
		if tokens[0].Type != TokenIllegal {
			t.Fatalf("%q: expected malformed number to be illegal, got %#v", input, tokens[0])
		}
	}
}

func TestLexerReportsIllegalUnknownCharacters(t *testing.T) {
	tokens := New(`local Int value = ~;`).Tokenize()

	unknownToken := tokens[4]
	if unknownToken.Type != TokenIllegal || unknownToken.Literal != "~" {
		t.Fatalf("expected illegal ~ token, got %#v", unknownToken)
	}
}

func TestLexerSkipsCommentAtEOF(t *testing.T) {
	tokens := New(`local Int value = 1; -- final comment`).Tokenize()

	if tokens[len(tokens)-1].Type != TokenEOFDescriptor {
		t.Fatalf("expected EOF after trailing comment, got %#v", tokens[len(tokens)-1])
	}
	if tokens[len(tokens)-2].Type != TokenSemicolon {
		t.Fatalf("expected semicolon before EOF, got %#v", tokens[len(tokens)-2])
	}
}

func TestLexerTracksLineAndColumn(t *testing.T) {
	tokens := New("local Int x = 1;\nreturn x;").Tokenize()

	returnToken := tokens[6]
	if returnToken.Type != TokenReturn {
		t.Fatalf("expected return token, got %#v", returnToken)
	}
	if returnToken.Line != 2 || returnToken.Column != 1 {
		t.Fatalf("expected return at line 2 column 1, got line %d column %d", returnToken.Line, returnToken.Column)
	}
}

func TestLexerTracksPositionAfterSkippedComment(t *testing.T) {
	tokens := New("-- ignored\n  local Int x = 1;").Tokenize()

	firstToken := tokens[0]
	if firstToken.Type != TokenLocal {
		t.Fatalf("expected local token, got %#v", firstToken)
	}
	if firstToken.Line != 2 || firstToken.Column != 3 {
		t.Fatalf("expected local at line 2 column 3, got line %d column %d", firstToken.Line, firstToken.Column)
	}
}

func TestLexerSkipsCRLFWhitespace(t *testing.T) {
	tokens := New("local Int x = 1;\r\nreturn x;").Tokenize()

	returnToken := tokens[6]
	if returnToken.Type != TokenReturn {
		t.Fatalf("expected return token, got %#v", returnToken)
	}
	if returnToken.Line != 2 || returnToken.Column != 1 {
		t.Fatalf("expected return at line 2 column 1, got line %d column %d", returnToken.Line, returnToken.Column)
	}
}

func TestLexerTokenizesFixtureProgramsWithoutIllegalTokens(t *testing.T) {
	programs, err := file.DiscoverPrograms(filepath.Join("..", "..", "tests"))
	if err != nil {
		t.Fatalf("failed to discover fixture programs: %v", err)
	}

	for _, program := range programs {
		for _, source := range program.Files {
			tokens := New(strings.Join(source.Lines, "\n")).Tokenize()
			for _, token := range tokens {
				if token.Type == TokenIllegal {
					t.Fatalf("%s:%d:%d illegal token %q", source.Path, token.Line, token.Column, token.Literal)
				}
			}
		}
	}
}

func assertTokens(t *testing.T, input string, expected []Token) {
	t.Helper()

	tokens := New(input).Tokenize()
	if len(tokens) != len(expected) {
		t.Fatalf("expected %d tokens, got %d:\n%#v", len(expected), len(tokens), tokens)
	}

	for index, expectedToken := range expected {
		actualToken := tokens[index]
		if actualToken.Type != expectedToken.Type || actualToken.Literal != expectedToken.Literal {
			t.Fatalf("token %d: expected (%v, %q), got (%v, %q)", index, expectedToken.Type, expectedToken.Literal, actualToken.Type, actualToken.Literal)
		}
	}
}
