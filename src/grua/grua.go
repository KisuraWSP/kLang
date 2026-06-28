package grua

import (
	"fmt"
	"sort"
	"strings"

	"kLang/src/lexer"
)

const Extension = ".grua"

var allowedModules = map[string]bool{
	"basic": true,
	"file":  true,
	"io":    true,
	"repl":  true,
}

func AllowsModule(name string) bool {
	return allowedModules[name]
}

type Diagnostic struct {
	Line    int
	Column  int
	Message string
}

type edit struct {
	line        int
	startColumn int
	length      int
	replacement string
}

func Transpile(source string) (string, []Diagnostic) {
	tokens := lexer.New(source).Tokenize()
	diagnostics := validate(tokens)
	if len(diagnostics) != 0 {
		return "", diagnostics
	}

	edits := collectEdits(tokens)
	lines := strings.Split(source, "\n")
	for lineNumber, lineEdits := range editsByLine(edits) {
		if lineNumber <= 0 || lineNumber > len(lines) {
			continue
		}
		line := lines[lineNumber-1]
		for _, current := range lineEdits {
			start := current.startColumn - 1
			if start < 0 || start > len(line) || start+current.length > len(line) {
				continue
			}
			line = line[:start] + current.replacement + line[start+current.length:]
		}
		lines[lineNumber-1] = line
	}

	return terminateStatements(strings.Join(lines, "\n")), nil
}

func FormatDiagnostics(path string, diagnostics []Diagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	parts := make([]string, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		parts = append(parts, fmt.Sprintf(
			"%s:%d:%d: %s",
			path,
			diagnostic.Line,
			diagnostic.Column,
			diagnostic.Message,
		))
	}
	return fmt.Errorf("Grua subset validation failed:\n  %s", strings.Join(parts, "\n  "))
}

func validate(tokens []lexer.Token) []Diagnostic {
	var diagnostics []Diagnostic
	for index, token := range tokens {
		if token.Type == lexer.TokenEOFDescriptor {
			break
		}
		if token.Type == lexer.TokenIllegal {
			diagnostics = append(diagnostics, diagnostic(token, fmt.Sprintf("illegal token %q", token.Literal)))
			continue
		}
		if token.Type == lexer.TokenString && strings.Contains(token.Literal, "\n") {
			diagnostics = append(diagnostics, diagnostic(token, "multiline strings are not part of the Grua subset"))
		}
		if message := forbiddenFeature(token); message != "" {
			diagnostics = append(diagnostics, diagnostic(token, message))
		}
		switch token.Type {
		case lexer.TokenImport:
			diagnostics = append(diagnostics, validateImport(tokens, index)...)
		case lexer.TokenLocal:
			diagnostics = append(diagnostics, validateLocal(tokens, index)...)
		case lexer.TokenVar, lexer.TokenVal:
			diagnostics = append(diagnostics, validateGlobalInference(tokens, index)...)
		case lexer.TokenFunc:
			diagnostics = append(diagnostics, validateFunction(tokens, index)...)
		case lexer.TokenLeftSquareBrace:
			if startsListLiteral(tokens, index) {
				diagnostics = append(diagnostics, diagnostic(token, "Grua aggregate values use Table; List literals are unavailable"))
			}
		}
		if token.Type == lexer.TokenIdentifier {
			if token.Literal == "switch" && nextTokenOfTypeOnLine(tokens, index+1, lexer.TokenScopeBegin, token.Line) == -1 {
				diagnostics = append(diagnostics, diagnostic(token, "Grua switch header must end with '{' on the same line"))
			}
			if isDisallowedAggregateConstructor(token.Literal) && index+1 < len(tokens) &&
				(tokens[index+1].Type == lexer.TokenLeftBrace || tokens[index+1].Type == lexer.TokenLeftSquareBrace) {
				diagnostics = append(diagnostics, diagnostic(token, "Grua aggregate values use Table instead of "+token.Literal))
			}
		}
	}
	return deduplicateDiagnostics(diagnostics)
}

