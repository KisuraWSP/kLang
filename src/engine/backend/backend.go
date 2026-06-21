package backend

import (
	"kLang/src/engine/file"
	"kLang/src/parser"
)

type Request struct {
	Program file.Program
	Parsed  parser.ParsedProgram
}

type Diagnostic struct {
	File      string
	Line      int
	Column    int
	EndColumn int
	Rule      string
	Message   string
	Hint      string
}

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
