package backend

import (
	"kLang/src/diagnostic"
	"kLang/src/engine/file"
	"kLang/src/parser"
)

type Request struct {
	Program file.Program
	Parsed  parser.ParsedProgram
	Backend string
}

type Diagnostic = diagnostic.Diagnostic

type Artifact struct {
	Path    string
	Content []byte
}

type Output struct {
	Entry     string
	Artifacts []Artifact
}

type Backend interface {
	Name() string
	Check(Request) []Diagnostic
	Emit(Request) (Output, error)
	Package(Output, string) error
}
