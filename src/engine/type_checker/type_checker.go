package typechecker

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"kLang/src/engine/file"
	"kLang/src/parser"
)

const anyType = "T"
const movedType = "<moved>"

type Error struct {
	File    string
	Line    int
	Message string
}

type Warning struct {
	File    string
	Line    int
	Message string
}

type Report struct {
	Errors   []Error
	Warnings []Warning
}

func (report Report) Passed() bool {
	return len(report.Errors) == 0
}

type TypeChecker struct {
	functions map[string]functionSymbol
	globals   map[string]variableSymbol
	aliases   map[string]string
	traits    map[string]traitSymbol
	errors    []Error
	warnings  []Warning
	namespace string
}

type functionSymbol struct {
	Name               string
	Namespace          string
	Params             []variableSymbol
	ReturnType         string
	Deprecated         bool
	DeprecationMessage string
	File               string
	Line               int
	Body               string
	TypeRestrictions   map[string]string
}

type traitSymbol struct {
	Name    string
	Methods map[string]traitMethodSymbol
	File    string
	Line    int
}

type traitMethodSymbol struct {
	Name       string
	Params     []variableSymbol
	ReturnType string
	Line       int
}

type variableSymbol struct {
	Name    string
	Type    string
	Mutable bool
	Default string
	File    string
	Line    int
}

type sourceUnit struct {
	Path string
	Text string
}

func CheckProgram(program file.Program) Report {
	checker := &TypeChecker{
		functions: map[string]functionSymbol{},
		globals:   map[string]variableSymbol{},
		aliases:   map[string]string{},
		traits:    map[string]traitSymbol{},
	}

	units := make([]sourceUnit, 0, len(program.Files))
	for _, source := range program.Files {
		units = append(units, sourceUnit{
			Path: source.Path,
			Text: stripComments(strings.Join(source.Lines, "\n")),
		})
	}

	for _, unit := range units {
		checker.collectFunctions(unit, "")
	}
	checker.collectTraits(program)
	checker.collectAliases(program)
	for _, unit := range units {
		checker.collectGlobals(unit)
	}
	checker.collectASTGlobals(program)
	for _, unit := range units {
		checker.checkTopLevelCalls(unit)
	}
	for _, fn := range checker.functions {
		checker.checkFunction(fn)
	}
	checker.checkLexicalScopes(program)

	return Report{Errors: checker.errors, Warnings: checker.warnings}
}

func (checker *TypeChecker) collectFunctions(unit sourceUnit, namespace string) {
	index := 0
	for index < len(unit.Text) {
		nextNamespace := findKeyword(unit.Text, "namespace", index)
		nextFunction := findKeyword(unit.Text, "function", index)
		nextTrait := findKeyword(unit.Text, "trait", index)
		nextImpl := findKeyword(unit.Text, "impl", index)
		nextBlocked := nearestKeyword(nextTrait, nextImpl)

		if nextBlocked != -1 && (nextFunction == -1 || nextBlocked < nextFunction) &&
			(nextNamespace == -1 || nextBlocked < nextNamespace) {
			openBrace := findChar(unit.Text, '{', nextBlocked)
			if openBrace == -1 {
				index = nextBlocked + 1
				continue
			}
			closeBrace := matchBrace(unit.Text, openBrace)
			if closeBrace == -1 {
				checker.addError(unit.Path, lineAt(unit.Text, openBrace), "trait or impl block is missing a closing brace")
				return
			}
			index = closeBrace + 1
			continue
		}

		if nextNamespace != -1 && (nextFunction == -1 || nextNamespace < nextFunction) {
			name, openBrace := readNamedBlockHeader(unit.Text, nextNamespace+len("namespace"))
			if name == "" || openBrace == -1 {
				index = nextNamespace + len("namespace")
				continue
			}

			closeBrace := matchBrace(unit.Text, openBrace)
			if closeBrace == -1 {
				checker.addError(unit.Path, lineAt(unit.Text, openBrace), "namespace block is missing a closing brace")
				return
			}

			body := unit.Text[openBrace+1 : closeBrace]
			checker.collectFunctions(sourceUnit{Path: unit.Path, Text: body}, namespace+name+".")
			index = closeBrace + 1
			continue
		}

		if nextFunction == -1 {
			return
		}

		fn, end, err := parseFunction(unit, nextFunction, namespace)
		if err != nil {
			checker.addError(unit.Path, lineAt(unit.Text, nextFunction), err.Error())
			index = nextFunction + len("function")
			continue
		}

		if _, exists := checker.functions[fn.Name]; exists {
			checker.addError(fn.File, fn.Line, fmt.Sprintf("function %q is already defined", fn.Name))
		} else {
			checker.functions[fn.Name] = fn
		}
		index = end
	}
}

func (checker *TypeChecker) collectAliases(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectAliasStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectTraits(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectTraitStatements(source.Program.Statements, source.Path)
	}
	for _, source := range parsed.Sources {
		checker.checkImplStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectTraitStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.TraitStatement:
			if _, exists := checker.traits[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("trait %q is already defined", current.Name))
				continue
			}
			methods := map[string]traitMethodSymbol{}
			for _, method := range current.Methods {
				if _, exists := methods[method.Name]; exists {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("trait %s method %q is already defined", current.Name, method.Name))
					continue
				}
				params := make([]variableSymbol, 0, len(method.Params))
				for _, param := range method.Params {
					paramType := normalizeType(param.Type)
					if !isKnownType(paramType) {
						checker.addError(source, method.Pos.Line, fmt.Sprintf("trait method %s parameter %s uses unknown type %s", method.Name, param.Name, paramType))
					}
					params = append(params, variableSymbol{Name: param.Name, Type: paramType})
				}
				returnType := normalizeType(method.ReturnType)
				if !isKnownType(returnType) {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("trait method %s uses unknown return type %s", method.Name, returnType))
				}
				methods[method.Name] = traitMethodSymbol{Name: method.Name, Params: params, ReturnType: returnType, Line: method.Pos.Line}
			}
			checker.traits[current.Name] = traitSymbol{Name: current.Name, Methods: methods, File: source, Line: current.Pos.Line}
		case parser.NamespaceStatement:
			checker.collectTraitStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) checkImplStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ImplStatement:
			trait, ok := checker.traits[current.Trait]
			if !ok {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("impl targets unknown trait %q", current.Trait))
				continue
			}
			if !isKnownType(normalizeType(current.Type)) {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("impl %s uses unknown type %s", current.Trait, current.Type))
				continue
			}
			implemented := map[string]parser.FunctionStatement{}
			for _, method := range current.Methods {
				implemented[method.Name] = method
				expected, ok := trait.Methods[method.Name]
				if !ok {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("impl %s for %s defines unknown method %q", current.Trait, current.Type, method.Name))
					continue
				}
				checker.checkTraitMethodImpl(current, expected, method, source)
			}
			for name := range trait.Methods {
				if _, ok := implemented[name]; !ok {
					checker.addError(source, current.Pos.Line, fmt.Sprintf("impl %s for %s is missing method %q", current.Trait, current.Type, name))
				}
			}
		case parser.NamespaceStatement:
			checker.checkImplStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) checkTraitMethodImpl(impl parser.ImplStatement, expected traitMethodSymbol, actual parser.FunctionStatement, source string) {
	if len(expected.Params) != len(actual.Params) {
		checker.addError(source, actual.Pos.Line, fmt.Sprintf("impl %s method %s expects %d parameter(s), got %d", impl.Trait, actual.Name, len(expected.Params), len(actual.Params)))
		return
	}
	for index, expectedParam := range expected.Params {
		actualType := normalizeType(actual.Params[index].Type)
		if actualType != expectedParam.Type {
			checker.addError(source, actual.Pos.Line, fmt.Sprintf("impl %s method %s parameter %d expects %s, got %s", impl.Trait, actual.Name, index+1, expectedParam.Type, actualType))
		}
	}
	actualReturn := normalizeType(actual.ReturnType)
	if actualReturn != expected.ReturnType {
		checker.addError(source, actual.Pos.Line, fmt.Sprintf("impl %s method %s returns %s, expected %s", impl.Trait, actual.Name, actualReturn, expected.ReturnType))
	}
}

