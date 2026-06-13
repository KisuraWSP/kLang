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
	Name       string
	EntryPoint string
	Sources    []ParsedSource
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
	for _, source := range parsed.Sources {
		if entry := entryPointFromStatements(source.Program.Statements, ""); entry != "" {
			parsed.EntryPoint = entry
			break
		}
	}

	return parsed
}

func entryPointFromStatements(statements []Statement, namespace string) string {
	armed := false
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case EntryPointStatement:
			armed = true
		case FunctionStatement:
			if armed {
				return namespace + current.Name
			}
			armed = false
		case NamespaceStatement:
			if entry := entryPointFromStatements(current.Body, namespace+current.Name+"."); entry != "" {
				return entry
			}
			armed = false
		default:
			armed = false
		}
	}
	return ""
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
