package lexer

import (
	"path/filepath"
	"strings"
	"testing"

	"kLang/src/engine/file"
)

func TestLexerTokenizesVariableDeclaration(t *testing.T) {
	input := `local mut Map[String, Int] rowResults = {};`

	assertTokens(t, input, []Token{
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

func TestLexerReportsIllegalUnterminatedString(t *testing.T) {
	tokens := New(`local String text = "unterminated`).Tokenize()
	lastRealToken := tokens[len(tokens)-2]

	if lastRealToken.Type != TokenIllegal {
		t.Fatalf("expected illegal token, got %#v", lastRealToken)
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