func (checker *TypeChecker) collectAliasStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.AliasStatement:
			if current.Target == "" {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q is missing a namespace target", current.Name))
				continue
			}
			if _, exists := checker.aliases[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q is already defined", current.Name))
				continue
			}
			if !checker.namespaceExists(current.Target) {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q targets unknown namespace %q", current.Name, current.Target))
				continue
			}
			checker.aliases[current.Name] = current.Target
		case parser.NamespaceStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.FunctionStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.IfStatement:
			checker.collectAliasStatements(current.Consequence, source)
			checker.collectAliasStatements(current.Alternative, source)
			if current.ElseIf != nil {
				checker.collectAliasStatements([]parser.Statement{*current.ElseIf}, source)
			}
		case parser.LoopStatement:
			checker.collectAliasStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) collectGlobals(unit sourceUnit) {
	text := maskBlocks(unit.Text)
	for _, stmt := range splitStatements(text) {
		decl, ok := parseVariableDeclaration(stmt.Text, "global")
		if !ok {
			continue
		}

		decl.File = unit.Path
		decl.Line = lineAt(text, stmt.Start)
		if _, exists := checker.globals[decl.Name]; exists {
			checker.addError(unit.Path, decl.Line, fmt.Sprintf("global variable %q is already defined", decl.Name))
			continue
		}

		if decl.Expression != "" {
			exprType := checker.inferExpression(decl.Expression, map[string]variableSymbol{}, unit.Path, decl.Line)
			if !isAssignable(decl.Type, exprType) {
				checker.addError(unit.Path, decl.Line, fmt.Sprintf("cannot assign %s to global %s %s", exprType, decl.Type, decl.Name))
			}
		}

		checker.globals[decl.Name] = variableSymbol{
			Name:    decl.Name,
			Type:    decl.Type,
			Mutable: decl.Mutable,
			File:    unit.Path,
			Line:    decl.Line,
		}
	}
}

func (checker *TypeChecker) checkTopLevelCalls(unit sourceUnit) {
	text := maskBlocks(unit.Text)
	for _, stmt := range splitStatements(text) {
		current := trimStatementPrefix(stmt.Text)
		if strings.HasPrefix(current, "global ") || strings.HasPrefix(current, "import ") || strings.HasPrefix(current, "alias ") || current == "" {
			continue
		}
		if looksLikeCall(current) {
			checker.inferExpression(current, map[string]variableSymbol{}, unit.Path, lineAt(text, stmt.Start))
		}
	}
}

func (checker *TypeChecker) checkFunction(fn functionSymbol) {
	previousNamespace := checker.namespace
	checker.namespace = fn.Namespace
	defer func() {
		checker.namespace = previousNamespace
	}()

	locals := map[string]variableSymbol{}
	for _, param := range fn.Params {
		locals[param.Name] = param
	}

	for nestedName := range collectNestedFunctionNames(fn.Body) {
		locals[nestedName] = variableSymbol{Name: nestedName, Type: anyType}
	}
	for loopName := range collectEvaluationAssignmentNames(fn.Body) {
		locals[loopName] = variableSymbol{Name: loopName, Type: anyType, Mutable: true}
	}
	for _, param := range fn.Params {
		if param.Default == "" {
			continue
		}
		defaultType := checker.inferExpression(param.Default, locals, fn.File, fn.Line)
		if !isAssignable(param.Type, defaultType) {
			checker.addError(fn.File, fn.Line, fmt.Sprintf("parameter %s default expects %s, got %s", param.Name, param.Type, defaultType))
		}
	}

	body := maskNestedFunctions(fn.Body)
	for _, stmt := range splitStatements(body) {
		line := fn.Line + lineAt(body, stmt.Start) - 1
		current := trimStatementPrefix(stmt.Text)
		if current == "" {
			continue
		}

		if decl, ok := parseVariableDeclaration(current, "local"); ok {
			if decl.Expression != "" {
				exprType := checker.inferExpression(decl.Expression, locals, fn.File, line)
				if !isAssignable(decl.Type, exprType) {
					checker.addError(fn.File, line, fmt.Sprintf("cannot assign %s to local %s %s", exprType, decl.Type, decl.Name))
				}
				if movedName, ok := movedIdentifier(decl.Expression); ok {
					if movedVariable, exists := locals[movedName]; exists {
						movedVariable.Type = movedType
						locals[movedName] = movedVariable
					}
				}
			}

			locals[decl.Name] = variableSymbol{
				Name:    decl.Name,
				Type:    decl.Type,
				Mutable: decl.Mutable,
				File:    fn.File,
				Line:    line,
			}
			continue
		}

		if _, ok := parseVariableDeclaration(current, "global"); ok {
			continue
		}

		if strings.HasPrefix(current, "return ") {
			expr := strings.TrimSpace(strings.TrimPrefix(current, "return "))
			exprType := checker.inferExpression(expr, locals, fn.File, line)
			if !isAssignable(fn.ReturnType, exprType) {
				checker.addError(fn.File, line, fmt.Sprintf("function %s returns %s but return expression is %s", fn.Name, fn.ReturnType, exprType))
			}
			continue
		}

		if assignment, ok := parseAssignment(current); ok {
			checker.checkAssignment(assignment, locals, fn.File, line)
			if movedName, ok := movedIdentifier(assignment.Expr); ok {
				if movedVariable, exists := locals[movedName]; exists {
					movedVariable.Type = movedType
					locals[movedName] = movedVariable
				}
			}
			continue
		}

		if looksLikeCall(current) {
			checker.inferExpression(current, locals, fn.File, line)
		}
	}
}

type variableDeclaration struct {
	Mutable    bool
	Exported   bool
	Type       string
	Name       string
	Expression string
	File       string
	Line       int
}

type assignmentStatement struct {
	Target string
	Op     string
	Expr   string
}

type statement struct {
	Text  string
	Start int
}

func parseFunction(unit sourceUnit, start int, namespace string) (functionSymbol, int, error) {
	openBrace := findChar(unit.Text, '{', start)
	if openBrace == -1 {
		return functionSymbol{}, start, fmt.Errorf("function block is missing an opening brace")
	}
	closeBrace := matchBrace(unit.Text, openBrace)
	if closeBrace == -1 {
		return functionSymbol{}, openBrace, fmt.Errorf("function block is missing a closing brace")
	}

	header := strings.TrimSpace(unit.Text[start+len("function") : openBrace])
	nameEnd := strings.Index(header, "(")
	if nameEnd == -1 {
		return functionSymbol{}, openBrace, fmt.Errorf("function declaration is missing parameters")
	}
	name := strings.TrimSpace(header[:nameEnd])
	if name == "" {
		return functionSymbol{}, openBrace, fmt.Errorf("function declaration is missing a name")
	}
	typeRestrictions := map[string]string{}
	if bracket := strings.Index(name, "["); bracket != -1 {
		rawName := strings.TrimSpace(name[:bracket])
		restrictionText := strings.TrimSpace(name[bracket:])
		if rawName == "" || !strings.HasPrefix(restrictionText, "[") || !strings.HasSuffix(restrictionText, "]") {
			return functionSymbol{}, openBrace, fmt.Errorf("function declaration has malformed generic restrictions")
		}
		parsedRestrictions, err := parseTypeRestrictions(restrictionText[1 : len(restrictionText)-1])
		if err != nil {
			return functionSymbol{}, openBrace, err
		}
		name = rawName
		typeRestrictions = parsedRestrictions
	}

	paramsEnd := findMatchingIn(header, nameEnd, '(', ')')
	if paramsEnd == -1 {
		return functionSymbol{}, openBrace, fmt.Errorf("function declaration is missing a closing parameter brace")
	}

	params, err := parseParams(header[nameEnd+1 : paramsEnd])
	if err != nil {
		return functionSymbol{}, openBrace, err
	}
	for index := range params {
		params[index].Type = applyFunctionTypeRestrictions(params[index].Type, typeRestrictions)
	}

	afterParams := strings.TrimSpace(header[paramsEnd+1:])
	returnType := anyType
	if strings.HasPrefix(afterParams, ":") {
		returnType = normalizeType(strings.TrimSpace(afterParams[1:]))
	}
	returnType = applyFunctionTypeRestrictions(returnType, typeRestrictions)
	if !isKnownType(returnType) {
		return functionSymbol{}, openBrace, fmt.Errorf("function %s uses unknown return type %s", name, returnType)
	}

	fullName := namespace + name
	deprecated, deprecationMessage := functionDeprecatedMarker(unit.Text, start)
	return functionSymbol{
		Name:               fullName,
		Namespace:          namespace,
		Params:             params,
		ReturnType:         returnType,
		Deprecated:         deprecated,
		DeprecationMessage: deprecationMessage,
		File:               unit.Path,
		Line:               lineAt(unit.Text, start),
		Body:               unit.Text[openBrace+1 : closeBrace],
		TypeRestrictions:   typeRestrictions,
	}, closeBrace + 1, nil
}

