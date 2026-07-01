package runtime

import (
	"testing"

	"kLang/src/parser"
)

func FuzzListIndexingNeverPanics(f *testing.F) {
	f.Add(uint8(0), int64(0))
	f.Add(uint8(3), int64(2))
	f.Add(uint8(3), int64(-1))
	f.Fuzz(func(t *testing.T, rawLength uint8, rawIndex int64) {
		length := int(rawLength % 64)
		items := make([]Value, length)
		for index := range items {
			items[index] = IntValue(index)
		}
		index := int(rawIndex)
		if int64(index) != rawIndex {
			return
		}
		current := New()
		env := NewEnvironment(nil)
		if err := current.defineValue(env, "values", false, "List[Int]", Value{Kind: ValueList, Data: items}); err != nil {
			t.Fatal(err)
		}
		if err := current.defineValue(env, "index", false, "Int", IntValue(index)); err != nil {
			t.Fatal(err)
		}
		expression := parser.IndexExpression{
			Target: parser.IdentifierExpression{Name: "values"},
			Index:  parser.IdentifierExpression{Name: "index"},
		}
		value, err := current.evalIndex(expression, env)
		if index < 0 || index >= length {
			if err == nil {
				t.Fatalf("expected bounds error for length=%d index=%d, got %#v", length, index, value)
			}
			return
		}
		if err != nil || value.Kind != ValueInt || value.Data.(int) != index {
			t.Fatalf("unexpected indexed value for length=%d index=%d: %#v, %v", length, index, value, err)
		}
	})
}
