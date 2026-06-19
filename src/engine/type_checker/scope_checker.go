package typechecker

import (
	"fmt"
	"strings"

	"kLang/src/lexer"
	"kLang/src/parser"
)

func (checker *TypeChecker) collectASTGlobals(parsed parser.ParsedProgram) {
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
			if isDiscardIdentifier(current.Name) {
				continue
			}
			if current.Scope == "const" && !topLevel {
				continue
			}
			if current.Scope != "global" && current.Scope != "const" && !current.Exported {
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
		case parser.PrivateBlockStatement:
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		case parser.RegionStatement:
			continue
		case parser.AliasFunctionStatement:
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		case parser.TraitStatement:
			continue
		case parser.ImplStatement:
			continue
		case parser.EnumStatement:
			continue
		case parser.FunctionGroupStatement:
			continue
		case parser.TryCatchStatement:
			checker.collectASTGlobalsFromStatements(current.TryBody, source, false)
			checker.collectASTGlobalsFromStatements(current.CatchBody, source, false)
		case parser.MatchStatement:
			for _, matchCase := range current.Cases {
				checker.collectASTGlobalsFromStatements(matchCase.Body, source, false)
			}
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
		case parser.DeferStatement:
			if current.Stmt != nil {
				checker.collectASTGlobalsFromStatements([]parser.Statement{current.Stmt}, source, false)
			}
			checker.collectASTGlobalsFromStatements(current.Body, source, false)
		case parser.RunStatement:
			if current.Stmt != nil {
				checker.collectASTGlobalsFromStatements([]parser.Statement{current.Stmt}, source, false)
			}
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

func (checker *TypeChecker) checkLexicalScopes(entryPoint string, parsed parser.ParsedProgram) {
	for _, parseError := range parsed.Errors() {
		checker.addError(entryPoint, parseError.Line, parseError.Message)
	}
	if !parsed.Passed() {
		return
	}

	globalScope := newLexicalScope(nil)
	for _, global := range checker.globals {
		globalScope.define(global)
	}

	for _, source := range parsed.Sources {
		statements := filterModuleFunctions(source.Program.Statements, "", source.ModuleFunctionFilter)
		checker.checkScopeStatements(statements, globalScope, "", source.Path, false, true)
	}
}

func filterModuleFunctions(statements []parser.Statement, namespace string, filter map[string]bool) []parser.Statement {
	if filter == nil {
		return statements
	}
	filtered := make([]parser.Statement, 0, len(statements))
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.FunctionStatement:
			if filter[namespace+current.Name] {
				filtered = append(filtered, current)
			}
		case parser.NamespaceStatement:
			current.Body = filterModuleFunctions(current.Body, namespace+current.Name+".", filter)
			filtered = append(filtered, current)
		case parser.RunStatement:
			if current.Stmt != nil {
				filteredStmt := filterModuleFunctions([]parser.Statement{current.Stmt}, namespace, filter)
				if len(filteredStmt) == 1 {
					current.Stmt = filteredStmt[0]
				} else {
					current.Stmt = nil
				}
			}
			current.Body = filterModuleFunctions(current.Body, namespace, filter)
			filtered = append(filtered, current)
		default:
			filtered = append(filtered, current)
		}
	}
	return filtered
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
	case parser.ModuleDirectiveStatement:
		return
	case parser.EntryPointStatement:
		return
	case parser.AliasStatement:
		return
	case parser.RegionStatement:
		return
	case parser.AliasFunctionStatement:
		checker.checkAliasFunctionScope(current, scope, namespace, source)
	case parser.NamespaceStatement:
		checker.checkScopeStatements(current.Body, scope, namespace+current.Name+".", source, inLoop, topLevel)
	case parser.TraitStatement:
		return
	case parser.ImplStatement:
		return
	case parser.EnumStatement:
		return
	case parser.FunctionGroupStatement:
		return
	case parser.FunctionStatement:
		checker.checkFunctionScope(current, scope, namespace, source)
	case parser.VariableStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
		if isDiscardIdentifier(current.Name) {
			return
		}
		if current.Scope == "const" && topLevel {
			return
		}
		if (current.Scope == "global" || current.Exported) && current.Scope != "const" {
			return
		}
		if !scope.define(variableSymbol{Name: current.Name, Type: normalizeType(current.Type), Mutable: current.Mutable, Temporary: current.Temporary, File: source, Line: current.Pos.Line}) {
			checker.addError(source, current.Pos.Line, fmt.Sprintf("variable %q is already defined in this scope", current.Name))
		}
	case parser.MultiVariableStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
		if current.Scope == "const" && topLevel {
			return
		}
		if (current.Scope == "global" || current.Exported) && current.Scope != "const" {
			return
		}
		for _, binding := range current.Bindings {
			if isDiscardIdentifier(binding.Name) {
				continue
			}
			if !scope.define(variableSymbol{Name: binding.Name, Type: normalizeType(binding.Type), Mutable: current.Mutable, Temporary: current.Temporary, File: source, Line: current.Pos.Line}) {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("variable %q is already defined in this scope", binding.Name))
			}
		}
	case parser.ReturnStatement:
		if len(current.Values) != 0 {
			for _, value := range current.Values {
				checker.checkScopeExpression(value.Node, scope, namespace, source, current.Pos.Line)
			}
		} else {
			checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
		}
	case parser.ThrowStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.AssertStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.ReportStatement:
		checker.checkScopeExpression(current.Expression.Node, scope, namespace, source, current.Pos.Line)
	case parser.BreakStatement:
		if !inLoop {
			checker.addError(source, current.Pos.Line, "break is only allowed inside a loop")
		}
	case parser.ContinueStatement:
		if !inLoop {
			checker.addError(source, current.Pos.Line, "continue is only allowed inside a loop or pattern match case")
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
	case parser.MatchStatement:
		checker.checkMatchScope(current, scope, namespace, source, inLoop)
	case parser.LoopStatement:
		checker.checkLoopScope(current, scope, namespace, source)
	case parser.TryCatchStatement:
		checker.checkScopeStatements(current.TryBody, newLexicalScope(scope), namespace, source, inLoop, false)
		catchScope := newLexicalScope(scope)
		catchScope.define(variableSymbol{Name: current.ErrorName, Type: anyType, File: source, Line: current.Pos.Line})
		checker.checkScopeStatements(current.CatchBody, catchScope, namespace, source, inLoop, false)
	case parser.DeferStatement:
		if current.Stmt != nil {
			checker.checkScopeStatement(current.Stmt, scope, namespace, source, inLoop, topLevel)
		}
		if len(current.Body) != 0 {
			checker.checkScopeStatements(current.Body, newLexicalScope(scope), namespace, source, inLoop, false)
		}
	case parser.RunStatement:
		if current.Stmt != nil {
			checker.checkScopeStatement(current.Stmt, scope, namespace, source, inLoop, topLevel)
		}
		if len(current.Body) != 0 {
			checker.checkScopeStatements(current.Body, newLexicalScope(scope), namespace, source, inLoop, false)
		}
	case parser.PrivateBlockStatement:
		checker.checkScopeStatements(current.Body, newLexicalScope(scope), namespace, source, inLoop, false)
	}
}