func forbiddenFeature(token lexer.Token) string {
	switch token.Type {
	case lexer.TokenIf, lexer.TokenUnless:
		return "Grua conditions use switch and pattern-matching cases"
	case lexer.TokenWhile, lexer.TokenForEach, lexer.TokenDo, lexer.TokenDoWhile:
		return "Grua has one loop keyword: for"
	case lexer.TokenGlobal, lexer.TokenLet, lexer.TokenConst:
		return "Grua variables use local, local mut, var, or val with inferred types"
	case lexer.TokenAlias, lexer.TokenTrait, lexer.TokenImpl, lexer.TokenEnum, lexer.TokenStruct:
		return "Grua data aggregation uses the builtin Table type"
	case lexer.TokenRegion, lexer.TokenLazy, lexer.TokenTemp, lexer.TokenAsync, lexer.TokenAwait:
		return "this advanced kLang feature is not part of the Grua subset"
	case lexer.TokenInline, lexer.TokenPrivate, lexer.TokenDefer, lexer.TokenRun, lexer.TokenScope:
		return "this kLang declaration/control feature is not part of the Grua subset"
	case lexer.TokenNameSpace, lexer.TokenFuncGroup, lexer.TokenExport, lexer.TokenModule:
		return "Grua files do not declare kLang modules or namespaces"
	case lexer.TokenHash, lexer.TokenAt:
		return "kLang directives and declaration tags are not part of the Grua subset"
	case lexer.TokenTry, lexer.TokenCatch:
		return "Grua error flow uses Result values or throw, not try/catch"
	default:
		return ""
	}
}

func isDisallowedAggregateConstructor(name string) bool {
	switch name {
	case "List", "Map", "Set", "SIMD":
		return true
	default:
		return false
	}
}

func validateImport(tokens []lexer.Token, index int) []Diagnostic {
	if index+1 >= len(tokens) || tokens[index+1].Type != lexer.TokenString {
		return []Diagnostic{diagnostic(tokens[index], "Grua import expects a quoted module name")}
	}
	module := tokens[index+1]
	if allowedModules[module.Literal] ||
		strings.HasSuffix(module.Literal, Extension) ||
		strings.HasPrefix(module.Literal, "./") ||
		strings.HasPrefix(module.Literal, "../") {
		return nil
	}
	return []Diagnostic{diagnostic(
		module,
		fmt.Sprintf("Grua only exposes the basic, file, io, and repl modules; %q is unavailable", module.Literal),
	)}
}

func validateLocal(tokens []lexer.Token, index int) []Diagnostic {
	cursor := index + 1
	if cursor < len(tokens) && tokens[cursor].Type == lexer.TokenMut {
		cursor++
	}
	if cursor >= len(tokens) || tokens[cursor].Type != lexer.TokenIdentifier {
		return []Diagnostic{diagnostic(tokens[index], "local expects an inferred variable name")}
	}
	if cursor+1 >= len(tokens) || tokens[cursor+1].Type != lexer.TokenAssign {
		return []Diagnostic{diagnostic(tokens[cursor], "Grua local variables require inferred syntax: local [mut] name = value")}
	}
	return nil
}

func validateGlobalInference(tokens []lexer.Token, index int) []Diagnostic {
	if index+2 >= len(tokens) || tokens[index+1].Type != lexer.TokenIdentifier || tokens[index+2].Type != lexer.TokenAssign {
		return []Diagnostic{diagnostic(tokens[index], "Grua globals require inferred syntax: var name = value or val name = value")}
	}
	return nil
}

func validateFunction(tokens []lexer.Token, index int) []Diagnostic {
	open := nextTokenOfType(tokens, index+1, lexer.TokenLeftBrace)
	if open == -1 {
		return []Diagnostic{diagnostic(tokens[index], "function declaration requires a parameter list")}
	}
	if open > index+2 && tokens[index+2].Type == lexer.TokenLeftSquareBrace {
		return []Diagnostic{diagnostic(tokens[index+2], "Grua functions do not support generic type parameters")}
	}
	close := matchingParen(tokens, open)
	if close == -1 {
		return []Diagnostic{diagnostic(tokens[open], "unterminated function parameter list")}
	}

	var diagnostics []Diagnostic
	for _, parameter := range splitParameters(tokens[open+1 : close]) {
		if len(parameter) == 0 {
			continue
		}
		if parameter[0].Type != lexer.TokenIdentifier {
			diagnostics = append(diagnostics, diagnostic(parameter[0], "Grua parameters begin with a variable name"))
			continue
		}
		for _, token := range parameter[1:] {
			if token.Type == lexer.TokenInferReturn {
				diagnostics = append(diagnostics, diagnostic(token, "Grua parameter hints use name::Type, not kLang static name:Type syntax"))
			}
			if token.Type == lexer.TokenLeftSquareBrace || token.Type == lexer.TokenRightSquareBrace {
				diagnostics = append(diagnostics, diagnostic(token, "Grua type hints cannot use generic container types"))
			}
		}
	}
	return diagnostics
}

