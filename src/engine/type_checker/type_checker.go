package typechecker

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"kLang/src/engine/file"
	"kLang/src/lexer"
	"kLang/src/parser"
)

const anyType = "T"
const dynamicAnyType = "Any"
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
	functions       map[string]functionSymbol
	globalFunctions map[string][]string
	aliasFunctions  map[string]parser.AliasFunctionStatement
	regions         map[string]parser.RegionStatement
	groups          map[string][]string
	globals         map[string]variableSymbol
	aliases         map[string]string
	traits          map[string]traitSymbol
	enums           map[string]enumSymbol
	errors          []Error
	warnings        []Warning
	namespace       string
}

type functionSymbol struct {
	Name               string
	Namespace          string
	Params             []variableSymbol
	ReturnType         string
	ReturnTypes        []returnValueSymbol
	Async              bool
	Inline             bool
	Private            bool
	Deprecated         bool
	DeprecationMessage string
	File               string
	Line               int
	Body               string
	TypeRestrictions   map[string]string
}

type returnValueSymbol struct {
	Name    string
	Type    string
	Mutable bool
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

type enumSymbol struct {
	Name     string
	Variants map[string]enumVariantSymbol
	File     string
	Line     int
}

type enumVariantSymbol struct {
	Name    string
	Ordinal int
	Line    int
}

type variableSymbol struct {
	Name         string
	Type         string
	InferredType string
	KnownSome    bool
	Mutable      bool
	Default      string
	File         string
	Line         int
}

type sourceUnit struct {
	Path                 string
	Text                 string
	ModuleFunctionFilter map[string]bool
}

func CheckProgram(program file.Program) Report {
	checker := &TypeChecker{
		functions:       map[string]functionSymbol{},
		globalFunctions: map[string][]string{},
		aliasFunctions:  map[string]parser.AliasFunctionStatement{},
		regions:         map[string]parser.RegionStatement{},
		groups:          map[string][]string{},
		globals:         map[string]variableSymbol{},
		aliases:         map[string]string{},
		traits:          map[string]traitSymbol{},
		enums:           map[string]enumSymbol{},
	}

	units := make([]sourceUnit, 0, len(program.Files))
	for _, source := range program.Files {
		units = append(units, sourceUnit{
			Path:                 source.Path,
			Text:                 stripComments(strings.Join(source.Lines, "\n")),
			ModuleFunctionFilter: source.ModuleFunctionFilter,
		})
	}

	for _, unit := range units {
		checker.collectFunctions(unit, "", false)
	}
	checker.collectTraits(program)
	checker.collectEnums(program)
	checker.collectAliases(program)
	checker.collectAliasFunctionsAndRegions(program)
	checker.collectFunctionGroups(program)
	for _, unit := range units {
		checker.collectGlobals(unit)
	}
	checker.globals["Args"] = variableSymbol{Name: "Args", Type: "List[String]", Mutable: false}
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

func (checker *TypeChecker) collectFunctionGroups(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectFunctionGroupStatements(source.Program.Statements, "", source.Path)
	}
}

func (checker *TypeChecker) collectAliasFunctionsAndRegions(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectAliasFunctionStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectAliasFunctionStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.RegionStatement:
			if _, exists := checker.regions[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("region %q is already defined", current.Name))
				continue
			}
			checker.regions[current.Name] = current
		case parser.AliasFunctionStatement:
			if _, exists := checker.aliasFunctions[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias function %q is already defined", current.Name))
				continue
			}
			checker.aliasFunctions[current.Name] = current
			checker.collectAliasFunctionStatements(current.Body, source)
		case parser.NamespaceStatement:
			checker.collectAliasFunctionStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) collectFunctionGroupStatements(statements []parser.Statement, namespace string, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.FunctionGroupStatement:
			name := namespace + current.Name
			if _, exists := checker.groups[name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("function_group %q is already defined", name))
				continue
			}
			for _, member := range current.Functions {
				if _, ok := checker.lookupFunction(member); !ok {
					checker.addError(source, current.Pos.Line, fmt.Sprintf("function_group %s references unknown function %q", name, member))
				}
			}
			checker.groups[name] = append([]string(nil), current.Functions...)
		case parser.NamespaceStatement:
			checker.collectFunctionGroupStatements(current.Body, namespace+current.Name+".", source)
		case parser.AliasFunctionStatement:
			checker.collectFunctionGroupStatements(current.Body, namespace, source)
		}
	}
}

func (checker *TypeChecker) collectFunctions(unit sourceUnit, namespace string, globalNamespace bool) {
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
			checker.collectFunctions(sourceUnit{Path: unit.Path, Text: body, ModuleFunctionFilter: unit.ModuleFunctionFilter}, namespace+name+".", globalNamespace || isGlobalNamespaceAt(unit.Text, nextNamespace))
			index = closeBrace + 1
			continue
		}

		if nextFunction == -1 {
			return
		}
		if aliasStart, ok := aliasFunctionStartBefore(unit.Text, nextFunction); ok {
			end := findAliasFunctionEnd(unit.Text, aliasStart)
			if end == -1 {
				index = nextFunction + len("function")
			} else {
				index = end
			}
			continue
		}

		fn, end, err := parseFunction(unit, nextFunction, namespace)
		if err != nil {
			checker.addError(unit.Path, lineAt(unit.Text, nextFunction), err.Error())
			index = nextFunction + len("function")
			continue
		}

		if _, exists := checker.functions[fn.Name]; exists {
			checker.addError(fn.File, fn.Line, fmt.Sprintf("function %q is already defined", fn.Name))
		} else if unit.ModuleFunctionFilter == nil || unit.ModuleFunctionFilter[fn.Name] {
			checker.functions[fn.Name] = fn
			if globalNamespace {
				checker.globalFunctions[unqualifiedFunctionName(fn.Name)] = append(checker.globalFunctions[unqualifiedFunctionName(fn.Name)], fn.Name)
			}
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

func (checker *TypeChecker) collectEnums(program file.Program) {
	parsed := parser.ParseLoadedProgram(program)
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectEnumStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectEnumStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.EnumStatement:
			if _, exists := checker.enums[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("enum %q is already defined", current.Name))
				continue
			}
			enum := enumSymbol{Name: current.Name, Variants: map[string]enumVariantSymbol{}, File: source, Line: current.Pos.Line}
			seenOrdinals := map[int]string{}
			for _, variant := range current.Variants {
				if _, exists := enum.Variants[variant.Name]; exists {
					checker.addError(source, variant.Pos.Line, fmt.Sprintf("enum %s variant %q is already defined", current.Name, variant.Name))
					continue
				}
				if previous, exists := seenOrdinals[variant.Ordinal]; exists {
					checker.addError(source, variant.Pos.Line, fmt.Sprintf("enum %s ordinal %d is already used by %s", current.Name, variant.Ordinal, previous))
					continue
				}
				seenOrdinals[variant.Ordinal] = variant.Name
				enum.Variants[variant.Name] = enumVariantSymbol{Name: variant.Name, Ordinal: variant.Ordinal, Line: variant.Pos.Line}
			}
			checker.enums[current.Name] = enum
		case parser.NamespaceStatement:
			checker.collectEnumStatements(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.collectEnumStatements(current.Body, source)
		case parser.AliasFunctionStatement:
			checker.collectEnumStatements(current.Body, source)
		}
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
		case parser.AliasFunctionStatement:
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
		case parser.AliasFunctionStatement:
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
		case parser.AliasFunctionStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.FunctionStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.IfStatement:
			checker.collectAliasStatements(current.Consequence, source)
			checker.collectAliasStatements(current.Alternative, source)
			if current.ElseIf != nil {
				checker.collectAliasStatements([]parser.Statement{*current.ElseIf}, source)
			}
		case parser.MatchStatement:
			for _, matchCase := range current.Cases {
				checker.collectAliasStatements(matchCase.Body, source)
			}
		case parser.LoopStatement:
			checker.collectAliasStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) collectGlobals(unit sourceUnit) {
	text := maskBlocks(unit.Text)
	for _, stmt := range splitStatements(text) {
		decl, ok := parseGlobalLikeDeclaration(stmt.Text)
		if !ok {
			continue
		}

		decl.File = unit.Path
		decl.Line = lineAt(text, stmt.Start)
		if _, exists := checker.globals[decl.Name]; exists {
			checker.addError(unit.Path, decl.Line, fmt.Sprintf("global variable %q is already defined", decl.Name))
			continue
		}
		if decl.Type != anyType {
			checker.checkDeclaredType(decl.Type, unit.Path, decl.Line)
		}

		if decl.Expression != "" {
			if decl.Scope == "const" && !isCompileTimeConstantExpression(decl.Expression) {
				checker.addError(unit.Path, decl.Line, fmt.Sprintf("const %s requires a compile-time constant initializer", decl.Name))
			}
			exprType := checker.inferExpression(decl.Expression, map[string]variableSymbol{}, unit.Path, decl.Line)
			if decl.Inferred && decl.Type == anyType {
				decl.Type = exprType
			}
			if !isAssignable(decl.Type, exprType) {
				checker.addError(unit.Path, decl.Line, fmt.Sprintf("cannot assign %s to global %s %s", exprType, decl.Type, decl.Name))
			}
		} else if decl.Inferred && decl.Type == anyType {
			checker.addError(unit.Path, decl.Line, fmt.Sprintf("%s %s requires an initializer", decl.Scope, decl.Name))
			decl.Type = anyType
		}

		checker.globals[decl.Name] = variableSymbol{
			Name:      decl.Name,
			Type:      decl.Type,
			KnownSome: isKnownSomeInitializer(decl.Expression),
			Mutable:   decl.Mutable,
			File:      unit.Path,
			Line:      decl.Line,
		}
	}
}