func (checker *TypeChecker) checkMatchScope(stmt parser.MatchStatement, scope *lexicalScope, namespace string, source string, inLoop bool) {
	checker.checkScopeExpression(stmt.Value.Node, scope, namespace, source, stmt.Pos.Line)
	locals := scopeVariables(scope)
	valueType := checker.inferMatchExpressionType(stmt.Value, locals, source, stmt.Pos.Line)
	valueType = normalizeType(valueType)
	if !checker.isPatternMatchType(valueType) {
		checker.addError(source, stmt.Pos.Line, fmt.Sprintf("pattern match value must be Bool, String, Int, Float, Enum, Option, Result, List, or Table, got %s", valueType))
	}

	hasDefault := false
	boolCases := map[string]bool{}
	enumCases := map[string]bool{}
	optionCases := map[string]bool{}
	resultCases := map[string]bool{}
	hasWildcard := false
	for _, matchCase := range stmt.Cases {
		caseScope := newLexicalScope(scope)
		if matchCase.Default {
			if hasDefault {
				checker.addError(source, matchCase.Pos.Line, "pattern match can only have one default case")
			}
			hasDefault = true
		} else {
			info := checker.checkPatternScope(matchCase.Pattern.Node, valueType, caseScope, namespace, source, matchCase.Pos.Line)
			if info.Wildcard {
				hasWildcard = true
			}
			switch info.Kind {
			case "bool":
				boolCases[info.Name] = true
			case "enum":
				enumCases[info.Name] = true
			case "option":
				optionCases[info.Name] = true
			case "result":
				resultCases[info.Name] = true
			}
		}
		checker.checkScopeStatements(matchCase.Body, caseScope, namespace, source, true, false)
	}

	if !stmt.Partial && !hasDefault {
		if hasWildcard {
			return
		}
		if valueType == "Bool" && boolCases["True"] && boolCases["False"] {
			return
		}
		if enum, ok := checker.enums[valueType]; ok && len(enumCases) == len(enum.Variants) {
			return
		}
		if _, ok := optionElementType(valueType); ok && optionCases["Some"] && optionCases["None"] {
			return
		}
		if _, _, ok := resultValueTypes(valueType); ok && resultCases["Ok"] && resultCases["Err"] {
			return
		}
		checker.addError(source, stmt.Pos.Line, "pattern match is not exhaustive; add case: or mark it partial")
	}
}

