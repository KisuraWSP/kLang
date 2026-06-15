package context

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

type Phase string

const (
	PhaseSource  Phase = "SOURCE"
	PhaseModule  Phase = "MODULE"
	PhaseParse   Phase = "PARSE"
	PhaseType    Phase = "TYPE"
	PhaseRuntime Phase = "RUNTIME"
	PhaseBackend Phase = "BACKEND"
	PhaseWASM    Phase = "WASM"
)

type SourceContext struct {
	File  string
	Lines []string
}

type ErrorContext struct {
	Phase      Phase
	File       string
	Line       int
	Column     int
	Message    string
	Rule       string
	Hint       string
	SourceLine string
}

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
			Lines: append([]string(nil), source.Lines...),
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
	if err.Column <= 0 {
		err.Column = 1
	}
	lines := ctx.SourceLines(err.File)
	if err.Line > 0 && err.Line <= len(lines) {
		err.SourceLine = lines[err.Line-1]
	}
	return err
}

func ModuleErrors(program file.Program, report modulesystem.Report) []ErrorContext {
	ctx := New(program)
	errors := make([]ErrorContext, 0, len(report.Errors))
	for _, err := range report.Errors {
		errors = append(errors, ctx.WithSource(ErrorContext{
			Phase:   PhaseModule,
			File:    err.File,
			Line:    err.Line,
			Column:  err.Column,
			Message: err.Message,
			Rule:    "import resolution",
			Hint:    "Check that the imported module exists, is spelled correctly, and is reachable from this workspace.",
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
				Phase:   PhaseParse,
				File:    source.Path,
				Line:    err.Line,
				Column:  err.Column,
				Message: err.Message,
				Rule:    "syntax",
				Hint:    "The parser could not understand this part of the program. Check the syntax around the marked code.",
			}))
		}
	}
	return errors
}

func TypeErrors(program file.Program, report typechecker.Report) []ErrorContext {
	ctx := New(program)
	errors := make([]ErrorContext, 0, len(report.Errors))
	for _, err := range report.Errors {
		errors = append(errors, ctx.WithSource(ErrorContext{
			Phase:   PhaseType,
			File:    err.File,
			Line:    err.Line,
			Column:  1,
			Message: HumanTypeMessage(err.Message),
			Rule:    "static semantics",
			Hint:    "I found a conflict between what this code produces and what the surrounding program expects.",
		}))
	}
	return errors
}

func RuntimeError(program file.Program, err error) ErrorContext {
	ctx := New(program)
	line, column, message := RuntimeErrorParts(err)
	return ctx.WithSource(ErrorContext{
		Phase:   PhaseRuntime,
		File:    program.EntryPoint,
		Line:    line,
		Column:  column,
		Message: message,
		Rule:    "runtime semantics",
		Hint:    "The program reached this code while running and could not continue safely.",
	})
}

func BackendError(program file.Program, backend string, err error) ErrorContext {
	ctx := New(program)
	phase := PhaseBackend
	if strings.EqualFold(backend, "WASM") {
		phase = PhaseWASM
	}
	return ctx.WithSource(ErrorContext{
		Phase:   phase,
		File:    program.EntryPoint,
		Line:    0,
		Column:  1,
		Message: err.Error(),
		Rule:    "backend contract",
		Hint:    fmt.Sprintf("Check the %s backend configuration and any generated bundle requirements.", backend),
	})
}

func RuntimeErrorParts(err error) (int, int, string) {
	message := err.Error()
	pattern := regexp.MustCompile(`line ([0-9]+):([0-9]+): (.*)`)
	matches := pattern.FindStringSubmatch(message)
	if len(matches) != 4 {
		return 0, 1, message
	}
	line, _ := strconv.Atoi(matches[1])
	column, _ := strconv.Atoi(matches[2])
	return line, column, matches[3]
}

func HumanTypeMessage(message string) string {
	switch {
	case strings.Contains(message, "cannot assign"):
		return message + "\n\nThis value does not have the type declared for the variable."
	case strings.Contains(message, "argument") && strings.Contains(message, "expects"):
		return message + "\n\nThis function call is passing a value with an unexpected type."
	case strings.Contains(message, "unknown identifier"):
		return message + "\n\nThis name has not been declared in the current scope."
	default:
		return message
	}
}