func functionDeprecatedMarker(input string, functionStart int) (bool, string) {
	prefix := strings.TrimSpace(input[:functionStart])
	if prefix == "" {
		return false, ""
	}

	lineStart := strings.LastIndex(prefix, "\n")
	marker := strings.TrimSpace(prefix[lineStart+1:])
	if marker == "@deprecated" {
		return true, ""
	}
	if strings.HasPrefix(marker, "@deprecated(") && strings.HasSuffix(marker, ")") {
		message := strings.TrimSpace(marker[len("@deprecated(") : len(marker)-1])
		if strings.HasPrefix(message, "\"") && strings.HasSuffix(message, "\"") && len(message) >= 2 {
			return true, message[1 : len(message)-1]
		}
	}
	return false, ""
}

func parseParams(input string) ([]variableSymbol, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}

	parts := splitTopLevel(input, ',')
	params := make([]variableSymbol, 0, len(parts))
	for _, part := range parts {
		defaultValue := ""
		if assignIndex := findTopLevelOperator(part, []string{"="}); assignIndex != -1 {
			defaultValue = strings.TrimSpace(part[assignIndex+1:])
			part = strings.TrimSpace(part[:assignIndex])
			if defaultValue == "" {
				return nil, fmt.Errorf("function parameter %q has an empty default value", strings.TrimSpace(part))
			}
		}
		colon := strings.Index(part, ":")
		if colon == -1 {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		name := strings.TrimSpace(part[:colon])
		typeName := normalizeType(part[colon+1:])
		if name == "" || typeName == "" {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		if !isKnownType(typeName) {
			return nil, fmt.Errorf("function parameter %s uses unknown type %s", name, typeName)
		}
		params = append(params, variableSymbol{Name: name, Type: typeName, Default: defaultValue})
	}
	return params, nil
}

func parseVariableDeclaration(stmt string, scope string) (variableDeclaration, bool) {
	stmt = trimStatementPrefix(stmt)
	exported := false
	if strings.HasPrefix(stmt, "export ") {
		exported = true
		stmt = strings.TrimSpace(strings.TrimPrefix(stmt, "export"))
	}
	if !strings.HasPrefix(stmt, scope+" ") {
		return variableDeclaration{}, false
	}

	rest := strings.TrimSpace(strings.TrimPrefix(stmt, scope))
	mutable := false
	if strings.HasPrefix(rest, "mut ") {
		mutable = true
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "mut"))
	}

	assignIndex := findTopLevelOperator(rest, []string{"="})
	left := strings.TrimSpace(rest)
	expr := ""
	if assignIndex != -1 {
		left = strings.TrimSpace(rest[:assignIndex])
		expr = strings.TrimSpace(rest[assignIndex+1:])
	}
	typeName, name, ok := splitTypeAndName(left)
	if !ok || !isKnownType(typeName) {
		return variableDeclaration{}, false
	}

	return variableDeclaration{
		Mutable:    mutable,
		Exported:   exported,
		Type:       typeName,
		Name:       name,
		Expression: expr,
	}, true
}

func parseAssignment(stmt string) (assignmentStatement, bool) {
	stmt = trimStatementPrefix(stmt)
	if strings.Contains(stmt, ":=") {
		return assignmentStatement{}, false
	}
	if strings.HasPrefix(stmt, "global ") || strings.HasPrefix(stmt, "local ") || strings.HasPrefix(stmt, "export ") ||
		strings.HasPrefix(stmt, "if ") || strings.HasPrefix(stmt, "else ") || strings.HasPrefix(stmt, "unless ") ||
		strings.HasPrefix(stmt, "while ") || strings.HasPrefix(stmt, "for ") || strings.HasPrefix(stmt, "do_while ") {
		return assignmentStatement{}, false
	}

	operators := []string{"+=", "-=", "*=", "/=", "="}
	index := findTopLevelOperator(stmt, operators)
	if index == -1 {
		return assignmentStatement{}, false
	}

	op := ""
	for _, candidate := range operators {
		if strings.HasPrefix(stmt[index:], candidate) {
			op = candidate
			break
		}
	}
	if op == "" || strings.HasPrefix(stmt[index:], "==") {
		return assignmentStatement{}, false
	}

	target := strings.TrimSpace(stmt[:index])
	expr := strings.TrimSpace(stmt[index+len(op):])
	if target == "" || expr == "" {
		return assignmentStatement{}, false
	}

	return assignmentStatement{Target: target, Op: op, Expr: expr}, true
}

func movedIdentifier(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "move ") {
		return "", false
	}
	name := strings.TrimSpace(strings.TrimPrefix(expr, "move"))
	if !isSimpleIdentifier(name) {
		return "", false
	}
	return name, true
}

func (checker *TypeChecker) checkAssignment(assignment assignmentStatement, locals map[string]variableSymbol, source string, line int) {
	baseName := assignment.Target
	targetType := ""

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(baseName); ok {
		baseName = strings.TrimSpace(targetExpr)
		if !isSimpleIdentifier(baseName) {
			checker.addError(source, line, "assignment target must be an lvalue")
			return
		}
		base, ok := checker.lookupVariable(baseName, locals)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", baseName))
			return
		}
		if base.Type == movedType {
			checker.addError(source, line, fmt.Sprintf("variable %q was moved", baseName))
			return
		}
		if !base.Mutable {
			checker.addError(source, line, fmt.Sprintf("cannot mutate immutable variable %q", baseName))
			return
		}
		indexType := checker.inferExpression(indexExpr, locals, source, line)
		targetType = checker.checkIndexedAssignmentTarget(base.Type, indexType, source, line)
	} else {
		if !isSimpleIdentifier(baseName) {
			checker.addError(source, line, "assignment target must be an lvalue")
			return
		}
		variable, ok := checker.lookupVariable(baseName, locals)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", baseName))
			return
		}
		if variable.Type == movedType {
			checker.addError(source, line, fmt.Sprintf("variable %q was moved", baseName))
			return
		}
		if !variable.Mutable {
			checker.addError(source, line, fmt.Sprintf("cannot mutate immutable variable %q", baseName))
			return
		}
		targetType = variable.Type
	}

	exprType := checker.inferExpression(assignment.Expr, locals, source, line)
	if assignment.Op != "=" && !isNumeric(targetType) && targetType != "String" {
		checker.addError(source, line, fmt.Sprintf("operator %s cannot be used with %s", assignment.Op, targetType))
		return
	}
	if !isAssignable(targetType, exprType) {
		checker.addError(source, line, fmt.Sprintf("cannot assign %s to %s", exprType, targetType))
	}
	if !strings.Contains(assignment.Target, "[") {
		if variable, ok := locals[assignment.Target]; ok && variable.Type == anyType && exprType != anyType {
			variable.Type = exprType
			locals[assignment.Target] = variable
		}
	}
}