type matchPatternInfo struct {
	Kind     string
	Name     string
	Wildcard bool
}

func (checker *TypeChecker) checkPatternScope(pattern parser.ExpressionNode, valueType string, scope *lexicalScope, namespace string, source string, line int) matchPatternInfo {
	valueType = normalizeType(valueType)
	switch current := pattern.(type) {
	case parser.IdentifierExpression:
		if current.Name == "_" {
			return matchPatternInfo{Wildcard: true}
		}
		if variable, ok := scope.lookup(current.Name); ok {
			patternType := normalizeType(variable.Type)
			if valueType != anyType && patternType != anyType && valueType != patternType {
				checker.addError(source, line, fmt.Sprintf("case pattern type %s does not match %s", patternType, valueType))
			}
			return matchPatternInfo{}
		}
		scope.define(variableSymbol{Name: current.Name, Type: valueType, File: source, Line: line})
		return matchPatternInfo{Wildcard: true}
	case parser.GroupExpression:
		return checker.checkPatternScope(current.Inner, valueType, scope, namespace, source, line)
	case parser.CallExpression:
		return checker.checkConstructorPatternScope(current, valueType, scope, namespace, source, line)
	case parser.ListExpression:
		return checker.checkListPatternScope(current, valueType, scope, namespace, source, line)
	case parser.MapExpression:
		return checker.checkTablePatternScope(current, valueType, scope, namespace, source, line)
	case parser.SelectorExpression:
		checker.checkScopeExpression(pattern, scope, namespace, source, line)
		patternType := anyType
		if enumName, variantName, ok := enumPatternLiteral(pattern); ok && checker.enumVariantExists(enumName, variantName) {
			patternType = enumName
			if valueType != anyType && valueType != patternType {
				checker.addError(source, line, fmt.Sprintf("case pattern type %s does not match %s", patternType, valueType))
			}
			return matchPatternInfo{Kind: "enum", Name: variantName}
		}
		return matchPatternInfo{}
	default:
		expr := parser.Expression{Node: pattern}
		patternType := normalizeType(checker.inferMatchExpressionType(expr, scopeVariables(scope), source, line))
		if !checker.isPatternMatchType(patternType) {
			checker.addError(source, line, fmt.Sprintf("case pattern must be Bool, String, Int, Float, Enum, Option, Result, List, or Table, got %s", patternType))
			return matchPatternInfo{}
		}
		if valueType != anyType && patternType != anyType && valueType != patternType {
			checker.addError(source, line, fmt.Sprintf("case pattern type %s does not match %s", patternType, valueType))
		}
		if valueType == "Bool" && patternType == "Bool" {
			if value, ok := boolPatternLiteral(pattern); ok && value {
				return matchPatternInfo{Kind: "bool", Name: "True"}
			}
			if value, ok := boolPatternLiteral(pattern); ok && !value {
				return matchPatternInfo{Kind: "bool", Name: "False"}
			}
		}
		if checker.enumExists(valueType) && patternType == valueType {
			if _, variant, ok := enumPatternLiteral(pattern); ok {
				return matchPatternInfo{Kind: "enum", Name: variant}
			}
		}
		return matchPatternInfo{}
	}
}