func (checker *TypeChecker) checkTopLevelCalls(unit sourceUnit) {
	text := maskBlocks(unit.Text)
	for _, stmt := range splitStatements(text) {
		current := trimStatementPrefix(stmt.Text)
		if strings.HasPrefix(current, "global ") || strings.HasPrefix(current, "import ") || strings.HasPrefix(current, "alias ") ||
			strings.HasPrefix(current, "module(") || strings.HasPrefix(current, "module_caller(") ||
			strings.HasPrefix(current, "run ") || strings.HasPrefix(current, "run{") || strings.HasPrefix(current, "run {") || current == "" {
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
	for _, returnValue := range fn.ReturnTypes {
		if returnValue.Name == "" {
			continue
		}
		locals[returnValue.Name] = variableSymbol{Name: returnValue.Name, Type: returnValue.Type, Mutable: returnValue.Mutable, File: fn.File, Line: fn.Line}
	}

	for nestedName := range collectNestedFunctionNames(fn.Body) {
		locals[nestedName] = variableSymbol{Name: nestedName, Type: anyType}
	}
	for catchName := range collectCatchNames(fn.Body) {
		locals[catchName] = variableSymbol{Name: catchName, Type: anyType}
	}
	for loopName := range collectEvaluationAssignmentNames(fn.Body) {
		locals[loopName] = variableSymbol{Name: loopName, Type: anyType, Mutable: true}
	}
	for _, param := range fn.Params {
		if param.Default == "" {
			continue
		}
		defaultType := checker.inferExpression(param.Default, locals, fn.File, fn.Line)
		if param.Type == anyType {
			param.Type = defaultType
			locals[param.Name] = param
		}
		if !isAssignable(param.Type, defaultType) {
			checker.addError(fn.File, fn.Line, fmt.Sprintf("parameter %s default expects %s, got %s", param.Name, param.Type, defaultType))
		}
	}
	checker.checkFunctionNullSafety(fn)
	if parsedFn, ok := parseFunctionBodyForSemanticCheck(fn); ok {
		checker.checkSemanticStatements(fn, parsedFn.Body, locals, map[string]bool{})
	}
}

func (checker *TypeChecker) checkTupleReturn(fn functionSymbol, expr string, locals map[string]variableSymbol, line int) {
	parts := splitTopLevel(expr, ',')
	if len(parts) != len(fn.ReturnTypes) {
		checker.addError(fn.File, line, fmt.Sprintf("function %s returns %d value(s), got %d", fn.Name, len(fn.ReturnTypes), len(parts)))
		return
	}
	for index, part := range parts {
		exprType := checker.inferExpression(part, locals, fn.File, line)
		expected := fn.ReturnTypes[index]
		if !isAssignable(expected.Type, exprType) {
			name := fmt.Sprintf("return value %d", index+1)
			if expected.Name != "" {
				name = fmt.Sprintf("return value %q", expected.Name)
			}
			checker.addError(fn.File, line, fmt.Sprintf("function %s %s expects %s but got %s", fn.Name, name, expected.Type, exprType))
		}
	}
}

func parseFunctionBodyForSemanticCheck(fn functionSymbol) (parser.FunctionStatement, bool) {
	source := fmt.Sprintf("function __Semantic() : T {\n%s\n}", fn.Body)
	program, errors := parser.Parse(source)
	if len(errors) != 0 || len(program.Statements) == 0 {
		return parser.FunctionStatement{}, false
	}
	parsedFn, ok := program.Statements[0].(parser.FunctionStatement)
	return parsedFn, ok
}

func (checker *TypeChecker) checkSemanticStatements(fn functionSymbol, statements []parser.Statement, locals map[string]variableSymbol, declared map[string]bool) {
	for _, stmt := range statements {
		checker.checkSemanticStatement(fn, stmt, locals, declared)
	}
}

func (checker *TypeChecker) checkSemanticStatement(fn functionSymbol, stmt parser.Statement, locals map[string]variableSymbol, declared map[string]bool) {
	line := semanticLine(fn, stmt.Position())
	switch current := stmt.(type) {
	case parser.VariableStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		if current.Scope == "global" || current.Exported {
			return
		}
		inferredType := ""
		typeName := normalizeType(current.Type)
		if typeName != anyType {
			checker.checkDeclaredType(typeName, fn.File, line)
		}
		if current.Expression.Node != nil {
			exprSource := expressionSource(current.Expression)
			if current.Scope == "const" && !isCompileTimeConstantExpression(exprSource) {
				checker.addError(fn.File, line, fmt.Sprintf("const %s requires a compile-time constant initializer", current.Name))
			}
			exprType := checker.inferExpression(exprSource, locals, fn.File, line)
			if current.Inferred && typeName == anyType {
				typeName = exprType
			}
			if !isAssignable(typeName, exprType) {
				checker.addError(fn.File, line, fmt.Sprintf("cannot assign %s to local %s %s", exprType, typeName, current.Name))
			}
			if !current.Inferred && typeName == anyType && exprType != anyType {
				inferredType = exprType
			}
			if !current.Lazy {
				checker.markMovedFromExpression(current.Expression, locals)
			}
		}
		if isDiscardIdentifier(current.Name) {
			return
		}
		locals[current.Name] = variableSymbol{
			Name:         current.Name,
			Type:         typeName,
			InferredType: inferredType,
			KnownSome:    isKnownSomeInitializer(expressionSource(current.Expression)),
			Mutable:      current.Mutable,
			File:         fn.File,
			Line:         line,
		}
		declared[current.Name] = true
	case parser.ReturnStatement:
		if len(current.Values) != 0 {
			parts := make([]string, 0, len(current.Values))
			for _, value := range current.Values {
				parts = append(parts, expressionSource(value))
				checker.checkSemanticExpression(fn, value, locals, line)
			}
			checker.checkTupleReturn(fn, strings.Join(parts, ","), locals, line)
			return
		}
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		exprType := checker.inferExpression(expressionSource(current.Expression), locals, fn.File, line)
		if len(fn.ReturnTypes) == 0 && !isAssignable(fn.ReturnType, exprType) {
			checker.addError(fn.File, line, fmt.Sprintf("function %s returns %s but return expression is %s", fn.Name, fn.ReturnType, exprType))
		}
	case parser.ThrowStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
	case parser.AssignmentStatement:
		assignment := assignmentStatement{Target: expressionSource(current.Target), Op: current.Operator, Expr: expressionSource(current.Expression)}
		checker.checkAssignment(assignment, locals, fn.File, line)
		checker.markMovedFromExpression(current.Expression, locals)
	case parser.ExpressionStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
	case parser.IfStatement:
		checker.checkSemanticExpression(fn, current.Condition, locals, line)
		checker.checkSemanticChildBlock(fn, current.Consequence, locals)
		if current.ElseIf != nil {
			checker.checkSemanticChildBlock(fn, []parser.Statement{*current.ElseIf}, locals)
		}
		checker.checkSemanticChildBlock(fn, current.Alternative, locals)
	case parser.MatchStatement:
		checker.checkSemanticExpression(fn, current.Value, locals, line)
		for _, matchCase := range current.Cases {
			checker.checkSemanticExpression(fn, matchCase.Pattern, locals, semanticLine(fn, matchCase.Pos))
			checker.checkSemanticChildBlock(fn, matchCase.Body, locals)
		}
	case parser.LoopStatement:
		checker.checkSemanticLoop(fn, current, locals, line)
	case parser.TryCatchStatement:
		checker.checkSemanticChildBlock(fn, current.TryBody, locals)
		catchLocals := copyLocals(locals)
		catchLocals[current.ErrorName] = variableSymbol{Name: current.ErrorName, Type: anyType, File: fn.File, Line: line}
		checker.checkSemanticChildBlockWithLocals(fn, current.CatchBody, locals, catchLocals, map[string]bool{current.ErrorName: true})
	case parser.DeferStatement:
		if current.Stmt != nil {
			checker.checkSemanticChildBlock(fn, []parser.Statement{current.Stmt}, locals)
		}
		checker.checkSemanticChildBlock(fn, current.Body, locals)
	case parser.RunStatement:
		if current.Stmt != nil {
			checker.checkSemanticChildBlock(fn, []parser.Statement{current.Stmt}, locals)
		}
		checker.checkSemanticChildBlock(fn, current.Body, locals)
	case parser.PrivateBlockStatement:
		checker.checkSemanticChildBlock(fn, current.Body, locals)
	case parser.FunctionStatement:
		return
	}
}

func (checker *TypeChecker) checkSemanticExpression(fn functionSymbol, expr parser.Expression, locals map[string]variableSymbol, line int) {
	if expr.Node == nil || len(expr.Tokens) == 0 {
		return
	}
	checker.inferExpression(expressionSource(expr), locals, fn.File, line)
}

func (checker *TypeChecker) checkSemanticLoop(fn functionSymbol, stmt parser.LoopStatement, locals map[string]variableSymbol, line int) {
	loopLocals := copyLocals(locals)
	declared := map[string]bool{}
	if init, condition, post, ok := parseCStyleScopeHeader(stmt.Header); ok {
		checker.checkSemanticLoopHeaderPart(fn, init, loopLocals, declared, line)
		checker.checkSemanticExpression(fn, condition, loopLocals, line)
		checker.checkSemanticLoopHeaderPart(fn, post, loopLocals, declared, line)
		checker.checkSemanticChildBlockWithLocals(fn, stmt.Body, locals, loopLocals, declared)
		return
	}
	if iterator, iterable, ok := parseRangeScopeHeader(stmt.Header); ok {
		checker.checkSemanticExpression(fn, iterable, locals, line)
		loopLocals[iterator] = variableSymbol{Name: iterator, Type: "Int", File: fn.File, Line: line}
		declared[iterator] = true
		checker.checkSemanticChildBlockWithLocals(fn, stmt.Body, locals, loopLocals, declared)
		return
	}
	if name, expr, ok := parseEvaluationScopeHeader(stmt.Header); ok {
		checker.checkSemanticExpression(fn, expr, locals, line)
		loopLocals[name] = variableSymbol{Name: name, Type: anyType, Mutable: true, File: fn.File, Line: line}
		declared[name] = true
	} else {
		checker.checkSemanticExpression(fn, stmt.Header, locals, line)
	}
	checker.checkSemanticChildBlockWithLocals(fn, stmt.Body, locals, loopLocals, declared)
}

func (checker *TypeChecker) checkSemanticLoopHeaderPart(fn functionSymbol, expr parser.Expression, locals map[string]variableSymbol, declared map[string]bool, line int) {
	if len(expr.Tokens) == 0 {
		return
	}
	if stmt, ok := semanticLoopHeaderStatement(expr); ok {
		checker.checkSemanticStatement(fn, stmt, locals, declared)
		return
	}
	checker.checkSemanticExpression(fn, expr, locals, line)
}

func semanticLoopHeaderStatement(expr parser.Expression) (parser.Statement, bool) {
	tokens := expr.Tokens
	if len(tokens) == 0 {
		return nil, false
	}
	if tokens[0].Type == lexer.TokenLocal || tokens[0].Type == lexer.TokenLet || tokens[0].Type == lexer.TokenConst {
		parserSource := expressionTokensSource(tokens) + ";"
		program, errors := parser.Parse(parserSource)
		if len(errors) == 0 && len(program.Statements) == 1 {
			return program.Statements[0], true
		}
	}
	if index := assignmentOperatorIndex(tokens); index != -1 {
		target := parser.Expression{Tokens: tokens[:index], Node: parser.ParseExpressionTokens(tokens[:index])}
		value := parser.Expression{Tokens: tokens[index+1:], Node: parser.ParseExpressionTokens(tokens[index+1:])}
		return parser.AssignmentStatement{
			Pos:        parser.Position{Line: tokens[0].Line, Column: tokens[0].Column},
			Target:     target,
			Operator:   tokens[index].Literal,
			Expression: value,
		}, true
	}
	return nil, false
}

func (checker *TypeChecker) checkSemanticChildBlock(fn functionSymbol, statements []parser.Statement, parent map[string]variableSymbol) {
	child := copyLocals(parent)
	checker.checkSemanticChildBlockWithLocals(fn, statements, parent, child, map[string]bool{})
}

func (checker *TypeChecker) checkSemanticChildBlockWithLocals(fn functionSymbol, statements []parser.Statement, parent map[string]variableSymbol, child map[string]variableSymbol, declared map[string]bool) {
	checker.checkSemanticStatements(fn, statements, child, declared)
	for name, variable := range child {
		if declared[name] {
			continue
		}
		if _, exists := parent[name]; exists {
			parent[name] = variable
		}
	}
}

func (checker *TypeChecker) markMovedFromExpression(expr parser.Expression, locals map[string]variableSymbol) {
	if movedName, ok := movedIdentifier(expressionSource(expr)); ok {
		if movedVariable, exists := locals[movedName]; exists {
			movedVariable.Type = movedType
			locals[movedName] = movedVariable
		}
	}
}

func semanticLine(fn functionSymbol, pos parser.Position) int {
	if pos.Line <= 0 {
		return fn.Line
	}
	return fn.Line + pos.Line - 2
}

func expressionTokensSource(tokens []lexer.Token) string {
	var builder strings.Builder
	var previous lexer.Token
	hasPrevious := false
	for _, token := range tokens {
		literal := tokenSourceLiteral(token)
		if literal == "" {
			continue
		}
		if builder.Len() > 0 && tokenNeedsLeadingSpace(previous, token, hasPrevious) {
			builder.WriteByte(' ')
		}
		builder.WriteString(literal)
		previous = token
		hasPrevious = true
	}
	return builder.String()
}

func expressionSource(expr parser.Expression) string {
	return expressionTokensSource(expr.Tokens)
}

func tokenSourceLiteral(token lexer.Token) string {
	switch token.Type {
	case lexer.TokenString:
		return strconv.Quote(token.Literal)
	case lexer.TokenChar:
		return "'" + strings.ReplaceAll(token.Literal, "'", "\\'") + "'"
	default:
		return token.Literal
	}
}

func tokenNeedsLeadingSpace(previous lexer.Token, token lexer.Token, hasPrevious bool) bool {
	if hasPrevious {
		switch previous.Type {
		case lexer.TokenDot, lexer.TokenNamespaceAccess, lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace:
			return false
		}
	}
	switch token.Type {
	case lexer.TokenLeftBrace, lexer.TokenRightBrace, lexer.TokenLeftSquareBrace, lexer.TokenRightSquareBrace,
		lexer.TokenDot, lexer.TokenNamespaceAccess, lexer.TokenComma, lexer.TokenSemicolon, lexer.TokenQuestion,
		lexer.TokenBang:
		return false
	default:
		return true
	}
}

type nullSafetySymbol struct {
	Type      string
	KnownSome bool
}

func (checker *TypeChecker) checkFunctionNullSafety(fn functionSymbol) {
	source := fmt.Sprintf("function __NullSafety() : T {\n%s\n}", fn.Body)
	program, errors := parser.Parse(source)
	if len(errors) != 0 || len(program.Statements) == 0 {
		return
	}
	parsedFn, ok := program.Statements[0].(parser.FunctionStatement)
	if !ok {
		return
	}
	env := map[string]nullSafetySymbol{}
	for name, global := range checker.globals {
		env[name] = nullSafetySymbol{Type: normalizeType(global.Type), KnownSome: global.KnownSome}
	}
	for _, param := range fn.Params {
		env[param.Name] = nullSafetySymbol{Type: normalizeType(param.Type)}
	}
	checker.checkNullSafetyStatements(parsedFn.Body, env, fn.File, fn.Line)
}

func (checker *TypeChecker) checkNullSafetyStatements(statements []parser.Statement, env map[string]nullSafetySymbol, source string, baseLine int) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.VariableStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
			if isDiscardIdentifier(current.Name) {
				continue
			}
			typeName := normalizeType(current.Type)
			if current.Inferred && typeName == anyType {
				typeName = checker.nullSafetyExpressionType(current.Expression.Node, env)
			}
			if _, ok := optionElementType(typeName); ok {
				env[current.Name] = nullSafetySymbol{Type: typeName, KnownSome: nullSafetyExpressionIsKnownSome(current.Expression.Node)}
			} else {
				env[current.Name] = nullSafetySymbol{Type: typeName}
			}
		case parser.AssignmentStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
			if target, ok := current.Target.Node.(parser.IdentifierExpression); ok {
				if symbol, exists := env[target.Name]; exists {
					symbol.KnownSome = nullSafetyExpressionIsKnownSome(current.Expression.Node)
					env[target.Name] = symbol
				}
			}
		case parser.ReturnStatement:
			if len(current.Values) == 0 {
				checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
			}
			for _, expr := range current.Values {
				checker.checkNullSafetyExpression(expr.Node, env, source, baseLine)
			}
		case parser.ThrowStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
		case parser.ExpressionStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
		case parser.IfStatement:
			checker.checkNullSafetyExpression(current.Condition.Node, env, source, baseLine)
			guarded := copyNullSafetyEnv(env)
			for name := range nullSafetySomeGuards(current.Condition.Node) {
				if symbol, ok := guarded[name]; ok {
					if _, option := optionElementType(symbol.Type); option {
						symbol.KnownSome = true
						guarded[name] = symbol
					}
				}
			}
			checker.checkNullSafetyStatements(current.Consequence, guarded, source, baseLine)
			if current.ElseIf != nil {
				checker.checkNullSafetyStatements([]parser.Statement{*current.ElseIf}, copyNullSafetyEnv(env), source, baseLine)
			}
			checker.checkNullSafetyStatements(current.Alternative, copyNullSafetyEnv(env), source, baseLine)
		case parser.LoopStatement:
			checker.checkNullSafetyExpression(current.Header.Node, env, source, baseLine)
			guarded := copyNullSafetyEnv(env)
			for name := range nullSafetySomeGuards(current.Header.Node) {
				if symbol, ok := guarded[name]; ok {
					if _, option := optionElementType(symbol.Type); option {
						symbol.KnownSome = true
						guarded[name] = symbol
					}
				}
			}
			checker.checkNullSafetyStatements(current.Body, guarded, source, baseLine)
		case parser.TryCatchStatement:
			checker.checkNullSafetyStatements(current.TryBody, copyNullSafetyEnv(env), source, baseLine)
			catchEnv := copyNullSafetyEnv(env)
			catchEnv[current.ErrorName] = nullSafetySymbol{Type: anyType}
			checker.checkNullSafetyStatements(current.CatchBody, catchEnv, source, baseLine)
		case parser.DeferStatement:
			checker.checkNullSafetyStatements(current.Body, copyNullSafetyEnv(env), source, baseLine)
			if current.Stmt != nil {
				checker.checkNullSafetyStatements([]parser.Statement{current.Stmt}, copyNullSafetyEnv(env), source, baseLine)
			}
		case parser.RunStatement:
			checker.checkNullSafetyStatements(current.Body, copyNullSafetyEnv(env), source, baseLine)
			if current.Stmt != nil {
				checker.checkNullSafetyStatements([]parser.Statement{current.Stmt}, copyNullSafetyEnv(env), source, baseLine)
			}
		case parser.PrivateBlockStatement:
			checker.checkNullSafetyStatements(current.Body, copyNullSafetyEnv(env), source, baseLine)
		case parser.MatchStatement:
			checker.checkNullSafetyExpression(current.Value.Node, env, source, baseLine)
			for _, matchCase := range current.Cases {
				checker.checkNullSafetyExpression(matchCase.Pattern.Node, env, source, baseLine)
				checker.checkNullSafetyStatements(matchCase.Body, copyNullSafetyEnv(env), source, baseLine)
			}
		}
	}
}