func (checker *TypeChecker) inferExpression(expr string, locals map[string]variableSymbol, source string, line int) string {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ";")
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "call "))
	expr = normalizeNamespaceAccess(expr)
	expr = trimOuterParens(expr)
	if expr == "" {
		return anyType
	}
	if strings.HasPrefix(expr, "move ") {
		target := strings.TrimSpace(strings.TrimPrefix(expr, "move"))
		if !isSimpleIdentifier(target) {
			checker.addError(source, line, "move expects a variable")
			return anyType
		}
		return checker.inferExpression(target, locals, source, line)
	}
	if condition, consequence, alternative, ok := splitConditionalExpression(expr); ok {
		checker.inferExpression(condition, locals, source, line)
		consequenceType := checker.inferExpression(consequence, locals, source, line)
		alternativeType := checker.inferExpression(alternative, locals, source, line)
		if isAssignable(consequenceType, alternativeType) {
			return consequenceType
		}
		if isAssignable(alternativeType, consequenceType) {
			return alternativeType
		}
		checker.addError(source, line, fmt.Sprintf("conditional branches have incompatible types %s and %s", consequenceType, alternativeType))
		return anyType
	}

	if strings.HasPrefix(expr, "\"") && strings.HasSuffix(expr, "\"") {
		return "String"
	}
	if strings.HasPrefix(expr, "'") && strings.HasSuffix(expr, "'") {
		return "Char"
	}
	if expr == "True" || expr == "False" {
		return "Bool"
	}
	if isIntegerLiteral(expr) {
		return "Int"
	}
	if isFloatLiteral(expr) {
		return "Float"
	}
	if expr == "{}" {
		return "Map[T,T]"
	}
	if expr == "[]" {
		return "List[T]"
	}
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		return checker.inferListLiteral(expr, locals, source, line)
	}

	if index := findTopLevelOperator(expr, []string{"|>"}); index != -1 {
		return checker.checkPipe(expr[:index], expr[index+len("|>"):], locals, source, line)
	}
	if strings.HasPrefix(expr, "not ") {
		checker.inferExpression(strings.TrimSpace(strings.TrimPrefix(expr, "not")), locals, source, line)
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{" or "}); index != -1 {
		checker.inferExpression(expr[:index], locals, source, line)
		checker.inferExpression(expr[index+len(" or "):], locals, source, line)
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{" xor "}); index != -1 {
		checker.inferExpression(expr[:index], locals, source, line)
		checker.inferExpression(expr[index+len(" xor "):], locals, source, line)
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{" and "}); index != -1 {
		checker.inferExpression(expr[:index], locals, source, line)
		checker.inferExpression(expr[index+len(" and "):], locals, source, line)
		return "Bool"
	}
	if index, operator := findTopLevelOperatorWithMatch(expr, []string{"==", "!=", ">=", "<=", ">", "<"}); index != -1 {
		checker.inferExpression(expr[:index], locals, source, line)
		checker.inferExpression(expr[index+len(operator):], locals, source, line)
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{"+", "-"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+1:], locals, source, line)
		if left == "String" || right == "String" {
			return "String"
		}
		return numericResult(left, right)
	}
	if index, operator := findTopLevelOperatorWithMatch(expr, []string{"*", "//", "/", "%"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+len(operator):], locals, source, line)
		return numericResult(left, right)
	}
	if index := findTopLevelOperator(expr, []string{" as "}); index != -1 && index > 0 {
		sourceType := checker.inferExpression(expr[:index], locals, source, line)
		targetType := normalizeType(expr[index+len(" as "):])
		if !isKnownType(targetType) {
			checker.addError(source, line, fmt.Sprintf("unknown cast target type %s", targetType))
			return anyType
		}
		if !canCast(sourceType, targetType) {
			checker.addError(source, line, fmt.Sprintf("cannot cast %s to %s", sourceType, targetType))
		}
		return targetType
	}
	if index := findTopLevelOperator(expr, []string{"**"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+len("**"):], locals, source, line)
		return numericResult(left, right)
	}
	if inner, ok := splitPostfixNullCheckExpression(expr); ok {
		checker.inferExpression(inner, locals, source, line)
		return "Bool"
	}

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(expr); ok {
		targetType := checker.inferExpression(targetExpr, locals, source, line)
		indexType := checker.inferExpression(indexExpr, locals, source, line)
		return checker.checkIndexExpression(targetType, indexType, source, line)
	}

	if callName, args, ok := parseFunctionCall(expr); ok {
		return checker.checkCall(callName, args, locals, source, line)
	}

	if variable, ok := checker.lookupVariable(expr, locals); ok {
		if variable.Type == movedType {
			checker.addError(source, line, fmt.Sprintf("variable %q was moved", expr))
			return anyType
		}
		return variable.Type
	}
	if fn, ok := checker.lookupFunction(expr); ok {
		return functionTypeForSymbol(fn)
	}

	checker.addError(source, line, fmt.Sprintf("unknown expression %q", expr))
	return anyType
}

func (checker *TypeChecker) checkPipe(left string, right string, locals map[string]variableSymbol, source string, line int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(right), "call "))
	if left == "" || right == "" {
		checker.addError(source, line, "pipe operator requires a value and a function target")
		return anyType
	}

	if callName, args, ok := parseFunctionCall(right); ok {
		return checker.checkCall(callName, append([]string{left}, args...), locals, source, line)
	}
	if isIdentifierPath(right) {
		return checker.checkCall(right, []string{left}, locals, source, line)
	}

	checker.inferExpression(left, locals, source, line)
	checker.addError(source, line, "pipe target must be a function or function call")
	return anyType
}

func (checker *TypeChecker) checkIndexExpression(targetType string, indexType string, source string, line int) string {
	targetType = normalizeType(targetType)
	indexType = normalizeType(indexType)
	switch {
	case targetType == anyType:
		return anyType
	case targetType == "String":
		if !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("String index must be Int, got %s", indexType))
		}
		return "Char"
	case strings.HasPrefix(targetType, "List[") && strings.HasSuffix(targetType, "]"):
		if !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("List index must be Int, got %s", indexType))
		}
		return indexedValueType(targetType)
	case strings.HasPrefix(targetType, "Map[") && strings.HasSuffix(targetType, "]"):
		keyType, valueType, ok := indexedMapTypes(targetType)
		if ok && !isAssignable(keyType, indexType) {
			checker.addError(source, line, fmt.Sprintf("Map index expects %s, got %s", keyType, indexType))
		}
		return valueType
	default:
		checker.addError(source, line, fmt.Sprintf("%s is not indexable", targetType))
		return anyType
	}
}

func (checker *TypeChecker) checkIndexedAssignmentTarget(targetType string, indexType string, source string, line int) string {
	targetType = normalizeType(targetType)
	indexType = normalizeType(indexType)
	switch {
	case targetType == anyType:
		return anyType
	case targetType == "String":
		checker.addError(source, line, "String indexes cannot be assigned")
		return "Char"
	case strings.HasPrefix(targetType, "List[") && strings.HasSuffix(targetType, "]"):
		if !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("List index must be Int, got %s", indexType))
		}
		return indexedValueType(targetType)
	case strings.HasPrefix(targetType, "Map[") && strings.HasSuffix(targetType, "]"):
		keyType, valueType, ok := indexedMapTypes(targetType)
		if ok && !isAssignable(keyType, indexType) {
			checker.addError(source, line, fmt.Sprintf("Map index expects %s, got %s", keyType, indexType))
		}
		return valueType
	default:
		checker.addError(source, line, fmt.Sprintf("%s is not index-assignable", targetType))
		return anyType
	}
}

func (checker *TypeChecker) inferListLiteral(expr string, locals map[string]variableSymbol, source string, line int) string {
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	if inner == "" {
		return "List[T]"
	}
	if valueExpr, iterator, iterableExpr, conditionExpr, ok := splitListComprehensionLiteral(inner); ok {
		return checker.inferListComprehension(valueExpr, iterator, iterableExpr, conditionExpr, locals, source, line)
	}

	items := splitTopLevel(inner, ',')
	itemType := ""
	for _, item := range items {
		currentType := checker.inferExpression(item, locals, source, line)
		if itemType == "" {
			itemType = currentType
			continue
		}
		if !isAssignable(itemType, currentType) {
			itemType = anyType
		}
	}
	return "List[" + itemType + "]"
}

func (checker *TypeChecker) inferListComprehension(valueExpr string, iterator string, iterableExpr string, conditionExpr string, locals map[string]variableSymbol, source string, line int) string {
	iterableType := checker.inferExpression(iterableExpr, locals, source, line)
	itemType, ok := iterableItemType(iterableType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("list comprehension cannot iterate over %s", iterableType))
		itemType = anyType
	}

	scopedLocals := copyLocals(locals)
	scopedLocals[iterator] = variableSymbol{Name: iterator, Type: itemType, File: source, Line: line}
	if conditionExpr != "" {
		checker.inferExpression(conditionExpr, scopedLocals, source, line)
	}
	valueType := checker.inferExpression(valueExpr, scopedLocals, source, line)
	return "List[" + valueType + "]"
}

