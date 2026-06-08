package typechecker

import (
	"fmt"
	"strings"

	"kLang/src/engine/file"
	"kLang/src/lexer"
	"kLang/src/parser"
)

func (checker *TypeChecker) collectASTGlobals(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectASTGlobalsFromStatements(source.Program.Statements, source.Path, true)
	}
}

func (checker *TypeChecker) collectASTGlobalsFromStatements(statements []parser.Statement, source string, topLevel bool) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.VariableStatement:
			if current.Scope != "global" && !current.Exported {
				continue
			}
			if existing, exists := checker.globals[current.Name]; exists {
				if topLevel && existing.File == source {
					continue
				}
				checker.addError(source, current.Pos.Line, fmt.Sprintf("global variable %q is already defined", current.Name))
				continue
			}
			checker.globals[current.Name] = variableSymbol{
				Name:    current.Name,
				Type:    normalizeType(current.Type),
				Mutable: current.Mutable,
				File:    source,
				Line:    current.Pos.Line,
			}
		case parser.NamespaceStatement:
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		case parser.FunctionStatement:
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		case parser.IfStatement:
			checker.collectASTGlobalsFromStatements(current.Consequence, source, false)
			checker.collectASTGlobalsFromStatements(current.Alternative, source, false)
			if current.ElseIf != nil {
				checker.collectASTGlobalsFromStatements([]parser.Statement{*current.ElseIf}, source, false)
			}
		case parser.LoopStatement:
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		}
	}
}

type lexicalScope struct {
	parent    *lexicalScope
	variables map[string]variableSymbol
}

func newLexicalScope(parent *lexicalScope) *lexicalScope {
	return &lexicalScope{
		parent:    parent,
		variables: map[string]variableSymbol{},
	}
}

func (scope *lexicalScope) define(variable variableSymbol) bool {
	if _, exists := scope.variables[variable.Name]; exists {
		return false
	}
	scope.variables[variable.Name] = variable
	return true
}

func (scope *lexicalScope) lookup(name string) (variableSymbol, bool) {
	if variable, ok := scope.variables[name]; ok {
		return variable, true
	}
	if scope.parent != nil {
		return scope.parent.lookup(name)
	}
	return variableSymbol{}, false
}

func (checker *TypeChecker) checkLexicalScopes(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	for _, parseError := range parsed.Errors() {
		checker.addError(program.EntryPoint, parseError.Line, parseError.Message)
	}
	if !parsed.Passed() {
		return
	}

	globalScope := newLexicalScope(nil)
	for _, global := range checker.globals {
		globalScope.define(global)
	}

	for _, source := range parsed.Sources {
		checker.checkScopeStatements(source.Program.Statements, globalScope, "", source.Path, false, true)
	}
}

func (checker *TypeChecker) checkScopeStatements(statements []parser.Statement, scope *lexicalScope, namespace string, source string, inLoop bool, topLevel bool) {
	checker.predeclareLocalFunctions(statements, scope, namespace, source)

	for _, stmt := range statements {
		checker.checkScopeStatement(stmt, scope, namespace, source, inLoop, topLevel)
	}
}

func (checker *TypeChecker) predeclareLocalFunctions(statements []parser.Statement, scope *lexicalScope, namespace string, source string) {
	for _, stmt := range statements {
		fn, ok := stmt.(parser.FunctionStatement)
		if !ok {
			continue
		}
		if _, globalFunction := checker.functions[namespace+fn.Name]; globalFunction {
			continue
		}
		if !scope.define(variableSymbol{Name: fn.Name, Type: anyType, File: source, Line: fn.Pos.Line}) {
			checker.addError(source, fn.Pos.Line, fmt.Sprintf("variable %q is already defined in this scope", fn.Name))
		}
	}
}

