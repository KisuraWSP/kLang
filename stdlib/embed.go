package stdlib

import (
	"embed"
	"strings"
)

//go:embed raylib.klang
var embedded embed.FS

// Source returns a standard-library module embedded in the kLang executable.
func Source(name string) ([]string, bool) {
	if name != "raylib" && name != "raylib.klang" {
		return nil, false
	}
	content, err := embedded.ReadFile("raylib.klang")
	if err != nil {
		return nil, false
	}
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n"), true
}