func collectEdits(tokens []lexer.Token) []edit {
	var edits []edit
	for index, token := range tokens {
		switch {
		case token.Type == lexer.TokenFunc:
			edits = append(edits, functionHintEdits(tokens, index)...)
		case token.Type == lexer.TokenIdentifier && token.Literal == "switch":
			if scope := nextTokenOfTypeOnLine(tokens, index+1, lexer.TokenScopeBegin, token.Line); scope != -1 {
				edits = append(edits,
					edit{line: token.Line, startColumn: token.Column, length: len(token.Literal), replacement: "if"},
					edit{line: tokens[scope].Line, startColumn: tokens[scope].Column, replacement: "== "},
				)
			}
		case token.Type == lexer.TokenFor:
			if forHeaderUsesIn(tokens, index) {
				edits = append(edits, edit{
					line: token.Line, startColumn: token.Column, length: len(token.Literal), replacement: "for_each",
				})
			} else {
				edits = append(edits, cStyleForEdits(tokens, index)...)
			}
		}
	}
	return edits
}

func cStyleForEdits(tokens []lexer.Token, index int) []edit {
	scope := nextTokenOfType(tokens, index+1, lexer.TokenScopeBegin)
	if scope == -1 {
		return nil
	}
	header := tokens[index+1 : scope]
	commaCount := 0
	hasInitializer := false
	for _, token := range header {
		if token.Type == lexer.TokenComma {
			commaCount++
		}
		if token.Type == lexer.TokenEvaluationAssign {
			hasInitializer = true
		}
	}
	if commaCount != 2 || !hasInitializer {
		return nil
	}
	edits := make([]edit, 0, 2)
	for _, token := range header {
		if token.Type == lexer.TokenComma {
			edits = append(edits, edit{
				line: token.Line, startColumn: token.Column, length: len(token.Literal), replacement: ";",
			})
		}
	}
	return edits
}

func functionHintEdits(tokens []lexer.Token, index int) []edit {
	open := nextTokenOfType(tokens, index+1, lexer.TokenLeftBrace)
	if open == -1 {
		return nil
	}
	close := matchingParen(tokens, open)
	if close == -1 {
		return nil
	}

	var edits []edit
	offset := open + 1
	for _, parameter := range splitParameters(tokens[offset:close]) {
		if len(parameter) == 0 {
			continue
		}
		hasHint := false
		for _, token := range parameter[1:] {
			if token.Type == lexer.TokenNamespaceAccess {
				hasHint = true
				edits = append(edits, edit{
					line: token.Line, startColumn: token.Column, length: len(token.Literal), replacement: ":",
				})
			}
		}
		if !hasHint {
			name := parameter[0]
			edits = append(edits, edit{
				line: name.Line, startColumn: name.Column + len(name.Literal), replacement: " : Any",
			})
		}
	}
	return edits
}

func forHeaderUsesIn(tokens []lexer.Token, index int) bool {
	scope := nextTokenOfType(tokens, index+1, lexer.TokenScopeBegin)
	if scope == -1 || scope <= index+2 {
		return false
	}
	header := tokens[index+1 : scope]
	return len(header) >= 3 && header[0].Type == lexer.TokenIdentifier && header[1].Type == lexer.TokenIn
}

func startsListLiteral(tokens []lexer.Token, index int) bool {
	if index == 0 {
		return true
	}
	switch tokens[index-1].Type {
	case lexer.TokenAssign, lexer.TokenEvaluationAssign, lexer.TokenReturn, lexer.TokenComma,
		lexer.TokenLeftBrace, lexer.TokenScopeBegin, lexer.TokenInferReturn:
		return true
	default:
		return false
	}
}

func nextTokenOfType(tokens []lexer.Token, start int, tokenType lexer.TokenType) int {
	for index := start; index < len(tokens); index++ {
		if tokens[index].Type == tokenType {
			return index
		}
		if tokens[index].Type == lexer.TokenEOFDescriptor {
			return -1
		}
	}
	return -1
}

func nextTokenOfTypeOnLine(tokens []lexer.Token, start int, tokenType lexer.TokenType, line int) int {
	for index := start; index < len(tokens) && tokens[index].Line == line; index++ {
		if tokens[index].Type == tokenType {
			return index
		}
	}
	return -1
}

func matchingParen(tokens []lexer.Token, open int) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Type {
		case lexer.TokenLeftBrace:
			depth++
		case lexer.TokenRightBrace:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func splitParameters(tokens []lexer.Token) [][]lexer.Token {
	if len(tokens) == 0 {
		return nil
	}
	var parameters [][]lexer.Token
	start := 0
	depth := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace, lexer.TokenScopeBegin:
			depth++
		case lexer.TokenRightBrace, lexer.TokenRightSquareBrace, lexer.TokenScopeEnd:
			depth--
		case lexer.TokenComma:
			if depth == 0 {
				parameters = append(parameters, tokens[start:index])
				start = index + 1
			}
		}
	}
	parameters = append(parameters, tokens[start:])
	return parameters
}