func (checker *TypeChecker) checkNullSafetyExpression(expr parser.ExpressionNode, env map[string]nullSafetySymbol, source string, baseLine int) {
	switch current := expr.(type) {
	case nil, parser.LiteralExpression, parser.IdentifierExpression:
		return
	case parser.GroupExpression:
		checker.checkNullSafetyExpression(current.Inner, env, source, baseLine)
	case parser.UnaryExpression:
		checker.checkNullSafetyExpression(current.Right, env, source, baseLine)
	case parser.BinaryExpression:
		checker.checkNullSafetyExpression(current.Left, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Right, env, source, baseLine)
	case parser.CallExpression:
		checker.checkNullSafetyExpression(current.Callee, env, source, baseLine)
		for _, arg := range current.Arguments {
			checker.checkNullSafetyExpression(arg, env, source, baseLine)
		}
	case parser.IndexExpression:
		checker.checkNullSafetyExpression(current.Target, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Index, env, source, baseLine)
	case parser.SelectorExpression:
		checker.checkNullSafetyExpression(current.Target, env, source, baseLine)
		if current.Field == "value" {
			if target, ok := current.Target.(parser.IdentifierExpression); ok {
				if symbol, exists := env[target.Name]; exists {
					if _, option := optionElementType(symbol.Type); option && !symbol.KnownSome {
						line := baseLine + selectorLine(current.Target) - 1
						checker.addError(source, line, fmt.Sprintf("Option value %s must be checked with .some before accessing .value", target.Name))
					}
				}
			}
		}
	case parser.CastExpression:
		checker.checkNullSafetyExpression(current.Value, env, source, baseLine)
	case parser.NullCheckExpression:
		checker.checkNullSafetyExpression(current.Value, env, source, baseLine)
	case parser.PropagateExpression:
		checker.checkNullSafetyExpression(current.Value, env, source, baseLine)
	case parser.ConditionalExpression:
		checker.checkNullSafetyExpression(current.Condition, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Consequence, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Alternative, env, source, baseLine)
	case parser.ListExpression:
		for _, item := range current.Items {
			checker.checkNullSafetyExpression(item, env, source, baseLine)
		}
	case parser.ListComprehensionExpression:
		checker.checkNullSafetyExpression(current.Iterable, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Condition, env, source, baseLine)
		checker.checkNullSafetyExpression(current.Value, env, source, baseLine)
	case parser.MapExpression:
		for _, entry := range current.Entries {
			checker.checkNullSafetyExpression(entry.Key, env, source, baseLine)
			checker.checkNullSafetyExpression(entry.Value, env, source, baseLine)
		}
	case parser.LambdaExpression:
		lambdaEnv := copyNullSafetyEnv(env)
		for _, param := range current.Params {
			lambdaEnv[param.Name] = nullSafetySymbol{Type: normalizeType(param.Type)}
		}
		checker.checkNullSafetyStatements(current.Body, lambdaEnv, source, baseLine)
	}
}

