package lexer

import "testing"

func TestLexesJSONStructTag(t *testing.T) {
	tokens := New("this.id `json:\"user_id\"`;").Tokenize()
	found := false
	for _, token := range tokens {
		if token.Type == TokenStructTag {
			found = true
			if token.Literal != `json:"user_id"` {
				t.Fatalf("unexpected struct tag literal: %q", token.Literal)
			}
		}
	}
	if !found {
		t.Fatalf("expected struct tag token, got %#v", tokens)
	}
}
