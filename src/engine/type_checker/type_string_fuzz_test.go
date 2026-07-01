package typechecker

import "testing"

func FuzzTypeStringParsingNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"Int", "List[String]", "Map[String,List[Option[Int]]]", "Result[T,Atom]", "List[", "😀[T]",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, typeName string) {
		normalized := normalizeType(typeName)
		_, _, _ = splitGenericType(normalized)
		_ = isAssignable(normalized, normalized)
	})
}