func (checker *TypeChecker) nullSafetyExpressionType(expr parser.ExpressionNode, env map[string]nullSafetySymbol) string {
	switch current := expr.(type) {
	case parser.IdentifierExpression:
		if symbol, ok := env[current.Name]; ok {
			return symbol.Type
		}
	case parser.CallExpression:
		if callee, ok := current.Callee.(parser.IdentifierExpression); ok {
			switch callee.Name {
			case "Some":
				return "Option[T]"
			case "None":
				return "Option[T]"
			}
		}
	}
	return anyType
}

func nullSafetyExpressionIsKnownSome(expr parser.ExpressionNode) bool {
	call, ok := expr.(parser.CallExpression)
	if !ok {
		return false
	}
	callee, ok := call.Callee.(parser.IdentifierExpression)
	return ok && callee.Name == "Some"
}

func nullSafetySomeGuards(expr parser.ExpressionNode) map[string]bool {
	guards := map[string]bool{}
	switch current := expr.(type) {
	case parser.SelectorExpression:
		if current.Field == "some" {
			if target, ok := current.Target.(parser.IdentifierExpression); ok {
				guards[target.Name] = true
			}
		}
	case parser.GroupExpression:
		for name := range nullSafetySomeGuards(current.Inner) {
			guards[name] = true
		}
	case parser.BinaryExpression:
		for name := range nullSafetySomeGuards(current.Left) {
			guards[name] = true
		}
		for name := range nullSafetySomeGuards(current.Right) {
			guards[name] = true
		}
	case parser.UnaryExpression:
		if current.Operator != "not" {
			for name := range nullSafetySomeGuards(current.Right) {
				guards[name] = true
			}
		}
	}
	return guards
}

func copyNullSafetyEnv(env map[string]nullSafetySymbol) map[string]nullSafetySymbol {
	copied := make(map[string]nullSafetySymbol, len(env))
	for name, symbol := range env {
		copied[name] = symbol
	}
	return copied
}

func selectorLine(expr parser.ExpressionNode) int {
	// The string checker reports function-level lines today; keep this pass aligned
	// until expression nodes carry source positions.
	return 1
}