func (checker *TypeChecker) checkConstructorPatternScope(pattern parser.CallExpression, valueType string, scope *lexicalScope, namespace string, source string, line int) matchPatternInfo {
	callee, ok := pattern.Callee.(parser.IdentifierExpression)
	if !ok {
		checker.checkScopeExpression(pattern, scope, namespace, source, line)
		return matchPatternInfo{}
	}
	switch callee.Name {
	case "Some":
		elementType, ok := optionElementType(valueType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("case pattern type Option[T] does not match %s", valueType))
			return matchPatternInfo{}
		}
		if len(pattern.Arguments) != 1 {
			checker.addError(source, line, "Some pattern expects one argument")
			return matchPatternInfo{Kind: "option", Name: "Some"}
		}
		checker.checkPatternScope(pattern.Arguments[0], elementType, scope, namespace, source, line)
		return matchPatternInfo{Kind: "option", Name: "Some"}
	case "None":
		if _, ok := optionElementType(valueType); !ok {
			checker.addError(source, line, fmt.Sprintf("case pattern type Option[T] does not match %s", valueType))
		}
		if len(pattern.Arguments) != 0 {
			checker.addError(source, line, "None pattern expects no arguments")
		}
		return matchPatternInfo{Kind: "option", Name: "None"}
	case "Ok":
		okType, _, ok := resultValueTypes(valueType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("case pattern type Result[T,E] does not match %s", valueType))
			return matchPatternInfo{}
		}
		if len(pattern.Arguments) != 1 {
			checker.addError(source, line, "Ok pattern expects one argument")
			return matchPatternInfo{Kind: "result", Name: "Ok"}
		}
		checker.checkPatternScope(pattern.Arguments[0], okType, scope, namespace, source, line)
		return matchPatternInfo{Kind: "result", Name: "Ok"}
	case "Err":
		_, errType, ok := resultValueTypes(valueType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("case pattern type Result[T,E] does not match %s", valueType))
			return matchPatternInfo{}
		}
		if len(pattern.Arguments) != 1 {
			checker.addError(source, line, "Err pattern expects one argument")
			return matchPatternInfo{Kind: "result", Name: "Err"}
		}
		checker.checkPatternScope(pattern.Arguments[0], errType, scope, namespace, source, line)
		return matchPatternInfo{Kind: "result", Name: "Err"}
	default:
		checker.checkScopeExpression(pattern, scope, namespace, source, line)
		patternType := anyType
		if valueType != anyType && patternType != anyType && valueType != patternType {
			checker.addError(source, line, fmt.Sprintf("case pattern type %s does not match %s", patternType, valueType))
		}
		return matchPatternInfo{}
	}
}

func (checker *TypeChecker) checkListPatternScope(pattern parser.ListExpression, valueType string, scope *lexicalScope, namespace string, source string, line int) matchPatternInfo {
	elementType, ok := listElementTypeName(valueType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("case pattern type List[T] does not match %s", valueType))
		return matchPatternInfo{}
	}
	for _, item := range pattern.Items {
		checker.checkPatternScope(item, elementType, scope, namespace, source, line)
	}
	return matchPatternInfo{}
}

func (checker *TypeChecker) checkTablePatternScope(pattern parser.MapExpression, valueType string, scope *lexicalScope, namespace string, source string, line int) matchPatternInfo {
	if valueType != anyType && valueType != "Table" && !strings.HasPrefix(valueType, "Map[") {
		checker.addError(source, line, fmt.Sprintf("case pattern type Table does not match %s", valueType))
		return matchPatternInfo{}
	}
	valueElementType := anyType
	if _, mapValue, ok := indexedMapTypes(valueType); ok {
		valueElementType = mapValue
	}
	for _, entry := range pattern.Entries {
		checker.checkScopeExpression(entry.Key, scope, namespace, source, line)
		checker.checkPatternScope(entry.Value, valueElementType, scope, namespace, source, line)
	}
	return matchPatternInfo{}
}

