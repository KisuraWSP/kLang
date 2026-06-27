package parser

import "fmt"

const DefaultEntryPoint = "Main"

type EntryPointDiagnostic struct {
	File    string
	Line    int
	Column  int
	Message string
}

type entryPointCandidate struct {
	Name     string
	File     string
	Function FunctionStatement
}

func ResolveEntryPoint(program ParsedProgram) (string, []EntryPointDiagnostic) {
	var defaults []entryPointCandidate
	var custom []entryPointCandidate
	var diagnostics []EntryPointDiagnostic

	for _, source := range program.Sources {
		for _, statement := range source.Program.Statements {
			if function, ok := statement.(FunctionStatement); ok && function.Name == DefaultEntryPoint {
				defaults = append(defaults, entryPointCandidate{
					Name:     DefaultEntryPoint,
					File:     source.Path,
					Function: function,
				})
			}
		}
		collectCustomEntryPoints(source.Program.Statements, "", source.Path, &custom, &diagnostics)
	}

	if len(custom) > 1 {
		for _, candidate := range custom[1:] {
			diagnostics = append(diagnostics, EntryPointDiagnostic{
				File:    candidate.File,
				Line:    candidate.Function.Pos.Line,
				Column:  candidate.Function.Pos.Column,
				Message: "only one #set_entry_point_to_here directive is allowed per program",
			})
		}
	}
	if len(custom) > 0 {
		diagnostics = append(diagnostics, validateEntryPointSignature(custom[0])...)
		return custom[0].Name, diagnostics
	}

	if len(defaults) == 0 {
		diagnostics = append(diagnostics, EntryPointDiagnostic{
			File:    entryPointDiagnosticFile(program),
			Line:    1,
			Column:  1,
			Message: "program must define function Main() : Int or mark one function with #set_entry_point_to_here",
		})
		return "", diagnostics
	}
	if len(defaults) > 1 {
		for _, candidate := range defaults[1:] {
			diagnostics = append(diagnostics, EntryPointDiagnostic{
				File:    candidate.File,
				Line:    candidate.Function.Pos.Line,
				Column:  candidate.Function.Pos.Column,
				Message: "program can define only one top-level Main entry function",
			})
		}
	}
	diagnostics = append(diagnostics, validateEntryPointSignature(defaults[0])...)
	return DefaultEntryPoint, diagnostics
}

func collectCustomEntryPoints(statements []Statement, namespace string, source string, candidates *[]entryPointCandidate, diagnostics *[]EntryPointDiagnostic) {
	for index, statement := range statements {
		switch current := statement.(type) {
		case EntryPointStatement:
			if index+1 >= len(statements) {
				*diagnostics = append(*diagnostics, EntryPointDiagnostic{
					File: source, Line: current.Pos.Line, Column: current.Pos.Column,
					Message: "#set_entry_point_to_here must be immediately followed by a function",
				})
				continue
			}
			function, ok := statements[index+1].(FunctionStatement)
			if !ok {
				*diagnostics = append(*diagnostics, EntryPointDiagnostic{
					File: source, Line: current.Pos.Line, Column: current.Pos.Column,
					Message: "#set_entry_point_to_here must be immediately followed by a function",
				})
				continue
			}
			*candidates = append(*candidates, entryPointCandidate{
				Name:     namespace + function.Name,
				File:     source,
				Function: function,
			})
		case NamespaceStatement:
			collectCustomEntryPoints(current.Body, namespace+current.Name+".", source, candidates, diagnostics)
		case PrivateBlockStatement:
			collectCustomEntryPoints(current.Body, namespace, source, candidates, diagnostics)
		case ScopeStatement:
			collectCustomEntryPoints(current.Body, namespace, source, candidates, diagnostics)
		}
	}
}

func validateEntryPointSignature(candidate entryPointCandidate) []EntryPointDiagnostic {
	function := candidate.Function
	if len(function.Params) == 0 && len(function.TypeParams) == 0 && !function.Async &&
		function.ReturnType == "Int" && len(function.ReturnValues) == 0 {
		return nil
	}
	return []EntryPointDiagnostic{{
		File:   candidate.File,
		Line:   function.Pos.Line,
		Column: function.Pos.Column,
		Message: fmt.Sprintf(
			"entry point %s must have signature function %s() : Int",
			candidate.Name,
			function.Name,
		),
	}}
}

func entryPointDiagnosticFile(program ParsedProgram) string {
	if len(program.Sources) > 0 {
		return program.Sources[0].Path
	}
	return "<program>"
}