type variableDeclaration struct {
	Scope      string
	Inferred   bool
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
	async := functionHasPrefixKeyword(unit.Text, start, "async")
	inline := functionHasPrefixKeyword(unit.Text, start, "inline")
	private := functionHasPrefixKeyword(unit.Text, start, "private")
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
	var returnTypes []returnValueSymbol
	if strings.HasPrefix(afterParams, ":") {
		parsedReturnType, parsedReturnTypes, err := parseReturnSignatureText(strings.TrimSpace(afterParams[1:]))
		if err != nil {
			return functionSymbol{}, openBrace, err
		}
		returnType = parsedReturnType
		returnTypes = parsedReturnTypes
	}
	returnType = applyFunctionTypeRestrictions(returnType, typeRestrictions)
	for index := range returnTypes {
		returnTypes[index].Type = applyFunctionTypeRestrictions(returnTypes[index].Type, typeRestrictions)
	}
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
		ReturnTypes:        returnTypes,
		Async:              async,
		Inline:             inline,
		Private:            private,
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

func parseReturnSignatureText(input string) (string, []returnValueSymbol, error) {
	input = strings.TrimSpace(input)
	if !strings.HasPrefix(input, "(") {
		return normalizeType(input), nil, nil
	}
	closeIndex := findMatchingIn(input, 0, '(', ')')
	if closeIndex == -1 {
		return "", nil, fmt.Errorf("function return tuple is missing a closing ')'")
	}
	inner := strings.TrimSpace(input[1:closeIndex])
	if inner == "" {
		return "", nil, fmt.Errorf("function return tuple must contain at least one type")
	}
	parts := splitTopLevel(inner, ',')
	returnValues := make([]returnValueSymbol, 0, len(parts))
	typeParts := make([]string, 0, len(parts))
	for _, part := range parts {
		current := strings.TrimSpace(part)
		mutable := false
		if strings.HasPrefix(current, "mut ") {
			mutable = true
			current = strings.TrimSpace(strings.TrimPrefix(current, "mut"))
		}
		name := ""
		typeName := current
		if colon := strings.Index(current, ":"); colon != -1 {
			name = strings.TrimSpace(current[:colon])
			typeName = strings.TrimSpace(current[colon+1:])
		}
		typeName = normalizeType(typeName)
		if typeName == "" {
			return "", nil, fmt.Errorf("function return tuple contains an empty type")
		}
		returnValues = append(returnValues, returnValueSymbol{Name: name, Type: typeName, Mutable: mutable})
		typeParts = append(typeParts, typeName)
	}
	return "(" + strings.Join(typeParts, ",") + ")", returnValues, nil
}

func functionHasPrefixKeyword(input string, functionStart int, keyword string) bool {
	prefix := strings.TrimRightFunc(input[:functionStart], unicode.IsSpace)
	if !strings.HasSuffix(prefix, keyword) {
		lineStart := strings.LastIndexAny(prefix, "\n;}")
		marker := strings.TrimSpace(prefix[lineStart+1:])
		for _, field := range strings.Fields(marker) {
			if field == keyword {
				return true
			}
		}
		return false
	}
	before := strings.TrimSpace(prefix[:len(prefix)-len(keyword)])
	return before == "" || strings.HasSuffix(before, "\n") || strings.HasSuffix(before, ";") || strings.HasSuffix(before, "}")
}

func parseParams(input string) ([]variableSymbol, error) {
	if strings.TrimSpace(input) == "" {
		return nil, nil
	}

	parts := splitTopLevel(input, ',')
	params := make([]variableSymbol, 0, len(parts))
	for _, part := range parts {
		defaultValue := ""
		evalAssign := findTopLevelOperator(part, []string{":="})
		if evalAssign != -1 {
			name := strings.TrimSpace(part[:evalAssign])
			defaultValue = strings.TrimSpace(part[evalAssign+len(":="):])
			mutable := false
			if strings.HasPrefix(name, "mut ") {
				mutable = true
				name = strings.TrimSpace(strings.TrimPrefix(name, "mut"))
			}
			if name == "" || defaultValue == "" {
				return nil, fmt.Errorf("function parameter %q must include a name and default value", strings.TrimSpace(part))
			}
			params = append(params, variableSymbol{Name: name, Type: anyType, Mutable: mutable, Default: defaultValue})
			continue
		}
		if assignIndex := findTopLevelOperator(part, []string{"="}); assignIndex != -1 {
			defaultValue = strings.TrimSpace(part[assignIndex+1:])
			part = strings.TrimSpace(part[:assignIndex])
			if defaultValue == "" {
				return nil, fmt.Errorf("function parameter %q has an empty default value", strings.TrimSpace(part))
			}
		}
		colon := strings.Index(part, ":")
		if colon == -1 {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type or name := Default", strings.TrimSpace(part))
		}
		name := strings.TrimSpace(part[:colon])
		mutable := false
		if strings.HasPrefix(name, "mut ") {
			mutable = true
			name = strings.TrimSpace(strings.TrimPrefix(name, "mut"))
		}
		typeName := normalizeType(part[colon+1:])
		if name == "" || typeName == "" {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		if !isKnownType(typeName) {
			return nil, fmt.Errorf("function parameter %s uses unknown type %s", name, typeName)
		}
		params = append(params, variableSymbol{Name: name, Type: typeName, Mutable: mutable, Default: defaultValue})
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

	assignIndex := findTopLevelAssignment(rest)
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
		Scope:      scope,
		Mutable:    mutable,
		Exported:   exported,
		Type:       typeName,
		Name:       name,
		Expression: expr,
	}, true
}

func isGlobalNamespaceAt(text string, namespaceIndex int) bool {
	prefix := strings.TrimSpace(text[:namespaceIndex])
	if prefix == "" {
		return false
	}
	fields := strings.Fields(prefix)
	return len(fields) > 0 && fields[len(fields)-1] == "global"
}

func unqualifiedFunctionName(name string) string {
	if index := strings.LastIndex(name, "."); index != -1 {
		return name[index+1:]
	}
	return name
}

func parseGlobalLikeDeclaration(stmt string) (variableDeclaration, bool) {
	if strings.HasPrefix(trimStatementPrefix(stmt), "global namespace ") {
		return variableDeclaration{}, false
	}
	if decl, ok := parseVariableDeclaration(stmt, "global"); ok {
		return decl, true
	}
	current := trimStatementPrefix(stmt)
	switch {
	case strings.HasPrefix(current, "val "):
		return parseInferredDeclaration(current, "global", false, "val")
	case strings.HasPrefix(current, "var "):
		return parseInferredDeclaration(current, "global", true, "var")
	case strings.HasPrefix(current, "const "):
		return parseInferredDeclaration(current, "const", false, "const")
	default:
		return variableDeclaration{}, false
	}
}

func parseFunctionLocalDeclaration(stmt string) (variableDeclaration, bool) {
	if decl, ok := parseVariableDeclaration(stmt, "local"); ok {
		return decl, true
	}
	current := trimStatementPrefix(stmt)
	switch {
	case strings.HasPrefix(current, "let "):
		return parseInferredDeclaration(current, "local", false, "let")
	case strings.HasPrefix(current, "const "):
		return parseInferredDeclaration(current, "const", false, "const")
	default:
		return variableDeclaration{}, false
	}
}

func parseInferredDeclaration(stmt string, scope string, mutable bool, keyword string) (variableDeclaration, bool) {
	rest := strings.TrimSpace(strings.TrimPrefix(stmt, keyword))
	if keyword == "let" && strings.HasPrefix(rest, "mut ") {
		mutable = true
		rest = strings.TrimSpace(strings.TrimPrefix(rest, "mut"))
	}
	assignIndex := findTopLevelAssignment(rest)
	if assignIndex == -1 {
		return variableDeclaration{Scope: scope, Mutable: mutable, Type: anyType}, false
	}
	left := strings.TrimSpace(rest[:assignIndex])
	expr := strings.TrimSpace(rest[assignIndex+1:])
	parts := strings.Fields(left)
	if len(parts) == 0 || len(parts) > 2 {
		return variableDeclaration{}, false
	}
	typeName := anyType
	name := parts[0]
	if len(parts) == 2 {
		typeName = normalizeType(inferredExplicitTypeName(parts[0]))
		name = parts[1]
	}
	return variableDeclaration{
		Scope:      scope,
		Inferred:   true,
		Mutable:    mutable,
		Type:       typeName,
		Name:       name,
		Expression: expr,
	}, true
}

func inferredExplicitTypeName(typeName string) string {
	if typeName == "size" {
		return "Int"
	}
	return typeName
}

func isCompileTimeConstantExpression(expr string) bool {
	tokens := lexer.New(expr).Tokenize()
	if len(tokens) > 0 && tokens[len(tokens)-1].Type == lexer.TokenEOFDescriptor {
		tokens = tokens[:len(tokens)-1]
	}
	if len(tokens) == 0 {
		return false
	}
	return isCompileTimeConstantNode(parser.ParseExpressionTokens(tokens))
}

func isCompileTimeConstantNode(node parser.ExpressionNode) bool {
	switch current := node.(type) {
	case parser.LiteralExpression:
		return true
	case parser.GroupExpression:
		return isCompileTimeConstantNode(current.Inner)
	case parser.UnaryExpression:
		switch current.Operator {
		case "-", "not":
			return isCompileTimeConstantNode(current.Right)
		default:
			return false
		}
	case parser.BinaryExpression:
		return isCompileTimeConstantNode(current.Left) && isCompileTimeConstantNode(current.Right)
	case parser.CastExpression:
		return isCompileTimeConstantNode(current.Value)
	case parser.NullCheckExpression:
		return isCompileTimeConstantNode(current.Value)
	case parser.ConditionalExpression:
		return isCompileTimeConstantNode(current.Condition) &&
			isCompileTimeConstantNode(current.Consequence) &&
			isCompileTimeConstantNode(current.Alternative)
	case parser.ListExpression:
		for _, item := range current.Items {
			if !isCompileTimeConstantNode(item) {
				return false
			}
		}
		return true
	case parser.MapExpression:
		for _, entry := range current.Entries {
			if !isCompileTimeConstantNode(entry.Key) || !isCompileTimeConstantNode(entry.Value) {
				return false
			}
		}
		return true
	case parser.SelectorExpression:
		target, ok := current.Target.(parser.IdentifierExpression)
		return ok && current.Field == "sizeof" && isKnownType(target.Name)
	default:
		return false
	}
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

func findTopLevelAssignment(input string) int {
	index := findTopLevelOperator(input, []string{"="})
	for index != -1 {
		if strings.HasPrefix(input[index:], "==") ||
			(index+1 < len(input) && input[index+1] == '=') ||
			(index > 0 && strings.ContainsAny(input[index-1:index], "=!<>:+-*/")) {
			next := findTopLevelOperator(input[:index], []string{"="})
			index = next
			continue
		}
		return index
	}
	return -1
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
	targetName := ""

	if isDiscardIdentifier(baseName) {
		if assignment.Op != "=" {
			checker.addError(source, line, "discard assignment only supports =")
			return
		}
		checker.inferExpression(assignment.Expr, locals, source, line)
		return
	}

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(baseName); ok {
		baseName = strings.TrimSpace(targetExpr)
		targetName = baseName
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
		targetName = baseName
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
		if variable, ok := locals[targetName]; ok && strings.HasPrefix(normalizeType(variable.Type), "Option[") {
			variable.KnownSome = isKnownSomeInitializer(assignment.Expr)
			locals[targetName] = variable
		}
	}
}

func isDiscardIdentifier(name string) bool {
	return strings.TrimSpace(name) == "_"
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
	if typeName, ok := splitSizeofExpression(expr); ok {
		if isKnownType(typeName) {
			return "Int"
		}
		checker.addError(source, line, fmt.Sprintf("unknown type %s", typeName))
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
	if strings.HasPrefix(expr, "copy ") {
		return checker.inferExpression(strings.TrimSpace(strings.TrimPrefix(expr, "copy")), locals, source, line)
	}
	if strings.HasPrefix(expr, "clone ") {
		return checker.inferExpression(strings.TrimSpace(strings.TrimPrefix(expr, "clone")), locals, source, line)
	}
	if strings.HasPrefix(expr, "await ") {
		valueType := checker.inferExpression(strings.TrimSpace(strings.TrimPrefix(expr, "await")), locals, source, line)
		if inner, ok := awaitableType(valueType); ok {
			return inner
		}
		checker.addError(source, line, fmt.Sprintf("await expects Awaitable, got %s", valueType))
		return anyType
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
	if strings.HasPrefix(expr, "{") && strings.HasSuffix(expr, "}") {
		return "Map[T,T]"
	}
	if expr == "[]" {
		return "List[T]"
	}
	if strings.HasPrefix(expr, "[") && strings.HasSuffix(expr, "]") {
		return checker.inferListLiteral(expr, locals, source, line)
	}
	if strings.HasPrefix(expr, "fun") {
		return checker.inferLambdaExpression(expr, locals, source, line)
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
	if inner, ok := splitPostfixPropagateExpression(expr); ok {
		resultType := checker.inferExpression(inner, locals, source, line)
		okType, _, ok := resultValueTypes(resultType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("! expects Result, got %s", resultType))
			return anyType
		}
		return okType
	}

	if targetExpr, indexExpr, ok := splitTrailingIndexExpression(expr); ok {
		targetType := checker.inferExpression(targetExpr, locals, source, line)
		indexType := checker.inferExpression(indexExpr, locals, source, line)
		return checker.checkIndexExpression(targetType, indexType, source, line)
	}

	if targetExpr, fieldName, ok := splitTrailingSelectorExpression(expr); ok {
		if fieldName == "sizeof" && isKnownType(normalizeType(targetExpr)) {
			return "Int"
		}
		if checker.enumVariantExists(targetExpr, fieldName) {
			return normalizeType(targetExpr)
		}
		targetType := checker.inferExpression(targetExpr, locals, source, line)
		if fieldType, ok := checker.selectorFieldType(targetType, fieldName); ok {
			return fieldType
		}
		if targetType == anyType || targetType == "Table" || strings.HasPrefix(normalizeType(targetType), "Map[") {
			return anyType
		}
		if !checker.functionExists(expr, checker.namespace) && !checker.namespaceExists(expr) {
			checker.addError(source, line, fmt.Sprintf("%s has no field %q", targetType, fieldName))
		}
		return anyType
	}

	if targetExpr, fieldName, ok := splitIdentifierSelectorCallName(expr); ok {
		if variable, exists := checker.lookupVariable(targetExpr, locals); exists {
			targetType := variable.Type
			if variable.InferredType != "" {
				targetType = variable.InferredType
			}
			if fieldType, ok := checker.selectorFieldType(targetType, fieldName); ok {
				return fieldType
			}
			if targetType == anyType || targetType == "Table" || strings.HasPrefix(normalizeType(targetType), "Map[") {
				return anyType
			}
		}
	}

	if calleeExpr, args, ok := parseCallableExpressionCall(expr); ok {
		calleeType := checker.inferExpression(calleeExpr, locals, source, line)
		if calleeType == anyType {
			for _, arg := range args {
				checker.inferExpression(arg, locals, source, line)
			}
			return anyType
		}
		paramTypes, returnType, ok := functionValueType(calleeType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("%s is not callable", calleeType))
			return anyType
		}
		return checker.checkCallbackCall(calleeExpr, paramTypes, returnType, args, locals, source, line)
	}

	if callName, args, ok := parseFunctionCall(expr); ok {
		return checker.checkCall(callName, args, locals, source, line)
	}

	if enumName, variantName, ok := splitIdentifierSelectorCallName(expr); ok && checker.enumVariantExists(enumName, variantName) {
		return normalizeType(enumName)
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
	if checker.functionGroupExists(expr) {
		return anyType
	}

	checker.addError(source, line, fmt.Sprintf("unknown expression %q", expr))
	return anyType
}

func (checker *TypeChecker) aliasMethodType(typeName string, methodName string) (string, bool) {
	typeName = normalizeType(typeName)
	alias, ok := checker.aliasFunctions[typeName]
	if !ok {
		return "", false
	}
	for _, method := range alias.Methods {
		if method.Name != methodName {
			continue
		}
		parts := make([]string, 0, len(method.Params)+1)
		for _, param := range method.Params {
			parts = append(parts, normalizeType(param.Type))
		}
		parts = append(parts, normalizeType(method.ReturnType))
		return "Function[" + strings.Join(parts, ",") + "]", true
	}
	return "", false
}

func (checker *TypeChecker) enumExists(name string) bool {
	_, ok := checker.enums[normalizeType(name)]
	return ok
}

func (checker *TypeChecker) enumVariantExists(enumName string, variantName string) bool {
	enum, ok := checker.enums[normalizeType(enumName)]
	if !ok {
		return false
	}
	_, ok = enum.Variants[variantName]
	return ok
}

func (checker *TypeChecker) selectorFieldType(targetType string, fieldName string) (string, bool) {
	targetType = normalizeType(targetType)
	if methodType, ok := checker.aliasMethodType(targetType, fieldName); ok {
		return methodType, true
	}
	if fieldType, ok := builtinProtocolFieldType(targetType, fieldName); ok {
		return fieldType, true
	}
	if methodType, ok := builtinProtocolMethodType(targetType, fieldName); ok {
		return methodType, true
	}
	if checker.enumExists(targetType) {
		switch fieldName {
		case "ordinal":
			return "Int", true
		case "name", "variant":
			return "String", true
		}
	}
	if optionType, ok := optionElementType(targetType); ok {
		switch fieldName {
		case "value":
			return optionType, true
		case "some":
			return "Bool", true
		}
	}
	if okType, _, ok := resultValueTypes(targetType); ok {
		switch fieldName {
		case "value":
			return okType, true
		case "ok":
			return "Bool", true
		}
	}
	return "", false
}

func builtinProtocolFieldType(targetType string, fieldName string) (string, bool) {
	targetType = normalizeType(targetType)
	if fieldName != "count" {
		return "", false
	}
	if targetType == "String" || targetType == "Table" ||
		strings.HasPrefix(targetType, "List[") ||
		strings.HasPrefix(targetType, "Map[") ||
		strings.HasPrefix(targetType, "SIMD[") ||
		strings.HasPrefix(targetType, "Iterator[") {
		return "Int", true
	}
	return "", false
}

func builtinProtocolMethodType(targetType string, methodName string) (string, bool) {
	targetType = normalizeType(targetType)
	switch methodName {
	case "uppercase", "lowercase":
		if targetType == "String" || targetType == "Char" {
			return "Function[" + targetType + "]", true
		}
	case "times":
		if targetType == "Int" || targetType == "UInt" {
			return "Function[Function[Int,T],T]", true
		}
	}
	return "", false
}

func (checker *TypeChecker) checkDeclaredType(typeName string, source string, line int) {
	typeName = normalizeType(typeName)
	if !isKnownType(typeName) {
		checker.addError(source, line, fmt.Sprintf("unknown type %s", typeName))
		return
	}
	if !isArrayTypeName(typeName) {
		return
	}
	regionName := arrayRegionName(typeName)
	if regionName == "" {
		checker.addError(source, line, fmt.Sprintf("array type %s is missing a region", typeName))
		return
	}
	if _, ok := checker.regions[regionName]; !ok {
		checker.addError(source, line, fmt.Sprintf("array type %s uses unknown region %q", typeName, regionName))
	}
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
	case isArrayTypeName(targetType):
		if !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("Array index must be Int, got %s", indexType))
		}
		elementType, _ := arrayElementType(targetType)
		return elementType
	case strings.HasPrefix(targetType, "Map[") && strings.HasSuffix(targetType, "]"):
		keyType, valueType, ok := indexedMapTypes(targetType)
		if ok && !isAssignable(keyType, indexType) {
			checker.addError(source, line, fmt.Sprintf("Map index expects %s, got %s", keyType, indexType))
		}
		return valueType
	case targetType == "Table":
		return anyType
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
	case isArrayTypeName(targetType):
		if !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("Array index must be Int, got %s", indexType))
		}
		elementType, _ := arrayElementType(targetType)
		return elementType
	case strings.HasPrefix(targetType, "Map[") && strings.HasSuffix(targetType, "]"):
		keyType, valueType, ok := indexedMapTypes(targetType)
		if ok && !isAssignable(keyType, indexType) {
			checker.addError(source, line, fmt.Sprintf("Map index expects %s, got %s", keyType, indexType))
		}
		return valueType
	case targetType == "Table":
		return anyType
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

func (checker *TypeChecker) listLiteralArgumentTypes(expr string, locals map[string]variableSymbol, source string, line int) ([]string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasPrefix(expr, "[") || !strings.HasSuffix(expr, "]") {
		return nil, false
	}
	inner := strings.TrimSpace(expr[1 : len(expr)-1])
	if inner == "" {
		return []string{}, true
	}
	if _, _, _, _, ok := splitListComprehensionLiteral(inner); ok {
		return nil, false
	}
	items := splitTopLevel(inner, ',')
	types := make([]string, 0, len(items))
	for _, item := range items {
		types = append(types, checker.inferExpression(item, locals, source, line))
	}
	return types, true
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
	case "Table":
		if len(args) > 1 {
			checker.addError(source, line, "Table expects 0 to 1 argument(s)")
		}
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return "Table"
	case "iter":
		if len(args) != 1 {
			checker.addError(source, line, "iter expects 1 argument")
			return "Iterator[T]"
		}
		itemType, ok := iterableItemType(checker.inferExpression(args[0], locals, source, line))
		if !ok {
			checker.addError(source, line, "iter expects List, String, Table, Iterator, or range-compatible Int")
			itemType = anyType
		}
		return "Iterator[" + itemType + "]"
	case "next":
		if len(args) != 1 {
			checker.addError(source, line, "next expects 1 argument")
			return "Option[T]"
		}
		iteratorType := checker.inferExpression(args[0], locals, source, line)
		itemType, ok := iteratorItemType(iteratorType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("next expects Iterator, got %s", iteratorType))
			itemType = anyType
		}
		return "Option[" + itemType + "]"
	case "coroutine":
		if len(args) != 1 {
			checker.addError(source, line, "coroutine expects 1 argument")
			return "Coroutine[T]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		_, returnType, ok := functionValueType(argType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("coroutine expects Function, got %s", argType))
			return "Coroutine[T]"
		}
		return "Coroutine[" + returnType + "]"
	case "resume":
		if len(args) != 1 {
			checker.addError(source, line, "resume expects 1 argument")
			return "Option[T]"
		}
		coroutineType := checker.inferExpression(args[0], locals, source, line)
		itemType, ok := coroutineItemType(coroutineType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("resume expects Coroutine, got %s", coroutineType))
			itemType = anyType
		}
		return "Option[" + itemType + "]"
	case "spawn":
		if len(args) < 1 || len(args) > 2 {
			checker.addError(source, line, "spawn expects Function and optional List arguments")
			return "Thread[T]"
		}
		functionType := checker.inferExpression(args[0], locals, source, line)
		paramTypes, returnType, ok := functionValueType(functionType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("spawn expects Function, got %s", functionType))
			return "Thread[T]"
		}
		if len(args) == 2 {
			argsType := checker.inferExpression(args[1], locals, source, line)
			if !strings.HasPrefix(argsType, "List[") {
				checker.addError(source, line, fmt.Sprintf("spawn arguments expect List[T], got %s", argsType))
			}
			if listItems, ok := checker.listLiteralArgumentTypes(args[1], locals, source, line); ok {
				if len(listItems) != len(paramTypes) {
					checker.addError(source, line, fmt.Sprintf("spawn function expects %d argument(s), got %d", len(paramTypes), len(listItems)))
				}
				for index := 0; index < len(listItems) && index < len(paramTypes); index++ {
					if !isAssignable(paramTypes[index], listItems[index]) {
						checker.addError(source, line, fmt.Sprintf("spawn argument %d expects %s, got %s", index+1, paramTypes[index], listItems[index]))
					}
				}
			}
		} else if len(paramTypes) != 0 {
			checker.addError(source, line, fmt.Sprintf("spawn function expects %d argument(s), got 0", len(paramTypes)))
		}
		return "Thread[" + returnType + "]"
	case "join":
		if len(args) != 1 {
			checker.addError(source, line, "join expects 1 argument")
			return anyType
		}
		itemType, ok := threadItemType(checker.inferExpression(args[0], locals, source, line))
		if !ok {
			checker.addError(source, line, "join expects Thread[T]")
			return anyType
		}
		return itemType
	case "thread_status":
		if len(args) != 1 {
			checker.addError(source, line, "thread_status expects 1 argument")
			return "String"
		}
		if _, ok := threadItemType(checker.inferExpression(args[0], locals, source, line)); !ok {
			checker.addError(source, line, "thread_status expects Thread[T]")
		}
		return "String"
	case "Atomic":
		if len(args) != 1 {
			checker.addError(source, line, "Atomic expects 1 argument")
			return "Atomic[T]"
		}
		return "Atomic[" + checker.inferExpression(args[0], locals, source, line) + "]"
	case "atomic_load":
		if len(args) != 1 {
			checker.addError(source, line, "atomic_load expects 1 argument")
			return anyType
		}
		inner, ok := atomicItemType(checker.inferExpression(args[0], locals, source, line))
		if !ok {
			checker.addError(source, line, "atomic_load expects Atomic[T]")
			return anyType
		}
		return inner
	case "atomic_store":
		if len(args) != 2 {
			checker.addError(source, line, "atomic_store expects 2 arguments")
			return anyType
		}
		inner, ok := atomicItemType(checker.inferExpression(args[0], locals, source, line))
		valueType := checker.inferExpression(args[1], locals, source, line)
		if !ok {
			checker.addError(source, line, "atomic_store expects Atomic[T] as first argument")
			return anyType
		}
		if !isAssignable(inner, valueType) {
			checker.addError(source, line, fmt.Sprintf("atomic_store expects %s, got %s", inner, valueType))
		}
		return "Atomic[" + inner + "]"
	case "atomic_add":
		if len(args) != 2 {
			checker.addError(source, line, "atomic_add expects 2 arguments")
			return anyType
		}
		inner, ok := atomicItemType(checker.inferExpression(args[0], locals, source, line))
		valueType := checker.inferExpression(args[1], locals, source, line)
		if !ok {
			checker.addError(source, line, "atomic_add expects Atomic[T] as first argument")
			return anyType
		}
		if !isNumeric(inner) || !isNumeric(valueType) {
			checker.addError(source, line, fmt.Sprintf("atomic_add expects numeric Atomic and value, got %s and %s", inner, valueType))
		}
		return numericResult(inner, valueType)
	case "Program":
		if len(args) != 1 {
			checker.addError(source, line, "Program expects 1 argument")
			return "Program"
		}
		moduleType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("List[String]", moduleType) {
			checker.addError(source, line, fmt.Sprintf("Program module expects List[String], got %s", moduleType))
		}
		return "Program"
	case "BuildSystem":
		if len(args) != 4 {
			checker.addError(source, line, "BuildSystem expects 4 arguments")
			return "BuildSystem"
		}
		projectNameType := checker.inferExpression(args[0], locals, source, line)
		numberOfFilesType := checker.inferExpression(args[1], locals, source, line)
		filesType := checker.inferExpression(args[2], locals, source, line)
		backendType := checker.inferExpression(args[3], locals, source, line)
		if !isAssignable("String", projectNameType) {
			checker.addError(source, line, fmt.Sprintf("BuildSystem project_name expects String, got %s", projectNameType))
		}
		if !isAssignable("Int", numberOfFilesType) {
			checker.addError(source, line, fmt.Sprintf("BuildSystem number_of_files expects Int, got %s", numberOfFilesType))
		}
		if !isAssignable("List[String]", filesType) {
			checker.addError(source, line, fmt.Sprintf("BuildSystem files expects List[String], got %s", filesType))
		}
		if !isAssignable("String", backendType) {
			checker.addError(source, line, fmt.Sprintf("BuildSystem backend expects String, got %s", backendType))
		}
		if literalString, ok := quotedStringLiteral(args[3]); ok && !isBuildBackendName(literalString) {
			checker.addError(source, line, "BuildSystem backend must be WASM, JS, or Standalone")
		}
		return "BuildSystem"
	case "WorkSpace":
		if len(args) != 2 {
			checker.addError(source, line, "WorkSpace expects 2 arguments")
			return "WorkSpace"
		}
		programType := checker.inferExpression(args[0], locals, source, line)
		buildSystemType := checker.inferExpression(args[1], locals, source, line)
		if !isAssignable("Program", programType) {
			checker.addError(source, line, fmt.Sprintf("WorkSpace first argument expects Program, got %s", programType))
		}
		if !isAssignable("BuildSystem", buildSystemType) {
			checker.addError(source, line, fmt.Sprintf("WorkSpace second argument expects BuildSystem, got %s", buildSystemType))
		}
		return "WorkSpace"
	case "workspace_backend":
		if len(args) != 1 {
			checker.addError(source, line, "workspace_backend expects 1 argument")
			return "String"
		}
		workspaceType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("WorkSpace", workspaceType) {
			checker.addError(source, line, fmt.Sprintf("workspace_backend expects WorkSpace, got %s", workspaceType))
		}
		return "String"
	case "workspace_files":
		if len(args) != 1 {
			checker.addError(source, line, "workspace_files expects 1 argument")
			return "List[String]"
		}
		workspaceType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("WorkSpace", workspaceType) {
			checker.addError(source, line, fmt.Sprintf("workspace_files expects WorkSpace, got %s", workspaceType))
		}
		return "List[String]"
	case "workspace_manifest":
		if len(args) != 1 {
			checker.addError(source, line, "workspace_manifest expects 1 argument")
			return "String"
		}
		workspaceType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("WorkSpace", workspaceType) {
			checker.addError(source, line, fmt.Sprintf("workspace_manifest expects WorkSpace, got %s", workspaceType))
		}
		return "String"
	case "runtime_debug_loc", "runtime.debug.__LOC__", "runtime_debug_file", "runtime.debug.__FILE__", "runtime_debug_module", "runtime.debug.__MODULE__", "runtime_debug_function", "runtime.debug.__FUNCTION__":
		if len(args) != 0 {
			checker.addError(source, line, fmt.Sprintf("%s expects 0 arguments", name))
		}
		return "String"
	case "runtime_debug_line", "runtime.debug.__LINE__":
		if len(args) != 0 {
			checker.addError(source, line, fmt.Sprintf("%s expects 0 arguments", name))
		}
		return "Int"
	case "runtime_debug_pos", "runtime.debug.__POS__":
		if len(args) != 0 {
			checker.addError(source, line, fmt.Sprintf("%s expects 0 arguments", name))
		}
		return "Table"
	case "runtime_debug_loc_of", "runtime.debug.__LOC_OF__", "runtime_debug_line_of", "runtime.debug.__LINE_OF__", "runtime_debug_pos_of", "runtime.debug.__POS_OF__":
		if len(args) != 1 {
			checker.addError(source, line, fmt.Sprintf("%s expects 1 argument", name))
			return "Table"
		}
		checker.inferExpression(args[0], locals, source, line)
		return "Table"
	case "debug":
		if len(args) != 1 {
			checker.addError(source, line, "debug expects 1 argument")
			return anyType
		}
		return checker.inferExpression(args[0], locals, source, line)
	case "debug_type":
		if len(args) != 1 {
			checker.addError(source, line, "debug_type expects 1 argument")
			return "String"
		}
		checker.inferExpression(args[0], locals, source, line)
		return "String"
	case "debug_stack":
		if len(args) != 0 {
			checker.addError(source, line, "debug_stack expects 0 arguments")
		}
		return "List[String]"
	case "breakpoint":
		if len(args) > 1 {
			checker.addError(source, line, "breakpoint expects 0 to 1 arguments")
		}
		if len(args) == 1 {
			checker.inferExpression(args[0], locals, source, line)
		}
		return anyType
	case "js_import":
		if len(args) != 1 {
			checker.addError(source, line, "js_import expects 1 argument")
			return "JSModule"
		}
		pathType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("String", pathType) {
			checker.addError(source, line, fmt.Sprintf("js_import expects String, got %s", pathType))
		}
		return "JSModule"
	case "js_source":
		if len(args) != 1 {
			checker.addError(source, line, "js_source expects 1 argument")
			return "String"
		}
		moduleType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("JSModule", moduleType) {
			checker.addError(source, line, fmt.Sprintf("js_source expects JSModule, got %s", moduleType))
		}
		return "String"
	case "js_exports":
		if len(args) != 1 {
			checker.addError(source, line, "js_exports expects 1 argument")
			return "List[String]"
		}
		moduleType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("JSModule", moduleType) {
			checker.addError(source, line, fmt.Sprintf("js_exports expects JSModule, got %s", moduleType))
		}
		return "List[String]"
	case "js_call":
		if len(args) != 3 {
			checker.addError(source, line, "js_call expects 3 arguments")
			return "JSCall"
		}
		moduleType := checker.inferExpression(args[0], locals, source, line)
		apiType := checker.inferExpression(args[1], locals, source, line)
		argsType := checker.inferExpression(args[2], locals, source, line)
		if !isAssignable("JSModule", moduleType) {
			checker.addError(source, line, fmt.Sprintf("js_call expects JSModule, got %s", moduleType))
		}
		if !isAssignable("String", apiType) {
			checker.addError(source, line, fmt.Sprintf("js_call api expects String, got %s", apiType))
		}
		if !strings.HasPrefix(argsType, "List[") {
			checker.addError(source, line, fmt.Sprintf("js_call args expects List[T], got %s", argsType))
		}
		return "JSCall"
	case "Box", "Ref", "RefMut", "RefCell":
		if len(args) != 1 {
			checker.addError(source, line, fmt.Sprintf("%s expects 1 argument", name))
		}
		if len(args) == 1 {
			checker.inferExpression(args[0], locals, source, line)
		}
		return name
	case "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return name
	}

	if alias, ok := checker.aliasFunctions[name]; ok {
		required := requiredAliasParamCount(alias.Params)
		if len(args) < required || len(args) > len(alias.Params) {
			checker.addError(source, line, fmt.Sprintf("alias function %s expects %d to %d argument(s), got %d", name, required, len(alias.Params), len(args)))
			return alias.Name
		}
		for index, arg := range args {
			argType := checker.inferExpression(arg, locals, source, line)
			param := alias.Params[index]
			if !isAssignable(param.Type, argType) {
				checker.addError(source, line, fmt.Sprintf("alias function %s argument %d expects %s, got %s", name, index+1, param.Type, argType))
			}
		}
		return alias.Name
	}

	if variable, ok := checker.lookupVariable(name, locals); ok {
		if functionTypeArgs, functionReturnType, ok := functionValueType(variable.Type); ok {
			return checker.checkCallbackCall(name, functionTypeArgs, functionReturnType, args, locals, source, line)
		}
		if variable.Type == anyType {
			return anyType
		}
	}
	if checker.functionGroupExists(name) {
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return anyType
	}
	if resolved, ok, err := checker.resolveGlobalFunction(name); ok || err != "" {
		if err != "" {
			checker.addError(source, line, err)
			return anyType
		}
		name = resolved
	}

	fn, ok := checker.lookupFunction(name)
	if !ok {
		if targetExpr, methodName, selectorOK := splitIdentifierSelectorCallName(name); selectorOK {
			targetSymbol, targetOK := checker.lookupVariable(targetExpr, locals)
			if !targetOK {
				checker.addError(source, line, fmt.Sprintf("unknown function %q", name))
				return anyType
			}
			targetType := targetSymbol.Type
			if targetSymbol.InferredType != "" {
				targetType = targetSymbol.InferredType
			}
			if methodType, methodOK := checker.aliasMethodType(targetType, methodName); methodOK {
				paramTypes, returnType, _ := functionValueType(methodType)
				return checker.checkCallbackCall(name, paramTypes, returnType, args, locals, source, line)
			}
			if methodType, methodOK := builtinProtocolMethodType(targetType, methodName); methodOK {
				paramTypes, returnType, _ := functionValueType(methodType)
				return checker.checkCallbackCall(name, paramTypes, returnType, args, locals, source, line)
			}
		}
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
	if fn.Private && filepath.Clean(source) != filepath.Clean(fn.File) {
		checker.addError(source, line, fmt.Sprintf("function %s is private to %s", fn.Name, fn.File))
		return fn.ReturnType
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

	returnType := checker.inferGenericCallReturn(fn, args, locals, source, line)
	if fn.Async {
		return "Awaitable[" + returnType + "]"
	}
	return returnType
}

func (checker *TypeChecker) inferLambdaExpression(expr string, locals map[string]variableSymbol, source string, line int) string {
	program, errors := parser.Parse("local T __lambda = " + expr + ";")
	if len(errors) != 0 || len(program.Statements) == 0 {
		checker.addError(source, line, "invalid lambda expression")
		return anyType
	}
	decl, ok := program.Statements[0].(parser.VariableStatement)
	if !ok {
		return anyType
	}
	lambda, ok := decl.Expression.Node.(parser.LambdaExpression)
	if !ok {
		return anyType
	}
	lambdaScope := copyLocals(locals)
	parts := make([]string, 0, len(lambda.Params)+1)
	for _, param := range lambda.Params {
		paramType := normalizeType(param.Type)
		if !isKnownType(paramType) {
			checker.addError(source, line, fmt.Sprintf("lambda parameter %s uses unknown type %s", param.Name, paramType))
			paramType = anyType
		}
		lambdaScope[param.Name] = variableSymbol{Name: param.Name, Type: paramType}
		parts = append(parts, paramType)
	}
	returnType := normalizeType(lambda.ReturnType)
	if !isKnownType(returnType) {
		checker.addError(source, line, fmt.Sprintf("lambda uses unknown return type %s", returnType))
		returnType = anyType
	}
	parts = append(parts, returnType)
	_ = lambdaScope
	return "Function[" + strings.Join(parts, ",") + "]"
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

func requiredAliasParamCount(params []parser.Parameter) int {
	count := len(params)
	for count > 0 && params[count-1].Default.Node != nil {
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

func (checker *TypeChecker) functionGroupExists(name string) bool {
	name = checker.resolveAliasPath(normalizeNamespaceAccess(name))
	if _, ok := checker.groups[name]; ok {
		return true
	}
	if checker.namespace != "" && !strings.Contains(name, ".") {
		_, ok := checker.groups[checker.namespace+name]
		return ok
	}
	return false
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

func aliasFunctionStartBefore(input string, functionIndex int) (int, bool) {
	prefix := strings.TrimRightFunc(input[:functionIndex], unicode.IsSpace)
	start := len(prefix) - len("alias")
	if start < 0 || prefix[start:] != "alias" {
		return 0, false
	}
	if start > 0 && isIdentifierRune(rune(prefix[start-1])) {
		return 0, false
	}
	return start, true
}

func findAliasFunctionEnd(input string, start int) int {
	blockStart := strings.Index(input[start:], "{")
	if blockStart >= 0 {
		blockStart += start
		end := matchBrace(input, blockStart)
		if end != -1 {
			return end + 1
		}
	}
	index := start
	depth := 0
	for index < len(input) {
		nextDo := findKeyword(input, "do", index)
		nextEnd := findKeyword(input, "end", index)
		if nextEnd == -1 {
			return -1
		}
		if nextDo != -1 && nextDo < nextEnd {
			depth++
			index = nextDo + len("do")
			continue
		}
		if depth == 0 {
			return nextEnd + len("end")
		}
		depth--
		index = nextEnd + len("end")
	}
	return -1
}

func collectCatchNames(input string) map[string]bool {
	names := map[string]bool{}
	index := 0
	for index < len(input) {
		nextCatch := findKeyword(input, "catch", index)
		if nextCatch == -1 {
			break
		}
		start := nextCatch + len("catch")
		for start < len(input) && unicode.IsSpace(rune(input[start])) {
			start++
		}
		end := start
		for end < len(input) && isIdentifierRune(rune(input[end])) {
			end++
		}
		name := strings.TrimSpace(input[start:end])
		if isIdentifier(name) {
			names[name] = true
		}
		index = end
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
	switch input {
	case "int", "size":
		return "Int"
	case "bool":
		return "Bool"
	case "string":
		return "String"
	case "float":
		return "Float"
	case "Any":
		return dynamicAnyType
	}
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
	if parts, ok := tupleTypeParts(typeName); ok {
		for _, part := range parts {
			if !isKnownType(part) {
				return false
			}
		}
		return true
	}
	if _, allowed, ok := restrictedGenericType(typeName); ok {
		for _, option := range allowed {
			if !isKnownType(option) {
				return false
			}
		}
		return true
	}
	if typeName == anyType || typeName == dynamicAnyType || typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" || typeName == "Complex" ||
		typeName == "Table" || typeName == "Program" || typeName == "BuildSystem" || typeName == "WorkSpace" ||
		typeName == "JSModule" || typeName == "JSCall" || typeName == "Context" || typeName == "ErrorContext" {
		return true
	}
	if isCustomTypeName(typeName) {
		return true
	}
	if isArrayTypeName(typeName) {
		element, _ := arrayElementType(typeName)
		return isKnownType(element)
	}
	if isAllocatorType(typeName) {
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
	if strings.HasPrefix(typeName, "Awaitable[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Awaitable[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Iterator[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Iterator[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Coroutine[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Coroutine[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Thread[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Thread[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Atomic[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Atomic[") : len(typeName)-1])
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

func isCustomTypeName(typeName string) bool {
	if typeName == "" || strings.ContainsAny(typeName, "[],:|") {
		return false
	}
	first := rune(typeName[0])
	return unicode.IsUpper(first)
}

func isAllocatorType(typeName string) bool {
	switch typeName {
	case "Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return true
	default:
		return false
	}
}

func isArrayTypeName(typeName string) bool {
	typeName = normalizeType(typeName)
	return strings.Contains(typeName, "[") && strings.HasSuffix(typeName, "]") &&
		!strings.HasPrefix(typeName, "List[") && !strings.HasPrefix(typeName, "Map[") &&
		!strings.HasPrefix(typeName, "Option[") && !strings.HasPrefix(typeName, "Result[") &&
		!strings.HasPrefix(typeName, "SIMD[") && !strings.HasPrefix(typeName, "Function[") &&
		!strings.HasPrefix(typeName, "Awaitable[") && !strings.HasPrefix(typeName, "Iterator[") &&
		!strings.HasPrefix(typeName, "Coroutine[") && !strings.HasPrefix(typeName, "Thread[") &&
		!strings.HasPrefix(typeName, "Atomic[")
}

func arrayElementType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	index := strings.Index(typeName, "[")
	if index <= 0 || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	return normalizeType(typeName[:index]), true
}

func arrayRegionName(typeName string) string {
	typeName = normalizeType(typeName)
	index := strings.Index(typeName, "[")
	if index <= 0 || !strings.HasSuffix(typeName, "]") || !isArrayTypeName(typeName) {
		return ""
	}
	return strings.TrimSpace(typeName[index+1 : len(typeName)-1])
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

func awaitableType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Awaitable[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Awaitable[") : len(typeName)-1])
	return inner, inner != ""
}

func iteratorItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Iterator[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Iterator[") : len(typeName)-1])
	return inner, inner != ""
}

func coroutineItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Coroutine[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Coroutine[") : len(typeName)-1])
	return inner, inner != ""
}

func threadItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Thread[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Thread[") : len(typeName)-1])
	return inner, inner != ""
}

func atomicItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Atomic[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Atomic[") : len(typeName)-1])
	return inner, inner != ""
}

func quotedStringLiteral(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if len(expr) < 2 || !strings.HasPrefix(expr, `"`) || !strings.HasSuffix(expr, `"`) {
		return "", false
	}
	return strings.Trim(expr, `"`), true
}

func isBuildBackendName(value string) bool {
	switch value {
	case "WASM", "JS", "Standalone":
		return true
	default:
		return false
	}
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
	if targetParts, ok := tupleTypeParts(target); ok {
		sourceParts, sourceOK := tupleTypeParts(source)
		if !sourceOK || len(targetParts) != len(sourceParts) {
			return false
		}
		for index := range targetParts {
			if !isAssignable(targetParts[index], sourceParts[index]) {
				return false
			}
		}
		return true
	}
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
	if target == dynamicAnyType || source == dynamicAnyType {
		return true
	}
	if target == anyType || source == anyType {
		return true
	}
	if target == source {
		return true
	}
	if target == "Table" && (source == "Map[T,T]" || strings.HasPrefix(source, "Map[")) {
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
	if isArrayTypeName(target) && isArrayTypeName(source) {
		targetElement, targetOk := arrayElementType(target)
		sourceElement, sourceOk := arrayElementType(source)
		return targetOk && sourceOk && isAssignable(targetElement, sourceElement)
	}
	if isArrayTypeName(target) && source == "List[T]" {
		return true
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
	if strings.HasPrefix(target, "Awaitable[") && strings.HasPrefix(source, "Awaitable[") {
		return isAssignable(target[len("Awaitable["):len(target)-1], source[len("Awaitable["):len(source)-1])
	}
	if strings.HasPrefix(target, "Iterator[") && strings.HasPrefix(source, "Iterator[") {
		return isAssignable(target[len("Iterator["):len(target)-1], source[len("Iterator["):len(source)-1])
	}
	if strings.HasPrefix(target, "Coroutine[") && strings.HasPrefix(source, "Coroutine[") {
		return isAssignable(target[len("Coroutine["):len(target)-1], source[len("Coroutine["):len(source)-1])
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

func tupleTypeParts(typeName string) ([]string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "(") || !strings.HasSuffix(typeName, ")") {
		return nil, false
	}
	inner := strings.TrimSpace(typeName[1 : len(typeName)-1])
	if inner == "" {
		return nil, false
	}
	parts := splitTopLevel(inner, ',')
	for index := range parts {
		parts[index] = normalizeType(parts[index])
	}
	return parts, true
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
	case typeName == "Table":
		return anyType, true
	case strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]"):
		return typeName[5 : len(typeName)-1], true
	case strings.HasPrefix(typeName, "Iterator[") && strings.HasSuffix(typeName, "]"):
		return typeName[len("Iterator[") : len(typeName)-1], true
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

func isKnownSomeInitializer(expr string) bool {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return false
	}
	callName, _, ok := parseFunctionCall(expr)
	return ok && callName == "Some"
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

func parseCallableExpressionCall(expr string) (string, []string, bool) {
	expr = normalizeNamespaceAccess(strings.TrimSpace(expr))
	if !strings.HasSuffix(expr, ")") {
		return "", nil, false
	}
	open := trailingCallOpen(expr)
	if open <= 0 {
		return "", nil, false
	}
	callee := strings.TrimSpace(expr[:open])
	if callee == "" || isIdentifierPath(callee) {
		return "", nil, false
	}
	inner := strings.TrimSpace(expr[open+1 : len(expr)-1])
	var args []string
	if inner != "" {
		args = splitTopLevel(inner, ',')
	}
	return callee, args, true
}

func trailingCallOpen(expr string) int {
	depth := 0
	inString := false
	inChar := false
	for index := len(expr) - 1; index >= 0; index-- {
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
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
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
	thenIndex := strings.Index(expr, " then ")
	if thenIndex <= len("if ") {
		return "", "", "", false
	}
	afterThen := strings.TrimSpace(expr[thenIndex+len(" then "):])
	colonIndex := strings.LastIndex(afterThen, ":")
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

func splitTrailingSelectorExpression(expr string) (string, string, bool) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return "", "", false
	}

	depthParen := 0
	depthBracket := 0
	inString := false
	inChar := false
	for index := len(expr) - 1; index >= 0; index-- {
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
		case ')':
			depthParen++
		case '(':
			depthParen--
		case ']':
			depthBracket++
		case '[':
			depthBracket--
		case '.':
			if depthParen == 0 && depthBracket == 0 {
				target := strings.TrimSpace(expr[:index])
				field := strings.TrimSpace(expr[index+1:])
				if target == "" || !isIdentifier(field) || isIdentifierPath(expr) {
					return "", "", false
				}
				return target, field, true
			}
		}
	}
	return "", "", false
}

func splitSizeofExpression(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, ".sizeof") {
		return "", false
	}
	target := strings.TrimSpace(strings.TrimSuffix(expr, ".sizeof"))
	if target == "" || !isIdentifier(target) {
		return "", false
	}
	return normalizeType(target), true
}

func splitIdentifierSelectorCallName(name string) (string, string, bool) {
	name = strings.TrimSpace(name)
	if !isIdentifierPath(name) {
		return "", "", false
	}
	index := strings.LastIndex(name, ".")
	if index <= 0 || index == len(name)-1 {
		return "", "", false
	}
	target := strings.TrimSpace(name[:index])
	field := strings.TrimSpace(name[index+1:])
	return target, field, isIdentifier(target) && isIdentifier(field)
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

func splitPostfixPropagateExpression(expr string) (string, bool) {
	expr = strings.TrimSpace(expr)
	if !strings.HasSuffix(expr, "!") || strings.HasSuffix(expr, "!=") {
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