func (checker *TypeChecker) checkScopeStatement(stmt parser.Statement, scope *lexicalScope, namespace string, source string, inLoop bool, topLevel bool) {
	switch current := stmt.(type) {
	case parser.ImportStatement:
		return
	case parser.NamespaceStatement:
		checker.checkScopeStatements(current.Body, scope, namespace+current.Name+".", source, inLoop, topLevel)
	case parser.FunctionStatement:
		checker.checkFunctionScope(current, scope, namespace, source)
	case parser.VariableStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
		if current.Scope == "global" || current.Exported {
			return
		}
		if !scope.define(variableSymbol{Name: current.Name, Type: normalizeType(current.Type), Mutable: current.Mutable, File: source, Line: current.Pos.Line}) {
			checker.addError(source, current.Pos.Line, fmt.Sprintf("variable %q is already defined in this scope", current.Name))
		}
	case parser.ReturnStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.BreakStatement:
		if !inLoop {
			checker.addError(source, current.Pos.Line, "break is only allowed inside a loop")
		}
	case parser.ExpressionStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.AssignmentStatement:
		checker.checkAssignmentTargetScope(current.Target.Node, scope, namespace, source, current.Pos.Line)
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.IfStatement:
		checker.checkScopeExpression(current.Condition.Node, scope, namespace, source, current.Pos.Line)
		checker.checkScopeStatements(current.Consequence, newLexicalScope(scope), namespace, source, inLoop, false)
		if current.ElseIf != nil {
			checker.checkScopeStatement(*current.ElseIf, newLexicalScope(scope), namespace, source, inLoop, false)
		}
		checker.checkScopeStatements(current.Alternative, newLexicalScope(scope), namespace, source, inLoop, false)
	case parser.LoopStatement:
		checker.checkLoopScope(current, scope, namespace, source)
	}
}

func (checker *TypeChecker) checkFunctionScope(fn parser.FunctionStatement, parent *lexicalScope, namespace string, source string) {
	functionScope := newLexicalScope(parent)
	for _, param := range fn.Params {
		if !functionScope.define(variableSymbol{Name: param.Name, Type: normalizeType(param.Type), File: source, Line: fn.Pos.Line}) {
			checker.addError(source, fn.Pos.Line, fmt.Sprintf("parameter %q is already defined", param.Name))
		}
	}
	checker.checkScopeStatements(fn.Body, functionScope, namespace, source, false, false)
}

func (checker *TypeChecker) checkLoopScope(stmt parser.LoopStatement, parent *lexicalScope, namespace string, source string) {
	loopScope := newLexicalScope(parent)
	header := stmt.Header

	if init, condition, post, ok := parseCStyleScopeHeader(header); ok {
		checker.checkLoopHeaderPart(init, loopScope, parent, namespace, source, stmt.Pos.Line)
		checker.checkScopeExpression(condition.Node, loopScope, namespace, source, stmt.Pos.Line)
		checker.checkLoopHeaderPart(post, loopScope, loopScope, namespace, source, stmt.Pos.Line)
		checker.checkScopeStatements(stmt.Body, newLexicalScope(loopScope), namespace, source, true, false)
		return
	}

	if iterator, iterable, ok := parseRangeScopeHeader(header); ok {
		checker.checkScopeExpression(iterable.Node, parent, namespace, source, stmt.Pos.Line)
		loopScope.define(variableSymbol{Name: iterator, Type: "Int", File: source, Line: stmt.Pos.Line})
		checker.checkScopeStatements(stmt.Body, newLexicalScope(loopScope), namespace, source, true, false)
		return
	}

	if name, expr, ok := parseEvaluationScopeHeader(header); ok {
		checker.checkScopeExpression(expr.Node, parent, namespace, source, stmt.Pos.Line)
		loopScope.define(variableSymbol{Name: name, Type: anyType, Mutable: true, File: source, Line: stmt.Pos.Line})
	} else {
		checker.checkScopeExpression(header.Node, parent, namespace, source, stmt.Pos.Line)
	}
	checker.checkScopeStatements(stmt.Body, newLexicalScope(loopScope), namespace, source, true, false)
}

