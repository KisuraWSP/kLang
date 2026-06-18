package parser

import (
	"strings"
	"sync"

	sourcefile "kLang/src/engine/file"
)

type ParsedSource struct {
	Path                 string
	Program              *Program
	Errors               []Error
	ModuleFunctionFilter map[string]bool
}

type ParsedProgram struct {
	Name       string
	EntryPoint string
	Sources    []ParsedSource
}

func ParseSource(source sourcefile.SourceFile) ParsedSource {
	program, errors := Parse(strings.Join(source.Lines, "\n"))
	return ParsedSource{
		Path:                 source.Path,
		Program:              program,
		Errors:               errors,
		ModuleFunctionFilter: source.ModuleFunctionFilter,
	}
}

func ParseLoadedProgram(program sourcefile.Program) ParsedProgram {
	parsed := ParsedProgram{
		Name:    program.Name,
		Sources: make([]ParsedSource, len(program.Files)),
	}

	var wait sync.WaitGroup
	for index, source := range program.Files {
		wait.Add(1)
		go func(index int, source sourcefile.SourceFile) {
			defer wait.Done()
			parsed.Sources[index] = ParseSource(source)
		}(index, source)
	}
	wait.Wait()
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