func editsByLine(edits []edit) map[int][]edit {
	result := map[int][]edit{}
	for _, current := range edits {
		result[current.line] = append(result[current.line], current)
	}
	for line := range result {
		sort.Slice(result[line], func(left int, right int) bool {
			return result[line][left].startColumn > result[line][right].startColumn
		})
	}
	return result
}

func terminateStatements(source string) string {
	lines := strings.Split(source, "\n")
	inBlockComment := false
	tableDepth := 0
	for index, line := range lines {
		codeEnd, hasCode := gruaCodeEnd(line, &inBlockComment)
		if !hasCode {
			continue
		}
		code := strings.TrimSpace(line[:codeEnd])
		tableDepthBefore := tableDepth
		if tableDepth > 0 {
			tableDepth += braceDelta(code)
		} else if startsMultilineTable(code) {
			tableDepth = braceDelta(code)
		}
		closesTableExpression := strings.HasSuffix(code, "}") &&
			(strings.Contains(code, "=") ||
				strings.HasPrefix(code, "return {") ||
				tableDepthBefore > 0 && tableDepth == 0)
		if tableDepthBefore > 0 && tableDepth > 0 {
			continue
		}
		if code == "" || strings.HasSuffix(code, ";") || strings.HasSuffix(code, "{") ||
			(strings.HasSuffix(code, "}") && !closesTableExpression) ||
			strings.HasSuffix(code, ":") || strings.HasSuffix(code, ",") {
			continue
		}
		lines[index] = line[:codeEnd] + ";" + line[codeEnd:]
	}
	return strings.Join(lines, "\n")
}

func startsMultilineTable(code string) bool {
	open := strings.LastIndex(code, "{")
	if open == -1 || !strings.HasSuffix(code, "{") {
		return false
	}
	prefix := strings.TrimSpace(code[:open])
	if strings.HasSuffix(prefix, "return") {
		return true
	}
	if !strings.HasSuffix(prefix, "=") {
		return false
	}
	if len(prefix) < 2 {
		return true
	}
	previous := prefix[len(prefix)-2]
	return previous != '=' && previous != '!' && previous != '<' && previous != '>'
}

func braceDelta(code string) int {
	delta := 0
	quote := byte(0)
	escaped := false
	for index := 0; index < len(code); index++ {
		character := code[index]
		if quote != 0 {
			if escaped {
				escaped = false
			} else if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if character == '"' || character == '\'' {
			quote = character
			continue
		}
		if character == '{' {
			delta++
		} else if character == '}' {
			delta--
		}
	}
	return delta
}

func gruaCodeEnd(line string, inBlockComment *bool) (int, bool) {
	inString := byte(0)
	escaped := false
	hasCode := false
	lastCode := 0
	for index := 0; index < len(line); index++ {
		if *inBlockComment {
			if index+1 < len(line) && line[index] == '*' && line[index+1] == ')' {
				*inBlockComment = false
				index++
			}
			continue
		}
		if inString != 0 {
			hasCode = true
			lastCode = index + 1
			if escaped {
				escaped = false
			} else if line[index] == '\\' {
				escaped = true
			} else if line[index] == inString {
				inString = 0
			}
			continue
		}
		if index+1 < len(line) && line[index] == '-' && line[index+1] == '-' {
			break
		}
		if index+1 < len(line) && line[index] == '(' && line[index+1] == '*' {
			*inBlockComment = true
			index++
			continue
		}
		if line[index] == '"' || line[index] == '\'' {
			inString = line[index]
		}
		if line[index] != ' ' && line[index] != '\t' && line[index] != '\r' {
			hasCode = true
			lastCode = index + 1
		}
	}
	return lastCode, hasCode
}

func diagnostic(token lexer.Token, message string) Diagnostic {
	return Diagnostic{Line: token.Line, Column: token.Column, Message: message}
}

func deduplicateDiagnostics(diagnostics []Diagnostic) []Diagnostic {
	seen := map[string]bool{}
	result := make([]Diagnostic, 0, len(diagnostics))
	for _, current := range diagnostics {
		key := fmt.Sprintf("%d:%d:%s", current.Line, current.Column, current.Message)
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, current)
	}
	return result
}