func (checker *TypeChecker) inferMatchExpressionType(expr parser.Expression, locals map[string]variableSymbol, source string, line int) string {
	switch current := expr.Node.(type) {
	case parser.LiteralExpression:
		return normalizeType(current.Kind)
	case parser.IdentifierExpression:
		if variable, exists := checker.lookupVariable(current.Name, locals); exists {
			if variable.InferredType != "" {
				return variable.InferredType
			}
			return variable.Type
		}
	}
	return checker.inferExpression(expr.Literal(), locals, source, line)
}

func boolPatternLiteral(expr parser.ExpressionNode) (bool, bool) {
	literal, ok := expr.(parser.LiteralExpression)
	if !ok || literal.Kind != "Bool" {
		return false, false
	}
	return literal.Value == "True", true
}

func enumPatternLiteral(expr parser.ExpressionNode) (string, string, bool) {
	selector, ok := expr.(parser.SelectorExpression)
	if !ok {
		return "", "", false
	}
	target, ok := selector.Target.(parser.IdentifierExpression)
	if !ok {
		return "", "", false
	}
	return target.Name, selector.Field, true
}

func scopeVariables(scope *lexicalScope) map[string]variableSymbol {
	locals := map[string]variableSymbol{}
	for current := scope; current != nil; current = current.parent {
		for name, variable := range current.variables {
			if _, exists := locals[name]; !exists {
				locals[name] = variable
			}
		}
	}
	return locals
}

func (checker *TypeChecker) isPatternMatchType(typeName string) bool {
	typeName = normalizeType(typeName)
	switch typeName {
	case anyType, "Bool", "String", "Int", "Float", "Table":
		return true
	default:
		return checker.enumExists(typeName) ||
			strings.HasPrefix(typeName, "Option[") ||
			strings.HasPrefix(typeName, "Result[") ||
			strings.HasPrefix(typeName, "List[") ||
			strings.HasPrefix(typeName, "Map[")
	}
}

func listElementTypeName(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "List[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := normalizeType(typeName[len("List[") : len(typeName)-1])
	return elementType, elementType != ""
}

func (checker *TypeChecker) checkFunctionScope(fn parser.FunctionStatement, parent *lexicalScope, namespace string, source string) {
	functionScope := newLexicalScope(parent)
	for _, param := range fn.Params {
		if param.Default.Node != nil {
			checker.checkScopeExpression(param.Default.Node, functionScope, namespace, source, fn.Pos.Line)
		}
		if !functionScope.define(variableSymbol{Name: param.Name, Type: normalizeType(param.Type), Mutable: param.Mutable, File: source, Line: fn.Pos.Line}) {
			checker.addError(source, fn.Pos.Line, fmt.Sprintf("parameter %q is already defined", param.Name))
		}
	}
	checker.checkScopeStatements(fn.Body, functionScope, namespace, source, false, false)
}

func (checker *TypeChecker) checkAliasFunctionScope(alias parser.AliasFunctionStatement, parent *lexicalScope, namespace string, source string) {
	checker.checkScopeStatements(alias.Body, newLexicalScope(parent), namespace, source, false, false)
	for _, method := range alias.Methods {
		methodScope := newLexicalScope(parent)
		methodScope.define(variableSymbol{Name: "this", Type: alias.Name, File: source, Line: method.Pos.Line})
		for _, param := range method.Params {
			if !methodScope.define(variableSymbol{Name: param.Name, Type: normalizeType(param.Type), Mutable: param.Mutable, File: source, Line: method.Pos.Line}) {
				checker.addError(source, method.Pos.Line, fmt.Sprintf("parameter %q is already defined", param.Name))
			}
		}
		checker.checkScopeStatements(method.Body, methodScope, namespace, source, false, false)
	}
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
		if isDiscardIdentifier(current.Name) {
			return
		}
		if _, ok := scope.lookup(current.Name); !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", current.Name))
		}
	case parser.IndexExpression:
		if _, ok := current.Target.(parser.IdentifierExpression); !ok {
			checker.addError(source, line, "assignment target must be an lvalue")
			return
		}
		checker.checkScopeExpression(current.Target, scope, namespace, source, line)
		checker.checkScopeExpression(current.Index, scope, namespace, source, line)
	default:
		checker.addError(source, line, "assignment target must be an lvalue")
	}
}

