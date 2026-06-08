package typechecker

import (
	"fmt"
	"path/filepath"
	"strings"
	"unicode"

	"kLang/src/engine/file"
)

const anyType = "T"

type Error struct {
	File    string
	Line    int
	Message string
}

type Report struct {
	Errors []Error
}

func (report Report) Passed() bool {
	return len(report.Errors) == 0
}

type TypeChecker struct {
	functions map[string]functionSymbol
	globals   map[string]variableSymbol
	errors    []Error
	namespace string
}

type functionSymbol struct {
	Name       string
	Namespace  string
	Params     []variableSymbol
	ReturnType string
	File       string
	Line       int
	Body       string
}

type variableSymbol struct {
	Name    string
	Type    string
	Mutable bool
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

	return Report{Errors: checker.errors}
}

func (checker *TypeChecker) collectFunctions(unit sourceUnit, namespace string) {
	index := 0
	for index < len(unit.Text) {
		nextNamespace := findKeyword(unit.Text, "namespace", index)
		nextFunction := findKeyword(unit.Text, "function", index)

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

		exprType := checker.inferExpression(decl.Expression, map[string]variableSymbol{}, unit.Path, decl.Line)
		if !isAssignable(decl.Type, exprType) {
			checker.addError(unit.Path, decl.Line, fmt.Sprintf("cannot assign %s to global %s %s", exprType, decl.Type, decl.Name))
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
		if strings.HasPrefix(current, "global ") || strings.HasPrefix(current, "import ") || current == "" {
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

	body := maskNestedFunctions(fn.Body)
	for _, stmt := range splitStatements(body) {
		line := fn.Line + lineAt(body, stmt.Start) - 1
		current := trimStatementPrefix(stmt.Text)
		if current == "" {
			continue
		}

		if decl, ok := parseVariableDeclaration(current, "local"); ok {
			exprType := checker.inferExpression(decl.Expression, locals, fn.File, line)
			if !isAssignable(decl.Type, exprType) {
				checker.addError(fn.File, line, fmt.Sprintf("cannot assign %s to local %s %s", exprType, decl.Type, decl.Name))
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

	paramsEnd := findMatchingIn(header, nameEnd, '(', ')')
	if paramsEnd == -1 {
		return functionSymbol{}, openBrace, fmt.Errorf("function declaration is missing a closing parameter brace")
	}

	params, err := parseParams(header[nameEnd+1 : paramsEnd])
	if err != nil {
		return functionSymbol{}, openBrace, err
	}

	afterParams := strings.TrimSpace(header[paramsEnd+1:])
	if !strings.HasPrefix(afterParams, ":") {
		return functionSymbol{}, openBrace, fmt.Errorf("function %s is missing a return type", name)
	}
	returnType := normalizeType(strings.TrimSpace(afterParams[1:]))
	if !isKnownType(returnType) {
		return functionSymbol{}, openBrace, fmt.Errorf("function %s uses unknown return type %s", name, returnType)
	}

	fullName := namespace + name
	return functionSymbol{
		Name:       fullName,
		Namespace:  namespace,
		Params:     params,
		ReturnType: returnType,
		File:       unit.Path,
		Line:       lineAt(unit.Text, start),
		Body:       unit.Text[openBrace+1 : closeBrace],
	}, closeBrace + 1, nil
}

func parseParams(input string) ([]variableSymbol, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}

	parts := splitTopLevel(input, ',')
	params := make([]variableSymbol, 0, len(parts))
	for _, part := range parts {
		pieces := splitTopLevel(part, ':')
		if len(pieces) != 2 {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		name := strings.TrimSpace(pieces[0])
		typeName := normalizeType(pieces[1])
		if name == "" || typeName == "" {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		if !isKnownType(typeName) {
			return nil, fmt.Errorf("function parameter %s uses unknown type %s", name, typeName)
		}
		params = append(params, variableSymbol{Name: name, Type: typeName})
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
	if assignIndex == -1 {
		return variableDeclaration{}, false
	}

	left := strings.TrimSpace(rest[:assignIndex])
	expr := strings.TrimSpace(rest[assignIndex+1:])
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

func (checker *TypeChecker) checkAssignment(assignment assignmentStatement, locals map[string]variableSymbol, source string, line int) {
	baseName := assignment.Target
	targetType := ""

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(baseName); ok {
		baseName = strings.TrimSpace(targetExpr)
		if !isIdentifierPath(baseName) {
			checker.addError(source, line, "indexed assignment target must start from a variable")
			return
		}
		base, ok := checker.lookupVariable(baseName, locals)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", baseName))
			return
		}
		if !base.Mutable {
			checker.addError(source, line, fmt.Sprintf("cannot mutate immutable variable %q", baseName))
			return
		}
		indexType := checker.inferExpression(indexExpr, locals, source, line)
		targetType = checker.checkIndexedAssignmentTarget(base.Type, indexType, source, line)
	} else {
		variable, ok := checker.lookupVariable(baseName, locals)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("cannot assign to unknown variable %q", baseName))
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
}

func (checker *TypeChecker) inferExpression(expr string, locals map[string]variableSymbol, source string, line int) string {
	expr = strings.TrimSpace(expr)
	expr = strings.TrimSuffix(expr, ";")
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "call "))
	expr = trimOuterParens(expr)
	if expr == "" {
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

	if index := findTopLevelOperator(expr, []string{"==", "!=", ">=", "<=", ">", "<"}); index != -1 {
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{" and ", " or "}); index != -1 {
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
	if index := findTopLevelOperator(expr, []string{"*", "/", "%"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+1:], locals, source, line)
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

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(expr); ok {
		targetType := checker.inferExpression(targetExpr, locals, source, line)
		indexType := checker.inferExpression(indexExpr, locals, source, line)
		return checker.checkIndexExpression(targetType, indexType, source, line)
	}

	if callName, args, ok := parseFunctionCall(expr); ok {
		return checker.checkCall(callName, args, locals, source, line)
	}

	if variable, ok := checker.lookupVariable(expr, locals); ok {
		return variable.Type
	}
	if _, ok := checker.functions[expr]; ok {
		return anyType
	}

	checker.addError(source, line, fmt.Sprintf("unknown expression %q", expr))
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

func (checker *TypeChecker) checkCall(name string, args []string, locals map[string]variableSymbol, source string, line int) string {
	name = strings.TrimPrefix(strings.TrimSpace(name), "call ")
	switch name {
	case "print":
		return anyType
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
	}

	if variable, ok := checker.lookupVariable(name, locals); ok && variable.Type == anyType {
		return anyType
	}

	fn, ok := checker.functions[name]
	if !ok && checker.namespace != "" && !strings.Contains(name, ".") {
		fn, ok = checker.functions[checker.namespace+name]
	}
	if !ok {
		checker.addError(source, line, fmt.Sprintf("unknown function %q", name))
		return anyType
	}

	if len(args) != len(fn.Params) {
		checker.addError(source, line, fmt.Sprintf("function %s expects %d argument(s), got %d", name, len(fn.Params), len(args)))
		return fn.ReturnType
	}

	for index, arg := range args {
		argType := checker.inferExpression(arg, locals, source, line)
		param := fn.Params[index]
		if !isAssignable(param.Type, argType) {
			checker.addError(source, line, fmt.Sprintf("function %s argument %d expects %s, got %s", name, index+1, param.Type, argType))
		}
	}

	return fn.ReturnType
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

func (checker *TypeChecker) addError(source string, line int, message string) {
	checker.errors = append(checker.errors, Error{
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
		next := nextNamespace
		if next == -1 || (nextFunction != -1 && nextFunction < next) {
			next = nextFunction
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
	return strings.ReplaceAll(strings.TrimSpace(input), " ", "")
}

func isKnownType(typeName string) bool {
	if typeName == anyType || typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" {
		return true
	}
	if strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[5 : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Map[") && strings.HasSuffix(typeName, "]") {
		parts := splitTopLevel(typeName[4:len(typeName)-1], ',')
		return len(parts) == 2 && isKnownType(parts[0]) && isKnownType(parts[1])
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

func isIntegerIndexType(typeName string) bool {
	typeName = normalizeType(typeName)
	return typeName == anyType || typeName == "Int" || typeName == "UInt"
}

func isAssignable(target string, source string) bool {
	target = normalizeType(target)
	source = normalizeType(source)
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
	return false
}

func canCast(source string, target string) bool {
	source = normalizeType(source)
	target = normalizeType(target)
	if source == anyType || target == anyType || source == target {
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
	return false
}

func isScalarType(typeName string) bool {
	return typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char"
}

func isNumeric(typeName string) bool {
	return typeName == "Int" || typeName == "UInt" || typeName == "Float" || typeName == anyType
}

func numericResult(left string, right string) string {
	if left == anyType || right == anyType {
		return anyType
	}
	if left == "Float" || right == "Float" {
		return "Float"
	}
	if left == "UInt" && right == "UInt" {
		return "UInt"
	}
	return "Int"
}

func parseFunctionCall(expr string) (string, []string, bool) {
	expr = strings.TrimSpace(strings.TrimPrefix(expr, "call "))
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
						return start
					}
				}
			}
		}
	}
	return -1
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
