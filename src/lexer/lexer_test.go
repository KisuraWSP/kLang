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

func TestLexerTokenizesLiteralsNamespaceCallsAndOperators(t *testing.T) {
	input := `call random.RandomRange(-2, 3.5); local String text = "hello"; local Char letter = 'K'; unless True != False { return text; }`

	assertTokens(t, input, []Token{
		{Type: TokenCall, Literal: "call"},
		{Type: TokenIdentifier, Literal: "random"},
		{Type: TokenDot, Literal: "."},
		{Type: TokenIdentifier, Literal: "RandomRange"},
		{Type: TokenLeftBrace, Literal: "("},
		{Type: TokenMinus, Literal: "-"},
		{Type: TokenInt, Literal: "2"},
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
	for _, input := range []string{`123abc`, `1.2.3`, `10.`} {
		tokens := New(input).Tokenize()
		if tokens[0].Type != TokenIllegal {
			t.Fatalf("%q: expected malformed number to be illegal, got %#v", input, tokens[0])
		}
	}
}

func TestLexerReportsIllegalUnknownCharacters(t *testing.T) {
	tokens := New(`local Int value = @;`).Tokenize()

	atToken := tokens[4]
	if atToken.Type != TokenIllegal || atToken.Literal != "@" {
		t.Fatalf("expected illegal @ token, got %#v", atToken)
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
