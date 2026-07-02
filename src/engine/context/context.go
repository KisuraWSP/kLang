package context

import (
	"fmt"
	"path/filepath"
	"strings"

	"kLang/src/diagnostic"
	"kLang/src/engine/backend"
	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

type Phase = diagnostic.Phase

const (
	PhaseSource  = diagnostic.PhaseSource
	PhaseModule  = diagnostic.PhaseModule
	PhaseParse   = diagnostic.PhaseParse
	PhaseType    = diagnostic.PhaseType
	PhaseRuntime = diagnostic.PhaseRuntime
	PhaseBackend = diagnostic.PhaseBackend
	PhaseJS      = diagnostic.PhaseJS
	PhaseWASM    = diagnostic.PhaseWASM
)

type SourceContext struct {
	File  string
	Lines []string
}

type ErrorContext = diagnostic.Diagnostic

type Context struct {
	ProgramName string
	EntryPoint  string
	Backend     string
	Sources     map[string]SourceContext
	Errors      []ErrorContext
}

func New(program file.Program) *Context {
	ctx := &Context{
		ProgramName: program.Name,
		EntryPoint:  program.EntryPoint,
		Sources:     map[string]SourceContext{},
	}
	for _, source := range program.Files {
		ctx.Sources[filepath.Clean(source.Path)] = SourceContext{
			File:  source.Path,
			Lines: append([]string(nil), source.DisplayLines()...),
		}
	}
	return ctx
}

func (ctx *Context) Add(err ErrorContext) {
	err = ctx.WithSource(err)
	ctx.Errors = append(ctx.Errors, err)
}

func (ctx *Context) HasErrors() bool {
	return len(ctx.Errors) != 0
}

func (ctx *Context) SourceLines(path string) []string {
	source, ok := ctx.Sources[filepath.Clean(path)]
	if ok {
		return append([]string(nil), source.Lines...)
	}
	if lines, err := file.ReadLines(path); err == nil {
		return lines
	}
	return nil
}

func (ctx *Context) WithSource(err ErrorContext) ErrorContext {
	err = diagnostic.Normalize(err)
	if err.Column <= 0 {
		err.Column = 1
		err.Primary.StartColumn = 1
	}
	if err.EndLine <= 0 && err.Line > 0 {
		err.EndLine = err.Line
	}
	if err.EndColumn <= 0 {
		err.EndColumn = err.Column
	} else if err.EndColumn < err.Column {
		err.EndColumn = err.Column
	}
	lines := ctx.SourceLines(err.File)
	if err.Line > 0 && err.Line <= len(lines) {
		err.SourceLine = lines[err.Line-1]
	}
	err.Primary = diagnostic.Span{
		File:        err.File,
		StartLine:   err.Line,
		StartColumn: err.Column,
		EndLine:     err.EndLine,
		EndColumn:   err.EndColumn,
	}
	return err
}

func ModuleErrors(program file.Program, report modulesystem.Report) []ErrorContext {
	ctx := New(program)
	errors := make([]ErrorContext, 0, len(report.Errors))
	for _, err := range report.Errors {
		errors = append(errors, ctx.WithSource(ErrorContext{
			Code:      err.Code,
			Severity:  diagnostic.SeverityError,
			Phase:     PhaseModule,
			File:      err.File,
			Line:      err.Line,
			Column:    err.Column,
			EndLine:   err.EndLine,
			EndColumn: err.EndColumn,
			Message:   err.Message,
			Rule:      err.Rule,
			Hint:      err.Hint,
		}))
	}
	return errors
}

func ParseErrors(program file.Program, parsed parser.ParsedProgram) []ErrorContext {
	ctx := New(program)
	var errors []ErrorContext
	for _, source := range parsed.Sources {
		for _, err := range source.Errors {
			errors = append(errors, ctx.WithSource(ErrorContext{
				Code:      err.Code,
				Severity:  diagnostic.SeverityError,
				Phase:     PhaseParse,
				File:      source.Path,
				Line:      err.Line,
				Column:    err.Column,
				EndLine:   err.EndLine,
				EndColumn: err.EndColumn,
				Message:   err.Message,
				Rule:      err.Rule,
				Hint:      err.Hint,
			}))
		}
	}
	return errors
}

func TypeErrors(program file.Program, report typechecker.Report) []ErrorContext {
	ctx := New(program)
	errors := make([]ErrorContext, 0, len(report.Errors))
	for _, err := range report.Errors {
		message := err.Message
		if err.ExpectedType != "" && err.FoundType != "" {
			message = humanExpectedFoundMessage(message, err.ExpectedType, err.FoundType)
		}
		rule := err.Rule
		if rule == "" {
			rule = "static semantics"
		}
		hint := err.Hint
		if hint == "" {
			hint = "Fix the marked construct so it satisfies the language's static rules."
		}
		code := err.Code
		if code == "" {
			code = diagnostic.CodeStaticSemantics
		}
		diag := ErrorContext{
			Code:         code,
			Severity:     diagnostic.SeverityError,
			Phase:        PhaseType,
			File:         err.File,
			Line:         err.Line,
			Column:       err.Column,
			EndLine:      err.EndLine,
			EndColumn:    err.EndColumn,
			Message:      message,
			Rule:         rule,
			FeatureID:    err.FeatureID,
			Hint:         hint,
			Primary:      err.Primary,
			Labels:       err.Labels,
			Notes:        err.Notes,
			Helps:        err.Helps,
			Suggestions:  err.Suggestions,
			Fixes:        err.Fixes,
			ExpectedType: err.ExpectedType,
			FoundType:    err.FoundType,
		}
		errors = append(errors, ctx.WithSource(diag))
	}
	return errors
}

func RuntimeError(program file.Program, err error) ErrorContext {
	ctx := New(program)
	if carrier, ok := err.(diagnostic.Carrier); ok {
		value := carrier.Diagnostic()
		value.Phase = PhaseRuntime
		value.Severity = diagnostic.SeverityError
		if value.Code == "" {
			value.Code = diagnostic.CodeRuntimeFailure
		}
		if value.File == "" {
			value.File = program.EntryPoint
		}
		return ctx.WithSource(value)
	}
	return ctx.WithSource(ErrorContext{
		Code:     diagnostic.CodeRuntimeFailure,
		Severity: diagnostic.SeverityError,
		Phase:    PhaseRuntime,
		File:     program.EntryPoint,
		Line:     0,
		Column:   1,
		Message:  err.Error(),
		Rule:     "runtime semantics",
		Hint:     "The program reached this code while running and could not continue safely.",
	})
}

func BackendError(program file.Program, backend string, err error) ErrorContext {
	ctx := New(program)
	phase := PhaseBackend
	if strings.EqualFold(backend, "WASM") {
		phase = PhaseWASM
	} else if strings.EqualFold(backend, "JS") {
		phase = PhaseJS
	}
	return ctx.WithSource(ErrorContext{
		Code:     diagnostic.CodeBackendFailure,
		Severity: diagnostic.SeverityError,
		Phase:    phase,
		File:     program.EntryPoint,
		Line:     0,
		Column:   1,
		Message:  err.Error(),
		Rule:     "backend contract",
		Hint:     fmt.Sprintf("Check the %s backend configuration and any generated bundle requirements.", backend),
	})
}

func BackendDiagnostics(program file.Program, backendName string, diagnostics []backend.Diagnostic) []ErrorContext {
	ctx := New(program)
	phase := PhaseBackend
	if strings.EqualFold(backendName, "JS") {
		phase = PhaseJS
	} else if strings.EqualFold(backendName, "WASM") {
		phase = PhaseWASM
	}
	errors := make([]ErrorContext, 0, len(diagnostics))
	for _, backendDiagnostic := range diagnostics {
		hint := backendDiagnostic.Hint
		if hint == "" {
			hint = fmt.Sprintf("Use syntax supported by the %s backend or select a runtime packaging backend.", backendName)
		}
		rule := backendDiagnostic.Rule
		if rule == "" {
			rule = "backend feature support"
		}
		code := backendDiagnostic.Code
		if code == "" {
			code = diagnostic.CodeBackendUnsupported
		}
		errors = append(errors, ctx.WithSource(ErrorContext{
			Code: code, Severity: diagnostic.SeverityError,
			Phase: phase, File: backendDiagnostic.File, Line: backendDiagnostic.Line, Column: backendDiagnostic.Column,
			EndLine: backendDiagnostic.EndLine, EndColumn: backendDiagnostic.EndColumn, Primary: backendDiagnostic.Primary,
			Message: backendDiagnostic.Message, Rule: rule, FeatureID: backendDiagnostic.FeatureID, Hint: hint, Labels: backendDiagnostic.Labels,
			Notes: backendDiagnostic.Notes, Helps: backendDiagnostic.Helps, Suggestions: backendDiagnostic.Suggestions, Fixes: backendDiagnostic.Fixes,
		}))
	}
	return errors
}

func humanExpectedFoundMessage(message string, expected string, found string) string {
	return message + "\n\nExpected type:\n" + formatTypeTree(expected, "  ") +
		"\nFound type:\n" + formatTypeTree(found, "  ") +
		"\nThis value does not have the type declared for the variable."
}

func formatTypeTree(typeName string, indent string) string {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" {
		return indent + "<unknown>\n"
	}
	open := strings.Index(typeName, "[")
	if open == -1 || !strings.HasSuffix(typeName, "]") {
		return indent + typeName + "\n"
	}
	builder := strings.Builder{}
	builder.WriteString(indent)
	builder.WriteString(typeName[:open])
	builder.WriteString("\n")
	for _, part := range splitTypeArguments(typeName[open+1 : len(typeName)-1]) {
		builder.WriteString(formatTypeTree(part, indent+"  "))
	}
	return builder.String()
}

func splitTypeArguments(input string) []string {
	var parts []string
	start := 0
	depth := 0
	for index, ch := range input {
		switch ch {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(input[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}