func (checker *TypeChecker) checkCall(name string, args []string, locals map[string]variableSymbol, source string, line int) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "call ")
	name = checker.resolveAliasPath(normalizeNamespaceAccess(name))
	switch name {
	case "print":
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return anyType
	case "input":
		if len(args) > 1 {
			checker.addError(source, line, "input expects 0 to 1 argument(s)")
		}
		if len(args) == 1 {
			argType := checker.inferExpression(args[0], locals, source, line)
			if !isAssignable("String", argType) {
				checker.addError(source, line, fmt.Sprintf("input prompt expects String, got %s", argType))
			}
		}
		return "String"
	case "len":
		if len(args) != 1 {
			checker.addError(source, line, "len expects 1 argument")
		}
		return "Int"
	case "range":
		if len(args) != 1 {
			checker.addError(source, line, "range expects 1 argument")
		}
		return "Int"
	case "Some":
		if len(args) != 1 {
			checker.addError(source, line, "Some expects 1 argument")
			return "Option[T]"
		}
		return "Option[" + checker.inferExpression(args[0], locals, source, line) + "]"
	case "None":
		if len(args) != 0 {
			checker.addError(source, line, "None expects 0 arguments")
		}
		return "Option[T]"
	case "Ok":
		if len(args) != 1 {
			checker.addError(source, line, "Ok expects 1 argument")
			return "Result[T,T]"
		}
		return "Result[" + checker.inferExpression(args[0], locals, source, line) + ",T]"
	case "Err":
		if len(args) != 1 {
			checker.addError(source, line, "Err expects 1 argument")
			return "Result[T,T]"
		}
		return "Result[T," + checker.inferExpression(args[0], locals, source, line) + "]"
	case "Result":
		if len(args) != 1 {
			checker.addError(source, line, "Result expects 1 argument")
			return "Result[T,T]"
		}
		return "Result[" + checker.inferExpression(args[0], locals, source, line) + ",T]"
	case "Complex":
		if len(args) != 2 {
			checker.addError(source, line, "Complex expects 2 arguments")
			return "Complex"
		}
		for index, arg := range args {
			argType := checker.inferExpression(arg, locals, source, line)
			if !isNumeric(argType) {
				checker.addError(source, line, fmt.Sprintf("Complex argument %d must be numeric, got %s", index+1, argType))
			}
		}
		return "Complex"
	case "SIMD":
		if len(args) != 1 {
			checker.addError(source, line, "SIMD expects 1 argument")
			return "SIMD[T]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if strings.HasPrefix(argType, "List[") && strings.HasSuffix(argType, "]") {
			return "SIMD[" + argType[5:len(argType)-1] + "]"
		}
		checker.addError(source, line, fmt.Sprintf("SIMD expects List[T], got %s", argType))
		return "SIMD[T]"
	}

	if variable, ok := checker.lookupVariable(name, locals); ok {
		if functionTypeArgs, functionReturnType, ok := functionValueType(variable.Type); ok {
			return checker.checkCallbackCall(name, functionTypeArgs, functionReturnType, args, locals, source, line)
		}
		if variable.Type == anyType {
			return anyType
		}
	}

	fn, ok := checker.lookupFunction(name)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("unknown function %q", name))
		return anyType
	}
	if fn.Deprecated {
		message := fmt.Sprintf("function %s is deprecated", fn.Name)
		if fn.DeprecationMessage != "" {
			message += ": " + fn.DeprecationMessage
		}
		checker.addWarning(source, line, message)
	}

	required := requiredParamCount(fn.Params)
	if len(args) < required || len(args) > len(fn.Params) {
		checker.addError(source, line, fmt.Sprintf("function %s expects %d to %d argument(s), got %d", name, required, len(fn.Params), len(args)))
		return fn.ReturnType
	}

	for index, arg := range args {
		argType := checker.inferExpression(arg, locals, source, line)
		param := fn.Params[index]
		if !isAssignable(param.Type, argType) {
			checker.addError(source, line, fmt.Sprintf("function %s argument %d expects %s, got %s", name, index+1, param.Type, argType))
		}
	}

	return checker.inferGenericCallReturn(fn, args, locals, source, line)
}

func (checker *TypeChecker) checkCallbackCall(name string, paramTypes []string, returnType string, args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != len(paramTypes) {
		checker.addError(source, line, fmt.Sprintf("callback %s expects %d argument(s), got %d", name, len(paramTypes), len(args)))
		return returnType
	}
	for index, arg := range args {
		argType := checker.inferExpression(arg, locals, source, line)
		if !isAssignable(paramTypes[index], argType) {
			checker.addError(source, line, fmt.Sprintf("callback %s argument %d expects %s, got %s", name, index+1, paramTypes[index], argType))
		}
	}
	return returnType
}

func requiredParamCount(params []variableSymbol) int {
	count := len(params)
	for count > 0 && params[count-1].Default != "" {
		count--
	}
	return count
}

func (checker *TypeChecker) inferGenericCallReturn(fn functionSymbol, args []string, locals map[string]variableSymbol, source string, line int) string {
	solver := newConstraintSolver()
	for index, param := range fn.Params {
		if index >= len(args) && param.Default == "" {
			continue
		}
		expr := param.Default
		if index < len(args) {
			expr = args[index]
		}
		argType := checker.inferExpression(expr, locals, source, line)
		solver.unify(fn.Params[index].Type, argType)
	}
	return solver.apply(fn.ReturnType)
}

func (checker *TypeChecker) lookupVariable(name string, locals map[string]variableSymbol) (variableSymbol, bool) {
	if variable, ok := locals[name]; ok {
		return variable, true
	}
	if variable, ok := checker.globals[name]; ok {
		return variable, true
	}
	return variableSymbol{}, false
}

func (checker *TypeChecker) lookupFunction(name string) (functionSymbol, bool) {
	name = checker.resolveAliasPath(name)
	if fn, ok := checker.functions[name]; ok {
		return fn, true
	}
	if checker.namespace != "" && !strings.Contains(name, ".") {
		if fn, ok := checker.functions[checker.namespace+name]; ok {
			return fn, true
		}
	}
	return functionSymbol{}, false
}

func functionTypeForSymbol(fn functionSymbol) string {
	parts := make([]string, 0, len(fn.Params)+1)
	for _, param := range fn.Params {
		parts = append(parts, normalizeType(param.Type))
	}
	parts = append(parts, normalizeType(fn.ReturnType))
	return "Function[" + strings.Join(parts, ",") + "]"
}

func (checker *TypeChecker) resolveAliasPath(name string) string {
	name = normalizeNamespaceAccess(strings.TrimSpace(name))
	for alias, target := range checker.aliases {
		if name == alias {
			return target
		}
		if strings.HasPrefix(name, alias+".") {
			return target + strings.TrimPrefix(name, alias)
		}
	}
	return name
}

func (checker *TypeChecker) addError(source string, line int, message string) {
	checker.errors = append(checker.errors, Error{
		File:    filepath.Clean(source),
		Line:    line,
		Message: message,
	})
}

func (checker *TypeChecker) addWarning(source string, line int, message string) {
	checker.warnings = append(checker.warnings, Warning{
		File:    filepath.Clean(source),
		Line:    line,
		Message: message,
	})
}

func stripComments(input string) string {
	var output strings.Builder
	for _, line := range strings.Split(input, "\n") {
		inString := false
		inChar := false
		cut := len(line)
		for index := 0; index+1 < len(line); index++ {
			switch line[index] {
			case '"':
				if !inChar {
					inString = !inString
				}
			case '\'':
				if !inString {
					inChar = !inChar
				}
			case '-':
				if !inString && !inChar && line[index+1] == '-' {
					cut = index
					index = len(line)
				}
			}
		}
		output.WriteString(line[:cut])
		output.WriteByte('\n')
	}
	return output.String()
}

func maskBlocks(input string) string {
	chars := []rune(input)
	index := 0
	for index < len(input) {
		nextNamespace := findKeyword(input, "namespace", index)
		nextFunction := findKeyword(input, "function", index)
		nextTrait := findKeyword(input, "trait", index)
		nextImpl := findKeyword(input, "impl", index)
		next := nextNamespace
		if next == -1 || (nextFunction != -1 && nextFunction < next) {
			next = nextFunction
		}
		if next == -1 || (nextTrait != -1 && nextTrait < next) {
			next = nextTrait
		}
		if next == -1 || (nextImpl != -1 && nextImpl < next) {
			next = nextImpl
		}
		if next == -1 {
			break
		}
		open := findChar(input, '{', next)
		if open == -1 {
			break
		}
		close := matchBrace(input, open)
		if close == -1 {
			break
		}
		for pos := next; pos <= close && pos < len(chars); pos++ {
			if chars[pos] != '\n' {
				chars[pos] = ' '
			}
		}
		index = close + 1
	}
	return string(chars)
}

func maskNestedFunctions(input string) string {
	chars := []rune(input)
	index := 0
	for index < len(input) {
		nextFunction := findKeyword(input, "function", index)
		if nextFunction == -1 {
			break
		}
		open := findChar(input, '{', nextFunction)
		if open == -1 {
			break
		}
		close := matchBrace(input, open)
		if close == -1 {
			break
		}
		for pos := nextFunction; pos <= close && pos < len(chars); pos++ {
			if chars[pos] != '\n' {
				chars[pos] = ' '
			}
		}
		index = close + 1
	}
	return string(chars)
}

func collectNestedFunctionNames(input string) map[string]bool {
	names := map[string]bool{}
	index := 0
	for index < len(input) {
		nextFunction := findKeyword(input, "function", index)
		if nextFunction == -1 {
			break
		}
		openParen := findChar(input, '(', nextFunction)
		openBrace := findChar(input, '{', nextFunction)
		if openParen == -1 || openBrace == -1 || openParen > openBrace {
			index = nextFunction + len("function")
			continue
		}
		name := strings.TrimSpace(input[nextFunction+len("function") : openParen])
		if name != "" {
			names[name] = true
		}
		close := matchBrace(input, openBrace)
		if close == -1 {
			index = openBrace + 1
			continue
		}
		index = close + 1
	}
	return names
}

