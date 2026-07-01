package lexer

import "testing"

func FuzzLexerNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"", "function Main() : Int { return 0; }", "\"unterminated", "😀 := 0xFF_FF;", "(* nested (* comment *)",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		_ = New(source).Tokenize()
	})
}