func (checker *TypeChecker) checkScopeExpression(expr parser.ExpressionNode, scope *lexicalScope, namespace string, source string, line int) {
	switch current := expr.(type) {
	case nil:
		return
	case parser.IdentifierExpression:
		if current.Name == "this" {
			return
		}
		if _, ok := scope.lookup(current.Name); ok {
			return
		}
		if isBuiltinFunctionName(current.Name) || checker.functionExists(current.Name, namespace) ||
			checker.aliasFunctionExistsInNamespace(current.Name, namespace) ||
			checker.functionGroupExistsInNamespace(current.Name, namespace) ||
			checker.namespaceExists(current.Name) || checker.aliasExists(current.Name) || checker.enumExists(current.Name) {
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
		if current.Field == "sizeof" {
			if _, ok := typeExpressionNameFromNode(current.Target); ok {
				return
			}
		}
		if target, ok := current.Target.(parser.IdentifierExpression); ok && checker.enumVariantExists(target.Name, current.Field) {
			return
		}
		if checker.selectorFunctionExists(current) {
			return
		}
		checker.checkScopeExpression(current.Target, scope, namespace, source, line)
	case parser.CastExpression:
		checker.checkScopeExpression(current.Value, scope, namespace, source, line)
	case parser.NullCheckExpression:
		checker.checkScopeExpression(current.Value, scope, namespace, source, line)
	case parser.PropagateExpression:
		checker.checkScopeExpression(current.Value, scope, namespace, source, line)
	case parser.ConditionalExpression:
		checker.checkScopeExpression(current.Condition, scope, namespace, source, line)
		checker.checkScopeExpression(current.Consequence, scope, namespace, source, line)
		checker.checkScopeExpression(current.Alternative, scope, namespace, source, line)
	case parser.ListExpression:
		for _, item := range current.Items {
			checker.checkScopeExpression(item, scope, namespace, source, line)
		}
	case parser.ListComprehensionExpression:
		checker.checkScopeExpression(current.Iterable, scope, namespace, source, line)
		comprehensionScope := newLexicalScope(scope)
		comprehensionScope.define(variableSymbol{Name: current.Iterator, Type: anyType, File: source, Line: line})
		if current.Condition != nil {
			checker.checkScopeExpression(current.Condition, comprehensionScope, namespace, source, line)
		}
		checker.checkScopeExpression(current.Value, comprehensionScope, namespace, source, line)
	case parser.MapExpression:
		for _, entry := range current.Entries {
			checker.checkScopeExpression(entry.Key, scope, namespace, source, line)
			checker.checkScopeExpression(entry.Value, scope, namespace, source, line)
		}
	case parser.GroupExpression:
		checker.checkScopeExpression(current.Inner, scope, namespace, source, line)
	case parser.LambdaExpression:
		lambdaScope := newLexicalScope(scope)
		for _, param := range current.Params {
			if param.Default.Node != nil {
				checker.checkScopeExpression(param.Default.Node, lambdaScope, namespace, source, line)
			}
			if !lambdaScope.define(variableSymbol{Name: param.Name, Type: normalizeType(param.Type), Mutable: param.Mutable, File: source, Line: line}) {
				checker.addError(source, line, fmt.Sprintf("parameter %q is already defined", param.Name))
			}
		}
		checker.checkScopeStatements(current.Body, lambdaScope, namespace, source, false, false)
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
		if isBuiltinFunctionName(current.Name) || checker.functionExists(current.Name, namespace) ||
			checker.aliasFunctionExistsInNamespace(current.Name, namespace) ||
			checker.functionGroupExistsInNamespace(current.Name, namespace) {
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

func (checker *TypeChecker) functionGroupExistsInNamespace(name string, namespace string) bool {
	name = checker.resolveAliasPath(name)
	if _, ok := checker.groups[name]; ok {
		return true
	}
	if namespace != "" {
		if _, ok := checker.groups[namespace+name]; ok {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) aliasFunctionExistsInNamespace(name string, namespace string) bool {
	name = checker.resolveAliasPath(name)
	if _, ok := checker.aliasFunctions[name]; ok {
		return true
	}
	if namespace != "" {
		if _, ok := checker.aliasFunctions[namespace+name]; ok {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) functionExists(name string, namespace string) bool {
	name = checker.resolveAliasPath(name)
	if _, ok := checker.functions[name]; ok {
		return true
	}
	if _, ok, _ := checker.resolveGlobalFunction(name); ok {
		return true
	}
	if namespace != "" {
		if _, ok := checker.functions[namespace+name]; ok {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) resolveGlobalFunction(name string) (string, bool, string) {
	if strings.Contains(name, ".") {
		return "", false, ""
	}
	matches := checker.globalFunctions[name]
	if len(matches) == 0 {
		return "", false, ""
	}
	if len(matches) > 1 {
		return "", false, fmt.Sprintf("ambiguous global namespace function %q matches %s", name, strings.Join(matches, ", "))
	}
	return matches[0], true, ""
}

func (checker *TypeChecker) namespaceExists(name string) bool {
	name = checker.resolveAliasPath(name)
	prefix := name + "."
	for functionName := range checker.functions {
		if strings.HasPrefix(functionName, prefix) {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) aliasExists(name string) bool {
	_, ok := checker.aliases[name]
	return ok
}

func (checker *TypeChecker) selectorFunctionExists(expr parser.SelectorExpression) bool {
	path, ok := selectorPath(expr)
	if !ok {
		return false
	}
	resolved := checker.resolveAliasPath(path)
	if typeName, ok := runtimeTypeInfoCallTarget(resolved); ok && isKnownType(typeName) {
		return true
	}
	if isBuiltinFunctionName(resolved) {
		return true
	}
	_, ok = checker.functions[resolved]
	return ok
}

func selectorPath(expr parser.ExpressionNode) (string, bool) {
	switch current := expr.(type) {
	case parser.IdentifierExpression:
		return current.Name, true
	case parser.SelectorExpression:
		target, ok := selectorPath(current.Target)
		if !ok {
			return "", false
		}
		return target + "." + current.Field, true
	default:
		return "", false
	}
}

func isBuiltinFunctionName(name string) bool {
	switch name {
	case "print", "format", "printf", "input", "len", "range", "Some", "None", "Ok", "Err", "Result", "Complex", "SIMD", "Set",
		"Table", "iter", "next", "coroutine", "resume", "spawn", "join", "thread_status",
		"table_has", "has_key", "set_has", "table_delete", "table_keys", "table_values", "table_entries", "table_sequence_count", "table_set_fallback",
		"Atomic", "atomic_load", "atomic_store", "atomic_add",
		"Program", "BuildSystem", "WorkSpace", "workspace_backend", "workspace_files", "workspace_manifest",
		"runtime_debug_loc", "runtime_debug_file", "runtime_debug_line", "runtime_debug_module", "runtime_debug_pos", "runtime_debug_function",
		"runtime_debug_loc_of", "runtime_debug_line_of", "runtime_debug_pos_of",
		"runtime.debug.__LOC__", "runtime.debug.__FILE__", "runtime.debug.__LINE__", "runtime.debug.__MODULE__", "runtime.debug.__POS__", "runtime.debug.__FUNCTION__",
		"runtime.debug.__LOC_OF__", "runtime.debug.__LINE_OF__", "runtime.debug.__POS_OF__",
		"debug", "debug_type", "debug_stack", "debug_state", "breakpoint", "js_import", "js_source", "js_exports", "js_call",
		"Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return true
	default:
		return false
	}
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