func collectEvaluationAssignmentNames(input string) map[string]bool {
	names := map[string]bool{}
	for index := 0; index+1 < len(input); index++ {
		if input[index] != ':' || input[index+1] != '=' {
			continue
		}
		nameEnd := index
		nameStart := nameEnd - 1
		for nameStart >= 0 && unicode.IsSpace(rune(input[nameStart])) {
			nameStart--
		}
		nameEnd = nameStart + 1
		for nameStart >= 0 && isIdentifierRune(rune(input[nameStart])) {
			nameStart--
		}
		name := strings.TrimSpace(input[nameStart+1 : nameEnd])
		if isIdentifier(name) {
			names[name] = true
		}
	}
	return names
}

func splitStatements(input string) []statement {
	var statements []statement
	start := 0
	depthParen := 0
	depthBracket := 0
	inString := false
	inChar := false
	for index := 0; index < len(input); index++ {
		current := input[index]
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		switch current {
		case '(':
			depthParen++
		case ')':
			depthParen--
		case '[':
			depthBracket++
		case ']':
			depthBracket--
		case ';':
			if depthParen == 0 && depthBracket == 0 {
				text := strings.TrimSpace(input[start:index])
				if text != "" {
					statements = append(statements, statement{Text: text, Start: start})
				}
				start = index + 1
			}
		}
	}
	tail := strings.TrimSpace(input[start:])
	if tail != "" {
		statements = append(statements, statement{Text: tail, Start: start})
	}
	return statements
}

func trimStatementPrefix(stmt string) string {
	stmt = strings.TrimSpace(stmt)
	if strings.HasPrefix(stmt, "local ") || strings.HasPrefix(stmt, "global ") || strings.HasPrefix(stmt, "return ") {
		return stmt
	}
	lastBrace := -1
	if open := strings.LastIndex(stmt, "{"); open > lastBrace {
		lastBrace = open
	}
	if close := strings.LastIndex(stmt, "}"); close > lastBrace {
		lastBrace = close
	}
	if lastBrace != -1 {
		stmt = strings.TrimSpace(stmt[lastBrace+1:])
	}
	return stmt
}

func splitTypeAndName(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	depth := 0
	lastSpace := -1
	for index, char := range input {
		switch char {
		case '[':
			depth++
		case ']':
			depth--
		default:
			if unicode.IsSpace(char) && depth == 0 {
				lastSpace = index
			}
		}
	}
	if lastSpace == -1 {
		return "", "", false
	}
	return normalizeType(input[:lastSpace]), strings.TrimSpace(input[lastSpace+1:]), true
}

func normalizeType(input string) string {
	input = strings.TrimSpace(input)
	if canonical, ok := canonicalRestrictedType(input); ok {
		return canonical
	}
	return strings.ReplaceAll(input, " ", "")
}

func canonicalRestrictedType(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if !strings.Contains(input, " restrict[") {
		return "", false
	}
	parts := strings.SplitN(input, " restrict[", 2)
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || !strings.HasSuffix(parts[1], "]") {
		return "", false
	}
	name := strings.TrimSpace(parts[0])
	inner := strings.TrimSpace(parts[1][:len(parts[1])-1])
	if inner == "" {
		return "", false
	}
	allowed := splitTopLevel(inner, ',')
	for index, option := range allowed {
		allowed[index] = normalizeType(option)
	}
	return name + ":" + strings.Join(allowed, "|"), true
}

func parseTypeRestrictions(input string) (map[string]string, error) {
	restrictions := map[string]string{}
	if strings.TrimSpace(input) == "" {
		return restrictions, nil
	}
	for _, part := range splitTopLevel(input, ',') {
		canonical, ok := canonicalRestrictedType(part)
		if !ok {
			return nil, fmt.Errorf("generic restriction %q must be written as T restrict[Type,...]", strings.TrimSpace(part))
		}
		name, _, ok := restrictedGenericType(canonical)
		if !ok {
			return nil, fmt.Errorf("invalid generic restriction %q", strings.TrimSpace(part))
		}
		restrictions[name] = canonical
	}
	return restrictions, nil
}

func applyFunctionTypeRestrictions(typeName string, restrictions map[string]string) string {
	typeName = normalizeType(typeName)
	if restricted, ok := restrictions[typeName]; ok {
		return restricted
	}
	return typeName
}

func isKnownType(typeName string) bool {
	if _, allowed, ok := restrictedGenericType(typeName); ok {
		for _, option := range allowed {
			if !isKnownType(option) {
				return false
			}
		}
		return true
	}
	if typeName == anyType || typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" || typeName == "Complex" {
		return true
	}
	if strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[5 : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Map[") && strings.HasSuffix(typeName, "]") {
		parts := splitTopLevel(typeName[4:len(typeName)-1], ',')
		return len(parts) == 2 && isKnownType(parts[0]) && isKnownType(parts[1])
	}
	if strings.HasPrefix(typeName, "Option[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Option[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Result[") && strings.HasSuffix(typeName, "]") {
		parts := splitTopLevel(typeName[len("Result["):len(typeName)-1], ',')
		return len(parts) == 2 && isKnownType(parts[0]) && isKnownType(parts[1])
	}
	if strings.HasPrefix(typeName, "SIMD[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("SIMD[") : len(typeName)-1])
	}
	if params, returnType, ok := functionValueType(typeName); ok {
		if !isKnownType(returnType) {
			return false
		}
		for _, param := range params {
			if !isKnownType(param) {
				return false
			}
		}
		return true
	}
	return false
}

func indexedValueType(typeName string) string {
	if strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]") {
		return typeName[5 : len(typeName)-1]
	}
	if typeName == "String" {
		return "Char"
	}
	if strings.HasPrefix(typeName, "Map[") && strings.HasSuffix(typeName, "]") {
		_, valueType, ok := indexedMapTypes(typeName)
		if ok {
			return valueType
		}
	}
	return anyType
}

