package parser

import (
	"strings"
	"sync"

	sourcefile "kLang/src/engine/file"
	"kLang/src/lexer"
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
	text := strings.Join(source.Lines, "\n")
	tokens := lexer.New(text).Tokenize()
	return parseSourceWithTypeAliases(source, tokens, discoverTypeAliases(tokens))
}

func parseSourceWithTypeAliases(source sourcefile.SourceFile, tokens []lexer.Token, aliases map[string]string) ParsedSource {
	parser := NewWithTypeAliases(tokens, aliases)
	program := parser.ParseProgram()
	return ParsedSource{
		Path:                 source.Path,
		Program:              program,
		Errors:               parser.Errors(),
		ModuleFunctionFilter: source.ModuleFunctionFilter,
	}
}

func ParseLoadedProgram(program sourcefile.Program) ParsedProgram {
	parsed := ParsedProgram{
		Name:    program.Name,
		Sources: make([]ParsedSource, len(program.Files)),
	}

	tokensBySource := make([][]lexer.Token, len(program.Files))
	aliases := map[string]string{}
	for index, source := range program.Files {
		tokens := lexer.New(strings.Join(source.Lines, "\n")).Tokenize()
		tokensBySource[index] = tokens
		for name, target := range discoverTypeAliases(tokens) {
			aliases[name] = target
		}
	}

	var wait sync.WaitGroup
	for index, source := range program.Files {
		wait.Add(1)
		go func(index int, source sourcefile.SourceFile) {
			defer wait.Done()
			parsed.Sources[index] = parseSourceWithTypeAliases(source, tokensBySource[index], aliases)
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
		case ScopeStatement:
			if entry := entryPointFromStatements(current.Body, namespace); entry != "" {
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
