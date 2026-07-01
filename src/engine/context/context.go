package context

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
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
	if err.EndColumn > 0 && err.EndColumn < err.Column {
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
			Code:     diagnostic.CodeImportResolution,
			Severity: diagnostic.SeverityError,
			Phase:    PhaseModule,
			File:     err.File,
			Line:     err.Line,
			Column:   err.Column,
			Message:  err.Message,
			Rule:     "import resolution",
			Hint:     "Check that the imported module exists, is spelled correctly, and is reachable from this workspace.",
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
				Code:      diagnostic.CodeSyntax,
				Severity:  diagnostic.SeverityError,
				Phase:     PhaseParse,
				File:      source.Path,
				Line:      err.Line,
				Column:    err.Column,
				EndLine:   err.EndLine,
				EndColumn: err.EndColumn,
				Message:   err.Message,
				Rule:      "syntax",
				Hint:      "The parser could not understand this part of the program. Check the syntax around the marked code.",
			}))
		}
	}
	return errors
}

func TypeErrors(program file.Program, report typechecker.Report) []ErrorContext {
	ctx := New(program)
	errors := make([]ErrorContext, 0, len(report.Errors))
	candidates := diagnosticCandidates(program)
	for _, err := range report.Errors {
		message := err.Message
		if err.ExpectedType != "" && err.FoundType != "" {
			message = humanExpectedFoundMessage(message, err.ExpectedType, err.FoundType)
		} else {
			message = HumanTypeMessage(message)
		}
		rule := err.Rule
		if rule == "" {
			rule = typeErrorRule(err.Message)
		}
		hint := err.Hint
		if hint == "" {
			hint = typeErrorHint(err.Message, candidates)
		}
		code := err.Code
		if code == "" {
			code = legacyTypeErrorCode(err.Message)
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
		if diag.Column <= 0 {
			diag.Column, diag.EndColumn = diagnosticSpan(ctx.SourceLines(err.File), err.Line, err.Message)
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
	line, column, message := RuntimeErrorParts(err)
	return ctx.WithSource(ErrorContext{
		Code:     diagnostic.CodeRuntimeFailure,
		Severity: diagnostic.SeverityError,
		Phase:    PhaseRuntime,
		File:     program.EntryPoint,
		Line:     line,
		Column:   column,
		Message:  message,
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
	if expected, found, ok := expectedFoundTypes(message); ok {
		return humanExpectedFoundMessage(message, expected, found)
	}
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

func humanExpectedFoundMessage(message string, expected string, found string) string {
	return message + "\n\nExpected type:\n" + formatTypeTree(expected, "  ") +
		"\nFound type:\n" + formatTypeTree(found, "  ") +
		"\nThis value does not have the type declared for the variable."
}

func legacyTypeErrorCode(message string) string {
	switch {
	case strings.Contains(message, "unknown identifier"):
		return diagnostic.CodeUnknownIdentifier
	case strings.Contains(message, "unknown function"):
		return diagnostic.CodeUnknownFunction
	case strings.Contains(message, "unknown type"):
		return diagnostic.CodeUnknownType
	case strings.Contains(message, "transaction"):
		return diagnostic.CodeTransactionSafety
	case strings.Contains(message, "cannot assign"), strings.Contains(message, "expects"), strings.Contains(message, "return expression"):
		return diagnostic.CodeTypeMismatch
	default:
		return diagnostic.CodeStaticSemantics
	}
}

func typeErrorRule(message string) string {
	switch {
	case strings.Contains(message, "unknown identifier"):
		return "name resolution"
	case strings.Contains(message, "unknown function"):
		return "function resolution"
	case strings.Contains(message, "unknown type"):
		return "type resolution"
	case strings.Contains(message, "cannot assign"), strings.Contains(message, "expects"):
		return "type compatibility"
	default:
		return "static semantics"
	}
}

func typeErrorHint(message string, candidates map[string]bool) string {
	if name, ok := quotedPayload(message, `unknown identifier "([^"]+)"`); ok {
		if suggestion, found := closestCandidate(name, candidates); found {
			return fmt.Sprintf("Did you mean %q? Declare the name before this point, or qualify it if it comes from a namespace.", suggestion)
		}
		return "Declare this name in the current scope before using it, or qualify it with the namespace that defines it."
	}
	if name, ok := quotedPayload(message, `unknown function "([^"]+)"`); ok {
		if suggestion, found := closestCandidate(name, candidates); found {
			return fmt.Sprintf("Did you mean %q? If the function lives in another module, import that module or call it with its namespace.", suggestion)
		}
		return "Check the function spelling. If it lives in another module, import that module or use a qualified module call."
	}
	if name, ok := barePayload(message, `unknown type ([A-Za-z_][A-Za-z0-9_:\.\[\],]*)`); ok {
		if suggestion, found := closestCandidate(name, candidates); found {
			return fmt.Sprintf("Did you mean %q? Builtin and user-defined type names are case-sensitive.", suggestion)
		}
		return "Check the type spelling, import/declare the type, or use one of the builtin type names."
	}
	if expected, found, ok := expectedFoundTypes(message); ok {
		return fmt.Sprintf("Expected %s but found %s. Change the expression, add an explicit cast, or adjust the declared type.", expected, found)
	}
	return "I found a conflict between what this code produces and what the surrounding program expects."
}

func diagnosticSpan(lines []string, line int, message string) (int, int) {
	column := 1
	endColumn := 1
	if line <= 0 || line > len(lines) {
		return column, endColumn
	}
	source := lines[line-1]
	target := diagnosticTarget(message)
	if target == "" {
		return column, endColumn
	}
	if index := strings.Index(source, target); index != -1 {
		column = index + 1
		endColumn = column + len([]rune(target)) - 1
	}
	return column, endColumn
}

func diagnosticTarget(message string) string {
	patterns := []string{
		`unknown identifier "([^"]+)"`,
		`unknown function "([^"]+)"`,
		`unknown type ([A-Za-z_][A-Za-z0-9_:\.\[\],]*)`,
		`cannot assign [^ ]+ to (?:local |global )?[^ ]+ ([A-Za-z_][A-Za-z0-9_]*)`,
	}
	for _, pattern := range patterns {
		if value, ok := quotedPayload(message, pattern); ok {
			return value
		}
		if value, ok := barePayload(message, pattern); ok {
			return value
		}
	}
	return ""
}

func expectedFoundTypes(message string) (string, string, bool) {
	patterns := []string{
		`cannot assign ([^ ]+) to (?:local |global )?([^ ]+) [^ ]+`,
		`cannot assign ([^ ]+) to ([^ ]+)$`,
		`expects ([^,]+), got ([^ ]+)`,
		`returns ([^ ]+) but return expression is ([^ ]+)`,
	}
	for _, pattern := range patterns {
		matches := regexp.MustCompile(pattern).FindStringSubmatch(message)
		if len(matches) == 3 {
			if strings.HasPrefix(pattern, "cannot assign") {
				return strings.TrimSpace(matches[2]), strings.TrimSpace(matches[1]), true
			}
			return strings.TrimSpace(matches[1]), strings.TrimSpace(matches[2]), true
		}
	}
	return "", "", false
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

func diagnosticCandidates(program file.Program) map[string]bool {
	candidates := map[string]bool{
		"Int": true, "UInt": true, "String": true, "Float": true, "Bool": true, "Char": true,
		"List": true, "Map": true, "Set": true, "Option": true, "Result": true, "Table": true,
		"Any": true, "Type": true, "Context": true, "ErrorContext": true, "Function": true,
		"Some": true, "None": true, "Ok": true, "Err": true, "len": true, "print": true,
	}
	identifier := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	for _, source := range program.Files {
		for _, line := range source.Lines {
			for _, match := range identifier.FindAllString(line, -1) {
				if !diagnosticKeyword(match) {
					candidates[match] = true
				}
			}
		}
	}
	return candidates
}

func diagnosticKeyword(value string) bool {
	switch value {
	case "function", "return", "local", "global", "mut", "let", "var", "val", "const", "if", "case", "else", "for", "while", "true", "false", "True", "False", "import", "namespace", "enum", "trait", "impl":
		return true
	default:
		return false
	}
}

func closestCandidate(target string, candidates map[string]bool) (string, bool) {
	targetLower := strings.ToLower(target)
	best := ""
	bestDistance := 99
	for candidate := range candidates {
		if candidate == target {
			continue
		}
		distance := levenshtein(targetLower, strings.ToLower(candidate))
		if betterDiagnosticCandidate(target, candidate, best, distance, bestDistance) {
			best = candidate
			bestDistance = distance
		}
	}
	limit := 2
	if len([]rune(target)) > 8 {
		limit = 3
	}
	if best == "" || bestDistance > limit {
		return "", false
	}
	return best, true
}

func betterDiagnosticCandidate(target string, candidate string, best string, distance int, bestDistance int) bool {
	if distance < bestDistance {
		return true
	}
	if distance != bestDistance {
		return false
	}
	candidateSamePrefix := sameFirstFold(target, candidate)
	bestSamePrefix := sameFirstFold(target, best)
	if candidateSamePrefix != bestSamePrefix {
		return candidateSamePrefix
	}
	return best == "" || candidate < best
}

func sameFirstFold(left string, right string) bool {
	leftRunes := []rune(strings.ToLower(left))
	rightRunes := []rune(strings.ToLower(right))
	return len(leftRunes) > 0 && len(rightRunes) > 0 && leftRunes[0] == rightRunes[0]
}

func levenshtein(left string, right string) int {
	leftRunes := []rune(left)
	rightRunes := []rune(right)
	previous := make([]int, len(rightRunes)+1)
	for index := range previous {
		previous[index] = index
	}
	for i, leftRune := range leftRunes {
		current := make([]int, len(rightRunes)+1)
		current[0] = i + 1
		for j, rightRune := range rightRunes {
			cost := 1
			if leftRune == rightRune {
				cost = 0
			}
			current[j+1] = minInt(current[j]+1, previous[j+1]+1, previous[j]+cost)
		}
		previous = current
	}
	return previous[len(rightRunes)]
}

func minInt(values ...int) int {
	minimum := values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
	}
	return minimum
}

func quotedPayload(message string, pattern string) (string, bool) {
	return regexpPayload(message, pattern)
}

func barePayload(message string, pattern string) (string, bool) {
	return regexpPayload(message, pattern)
}

func regexpPayload(message string, pattern string) (string, bool) {
	matches := regexp.MustCompile(pattern).FindStringSubmatch(message)
	if len(matches) != 2 {
		return "", false
	}
	return matches[1], true
}