func (checker *TypeChecker) checkLoopHeaderPart(expr parser.Expression, loopScope *lexicalScope, expressionScope *lexicalScope, namespace string, source string, line int) {
	if len(expr.Tokens) == 0 {
		return
	}
	if name, value, ok := parseEvaluationScopeHeader(expr); ok {
		checker.checkScopeExpression(value.Node, expressionScope, namespace, source, line)
		if !loopScope.define(variableSymbol{Name: name, Type: "Int", Mutable: true, File: source, Line: line}) {
			checker.addError(source, line, fmt.Sprintf("variable %q is already defined in this scope", name))
		}
		return
	}
	if assignmentIndex := assignmentOperatorIndex(expr.Tokens); assignmentIndex != -1 {
		target := parser.ParseExpressionTokens(expr.Tokens[:assignmentIndex])
		value := parser.ParseExpressionTokens(expr.Tokens[assignmentIndex+1:])
		checker.checkAssignmentTargetScope(target, loopScope, namespace, source, line)
		checker.checkScopeExpression(value, loopScope, namespace, source, line)
		return
	}
	checker.checkScopeExpression(expr.Node, expressionScope, namespace, source, line)
}

func (checker *TypeChecker) checkAssignmentTargetScope(target parser.ExpressionNode, scope *lexicalScope, namespace string, source string, line int) {
	switch current := target.(type) {
	case parser.IdentifierExpression:
		if _, ok := scope.lookup(current.Name); !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", current.Name))
		}
	case parser.IndexExpression:
		checker.checkScopeExpression(current.Target, scope, namespace, source, line)
		checker.checkScopeExpression(current.Index, scope, namespace, source, line)
	default:
		checker.checkScopeExpression(target, scope, namespace, source, line)
	}
}

func (checker *TypeChecker) checkScopeExpression(expr parser.ExpressionNode, scope *lexicalScope, namespace string, source string, line int) {
	switch current := expr.(type) {
	case nil:
		return
	case parser.IdentifierExpression:
		if _, ok := scope.lookup(current.Name); ok {
			return
		}
		if isBuiltinFunctionName(current.Name) || checker.functionExists(current.Name, namespace) || checker.namespaceExists(current.Name) {
			return
		}
		checker.addError(source, line, fmt.Sprintf("unknown identifier %q", current.Name))
	case parser.LiteralExpression:
		return
	case parser.UnaryExpression:
		checker.checkScopeExpression(current.Right, scope, namespace, source, line)
	case parser.BinaryExpression:
		checker.checkScopeExpression(current.Left, scope, namespace, source, line)
		checker.checkScopeExpression(current.Right, scope, namespace, source, line)
	case parser.CallExpression:
		checker.checkCallScope(current.Callee, scope, namespace, source, line)
		for _, arg := range current.Arguments {
			checker.checkScopeExpression(arg, scope, namespace, source, line)
		}
	case parser.IndexExpression:
		checker.checkScopeExpression(current.Target, scope, namespace, source, line)
		checker.checkScopeExpression(current.Index, scope, namespace, source, line)
	case parser.SelectorExpression:
		if checker.selectorFunctionExists(current) {
			return
		}
		checker.checkScopeExpression(current.Target, scope, namespace, source, line)
	case parser.ListExpression:
		for _, item := range current.Items {
			checker.checkScopeExpression(item, scope, namespace, source, line)
		}
	case parser.MapExpression:
		for _, entry := range current.Entries {
			checker.checkScopeExpression(entry.Key, scope, namespace, source, line)
			checker.checkScopeExpression(entry.Value, scope, namespace, source, line)
		}
	case parser.GroupExpression:
		checker.checkScopeExpression(current.Inner, scope, namespace, source, line)
	case parser.RawExpression:
		return
	}
}

func (checker *TypeChecker) checkCallScope(callee parser.ExpressionNode, scope *lexicalScope, namespace string, source string, line int) {
	switch current := callee.(type) {
	case parser.IdentifierExpression:
		if _, ok := scope.lookup(current.Name); ok {
			return
		}
		if isBuiltinFunctionName(current.Name) || checker.functionExists(current.Name, namespace) {
			return
		}
		checker.addError(source, line, fmt.Sprintf("unknown function %q", current.Name))
	case parser.SelectorExpression:
		if checker.selectorFunctionExists(current) {
			return
		}
		checker.checkScopeExpression(current, scope, namespace, source, line)
	default:
		checker.checkScopeExpression(callee, scope, namespace, source, line)
	}
}