func indexedMapTypes(typeName string) (string, string, bool) {
	if !strings.HasPrefix(typeName, "Map[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	parts := splitTopLevel(typeName[4:len(typeName)-1], ',')
	if len(parts) != 2 {
		return "", "", false
	}
	return normalizeType(parts[0]), normalizeType(parts[1]), true
}

func optionElementType(typeName string) (string, bool) {
	if !strings.HasPrefix(typeName, "Option[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := normalizeType(typeName[len("Option[") : len(typeName)-1])
	return elementType, elementType != ""
}

func resultValueTypes(typeName string) (string, string, bool) {
	if !strings.HasPrefix(typeName, "Result[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	parts := splitTopLevel(typeName[len("Result["):len(typeName)-1], ',')
	if len(parts) != 2 {
		return "", "", false
	}
	return normalizeType(parts[0]), normalizeType(parts[1]), true
}

func functionValueType(typeName string) ([]string, string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Function[") || !strings.HasSuffix(typeName, "]") {
		return nil, "", false
	}
	parts := splitTopLevel(typeName[len("Function["):len(typeName)-1], ',')
	if len(parts) == 0 {
		return nil, "", false
	}
	for index := range parts {
		parts[index] = normalizeType(parts[index])
		if parts[index] == "" {
			return nil, "", false
		}
	}
	return parts[:len(parts)-1], parts[len(parts)-1], true
}

func isIntegerIndexType(typeName string) bool {
	typeName = normalizeType(typeName)
	return typeName == anyType || typeName == "Int" || typeName == "UInt"
}

func isAssignable(target string, source string) bool {
	target = normalizeType(target)
	source = normalizeType(source)
	solver := newConstraintSolver()
	if solver.unify(target, source) {
		return true
	}
	if _, allowed, ok := restrictedGenericType(target); ok {
		for _, option := range allowed {
			if isAssignable(option, source) {
				return true
			}
		}
		return false
	}
	if target == anyType || source == anyType {
		return true
	}
	if target == source {
		return true
	}
	if target == "Float" && (source == "Int" || source == "UInt") {
		return true
	}
	if target == "UInt" && source == "Int" {
		return true
	}
	if strings.HasPrefix(target, "List[") && strings.HasPrefix(source, "List[") {
		return isAssignable(target[5:len(target)-1], source[5:len(source)-1])
	}
	if strings.HasPrefix(target, "Map[") && source == "Map[T,T]" {
		return true
	}
	if strings.HasPrefix(target, "Option[") && strings.HasPrefix(source, "Option[") {
		targetElement, targetOk := optionElementType(target)
		sourceElement, sourceOk := optionElementType(source)
		return targetOk && sourceOk && isAssignable(targetElement, sourceElement)
	}
	if strings.HasPrefix(target, "Result[") && strings.HasPrefix(source, "Result[") {
		targetOkType, targetErrType, targetOk := resultValueTypes(target)
		sourceOkType, sourceErrType, sourceOk := resultValueTypes(source)
		return targetOk && sourceOk &&
			isAssignable(targetOkType, sourceOkType) &&
			isAssignable(targetErrType, sourceErrType)
	}
	if strings.HasPrefix(target, "SIMD[") && strings.HasPrefix(source, "SIMD[") {
		return isAssignable(target[len("SIMD["):len(target)-1], source[len("SIMD["):len(source)-1])
	}
	if targetParams, targetReturn, targetOk := functionValueType(target); targetOk {
		sourceParams, sourceReturn, sourceOk := functionValueType(source)
		if !sourceOk || len(targetParams) != len(sourceParams) {
			return false
		}
		for index := range targetParams {
			if !isAssignable(targetParams[index], sourceParams[index]) || !isAssignable(sourceParams[index], targetParams[index]) {
				return false
			}
		}
		return isAssignable(targetReturn, sourceReturn)
	}
	return false
}

type constraintSolver struct {
	substitutions map[string]string
}

func newConstraintSolver() *constraintSolver {
	return &constraintSolver{substitutions: map[string]string{}}
}

func (solver *constraintSolver) unify(left string, right string) bool {
	left = normalizeType(solver.apply(left))
	right = normalizeType(solver.apply(right))
	if left == right {
		return true
	}
	if isTypeVariable(left) {
		return solver.bind(left, right)
	}
	if isTypeVariable(right) {
		return solver.bind(right, left)
	}
	if left == "Float" && (right == "Int" || right == "UInt") {
		return true
	}
	if left == "UInt" && right == "Int" {
		return true
	}
	leftName, leftArgs, leftOk := splitGenericType(left)
	rightName, rightArgs, rightOk := splitGenericType(right)
	if !leftOk || !rightOk || leftName != rightName || len(leftArgs) != len(rightArgs) {
		return false
	}
	for index := range leftArgs {
		if !solver.unify(leftArgs[index], rightArgs[index]) {
			return false
		}
	}
	return true
}

func (solver *constraintSolver) bind(name string, typeName string) bool {
	if typeName == "" {
		return false
	}
	key := typeVariableKey(name)
	if _, allowed, ok := restrictedGenericType(name); ok {
		allowedMatch := false
		for _, option := range allowed {
			if isAssignable(option, typeName) {
				allowedMatch = true
				break
			}
		}
		if !allowedMatch {
			return false
		}
		name = key
	}
	if current, exists := solver.substitutions[name]; exists {
		return solver.unify(current, typeName)
	}
	solver.substitutions[name] = typeName
	return true
}

func (solver *constraintSolver) apply(typeName string) string {
	typeName = normalizeType(typeName)
	if resolved, exists := solver.substitutions[typeName]; exists {
		return solver.apply(resolved)
	}
	name, args, ok := splitGenericType(typeName)
	if !ok {
		return typeName
	}
	for index, arg := range args {
		args[index] = solver.apply(arg)
	}
	return name + "[" + strings.Join(args, ",") + "]"
}

func isTypeVariable(typeName string) bool {
	if typeName == anyType {
		return true
	}
	_, _, ok := restrictedGenericType(typeName)
	return ok
}

func typeVariableKey(typeName string) string {
	name, _, ok := restrictedGenericType(typeName)
	if ok {
		return name
	}
	return typeName
}

func restrictedGenericType(typeName string) (string, []string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, anyType+":") {
		return "", nil, false
	}
	parts := strings.Split(typeName[len(anyType)+1:], "|")
	if len(parts) == 0 {
		return "", nil, false
	}
	for index, part := range parts {
		parts[index] = normalizeType(part)
		if parts[index] == "" {
			return "", nil, false
		}
	}
	return anyType, parts, true
}

func splitGenericType(typeName string) (string, []string, bool) {
	typeName = normalizeType(typeName)
	open := strings.Index(typeName, "[")
	if open == -1 || !strings.HasSuffix(typeName, "]") {
		return "", nil, false
	}
	name := typeName[:open]
	inner := typeName[open+1 : len(typeName)-1]
	args := splitTopLevel(inner, ',')
	return name, args, name != "" && len(args) > 0
}

func canCast(source string, target string) bool {
	source = normalizeType(source)
	target = normalizeType(target)
	if source == anyType || target == anyType || source == target {
		return true
	}
	if _, allowed, ok := restrictedGenericType(source); ok {
		for _, option := range allowed {
			if !canCast(option, target) {
				return false
			}
		}
		return true
	}
	if isScalarType(source) && isScalarType(target) {
		return true
	}
	if strings.HasPrefix(target, "List[") && strings.HasPrefix(source, "List[") {
		return isAssignable(target, source)
	}
	if strings.HasPrefix(target, "Map[") && strings.HasPrefix(source, "Map[") {
		return isAssignable(target, source)
	}
	if strings.HasPrefix(target, "Option[") && strings.HasPrefix(source, "Option[") {
		return isAssignable(target, source)
	}
	if strings.HasPrefix(target, "Result[") && strings.HasPrefix(source, "Result[") {
		return isAssignable(target, source)
	}
	if strings.HasPrefix(target, "SIMD[") && strings.HasPrefix(source, "SIMD[") {
		return isAssignable(target, source)
	}
	return false
}

func isScalarType(typeName string) bool {
	return typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" || typeName == "Complex"
}

func isNumeric(typeName string) bool {
	return typeName == "Int" || typeName == "UInt" || typeName == "Float" || typeName == "Complex" ||
		typeName == anyType || strings.HasPrefix(typeName, "SIMD[")
}

func numericResult(left string, right string) string {
	if left == anyType || right == anyType {
		return anyType
	}
	if strings.HasPrefix(left, "SIMD[") {
		return left
	}
	if strings.HasPrefix(right, "SIMD[") {
		return right
	}
	if left == "Complex" || right == "Complex" {
		return "Complex"
	}
	if left == "Float" || right == "Float" {
		return "Float"
	}
	if left == "UInt" && right == "UInt" {
		return "UInt"
	}
	return "Int"
}

func iterableItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	switch {
	case typeName == anyType:
		return anyType, true
	case typeName == "String":
		return "Char", true
	case typeName == "Int" || typeName == "UInt":
		return "Int", true
	case strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]"):
		return typeName[5 : len(typeName)-1], true
	default:
		return "", false
	}
}

func copyLocals(locals map[string]variableSymbol) map[string]variableSymbol {
	copied := make(map[string]variableSymbol, len(locals)+1)
	for name, variable := range locals {
		copied[name] = variable
	}
	return copied
}

func parseFunctionCall(expr string) (string, []string, bool) {
	expr = normalizeNamespaceAccess(strings.TrimSpace(strings.TrimPrefix(expr, "call ")))
	open := findTopLevelChar(expr, '(')
	if open == -1 || !strings.HasSuffix(expr, ")") {
		return "", nil, false
	}
	close := findMatchingIn(expr, open, '(', ')')
	if close == -1 {
		return "", nil, false
	}
	name := strings.TrimSpace(expr[:open])
	if name == "" || !isIdentifierPath(name) {
		return "", nil, false
	}
	inner := strings.TrimSpace(expr[open+1 : close])
	var args []string
	if inner != "" {
		args = splitTopLevel(inner, ',')
	}
	return name, args, true
}

func normalizeNamespaceAccess(input string) string {
	return strings.ReplaceAll(input, "::", ".")
}

func splitListComprehensionLiteral(inner string) (string, string, string, string, bool) {
	forIndex := findTopLevelOperator(inner, []string{" for "})
	if forIndex <= 0 {
		return "", "", "", "", false
	}
	if findTopLevelOperator(inner[:forIndex], []string{","}) != -1 {
		return "", "", "", "", false
	}

	valueExpr := strings.TrimSpace(inner[:forIndex])
	afterFor := strings.TrimSpace(inner[forIndex+len(" for "):])
	inIndex := findTopLevelOperator(afterFor, []string{" in "})
	if inIndex <= 0 {
		return "", "", "", "", false
	}

	iterator := strings.TrimSpace(afterFor[:inIndex])
	if !isSimpleIdentifier(iterator) {
		return "", "", "", "", false
	}

	afterIn := strings.TrimSpace(afterFor[inIndex+len(" in "):])
	if afterIn == "" {
		return "", "", "", "", false
	}

	ifIndex := findTopLevelOperator(afterIn, []string{" if "})
	if ifIndex == -1 {
		return valueExpr, iterator, afterIn, "", true
	}

	iterableExpr := strings.TrimSpace(afterIn[:ifIndex])
	conditionExpr := strings.TrimSpace(afterIn[ifIndex+len(" if "):])
	if iterableExpr == "" || conditionExpr == "" {
		return "", "", "", "", false
	}
	return valueExpr, iterator, iterableExpr, conditionExpr, true
}

func splitConditionalExpression(expr string) (string, string, string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "if ") {
		return "", "", "", false
	}
	thenIndex := findTopLevelOperator(expr, []string{" then "})
	if thenIndex <= len("if ") {
		return "", "", "", false
	}
	afterThen := strings.TrimSpace(expr[thenIndex+len(" then "):])
	colonIndex := findTopLevelOperator(afterThen, []string{":"})
	if colonIndex <= 0 {
		return "", "", "", false
	}
	condition := strings.TrimSpace(expr[len("if "):thenIndex])
	consequence := trimExpressionReturn(strings.TrimSpace(afterThen[:colonIndex]))
	alternative := trimExpressionReturn(strings.TrimSpace(afterThen[colonIndex+1:]))
	return condition, consequence, alternative, condition != "" && consequence != "" && alternative != ""
}

