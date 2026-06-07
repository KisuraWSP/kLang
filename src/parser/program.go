package parser

import (
	"strings"

	sourcefile "kLang/src/engine/file"
)

type ParsedSource struct {
	Path    string
	Program *Program
	Errors  []Error
}

type ParsedProgram struct {
	Name    string
	Sources []ParsedSource
}

func ParseSource(source sourcefile.SourceFile) ParsedSource {
	program, errors := Parse(strings.Join(source.Lines, "\n"))
	return ParsedSource{
		Path:    source.Path,
		Program: program,
		Errors:  errors,
	}
}

func ParseLoadedProgram(program sourcefile.Program) ParsedProgram {
	parsed := ParsedProgram{
		Name:    program.Name,
		Sources: make([]ParsedSource, 0, len(program.Files)),
	}

	for _, source := range program.Files {
		parsed.Sources = append(parsed.Sources, ParseSource(source))
	}

	return parsed
}

func (program ParsedProgram) Errors() []Error {
	var errors []Error
	for _, source := range program.Sources {
		errors = append(errors, source.Errors...)
	}
	return errors
}

func (program ParsedProgram) Passed() bool {
	return len(program.Errors()) == 0
}