func (checker *TypeChecker) functionExists(name string, namespace string) bool {
	if _, ok := checker.functions[name]; ok {
		return true
	}
	if namespace != "" {
		if _, ok := checker.functions[namespace+name]; ok {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) namespaceExists(name string) bool {
	prefix := name + "."
	for functionName := range checker.functions {
		if strings.HasPrefix(functionName, prefix) {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) selectorFunctionExists(expr parser.SelectorExpression) bool {
	target, ok := expr.Target.(parser.IdentifierExpression)
	if !ok {
		return false
	}
	_, ok = checker.functions[target.Name+"."+expr.Field]
	return ok
}

func isBuiltinFunctionName(name string) bool {
	return name == "print" || name == "len" || name == "range"
}

func parseRangeScopeHeader(expr parser.Expression) (string, parser.Expression, bool) {
	tokens := expr.Tokens
	if len(tokens) < 5 || tokens[0].Type != lexer.TokenIdentifier || tokens[1].Type != lexer.TokenEvaluationAssign {
		return "", parser.Expression{}, false
	}
	if tokens[2].Literal != "range" {
		return "", parser.Expression{}, false
	}
	valueTokens := tokens[2:]
	return tokens[0].Literal, parser.Expression{Tokens: valueTokens, Node: parser.ParseExpressionTokens(valueTokens)}, true
}

func parseEvaluationScopeHeader(expr parser.Expression) (string, parser.Expression, bool) {
	tokens := expr.Tokens
	for index, token := range tokens {
		if token.Type != lexer.TokenEvaluationAssign || index == 0 || index+1 >= len(tokens) {
			continue
		}
		if tokens[index-1].Type != lexer.TokenIdentifier {
			return "", parser.Expression{}, false
		}
		valueTokens := tokens[index+1:]
		return tokens[index-1].Literal, parser.Expression{Tokens: valueTokens, Node: parser.ParseExpressionTokens(valueTokens)}, true
	}
	return "", parser.Expression{}, false
}

func parseCStyleScopeHeader(expr parser.Expression) (parser.Expression, parser.Expression, parser.Expression, bool) {
	parts := splitTopLevelScopeTokens(expr.Tokens, lexer.TokenSemicolon)
	if len(parts) != 3 {
		return parser.Expression{}, parser.Expression{}, parser.Expression{}, false
	}
	return expressionFromScopeTokens(parts[0]), expressionFromScopeTokens(parts[1]), expressionFromScopeTokens(parts[2]), true
}

func splitTopLevelScopeTokens(tokens []lexer.Token, separator lexer.TokenType) [][]lexer.Token {
	var parts [][]lexer.Token
	start := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftBrace:
			parenDepth++
		case lexer.TokenRightBrace:
			if parenDepth > 0 {
				parenDepth--
			}
		case lexer.TokenLeftSquareBrace:
			bracketDepth++
		case lexer.TokenRightSquareBrace:
			if bracketDepth > 0 {
				bracketDepth--
			}
		case lexer.TokenScopeBegin:
			braceDepth++
		case lexer.TokenScopeEnd:
			if braceDepth > 0 {
				braceDepth--
			}
		default:
			if token.Type == separator && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				parts = append(parts, trimScopeTokens(tokens[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, trimScopeTokens(tokens[start:]))
	return parts
}

func trimScopeTokens(tokens []lexer.Token) []lexer.Token {
	start := 0
	end := len(tokens)
	for start < end && tokens[start].Type == lexer.TokenSemicolon {
		start++
	}
	for end > start && tokens[end-1].Type == lexer.TokenSemicolon {
		end--
	}
	return tokens[start:end]
}

func expressionFromScopeTokens(tokens []lexer.Token) parser.Expression {
	tokens = trimScopeTokens(tokens)
	return parser.Expression{Tokens: tokens, Node: parser.ParseExpressionTokens(tokens)}
}

func assignmentOperatorIndex(tokens []lexer.Token) int {
	depth := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace:
			depth++
		case lexer.TokenRightBrace, lexer.TokenRightSquareBrace:
			if depth > 0 {
				depth--
			}
		case lexer.TokenAssign, lexer.TokenPlusEqual, lexer.TokenMinusEqual, lexer.TokenMultiEqual, lexer.TokenDivideEqual:
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}