func trimExpressionReturn(expr string) string {
	return strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(expr), "return "))
}

func isSimpleIdentifier(input string) bool {
	if input == "" {
		return false
	}
	for index, char := range input {
		if index == 0 {
			if !unicode.IsLetter(char) && char != '_' {
				return false
			}
			continue
		}
		if !isIdentifierRune(char) {
			return false
		}
	}
	return true
}

func splitTrailingIndexExpression(expr string) (string, string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, "]") {
		return "", "", false
	}

	depth := 0
	open := -1
	close := -1
	inString := false
	inChar := false
	for index := 0; index < len(expr); index++ {
		current := expr[index]
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		switch current {
		case '[':
			if depth == 0 {
				open = index
			}
			depth++
		case ']':
			depth--
			if depth < 0 {
				return "", "", false
			}
			if depth == 0 {
				close = index
			}
		}
	}
	if depth != 0 || open <= 0 || close != len(expr)-1 {
		return "", "", false
	}

	target := strings.TrimSpace(expr[:open])
	index := strings.TrimSpace(expr[open+1 : close])
	return target, index, target != "" && index != ""
}

func splitPostfixNullCheckExpression(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, "?") {
		return "", false
	}
	inner := strings.TrimSpace(expr[:len(expr)-1])
	if inner == "" || isInsideOpenStringOrChar(inner) {
		return "", false
	}
	return inner, true
}

func isInsideOpenStringOrChar(input string) bool {
	inString := false
	inChar := false
	for index := 0; index < len(input); index++ {
		current := input[index]
		if current == '\\' && index+1 < len(input) {
			index++
			continue
		}
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
	}
	return inString || inChar
}

func looksLikeCall(stmt string) bool {
	_, _, ok := parseFunctionCall(stmt)
	return ok
}

func splitTopLevel(input string, separator rune) []string {
	var parts []string
	start := 0
	depthParen := 0
	depthBracket := 0
	inString := false
	inChar := false
	for index, char := range input {
		if char == '"' && !inChar {
			inString = !inString
		}
		if char == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		switch char {
		case '(':
			depthParen++
		case ')':
			depthParen--
		case '[':
			depthBracket++
		case ']':
			depthBracket--
		default:
			if char == separator && depthParen == 0 && depthBracket == 0 {
				parts = append(parts, strings.TrimSpace(input[start:index]))
				start = index + len(string(char))
			}
		}
	}
	parts = append(parts, strings.TrimSpace(input[start:]))
	return parts
}

func findTopLevelOperator(input string, operators []string) int {
	index, _ := findTopLevelOperatorWithMatch(input, operators)
	return index
}

func findTopLevelOperatorWithMatch(input string, operators []string) (int, string) {
	depthParen := 0
	depthBracket := 0
	inString := false
	inChar := false
	for index := len(input) - 1; index >= 0; index-- {
		current := input[index]
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		switch current {
		case ')':
			depthParen++
		case '(':
			depthParen--
		case ']':
			depthBracket++
		case '[':
			depthBracket--
		default:
			if depthParen == 0 && depthBracket == 0 {
				for _, operator := range operators {
					start := index - len(operator) + 1
					if start >= 0 && input[start:index+1] == operator {
						if (operator == "+" || operator == "-") && start == 0 {
							continue
						}
						return start, operator
					}
				}
			}
		}
	}
	return -1, ""
}

func findTopLevelChar(input string, target byte) int {
	depthParen := 0
	depthBracket := 0
	inString := false
	inChar := false
	for index := 0; index < len(input); index++ {
		current := input[index]
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		if current == target && depthParen == 0 && depthBracket == 0 {
			return index
		}
		switch current {
		case '(':
			depthParen++
		case ')':
			depthParen--
		case '[':
			depthBracket++
		case ']':
			depthBracket--
		}
	}
	return -1
}

func findKeyword(input string, keyword string, start int) int {
	index := strings.Index(input[start:], keyword)
	for index != -1 {
		position := start + index
		beforeOK := position == 0 || !isIdentifierRune(rune(input[position-1]))
		after := position + len(keyword)
		afterOK := after >= len(input) || !isIdentifierRune(rune(input[after]))
		if beforeOK && afterOK {
			return position
		}
		nextStart := position + len(keyword)
		index = strings.Index(input[nextStart:], keyword)
		start = nextStart
	}
	return -1
}

func nearestKeyword(left int, right int) int {
	if left == -1 {
		return right
	}
	if right == -1 || left < right {
		return left
	}
	return right
}

func readNamedBlockHeader(input string, start int) (string, int) {
	rest := strings.TrimLeftFunc(input[start:], unicode.IsSpace)
	nameEnd := 0
	for nameEnd < len(rest) && isIdentifierRune(rune(rest[nameEnd])) {
		nameEnd++
	}
	name := rest[:nameEnd]
	open := findChar(input, '{', start)
	return name, open
}

func findChar(input string, target byte, start int) int {
	for index := start; index < len(input); index++ {
		if input[index] == target {
			return index
		}
	}
	return -1
}

func matchBrace(input string, open int) int {
	depth := 0
	inString := false
	inChar := false
	for index := open; index < len(input); index++ {
		current := input[index]
		if current == '"' && !inChar {
			inString = !inString
		}
		if current == '\'' && !inString {
			inChar = !inChar
		}
		if inString || inChar {
			continue
		}
		if current == '{' {
			depth++
		}
		if current == '}' {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func findMatchingIn(input string, open int, left byte, right byte) int {
	depth := 0
	for index := open; index < len(input); index++ {
		if input[index] == left {
			depth++
		}
		if input[index] == right {
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func trimOuterParens(input string) string {
	for strings.HasPrefix(input, "(") && strings.HasSuffix(input, ")") {
		close := findMatchingIn(input, 0, '(', ')')
		if close != len(input)-1 {
			return input
		}
		input = strings.TrimSpace(input[1 : len(input)-1])
	}
	return input
}

func lineAt(input string, index int) int {
	if index < 0 {
		return 1
	}
	line := 1
	for position := 0; position < index && position < len(input); position++ {
		if input[position] == '\n' {
			line++
		}
	}
	return line
}

func isIdentifierPath(input string) bool {
	parts := strings.Split(input, ".")
	for _, part := range parts {
		if !isIdentifier(part) {
			return false
		}
	}
	return true
}

func isIdentifier(input string) bool {
	if input == "" {
		return false
	}
	for index, char := range input {
		if index == 0 && unicode.IsDigit(char) {
			return false
		}
		if !isIdentifierRune(char) {
			return false
		}
	}
	return true
}

func isIdentifierRune(char rune) bool {
	return unicode.IsLetter(char) || unicode.IsDigit(char) || char == '_'
}

func isIntegerLiteral(input string) bool {
	if strings.HasPrefix(input, "-") {
		input = input[1:]
	}
	if input == "" {
		return false
	}
	for _, char := range input {
		if !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func isFloatLiteral(input string) bool {
	if strings.HasPrefix(input, "-") {
		input = input[1:]
	}
	seenDot := false
	seenDigit := false
	for _, char := range input {
		if char == '.' && !seenDot {
			seenDot = true
			continue
		}
		if !unicode.IsDigit(char) {
			return false
		}
		seenDigit = true
	}
	return seenDot && seenDigit
}
