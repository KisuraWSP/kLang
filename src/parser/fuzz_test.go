package parser

import "testing"

func FuzzParserNeverPanics(f *testing.F) {
	for _, seed := range []string{
		"", "function Main() : Int { return 0; }", "local List[Int] values = [1, 2];",
		"if True { return 1; } else { return 0; }", "@backend(\"JS\"); function Browser() {}",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		_, _ = Parse(source)
	})
}
