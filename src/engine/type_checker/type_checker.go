package typechecker

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"kLang/src/diagnostic"
	"kLang/src/engine/file"
	"kLang/src/lexer"
	"kLang/src/parser"
)

const anyType = "T"
const dynamicAnyType = "Any"
const movedType = "<moved>"

type Error = diagnostic.Diagnostic
type Warning = diagnostic.Diagnostic

type State struct {
	Phase    string
	Kind     string
	Name     string
	Type     string
	Function string
	File     string
	Line     int
	Mutable  bool
}

type Report struct {
	Errors   []Error
	Warnings []Warning
	States   []State
}

func (report Report) Passed() bool {
	return len(report.Errors) == 0
}

type TypeChecker struct {
	functions       map[string]functionSymbol
	globalFunctions map[string][]string
	aliasFunctions  map[string]parser.AliasFunctionStatement
	extensions      map[string]map[string]extensionMethodSymbol
	keywordMacros   map[string]parser.AliasStatement
	regions         map[string]parser.RegionStatement
	groups          map[string][]string
	globals         map[string]variableSymbol
	aliases         map[string]string
	typeAliases     map[string]string
	traits          map[string]traitSymbol
	traitImpls      map[string]map[string]bool
	enums           map[string]enumSymbol
	errors          []Error
	warnings        []Warning
	states          []State
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

type extensionMethodSymbol struct {
	Target string
	Method parser.FunctionStatement
	File   string
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
	ByRef        bool
	Temporary    bool
	Used         bool
	Parameter    bool
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
		extensions:      map[string]map[string]extensionMethodSymbol{},
		keywordMacros:   map[string]parser.AliasStatement{},
		regions:         map[string]parser.RegionStatement{},
		groups:          map[string][]string{},
		globals:         map[string]variableSymbol{},
		aliases:         map[string]string{},
		typeAliases:     map[string]string{},
		traits:          map[string]traitSymbol{},
		traitImpls:      map[string]map[string]bool{},
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
	parsed := parser.ParseLoadedProgram(program)
	entryPoint, entryDiagnostics := parser.ResolveEntryPoint(parsed)
	parsed.EntryPoint = entryPoint
	for _, diagnostic := range entryDiagnostics {
		checker.addError(diagnostic.File, diagnostic.Line, diagnostic.Message)
	}
	checker.collectTypeAliases(parsed)

	for _, unit := range units {
		checker.collectFunctions(unit, "", false)
	}
	checker.collectTraits(parsed)
	checker.collectEnums(parsed)
	checker.collectAliases(parsed)
	checker.collectAliasFunctionsAndRegions(parsed)
	checker.collectExtensions(parsed)
	checker.collectFunctionGroups(parsed)
	for _, unit := range units {
		checker.collectGlobals(unit)
	}
	checker.globals["Args"] = variableSymbol{Name: "Args", Type: "List[String]", Mutable: false}
	checker.recordState(State{Kind: "builtin", Name: "Args", Type: "List[String]", File: "<runtime>", Line: 0})
	checker.collectASTGlobals(parsed)
	for _, unit := range units {
		checker.checkTopLevelCalls(unit)
	}
	for _, fn := range checker.functions {
		checker.checkFunction(fn)
	}
	for _, source := range parsed.Sources {
		checker.checkAliasFunctionMethods(source.Program.Statements, source.Path)
		checker.checkExtensionMethods(source.Program.Statements, source.Path)
	}
	checker.checkLexicalScopes(program.EntryPoint, parsed)

	return Report{Errors: checker.errors, Warnings: checker.warnings, States: checker.states}
}

func (checker *TypeChecker) collectTypeAliases(parsed parser.ParsedProgram) {
	if !parsed.Passed() {
		return
	}
	var collect func([]parser.Statement, string)
	collect = func(statements []parser.Statement, source string) {
		for _, stmt := range statements {
			switch current := stmt.(type) {
			case parser.TypeAliasStatement:
				if _, exists := checker.typeAliases[current.Name]; exists {
					checker.addError(source, current.Pos.Line, fmt.Sprintf("type alias %q is already defined", current.Name))
					continue
				}
				if !isKnownType(current.Resolved) {
					checker.addError(source, current.Pos.Line, fmt.Sprintf("type alias %s targets unknown type %s", current.Name, current.Target))
					continue
				}
				checker.typeAliases[current.Name] = current.Resolved
			case parser.NamespaceStatement:
				collect(current.Body, source)
			case parser.PrivateBlockStatement:
				collect(current.Body, source)
			case parser.ScopeStatement:
				collect(current.Body, source)
			}
		}
	}
	for _, source := range parsed.Sources {
		collect(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) resolveTypeAlias(typeName string) string {
	resolved, _ := parser.ResolveTypeAlias(normalizeType(typeName), checker.typeAliases)
	return normalizeType(resolved)
}

func (checker *TypeChecker) recordState(state State) {
	if state.Phase == "" {
		state.Phase = "compile"
	}
	state.Type = normalizeType(state.Type)
	checker.states = append(checker.states, state)
}

func (checker *TypeChecker) collectFunctionGroups(parsed parser.ParsedProgram) {
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectFunctionGroupStatements(source.Program.Statements, "", source.Path)
	}
}

func (checker *TypeChecker) collectAliasFunctionsAndRegions(parsed parser.ParsedProgram) {
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
			checker.recordState(State{
				Kind: regionStateKind(current),
				Name: current.Name,
				Type: current.TypeName,
				File: source,
				Line: current.Pos.Line,
			})
		case parser.AliasFunctionStatement:
			if _, exists := checker.aliasFunctions[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias function %q is already defined", current.Name))
				continue
			}
			checker.checkAliasStructFieldTags(current, source)
			checker.aliasFunctions[current.Name] = current
			checker.collectAliasFunctionStatements(current.Body, source)
		case parser.NamespaceStatement:
			checker.collectAliasFunctionStatements(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.collectAliasFunctionStatements(current.Body, source)
		case parser.ScopeStatement:
			checker.collectAliasFunctionStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) collectExtensions(parsed parser.ParsedProgram) {
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectExtensionStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectExtensionStatements(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ExtensionStatement:
			target := checker.resolveTypeAlias(normalizeType(current.Target))
			if !isKnownType(target) && !checker.isStructAliasType(target) {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("extension target %s is not a known type", current.Target))
				continue
			}
			if checker.extensions[target] == nil {
				checker.extensions[target] = map[string]extensionMethodSymbol{}
			}
			for _, method := range current.Methods {
				if _, exists := checker.extensions[target][method.Name]; exists {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("extension method %s.%s is already defined", target, method.Name))
					continue
				}
				if _, exists := checker.aliasMethodType(target, method.Name); exists {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("extension method %s.%s conflicts with an alias method", target, method.Name))
					continue
				}
				if _, exists := builtinProtocolMethodType(target, method.Name); exists {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("extension method %s.%s conflicts with a builtin method", target, method.Name))
					continue
				}
				checker.extensions[target][method.Name] = extensionMethodSymbol{
					Target: target,
					Method: method,
					File:   source,
				}
			}
		case parser.NamespaceStatement:
			checker.collectExtensionStatements(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.collectExtensionStatements(current.Body, source)
		case parser.ScopeStatement:
			checker.collectExtensionStatements(current.Body, source)
		}
	}
}

func (checker *TypeChecker) checkAliasStructFieldTags(alias parser.AliasFunctionStatement, source string) {
	if len(alias.FieldTags) == 0 {
		return
	}
	if !alias.Struct {
		checker.addError(source, alias.Pos.Line, fmt.Sprintf("alias function %s uses struct field tags without '= struct'", alias.Name))
	}
	fields := map[string]bool{}
	for _, param := range alias.Params {
		fields[param.Name] = true
	}
	seenFields := map[string]bool{}
	seenJSONNames := map[string]string{}
	for _, tag := range alias.FieldTags {
		if !fields[tag.Field] {
			checker.addError(source, tag.Pos.Line, fmt.Sprintf("JSON tag references unknown struct field %q", tag.Field))
		}
		if seenFields[tag.Field] {
			checker.addError(source, tag.Pos.Line, fmt.Sprintf("struct field %q has more than one JSON tag", tag.Field))
		}
		seenFields[tag.Field] = true
		if previous, exists := seenJSONNames[tag.Name]; exists && previous != tag.Field {
			checker.addError(source, tag.Pos.Line, fmt.Sprintf("JSON name %q is already used by struct field %q", tag.Name, previous))
		}
		seenJSONNames[tag.Name] = tag.Field
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
		case parser.PrivateBlockStatement:
			checker.collectFunctionGroupStatements(current.Body, namespace, source)
		case parser.ScopeStatement:
			checker.collectFunctionGroupStatements(current.Body, namespace, source)
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
		if end, ok := enclosingExtensionEnd(unit.Text, nextFunction); ok {
			index = end
			continue
		}

		fn, end, err := parseFunction(unit, nextFunction, namespace)
		if err != nil {
			checker.addError(unit.Path, lineAt(unit.Text, nextFunction), err.Error())
			index = nextFunction + len("function")
			continue
		}
		for index := range fn.Params {
			fn.Params[index].Type = checker.resolveTypeAlias(fn.Params[index].Type)
		}
		fn.ReturnType = checker.resolveTypeAlias(fn.ReturnType)
		for index := range fn.ReturnTypes {
			fn.ReturnTypes[index].Type = checker.resolveTypeAlias(fn.ReturnTypes[index].Type)
		}
		for name, restriction := range fn.TypeRestrictions {
			fn.TypeRestrictions[name] = checker.resolveTypeAlias(restriction)
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

func (checker *TypeChecker) collectAliases(parsed parser.ParsedProgram) {
	if !parsed.Passed() {
		return
	}
	for _, source := range parsed.Sources {
		checker.collectAliasStatements(source.Program.Statements, source.Path)
	}
}

func (checker *TypeChecker) collectTraits(parsed parser.ParsedProgram) {
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

func (checker *TypeChecker) collectEnums(parsed parser.ParsedProgram) {
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
		case parser.ScopeStatement:
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
		case parser.PrivateBlockStatement:
			checker.collectTraitStatements(current.Body, source)
		case parser.ScopeStatement:
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
			implType := normalizeType(current.Type)
			if checker.traitImpls[current.Trait] == nil {
				checker.traitImpls[current.Trait] = map[string]bool{}
			}
			checker.traitImpls[current.Trait][implType] = true
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
		case parser.PrivateBlockStatement:
			checker.checkImplStatements(current.Body, source)
		case parser.ScopeStatement:
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
			if current.KeywordMacro {
				if _, exists := checker.keywordMacros[current.Name]; exists {
					checker.addError(source, current.Pos.Line, fmt.Sprintf("keyword macro %q is already defined", current.Name))
					continue
				}
				checker.keywordMacros[current.Name] = current
				continue
			}
			target := normalizeNamespaceAccess(current.Target)
			if target == "" {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q is missing a namespace target", current.Name))
				continue
			}
			if isKnownType(normalizeType(target)) {
				continue
			}
			if _, exists := checker.aliases[current.Name]; exists {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q is already defined", current.Name))
				continue
			}
			resolvedTarget := checker.resolveAliasPath(target)
			if resolvedTarget == current.Name || strings.HasPrefix(resolvedTarget, current.Name+".") {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q creates a namespace alias cycle", current.Name))
				continue
			}
			if !checker.namespaceExists(resolvedTarget) {
				checker.addError(source, current.Pos.Line, fmt.Sprintf("alias %q targets unknown namespace %q", current.Name, current.Target))
				continue
			}
			checker.aliases[current.Name] = resolvedTarget
		case parser.NamespaceStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.collectAliasStatements(current.Body, source)
		case parser.ScopeStatement:
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
		decl.Type = checker.resolveTypeAlias(decl.Type)
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
			if !checker.isAssignable(decl.Type, exprType) {
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
		checker.recordState(State{
			Kind:    decl.Scope,
			Name:    decl.Name,
			Type:    decl.Type,
			File:    unit.Path,
			Line:    decl.Line,
			Mutable: decl.Mutable,
		})
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
	declared := map[string]bool{}
	for _, param := range fn.Params {
		param.Parameter = true
		if param.Line == 0 {
			param.Line = fn.Line
		}
		locals[param.Name] = param
		checker.recordState(State{
			Kind:     "parameter",
			Name:     param.Name,
			Type:     param.Type,
			Function: fn.Name,
			File:     fn.File,
			Line:     param.Line,
			Mutable:  param.Mutable,
		})
		if !isDiscardIdentifier(param.Name) {
			declared[param.Name] = true
		}
	}
	checker.recordState(State{
		Kind:     "return",
		Name:     fn.Name,
		Type:     fn.ReturnType,
		Function: fn.Name,
		File:     fn.File,
		Line:     fn.Line,
	})
	for _, returnValue := range fn.ReturnTypes {
		if returnValue.Name == "" {
			continue
		}
		locals[returnValue.Name] = variableSymbol{Name: returnValue.Name, Type: returnValue.Type, Mutable: returnValue.Mutable, File: fn.File, Line: fn.Line}
		checker.recordState(State{
			Kind:     "named_return",
			Name:     returnValue.Name,
			Type:     returnValue.Type,
			Function: fn.Name,
			File:     fn.File,
			Line:     fn.Line,
			Mutable:  returnValue.Mutable,
		})
	}

	for nestedName := range collectNestedFunctionNames(fn.Body) {
		locals[nestedName] = variableSymbol{Name: nestedName, Type: anyType}
	}
	for catchName := range collectCatchNames(fn.Body) {
		locals[catchName] = variableSymbol{Name: catchName, Type: "Atom"}
	}
	for loopName := range collectEvaluationAssignmentNames(fn.Body) {
		locals[loopName] = variableSymbol{Name: loopName, Type: anyType, Mutable: true}
	}
	for _, param := range fn.Params {
		if param.Default == "" {
			continue
		}
		param.Parameter = true
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
	if parsedFn, ok := checker.parseFunctionBodyForSemanticCheck(fn); ok {
		checker.checkSemanticStatements(fn, parsedFn.Body, locals, declared)
	}
}

func (checker *TypeChecker) checkAliasFunctionMethods(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.AliasFunctionStatement:
			receiverType := checker.aliasConstructedType(current, nil)
			seenMethods := map[string]bool{}
			for _, method := range current.Methods {
				if seenMethods[method.Name] {
					checker.addError(source, method.Pos.Line, fmt.Sprintf("method %s.%s is already defined", current.Name, method.Name))
					continue
				}
				seenMethods[method.Name] = true
				if symbol, operator := parser.OperatorMethodSymbol(method.Name); operator {
					if len(method.Params) != 1 {
						checker.addError(source, method.Pos.Line, fmt.Sprintf("operator %s expects exactly one right-hand parameter", symbol))
					}
					if len(method.Params) == 1 && method.Params[0].Default.Node != nil {
						checker.addError(source, method.Pos.Line, fmt.Sprintf("operator %s cannot use a default parameter", symbol))
					}
					if parser.IsComparisonOperator(symbol) && normalizeType(method.ReturnType) != "Bool" {
						checker.addError(source, method.Pos.Line, fmt.Sprintf("comparison operator %s must return Bool", symbol))
					}
				}
				locals := map[string]variableSymbol{
					"this": {Name: "this", Type: receiverType, File: source, Line: method.Pos.Line},
				}
				declared := map[string]bool{"this": true}
				for _, param := range method.Params {
					paramType := normalizeType(param.Type)
					locals[param.Name] = variableSymbol{
						Name:      param.Name,
						Type:      paramType,
						Mutable:   param.Mutable,
						ByRef:     param.ByRef,
						Parameter: true,
						File:      source,
						Line:      method.Pos.Line,
					}
					if !isDiscardIdentifier(param.Name) {
						declared[param.Name] = true
					}
				}
				fn := functionSymbol{
					Name:       current.Name + "." + method.Name,
					Params:     localsToParams(method.Params, source, method.Pos.Line),
					ReturnType: normalizeType(method.ReturnType),
					File:       source,
					Line:       method.Pos.Line,
				}
				checker.checkSemanticStatements(fn, method.Body, locals, declared)
			}
			checker.checkAliasFunctionMethods(current.Body, source)
		case parser.NamespaceStatement:
			checker.checkAliasFunctionMethods(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.checkAliasFunctionMethods(current.Body, source)
		case parser.ScopeStatement:
			checker.checkAliasFunctionMethods(current.Body, source)
		}
	}
}

func (checker *TypeChecker) checkExtensionMethods(statements []parser.Statement, source string) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ExtensionStatement:
			target := checker.resolveTypeAlias(normalizeType(current.Target))
			for _, method := range current.Methods {
				symbol, ok := checker.extensions[target][method.Name]
				if !ok || symbol.File != source || symbol.Method.Pos != method.Pos {
					continue
				}
				locals := map[string]variableSymbol{
					"this": {Name: "this", Type: target, File: source, Line: method.Pos.Line},
				}
				declared := map[string]bool{"this": true}
				for _, param := range method.Params {
					locals[param.Name] = variableSymbol{
						Name:      param.Name,
						Type:      normalizeType(param.Type),
						Mutable:   param.Mutable,
						ByRef:     param.ByRef,
						Parameter: true,
						File:      source,
						Line:      method.Pos.Line,
					}
					if !isDiscardIdentifier(param.Name) {
						declared[param.Name] = true
					}
				}
				fn := functionSymbol{
					Name:       target + "." + method.Name,
					Params:     localsToParams(method.Params, source, method.Pos.Line),
					ReturnType: normalizeType(method.ReturnType),
					File:       source,
					Line:       method.Pos.Line,
				}
				checker.checkSemanticStatements(fn, method.Body, locals, declared)
			}
		case parser.NamespaceStatement:
			checker.checkExtensionMethods(current.Body, source)
		case parser.PrivateBlockStatement:
			checker.checkExtensionMethods(current.Body, source)
		case parser.ScopeStatement:
			checker.checkExtensionMethods(current.Body, source)
		}
	}
}

func localsToParams(params []parser.Parameter, source string, line int) []variableSymbol {
	symbols := make([]variableSymbol, 0, len(params))
	for _, param := range params {
		symbols = append(symbols, variableSymbol{
			Name:      param.Name,
			Type:      normalizeType(param.Type),
			Mutable:   param.Mutable,
			ByRef:     param.ByRef,
			Parameter: true,
			File:      source,
			Line:      line,
		})
	}
	return symbols
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
		if !checker.isAssignable(expected.Type, exprType) {
			name := fmt.Sprintf("return value %d", index+1)
			if expected.Name != "" {
				name = fmt.Sprintf("return value %q", expected.Name)
			}
			checker.addError(fn.File, line, fmt.Sprintf("function %s %s expects %s but got %s", fn.Name, name, expected.Type, exprType))
		}
	}
}

func (checker *TypeChecker) parseFunctionBodyForSemanticCheck(fn functionSymbol) (parser.FunctionStatement, bool) {
	var aliases strings.Builder
	for name, target := range checker.typeAliases {
		fmt.Fprintf(&aliases, "type %s = %s;\n", name, target)
	}
	source := aliases.String() + fmt.Sprintf("function __Semantic() : T {\n%s\n}", fn.Body)
	program, errors := parser.Parse(source)
	if len(errors) != 0 {
		return parser.FunctionStatement{}, false
	}
	for _, stmt := range program.Statements {
		if parsedFn, ok := stmt.(parser.FunctionStatement); ok {
			return parsedFn, true
		}
	}
	return parser.FunctionStatement{}, false
}

func (checker *TypeChecker) checkSemanticStatements(fn functionSymbol, statements []parser.Statement, locals map[string]variableSymbol, declared map[string]bool) {
	for _, stmt := range statements {
		checker.checkSemanticStatement(fn, stmt, locals, declared)
	}
	checker.reportUnusedDeclaredSymbols(fn, locals, declared)
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
		typeName := checker.resolveTypeAlias(current.Type)
		if typeName != anyType {
			checker.checkDeclaredType(typeName, fn.File, line)
		}
		if current.Expression.Node != nil {
			exprSource := expressionSource(current.Expression)
			if current.Scope == "const" && !isCompileTimeConstantExpression(exprSource) {
				checker.addError(fn.File, line, fmt.Sprintf("const %s requires a compile-time constant initializer", current.Name))
			}
			exprType := checker.inferParsedExpression(current.Expression, locals, fn.File, line)
			if current.Inferred && typeName == anyType {
				typeName = exprType
			}
			if !checker.isAssignable(typeName, exprType) {
				checker.addStructuredError(
					fn.File, line, 0, 0,
					diagnostic.CodeTypeMismatch,
					"type compatibility",
					fmt.Sprintf("cannot assign %s to local %s %s", exprType, typeName, current.Name),
					fmt.Sprintf("Expected %s but found %s. Change the expression, add an explicit cast, or adjust the declared type.", typeName, exprType),
					typeName, exprType,
				)
			}
			if !childTypeLiteralFits(typeName, exprSource) {
				checker.addError(fn.File, line, fmt.Sprintf("literal %s does not fit in %s", exprSource, typeName))
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
			Temporary:    current.Temporary,
			File:         fn.File,
			Line:         line,
		}
		checker.recordState(State{
			Kind:     variableStateKind(current),
			Name:     current.Name,
			Type:     typeName,
			Function: fn.Name,
			File:     fn.File,
			Line:     line,
			Mutable:  current.Mutable,
		})
		declared[current.Name] = true
	case parser.MultiVariableStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		if current.Scope == "global" || current.Exported {
			return
		}
		if current.Lazy {
			checker.addError(fn.File, line, "lazy multi-variable declarations are not supported")
			return
		}
		returnTypes, ok := checker.multiReturnTypesFromExpression(current.Expression, locals, fn.File, line)
		if !ok {
			checker.addError(fn.File, line, "multi-variable declarations require a call to a function with multiple return values")
			return
		}
		if len(returnTypes) != len(current.Bindings) {
			checker.addError(fn.File, line, fmt.Sprintf("multi-variable declaration expects %d value(s), got %d", len(current.Bindings), len(returnTypes)))
			return
		}
		for index, binding := range current.Bindings {
			typeName := checker.resolveTypeAlias(binding.Type)
			if typeName != anyType {
				checker.checkDeclaredType(typeName, fn.File, line)
			}
			actualType := returnTypes[index].Type
			if !checker.isAssignable(typeName, actualType) {
				checker.addError(fn.File, line, fmt.Sprintf("cannot assign return value %d (%s) to local %s %s", index+1, actualType, typeName, binding.Name))
			}
			if isDiscardIdentifier(binding.Name) {
				continue
			}
			locals[binding.Name] = variableSymbol{
				Name:      binding.Name,
				Type:      typeName,
				Mutable:   current.Mutable,
				Temporary: current.Temporary,
				File:      fn.File,
				Line:      line,
			}
			checker.recordState(State{
				Kind:     variableStateKindForMulti(current),
				Name:     binding.Name,
				Type:     typeName,
				Function: fn.Name,
				File:     fn.File,
				Line:     line,
				Mutable:  current.Mutable,
			})
			declared[binding.Name] = true
		}
	case parser.ReturnStatement:
		if len(current.Values) != 0 {
			for _, value := range current.Values {
				checker.checkSemanticExpression(fn, value, locals, line)
			}
			checker.checkTupleReturnExpressions(fn, current.Values, locals, line)
			return
		}
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		exprType := checker.inferParsedExpression(current.Expression, locals, fn.File, line)
		if len(fn.ReturnTypes) == 0 && !checker.isAssignable(fn.ReturnType, exprType) {
			checker.addStructuredError(
				fn.File, line, 0, 0,
				diagnostic.CodeTypeMismatch,
				"type compatibility",
				fmt.Sprintf("function %s returns %s but return expression is %s", fn.Name, fn.ReturnType, exprType),
				fmt.Sprintf("Return a %s value or change the function return type.", fn.ReturnType),
				fn.ReturnType, exprType,
			)
		}
	case parser.ThrowStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		exprType := checker.inferParsedExpression(current.Expression, locals, fn.File, line)
		if checker.resolveTypeAlias(exprType) != "Atom" {
			checker.addError(fn.File, line, fmt.Sprintf("throw expects Atom, got %s", exprType))
		}
	case parser.AssertStatement:
		checker.checkSemanticExpression(fn, current.Expression, locals, line)
		exprType := checker.inferParsedExpression(current.Expression, locals, fn.File, line)
		if !isAssignable("Bool", exprType) {
			checker.addError(fn.File, line, fmt.Sprintf("assert expects Bool, got %s", exprType))
		}
	case parser.ReportStatement:
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
		valueType := checker.inferParsedExpression(current.Value, locals, fn.File, line)
		for _, matchCase := range current.Cases {
			caseLocals := copyLocals(locals)
			declared := checker.semanticPatternCaptures(matchCase.Pattern.Node, valueType, caseLocals, fn.File, semanticLine(fn, matchCase.Pos))
			checker.checkSemanticChildBlockWithLocals(fn, matchCase.Body, locals, caseLocals, declared)
		}
	case parser.LoopStatement:
		checker.checkSemanticLoop(fn, current, locals, line)
	case parser.TryCatchStatement:
		checker.checkSemanticChildBlock(fn, current.TryBody, locals)
		catchLocals := copyLocals(locals)
		catchLocals[current.ErrorName] = variableSymbol{Name: current.ErrorName, Type: "Atom", File: fn.File, Line: line}
		checker.checkSemanticChildBlockWithLocals(fn, current.CatchBody, locals, catchLocals, map[string]bool{current.ErrorName: true})
	case parser.TransactionStatement:
		checker.checkSemanticChildBlock(fn, current.Body, locals)
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
	case parser.ScopeStatement:
		checker.checkSemanticChildBlock(fn, current.Body, locals)
	case parser.FunctionStatement:
		return
	}
}

func (checker *TypeChecker) checkSemanticExpression(fn functionSymbol, expr parser.Expression, locals map[string]variableSymbol, line int) {
	if expr.Node == nil || len(expr.Tokens) == 0 {
		return
	}
	checker.inferParsedExpression(expr, locals, fn.File, line)
}

func (checker *TypeChecker) inferParsedExpression(expr parser.Expression, locals map[string]variableSymbol, source string, line int) string {
	return checker.inferExpressionNode(expr.Node, expressionSource(expr), locals, source, line)
}

func (checker *TypeChecker) inferExpressionNode(node parser.ExpressionNode, fallback string, locals map[string]variableSymbol, source string, line int) string {
	switch current := node.(type) {
	case parser.LiteralExpression:
		return normalizeType(current.Kind)
	case parser.GroupExpression:
		return checker.inferExpressionNode(current.Inner, fallback, locals, source, line)
	case parser.CallExpression:
		if resultType, ok := checker.inferStructCastCall(current, locals, source, line); ok {
			return resultType
		}
		if resultType, ok := checker.inferStandaloneExtensionCall(current, locals, source, line); ok {
			return resultType
		}
		for _, argument := range current.Arguments {
			if call, ok := argument.(parser.CallExpression); ok {
				checker.inferStandaloneExtensionCall(call, locals, source, line)
			}
		}
		return checker.inferExpression(fallback, locals, source, line)
	default:
		return checker.inferExpression(fallback, locals, source, line)
	}
}

func (checker *TypeChecker) inferStructCastCall(call parser.CallExpression, locals map[string]variableSymbol, source string, line int) (string, bool) {
	selector, ok := call.Callee.(parser.SelectorExpression)
	if !ok || selector.Field != "cast_as" {
		return "", false
	}
	sourceType, ok := checker.extensionReceiverType(selector.Target, locals, source, line)
	if !ok {
		sourceType = anyType
	}
	if len(call.Arguments) != 1 {
		checker.addError(source, line, fmt.Sprintf("cast_as expects exactly 1 target type, got %d", len(call.Arguments)))
		return anyType, true
	}
	targetType, ok := checker.structCastTargetNode(call.Arguments[0])
	if !ok {
		checker.addError(source, line, "cast_as expects a builtin target type or struct alias name, for example value.cast_as(Table)")
		return anyType, true
	}
	return checker.checkStructCast(sourceType, targetType, source, line), true
}

func (checker *TypeChecker) structCastTargetNode(node parser.ExpressionNode) (string, bool) {
	identifier, ok := node.(parser.IdentifierExpression)
	if !ok {
		return "", false
	}
	targetType := checker.resolveTypeAlias(normalizeType(identifier.Name))
	switch targetType {
	case "Table", "JSON", "String":
		return targetType, true
	default:
		return targetType, checker.isStructAliasType(targetType)
	}
}

func (checker *TypeChecker) inferStandaloneExtensionCall(call parser.CallExpression, locals map[string]variableSymbol, source string, line int) (string, bool) {
	selector, ok := call.Callee.(parser.SelectorExpression)
	if !ok {
		return "", false
	}
	targetType, ok := checker.extensionReceiverType(selector.Target, locals, source, line)
	if !ok {
		return "", false
	}
	method, ok := checker.lookupExtensionMethod(targetType, selector.Field)
	if !ok {
		return "", false
	}
	checker.warnDeprecatedMethod(source, line, targetType, method.Method)
	return checker.checkExtensionNodeArguments(selector.Field, targetType, method, call.Arguments, locals, source, line), true
}

func (checker *TypeChecker) extensionReceiverType(node parser.ExpressionNode, locals map[string]variableSymbol, source string, line int) (string, bool) {
	switch current := node.(type) {
	case parser.LiteralExpression:
		return normalizeType(current.Kind), true
	case parser.IdentifierExpression:
		variable, ok := checker.lookupVariable(current.Name, locals)
		if !ok {
			return "", false
		}
		if variable.InferredType != "" {
			return variable.InferredType, true
		}
		return variable.Type, true
	case parser.GroupExpression:
		return checker.extensionReceiverType(current.Inner, locals, source, line)
	case parser.CallExpression:
		if resultType, ok := checker.inferStandaloneExtensionCall(current, locals, source, line); ok {
			return resultType, true
		}
		if callee, ok := current.Callee.(parser.IdentifierExpression); ok {
			if alias, exists := checker.aliasFunctions[callee.Name]; exists {
				return checker.aliasConstructedType(alias, nil), true
			}
		}
	}
	return "", false
}

func (checker *TypeChecker) checkExtensionNodeArguments(name string, receiverType string, symbol extensionMethodSymbol, args []parser.ExpressionNode, locals map[string]variableSymbol, source string, line int) string {
	paramTypes, returnType := extensionMethodSignature(receiverType, symbol)
	required := requiredAliasParamCount(symbol.Method.Params)
	if len(args) < required || len(args) > len(symbol.Method.Params) {
		checker.addError(source, line, fmt.Sprintf("method %s expects %d to %d argument(s), got %d", name, required, len(symbol.Method.Params), len(args)))
		return returnType
	}
	for index, arg := range args {
		argType, ok := checker.extensionReceiverType(arg, locals, source, line)
		if !ok {
			argType = anyType
		}
		paramType := paramTypes[index]
		if !checker.isAssignable(paramType, argType) {
			checker.addError(source, line, fmt.Sprintf("method %s argument %d expects %s, got %s", name, index+1, paramType, argType))
		}
	}
	return returnType
}

func (checker *TypeChecker) checkTupleReturnExpressions(fn functionSymbol, values []parser.Expression, locals map[string]variableSymbol, line int) {
	if len(values) != len(fn.ReturnTypes) {
		checker.addError(fn.File, line, fmt.Sprintf("function %s returns %d value(s), got %d", fn.Name, len(fn.ReturnTypes), len(values)))
		return
	}
	for index, value := range values {
		exprType := checker.inferParsedExpression(value, locals, fn.File, line)
		expected := fn.ReturnTypes[index]
		if !checker.isAssignable(expected.Type, exprType) {
			name := fmt.Sprintf("return value %d", index+1)
			if expected.Name != "" {
				name = fmt.Sprintf("return value %q", expected.Name)
			}
			checker.addError(fn.File, line, fmt.Sprintf("function %s %s expects %s but got %s", fn.Name, name, expected.Type, exprType))
		}
	}
}

func (checker *TypeChecker) multiReturnTypesFromExpression(expr parser.Expression, locals map[string]variableSymbol, source string, line int) ([]returnValueSymbol, bool) {
	call, ok := expr.Node.(parser.CallExpression)
	if !ok {
		return nil, false
	}
	name, ok := callCalleeName(call.Callee)
	if !ok {
		return nil, false
	}
	fn, ok := checker.lookupFunction(name)
	if !ok || len(fn.ReturnTypes) == 0 {
		return nil, false
	}
	returnTypes := make([]returnValueSymbol, len(fn.ReturnTypes))
	copy(returnTypes, fn.ReturnTypes)
	if len(fn.TypeRestrictions) == 0 {
		return returnTypes, true
	}
	solver := checker.newConstraintSolver()
	for index, param := range fn.Params {
		if index >= len(call.Arguments) {
			continue
		}
		argType := checker.inferExpressionNode(call.Arguments[index], "", locals, source, line)
		solver.unify(param.Type, argType)
	}
	for index := range returnTypes {
		returnTypes[index].Type = solver.apply(returnTypes[index].Type)
	}
	return returnTypes, true
}

func callCalleeName(node parser.ExpressionNode) (string, bool) {
	switch current := node.(type) {
	case parser.IdentifierExpression:
		return current.Name, true
	case parser.SelectorExpression:
		target, ok := callCalleeName(current.Target)
		if !ok {
			return "", false
		}
		return target + "." + current.Field, true
	default:
		return "", false
	}
}

func (checker *TypeChecker) checkSemanticLoop(fn functionSymbol, stmt parser.LoopStatement, locals map[string]variableSymbol, line int) {
	loopLocals := copyLocals(locals)
	declared := map[string]bool{}
	if stmt.Kind == "for_each" {
		iterator, iterable, ok := parseForEachScopeHeader(stmt.Header)
		if !ok {
			checker.checkSemanticExpression(fn, stmt.Header, locals, line)
			checker.checkSemanticChildBlockWithLocals(fn, stmt.Body, locals, loopLocals, declared)
			return
		}
		checker.checkSemanticExpression(fn, iterable, locals, line)
		itemType := anyType
		if inferred, ok := iterableItemType(checker.inferParsedExpression(iterable, locals, fn.File, line)); ok {
			itemType = inferred
		}
		loopLocals[iterator] = variableSymbol{Name: iterator, Type: itemType, File: fn.File, Line: line}
		declared[iterator] = true
		checker.checkSemanticChildBlockWithLocals(fn, stmt.Body, locals, loopLocals, declared)
		return
	}
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
	if tokens[0].Type == lexer.TokenIdentifier && len(tokens) >= 3 && tokens[1].Type == lexer.TokenEvaluationAssign {
		value := parser.Expression{Tokens: tokens[2:], Node: parser.ParseExpressionTokens(tokens[2:])}
		return parser.VariableStatement{
			Pos:        parser.Position{Line: tokens[0].Line, Column: tokens[0].Column},
			Scope:      "local",
			Inferred:   true,
			Mutable:    true,
			Type:       "T",
			Name:       tokens[0].Literal,
			Expression: value,
		}, true
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

func (checker *TypeChecker) semanticPatternCaptures(pattern parser.ExpressionNode, valueType string, locals map[string]variableSymbol, source string, line int) map[string]bool {
	declared := map[string]bool{}
	checker.collectSemanticPatternCaptures(pattern, normalizeType(valueType), locals, declared, source, line)
	return declared
}

func (checker *TypeChecker) collectSemanticPatternCaptures(pattern parser.ExpressionNode, valueType string, locals map[string]variableSymbol, declared map[string]bool, source string, line int) {
	switch current := pattern.(type) {
	case nil:
		return
	case parser.IdentifierExpression:
		if current.Name == "_" {
			return
		}
		if _, exists := locals[current.Name]; exists {
			return
		}
		locals[current.Name] = variableSymbol{Name: current.Name, Type: valueType, File: source, Line: line}
		declared[current.Name] = true
	case parser.GroupExpression:
		checker.collectSemanticPatternCaptures(current.Inner, valueType, locals, declared, source, line)
	case parser.CallExpression:
		callee, ok := current.Callee.(parser.IdentifierExpression)
		if !ok {
			return
		}
		switch callee.Name {
		case "Some":
			elementType, ok := optionElementType(valueType)
			if ok && len(current.Arguments) == 1 {
				checker.collectSemanticPatternCaptures(current.Arguments[0], elementType, locals, declared, source, line)
			}
		case "Ok":
			okType, _, ok := resultValueTypes(valueType)
			if ok && len(current.Arguments) == 1 {
				checker.collectSemanticPatternCaptures(current.Arguments[0], okType, locals, declared, source, line)
			}
		case "Err":
			_, errType, ok := resultValueTypes(valueType)
			if ok && len(current.Arguments) == 1 {
				checker.collectSemanticPatternCaptures(current.Arguments[0], errType, locals, declared, source, line)
			}
		}
	case parser.ListExpression:
		elementType := anyType
		if parsed, ok := listElementTypeName(valueType); ok {
			elementType = parsed
		}
		for _, item := range current.Items {
			checker.collectSemanticPatternCaptures(item, elementType, locals, declared, source, line)
		}
	case parser.MapExpression:
		valueElementType := anyType
		if _, mapValue, ok := indexedMapTypes(valueType); ok {
			valueElementType = mapValue
		}
		for _, entry := range current.Entries {
			checker.collectSemanticPatternCaptures(entry.Value, valueElementType, locals, declared, source, line)
		}
	}
}

func (checker *TypeChecker) reportUnusedDeclaredSymbols(fn functionSymbol, locals map[string]variableSymbol, declared map[string]bool) {
	for name := range declared {
		if isDiscardIdentifier(name) {
			continue
		}
		variable, ok := locals[name]
		if !ok || variable.Used || variable.Temporary {
			continue
		}
		if variable.Parameter {
			checker.addWarning(fn.File, variable.Line, fmt.Sprintf("unused parameter %q", name))
			continue
		}
		checker.addWarning(fn.File, variable.Line, fmt.Sprintf("unused variable %q", name))
	}
}

func variableStateKind(stmt parser.VariableStatement) string {
	if stmt.Temporary {
		return "temporary"
	}
	return stmt.Scope
}

func variableStateKindForMulti(stmt parser.MultiVariableStatement) string {
	if stmt.Temporary {
		return "temporary"
	}
	return stmt.Scope
}

func regionStateKind(stmt parser.RegionStatement) string {
	if stmt.Temporary {
		return "temporary_region"
	}
	return "region"
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
	KnownOk   bool
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
			} else if _, _, ok := resultValueTypes(typeName); ok {
				env[current.Name] = nullSafetySymbol{Type: typeName, KnownOk: nullSafetyExpressionIsKnownOk(current.Expression.Node)}
			} else {
				env[current.Name] = nullSafetySymbol{Type: typeName}
			}
		case parser.MultiVariableStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
			for _, binding := range current.Bindings {
				if isDiscardIdentifier(binding.Name) {
					continue
				}
				typeName := normalizeType(binding.Type)
				env[binding.Name] = nullSafetySymbol{Type: typeName}
			}
		case parser.AssignmentStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
			if target, ok := current.Target.Node.(parser.IdentifierExpression); ok {
				if symbol, exists := env[target.Name]; exists {
					symbol.KnownSome = nullSafetyExpressionIsKnownSome(current.Expression.Node)
					symbol.KnownOk = nullSafetyExpressionIsKnownOk(current.Expression.Node)
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
		case parser.AssertStatement:
			checker.checkNullSafetyExpression(current.Expression.Node, env, source, baseLine)
		case parser.ReportStatement:
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
			for name := range nullSafetyOkGuards(current.Condition.Node) {
				if symbol, ok := guarded[name]; ok {
					if _, _, result := resultValueTypes(symbol.Type); result {
						symbol.KnownOk = true
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
			for name := range nullSafetyOkGuards(current.Header.Node) {
				if symbol, ok := guarded[name]; ok {
					if _, _, result := resultValueTypes(symbol.Type); result {
						symbol.KnownOk = true
						guarded[name] = symbol
					}
				}
			}
			checker.checkNullSafetyStatements(current.Body, guarded, source, baseLine)
		case parser.TryCatchStatement:
			checker.checkNullSafetyStatements(current.TryBody, copyNullSafetyEnv(env), source, baseLine)
			catchEnv := copyNullSafetyEnv(env)
			catchEnv[current.ErrorName] = nullSafetySymbol{Type: "Atom"}
			checker.checkNullSafetyStatements(current.CatchBody, catchEnv, source, baseLine)
		case parser.TransactionStatement:
			checker.checkNullSafetyStatements(current.Body, copyNullSafetyEnv(env), source, baseLine)
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
		case parser.ScopeStatement:
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
						checker.addError(source, line, fmt.Sprintf("Option value %s must be checked with .some, pattern matched with Some(...), or unwrapped with option_unwrap_or before accessing .value", target.Name))
					}
					if _, errorType, result := resultValueTypes(symbol.Type); result && !symbol.KnownOk {
						line := baseLine + selectorLine(current.Target) - 1
						if checker.resolveTypeAlias(errorType) == "Atom" {
							checker.addError(source, line, fmt.Sprintf("Result value %s must be checked with .ok, pattern matched with Ok(...), or propagated with ! before accessing .value", target.Name))
						} else {
							checker.addError(source, line, fmt.Sprintf("Result value %s must be checked with .ok or pattern matched with Ok(...) before accessing .value", target.Name))
						}
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
			case "Ok", "Result":
				return "Result[T,T]"
			case "Err":
				return "Result[T,T]"
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

func nullSafetyExpressionIsKnownOk(expr parser.ExpressionNode) bool {
	call, ok := expr.(parser.CallExpression)
	if !ok {
		return false
	}
	callee, ok := call.Callee.(parser.IdentifierExpression)
	return ok && (callee.Name == "Ok" || callee.Name == "Result")
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

func nullSafetyOkGuards(expr parser.ExpressionNode) map[string]bool {
	guards := map[string]bool{}
	switch current := expr.(type) {
	case parser.SelectorExpression:
		if current.Field == "ok" {
			if target, ok := current.Target.(parser.IdentifierExpression); ok {
				guards[target.Name] = true
			}
		}
	case parser.GroupExpression:
		for name := range nullSafetyOkGuards(current.Inner) {
			guards[name] = true
		}
	case parser.BinaryExpression:
		for name := range nullSafetyOkGuards(current.Left) {
			guards[name] = true
		}
		for name := range nullSafetyOkGuards(current.Right) {
			guards[name] = true
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

func isStdlibImplementationSource(source string) bool {
	cleaned := filepath.ToSlash(filepath.Clean(source))
	return strings.HasPrefix(cleaned, "stdlib/") || strings.Contains(cleaned, "/stdlib/")
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
			byRef := false
			if strings.HasPrefix(name, "ref ") {
				byRef = true
				mutable = true
				name = strings.TrimSpace(strings.TrimPrefix(name, "ref"))
			}
			if strings.HasPrefix(name, "mut ") {
				mutable = true
				name = strings.TrimSpace(strings.TrimPrefix(name, "mut"))
			}
			if name == "" || defaultValue == "" {
				return nil, fmt.Errorf("function parameter %q must include a name and default value", strings.TrimSpace(part))
			}
			if byRef {
				return nil, fmt.Errorf("reference parameter %s cannot have a default value", name)
			}
			params = append(params, variableSymbol{Name: name, Type: anyType, Mutable: mutable, ByRef: byRef, Default: defaultValue})
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
		byRef := false
		if strings.HasPrefix(name, "ref ") {
			byRef = true
			mutable = true
			name = strings.TrimSpace(strings.TrimPrefix(name, "ref"))
		}
		if strings.HasPrefix(name, "mut ") {
			mutable = true
			name = strings.TrimSpace(strings.TrimPrefix(name, "mut"))
		}
		typeName := normalizeType(part[colon+1:])
		if name == "" || typeName == "" {
			return nil, fmt.Errorf("function parameter %q must be written as name : Type", strings.TrimSpace(part))
		}
		if byRef && defaultValue != "" {
			return nil, fmt.Errorf("reference parameter %s cannot have a default value", name)
		}
		params = append(params, variableSymbol{Name: name, Type: typeName, Mutable: mutable, ByRef: byRef, Default: defaultValue})
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
		if current.Field != "sizeof" {
			return false
		}
		_, ok := typeExpressionNameFromNode(current.Target)
		return ok
	default:
		return false
	}
}

func typeExpressionNameFromNode(expr parser.ExpressionNode) (string, bool) {
	switch current := expr.(type) {
	case parser.IdentifierExpression:
		typeName := normalizeType(current.Name)
		return typeName, isKnownType(typeName)
	case parser.SelectorExpression:
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			typeName := normalizeType(target.Name + "." + current.Field)
			return typeName, isKnownType(typeName)
		}
	case parser.CallExpression:
		selector, ok := current.Callee.(parser.SelectorExpression)
		if !ok || selector.Field != "child" || len(current.Arguments) != 1 {
			return "", false
		}
		target, ok := selector.Target.(parser.IdentifierExpression)
		if !ok {
			return "", false
		}
		literal, ok := current.Arguments[0].(parser.LiteralExpression)
		if !ok || literal.Kind != "Int" {
			return "", false
		}
		typeName := normalizeType(target.Name + ".child(" + literal.Value + ")")
		return typeName, isKnownType(typeName)
	}
	return "", false
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
		variable, ok := checker.lookupVariableNoUse(baseName, locals)
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
	if assignment.Op != "=" {
		operator := strings.TrimSuffix(assignment.Op, "=")
		if resultType, overloaded := checker.checkOverloadedBinaryOperator(targetType, operator, exprType, source, line); overloaded {
			if !checker.isAssignable(targetType, resultType) {
				checker.addError(source, line, fmt.Sprintf("operator %s returns %s, which cannot be assigned to %s", operator, resultType, targetType))
			}
			return
		}
		if !isNumeric(targetType) && targetType != "String" {
			checker.addError(source, line, fmt.Sprintf("operator %s cannot be used with %s", assignment.Op, targetType))
			return
		}
	}
	if !checker.isAssignable(targetType, exprType) {
		checker.addError(source, line, fmt.Sprintf("cannot assign %s to %s", exprType, targetType))
	}
	if !childTypeLiteralFits(targetType, assignment.Expr) {
		checker.addError(source, line, fmt.Sprintf("literal %s does not fit in %s", assignment.Expr, targetType))
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
	for name, macro := range checker.keywordMacros {
		prefix := name + " "
		if strings.HasPrefix(expr, prefix) {
			arguments := splitTopLevel(strings.TrimSpace(strings.TrimPrefix(expr, prefix)), ',')
			if len(arguments) == 1 && strings.TrimSpace(arguments[0]) == "" {
				arguments = nil
			}
			return checker.checkKeywordMacroCall(macro, arguments, locals, source, line)
		}
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
	if isAtomLiteral(expr) {
		return "Atom"
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
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+len(operator):], locals, source, line)
		if returnType, ok := checker.checkOverloadedBinaryOperator(left, operator, right, source, line); ok {
			return returnType
		}
		return "Bool"
	}
	if index := findTopLevelOperator(expr, []string{"+", "-"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+1:], locals, source, line)
		operator := expr[index : index+1]
		if returnType, ok := checker.checkOverloadedBinaryOperator(left, operator, right, source, line); ok {
			return returnType
		}
		if left == "String" || right == "String" {
			return "String"
		}
		return numericResult(left, right)
	}
	if index := findTopLevelOperator(expr, []string{"**"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+len("**"):], locals, source, line)
		if returnType, ok := checker.checkOverloadedBinaryOperator(left, "**", right, source, line); ok {
			return returnType
		}
		return numericResult(left, right)
	}
	if index, operator := findTopLevelOperatorWithMatch(expr, []string{"*", "//", "/", "%"}); index != -1 && index > 0 {
		left := checker.inferExpression(expr[:index], locals, source, line)
		right := checker.inferExpression(expr[index+len(operator):], locals, source, line)
		if returnType, ok := checker.checkOverloadedBinaryOperator(left, operator, right, source, line); ok {
			return returnType
		}
		return numericResult(left, right)
	}
	if index := findTopLevelOperator(expr, []string{" as "}); index != -1 && index > 0 {
		sourceType := checker.inferExpression(expr[:index], locals, source, line)
		targetType := checker.resolveTypeAlias(expr[index+len(" as "):])
		if !isKnownType(targetType) {
			checker.addError(source, line, fmt.Sprintf("unknown cast target type %s", targetType))
			return anyType
		}
		if !isBuiltinCastTarget(targetType) {
			checker.addError(source, line, fmt.Sprintf("cast target %s is not a builtin type", targetType))
			return targetType
		}
		if !canCast(sourceType, targetType) {
			checker.addError(source, line, fmt.Sprintf("cannot cast %s to %s", sourceType, targetType))
		}
		return targetType
	}
	if inner, ok := splitPostfixNullCheckExpression(expr); ok {
		checker.inferExpression(inner, locals, source, line)
		return "Bool"
	}
	if inner, ok := splitPostfixPropagateExpression(expr); ok {
		resultType := checker.inferExpression(inner, locals, source, line)
		okType, errorType, ok := resultValueTypes(resultType)
		if !ok {
			checker.addError(source, line, fmt.Sprintf("! expects Result, got %s", resultType))
			return anyType
		}
		if checker.resolveTypeAlias(errorType) != "Atom" {
			checker.addError(source, line, fmt.Sprintf("! only propagates Result[T, Atom], got %s", resultType))
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
	alias, substitutions, ok := checker.aliasTypeInfo(typeName)
	if !ok {
		return "", false
	}
	for _, method := range alias.Methods {
		if method.Name != methodName {
			continue
		}
		parts := make([]string, 0, len(method.Params)+1)
		for _, param := range method.Params {
			parts = append(parts, applyAliasTypeSubstitutions(normalizeType(param.Type), substitutions))
		}
		parts = append(parts, checker.aliasMethodReturnType(alias, method.ReturnType, substitutions))
		return "Function[" + strings.Join(parts, ",") + "]", true
	}
	return "", false
}

func (checker *TypeChecker) aliasMethod(typeName string, methodName string) (parser.FunctionStatement, bool) {
	alias, _, ok := checker.aliasTypeInfo(typeName)
	if !ok {
		return parser.FunctionStatement{}, false
	}
	for _, method := range alias.Methods {
		if method.Name == methodName {
			return method, true
		}
	}
	return parser.FunctionStatement{}, false
}

func (checker *TypeChecker) warnDeprecatedMethod(source string, line int, typeName string, method parser.FunctionStatement) {
	if !method.Deprecated || isStdlibImplementationSource(source) {
		return
	}
	message := fmt.Sprintf("method %s.%s is deprecated", typeName, method.Name)
	if method.DeprecationMessage != "" {
		message += ": " + method.DeprecationMessage
	}
	checker.addWarning(source, line, message)
}

func (checker *TypeChecker) checkOverloadedBinaryOperator(leftType string, operator string, rightType string, source string, line int) (string, bool) {
	methodName, ok := parser.OperatorMethodName(operator)
	if !ok {
		return "", false
	}
	methodType, ok := checker.aliasMethodType(leftType, methodName)
	if !ok {
		return "", false
	}
	paramTypes, returnType, ok := functionValueType(methodType)
	if !ok || len(paramTypes) != 1 {
		return returnType, true
	}
	if !checker.isAssignable(paramTypes[0], rightType) {
		checker.addError(source, line, fmt.Sprintf("operator %s on %s expects %s, got %s", operator, leftType, paramTypes[0], rightType))
	}
	return returnType, true
}

func (checker *TypeChecker) extensionMethodType(typeName string, methodName string) (string, bool) {
	symbol, ok := checker.lookupExtensionMethod(typeName, methodName)
	if !ok {
		return "", false
	}
	paramTypes, returnType := extensionMethodSignature(typeName, symbol)
	parts := append(paramTypes, returnType)
	return "Function[" + strings.Join(parts, ",") + "]", true
}

func (checker *TypeChecker) lookupExtensionMethod(typeName string, methodName string) (extensionMethodSymbol, bool) {
	typeName = checker.resolveTypeAlias(normalizeType(typeName))
	if child, ok := childType(typeName); ok {
		typeName = child.Parent
	}
	methods := checker.extensions[typeName]
	if methods == nil {
		if base, _, ok := splitGenericType(typeName); ok {
			methods = checker.extensions[base]
			if methods == nil {
				for declaredType, candidate := range checker.extensions {
					declaredBase, _, generic := splitGenericType(declaredType)
					if generic && declaredBase == base {
						methods = candidate
						break
					}
				}
			}
		}
	}
	symbol, ok := methods[methodName]
	return symbol, ok
}

func extensionMethodSignature(receiverType string, symbol extensionMethodSymbol) ([]string, string) {
	substitutions := map[string]string{}
	declaredBase, declaredArgs, declaredGeneric := splitGenericType(normalizeType(symbol.Target))
	actualBase, actualArgs, actualGeneric := splitGenericType(normalizeType(receiverType))
	if declaredGeneric && actualGeneric && declaredBase == actualBase {
		for index, declared := range declaredArgs {
			if index < len(actualArgs) {
				substitutions[normalizeType(declared)] = normalizeType(actualArgs[index])
			}
		}
	}
	paramTypes := make([]string, 0, len(symbol.Method.Params))
	for _, param := range symbol.Method.Params {
		paramTypes = append(paramTypes, applyAliasTypeSubstitutions(normalizeType(param.Type), substitutions))
	}
	returnType := applyAliasTypeSubstitutions(normalizeType(symbol.Method.ReturnType), substitutions)
	return paramTypes, returnType
}

func (checker *TypeChecker) aliasFieldType(typeName string, fieldName string) (string, bool) {
	alias, substitutions, ok := checker.aliasTypeInfo(typeName)
	if !ok {
		return "", false
	}
	for _, param := range alias.Params {
		if param.Name == fieldName {
			return applyAliasTypeSubstitutions(normalizeType(param.Type), substitutions), true
		}
	}
	switch fieldName {
	case "__type":
		return "String", true
	case "__hooks", "__methods", "__traits", "__impls":
		return "Int", true
	case "__struct":
		return "Bool", true
	default:
		return "", false
	}
}

func (checker *TypeChecker) isJSONSerializableType(typeName string) bool {
	typeName = normalizeType(typeName)
	switch typeName {
	case anyType, dynamicAnyType, "JSON", "String", "Char", "Int", "UInt", "Float", "Bool", "Null", "Table":
		return true
	}
	if checker.isStructAliasType(typeName) || checker.enums[typeName].Name != "" {
		return true
	}
	if elementType, ok := listElementTypeName(typeName); ok {
		return checker.isJSONSerializableType(elementType)
	}
	if keyType, valueType, ok := indexedMapTypes(typeName); ok {
		return keyType == "String" && checker.isJSONSerializableType(valueType)
	}
	if elementType, ok := optionElementType(typeName); ok {
		return checker.isJSONSerializableType(elementType)
	}
	return false
}

func (checker *TypeChecker) aliasTypeInfo(typeName string) (parser.AliasFunctionStatement, map[string]string, bool) {
	typeName = normalizeType(typeName)
	if alias, ok := checker.aliasFunctions[typeName]; ok {
		return alias, aliasDefaultSubstitutions(alias), true
	}
	base, args, ok := splitGenericType(typeName)
	if !ok {
		return parser.AliasFunctionStatement{}, nil, false
	}
	alias, exists := checker.aliasFunctions[base]
	if !exists {
		return parser.AliasFunctionStatement{}, nil, false
	}
	substitutions := aliasDefaultSubstitutions(alias)
	for index, typeParam := range alias.TypeParams {
		if index >= len(args) {
			break
		}
		substitutions[typeVariableKey(typeParam.Type)] = normalizeType(args[index])
		substitutions[typeParam.Name] = normalizeType(args[index])
	}
	return alias, substitutions, true
}

func aliasDefaultSubstitutions(alias parser.AliasFunctionStatement) map[string]string {
	substitutions := map[string]string{}
	for _, typeParam := range alias.TypeParams {
		substitutions[typeVariableKey(typeParam.Type)] = typeVariableKey(typeParam.Type)
		substitutions[typeParam.Name] = typeVariableKey(typeParam.Type)
	}
	return substitutions
}

func (checker *TypeChecker) aliasConstructedType(alias parser.AliasFunctionStatement, substitutions map[string]string) string {
	if len(alias.TypeParams) == 0 {
		return alias.Name
	}
	args := make([]string, 0, len(alias.TypeParams))
	for _, typeParam := range alias.TypeParams {
		key := typeVariableKey(typeParam.Type)
		if substitutions != nil {
			if concrete, ok := substitutions[key]; ok && concrete != "" {
				args = append(args, normalizeType(concrete))
				continue
			}
			if concrete, ok := substitutions[typeParam.Name]; ok && concrete != "" {
				args = append(args, normalizeType(concrete))
				continue
			}
		}
		args = append(args, key)
	}
	return alias.Name + "[" + strings.Join(args, ",") + "]"
}

func (checker *TypeChecker) aliasMethodReturnType(alias parser.AliasFunctionStatement, returnType string, substitutions map[string]string) string {
	returnType = normalizeType(returnType)
	if returnType == alias.Name {
		return checker.aliasConstructedType(alias, substitutions)
	}
	return applyAliasTypeSubstitutions(returnType, substitutions)
}

func applyAliasTypeSubstitutions(typeName string, substitutions map[string]string) string {
	typeName = normalizeType(typeName)
	if substitutions == nil {
		return typeName
	}
	if replacement, ok := substitutions[typeName]; ok {
		return normalizeType(replacement)
	}
	if name, _, ok := restrictedGenericType(typeName); ok {
		if replacement, exists := substitutions[name]; exists {
			return normalizeType(replacement)
		}
	}
	name, args, ok := splitGenericType(typeName)
	if !ok {
		return typeName
	}
	for index := range args {
		args[index] = applyAliasTypeSubstitutions(args[index], substitutions)
	}
	return name + "[" + strings.Join(args, ",") + "]"
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
	if targetType == "Parsable" || strings.HasPrefix(targetType, "Parsable[") {
		elementType := anyType
		if _, args, ok := splitGenericType(targetType); ok && len(args) == 1 {
			elementType = args[0]
		}
		switch fieldName {
		case "source", "original_source":
			return "String", true
		case "ast":
			return "List[Table]", true
		case "statement_count":
			return "Int", true
		case "runtime_type":
			return "Type", true
		case "runtime_info":
			return "Table", true
		case "cli_args", "source_args", "args", "keywords":
			return "List[String]", true
		case "message_poll":
			return "Table", true
		case "program":
			return "Program", true
		case "build_system":
			return "BuildSystem", true
		case "workspace":
			return "WorkSpace", true
		case "argument_type":
			return elementType, true
		}
	}
	if targetType == "JSON" {
		switch fieldName {
		case "kind":
			return "String", true
		case "count":
			return "Int", true
		default:
			return "JSON", true
		}
	}
	if targetType == "File" {
		switch fieldName {
		case "path", "name", "extension":
			return "String", true
		}
	}
	if targetType == "Atom" && fieldName == "name" {
		return "String", true
	}
	if targetType == "OS" {
		switch fieldName {
		case "name", "arch", "path_separator", "path_list_separator", "line_separator":
			return "String", true
		case "cpu_count":
			return "Int", true
		}
	}
	if targetType == "Type" {
		switch fieldName {
		case "name", "type_name", "category", "kind":
			return "String", true
		case "size", "alignment", "footprint", "field_count":
			return "Int", true
		case "fields", "serialization", "introspection", "layout":
			return "Table", true
		case "supports_serialization", "supports_introspection", "supports_memory_layout":
			return "Bool", true
		}
	}
	if fieldName == "cast_as" && checker.isStructAliasType(targetType) {
		return "Function[Type,T]", true
	}
	if methodType, ok := checker.aliasMethodType(targetType, fieldName); ok {
		return methodType, true
	}
	if methodType, ok := checker.extensionMethodType(targetType, fieldName); ok {
		return methodType, true
	}
	if fieldType, ok := checker.aliasFieldType(targetType, fieldName); ok {
		return fieldType, true
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
	if targetType == "String" || targetType == "Table" || targetType == "JSON" ||
		strings.HasPrefix(targetType, "List[") ||
		strings.HasPrefix(targetType, "Set[") ||
		strings.HasPrefix(targetType, "Map[") ||
		strings.HasPrefix(targetType, "SIMD[") ||
		strings.HasPrefix(targetType, "Iterator[") {
		return "Int", true
	}
	return "", false
}

func builtinProtocolMethodType(targetType string, methodName string) (string, bool) {
	targetType = normalizeType(targetType)
	if child, ok := childType(targetType); ok {
		targetType = child.Parent
	}
	if itemType, ok := pipelineItemType(targetType); ok {
		switch methodName {
		case "iter":
			return "Function[Iterator[" + itemType + "]]", true
		case "filter":
			return "Function[Function[" + itemType + ",Bool],Iterator[" + itemType + "]]", true
		case "map":
			return "Function[Function[" + itemType + ",T],Iterator[T]]", true
		case "skip", "limit":
			return "Function[Int,Iterator[" + itemType + "]]", true
		case "collect", "sort":
			return "Function[List[" + itemType + "]]", true
		case "first":
			return "Function[Option[" + itemType + "]]", true
		case "fold":
			return "Function[T,Function[T," + itemType + ",T],T]", true
		case "any", "all":
			return "Function[Function[" + itemType + ",Bool],Bool]", true
		case "for_each":
			return "Function[Function[" + itemType + ",T],Null]", true
		}
	}
	switch methodName {
	case "uppercase", "lowercase":
		if targetType == "String" || targetType == "Char" {
			return "Function[" + targetType + "]", true
		}
	case "times":
		if targetType == "Int" || targetType == "UInt" {
			return "Function[Function[Int,T],T]", true
		}
	case "read":
		if targetType == "File" {
			return "Function[Result[String,String]]", true
		}
	case "read_lines":
		if targetType == "File" {
			return "Function[Result[List[String],String]]", true
		}
	case "write", "append":
		if targetType == "File" {
			return "Function[String,Result[Int,String]]", true
		}
	case "exists":
		if targetType == "File" {
			return "Function[Result[Bool,String]]", true
		}
	case "size":
		if targetType == "File" {
			return "Function[Result[Int,String]]", true
		}
	case "create":
		if targetType == "File" {
			return "Function[Result[File,String]]", true
		}
	case "remove":
		if targetType == "File" {
			return "Function[Result[Bool,String]]", true
		}
	case "current_dir", "home_dir", "hostname":
		if targetType == "OS" {
			return "Function[Result[String,String]]", true
		}
	case "change_dir":
		if targetType == "OS" {
			return "Function[String,Result[Bool,String]]", true
		}
	case "temp_dir":
		if targetType == "OS" {
			return "Function[String]", true
		}
	case "process_id":
		if targetType == "OS" {
			return "Function[Int]", true
		}
	case "get_env":
		if targetType == "OS" {
			return "Function[String,Option[String]]", true
		}
	case "set_env":
		if targetType == "OS" {
			return "Function[String,String,Result[Bool,String]]", true
		}
	case "unset_env":
		if targetType == "OS" {
			return "Function[String,Result[Bool,String]]", true
		}
	case "environment":
		if targetType == "OS" {
			return "Function[Map[String,String]]", true
		}
	case "execute":
		if targetType == "OS" {
			return "Function[String,List[String],Result[Table,String]]", true
		}
	}
	return "", false
}

func pipelineItemType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	return iterableItemType(typeName)
}

func runtimeTypeInfoCallTarget(name string) (string, bool) {
	const suffix = ".get_runtime_type_info"
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	target := normalizeType(strings.TrimSuffix(name, suffix))
	if target == "" {
		return "", false
	}
	return target, true
}

func (checker *TypeChecker) checkDeclaredType(typeName string, source string, line int) {
	typeName = checker.resolveTypeAlias(typeName)
	if !isKnownType(typeName) {
		checker.addError(source, line, fmt.Sprintf("unknown type %s", typeName))
		return
	}
	if inner, ok := atomicItemType(typeName); ok && !checker.isThreadTransferSafeType(inner) {
		checker.addError(source, line, fmt.Sprintf("Atomic payload type %s is not thread-transfer-safe", inner))
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
		if !isTableKeyType(indexType) {
			checker.addError(source, line, fmt.Sprintf("Table index expects String, Int, UInt, Float, Bool, or Char key, got %s", indexType))
		}
		return anyType
	case targetType == "JSON":
		if indexType != anyType && indexType != "String" && !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("JSON index expects String or Int, got %s", indexType))
		}
		return "JSON"
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
		if !isTableKeyType(indexType) {
			checker.addError(source, line, fmt.Sprintf("Table index expects String, Int, UInt, Float, Bool, or Char key, got %s", indexType))
		}
		return anyType
	case targetType == "JSON":
		checker.addError(source, line, "JSON indexes cannot be assigned; JSON values are immutable")
		return "JSON"
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
	if macro, ok := checker.keywordMacros[name]; ok {
		return checker.checkKeywordMacroCall(macro, args, locals, source, line)
	}
	if typeName, ok := runtimeTypeInfoCallTarget(name); ok {
		if len(args) != 0 {
			checker.addError(source, line, fmt.Sprintf("%s expects 0 arguments", name))
		}
		if !isKnownType(typeName) {
			checker.addError(source, line, fmt.Sprintf("unknown type %s", typeName))
		}
		return "Type"
	}
	switch name {
	case "get_args_from_parsable":
		if len(args) != 0 {
			checker.addError(source, line, "get_args_from_parsable expects 0 arguments")
		}
		return "List[T]"
	case "macro_context":
		if len(args) != 0 {
			checker.addError(source, line, "macro_context expects 0 arguments")
		}
		return "Table"
	case "macro_expand":
		if len(args) < 1 || len(args) > 2 {
			checker.addError(source, line, "macro_expand expects source and an optional List[String] of source arguments")
			return "Parsable[T]"
		}
		if sourceType := checker.inferExpression(args[0], locals, source, line); !isAssignable("String", sourceType) {
			checker.addError(source, line, fmt.Sprintf("macro_expand source expects String, got %s", sourceType))
		}
		if len(args) == 2 {
			argType := checker.inferExpression(args[1], locals, source, line)
			if !isAssignable("List[String]", argType) {
				checker.addError(source, line, fmt.Sprintf("macro_expand source arguments expect List[String], got %s", argType))
			}
		}
		return "Parsable[T]"
	case "Parsable":
		if len(args) < 1 || len(args) > 2 {
			checker.addError(source, line, "Parsable expects source and an optional List[String] of source arguments")
			return "Parsable[T]"
		}
		if sourceType := checker.inferExpression(args[0], locals, source, line); !isAssignable("String", sourceType) {
			checker.addError(source, line, fmt.Sprintf("Parsable source expects String, got %s", sourceType))
		}
		if len(args) == 2 {
			argType := checker.inferExpression(args[1], locals, source, line)
			if !isAssignable("List[String]", argType) {
				checker.addError(source, line, fmt.Sprintf("Parsable source arguments expect List[String], got %s", argType))
			}
		}
		return "Parsable[T]"
	case "File":
		if len(args) != 1 {
			checker.addError(source, line, "File expects 1 argument")
			return "File"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("String", argType) {
			checker.addError(source, line, fmt.Sprintf("File path expects String, got %s", argType))
		}
		return "File"
	case "file_read", "file_read_lines", "file_exists", "file_size", "file_create", "file_remove":
		if len(args) != 1 {
			checker.addError(source, line, name+" expects 1 argument")
		} else {
			argType := checker.inferExpression(args[0], locals, source, line)
			if !isAssignable("File", argType) {
				checker.addError(source, line, fmt.Sprintf("%s expects File, got %s", name, argType))
			}
		}
		switch name {
		case "file_read":
			return "Result[String,String]"
		case "file_read_lines":
			return "Result[List[String],String]"
		case "file_exists":
			return "Result[Bool,String]"
		case "file_size":
			return "Result[Int,String]"
		case "file_create":
			return "Result[File,String]"
		default:
			return "Result[Bool,String]"
		}
	case "file_write", "file_append":
		if len(args) != 2 {
			checker.addError(source, line, name+" expects 2 arguments")
		} else {
			fileType := checker.inferExpression(args[0], locals, source, line)
			contentType := checker.inferExpression(args[1], locals, source, line)
			if !isAssignable("File", fileType) {
				checker.addError(source, line, fmt.Sprintf("%s expects File as first argument, got %s", name, fileType))
			}
			if !isAssignable("String", contentType) {
				checker.addError(source, line, fmt.Sprintf("%s content expects String, got %s", name, contentType))
			}
		}
		return "Result[Int,String]"
	case "Atom":
		if len(args) != 1 {
			checker.addError(source, line, "Atom expects 1 argument")
			return "Atom"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("String", argType) {
			checker.addError(source, line, fmt.Sprintf("Atom name expects String, got %s", argType))
		}
		return "Atom"
	case "OS":
		if len(args) != 0 {
			checker.addError(source, line, "OS expects 0 arguments")
		}
		return "OS"
	case "os_current_dir", "os_home_dir", "os_hostname":
		checker.checkOSArguments(name, args, []string{"OS"}, locals, source, line)
		return "Result[String,String]"
	case "os_change_dir":
		checker.checkOSArguments(name, args, []string{"OS", "String"}, locals, source, line)
		return "Result[Bool,String]"
	case "os_temp_dir":
		checker.checkOSArguments(name, args, []string{"OS"}, locals, source, line)
		return "String"
	case "os_process_id":
		checker.checkOSArguments(name, args, []string{"OS"}, locals, source, line)
		return "Int"
	case "os_get_env":
		checker.checkOSArguments(name, args, []string{"OS", "String"}, locals, source, line)
		return "Option[String]"
	case "os_set_env":
		checker.checkOSArguments(name, args, []string{"OS", "String", "String"}, locals, source, line)
		return "Result[Bool,String]"
	case "os_unset_env":
		checker.checkOSArguments(name, args, []string{"OS", "String"}, locals, source, line)
		return "Result[Bool,String]"
	case "os_environment":
		checker.checkOSArguments(name, args, []string{"OS"}, locals, source, line)
		return "Map[String,String]"
	case "os_execute":
		checker.checkOSArguments(name, args, []string{"OS", "String", "List[String]"}, locals, source, line)
		return "Result[Table,String]"
	case "parsable_source":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "String"
	case "parsable_ast":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "List[Table]"
	case "parsable_args":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "List[String]"
	case "parsable_runtime_info":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "Table"
	case "parsable_workspace":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "WorkSpace"
	case "parsable_with_source", "parsable_append":
		checker.checkParsableArguments(name, args, 2, locals, source, line)
		return "Result[Parsable[T],String]"
	case "parsable_replace":
		checker.checkParsableArguments(name, args, 3, locals, source, line)
		return "Result[Parsable[T],String]"
	case "parsable_begin_polling":
		checker.checkParsableArguments(name, args, 1, locals, source, line)
		return "Parsable[T]"
	case "parsable_poll_message", "parsable_intercept_message":
		checker.checkParsableMessageArguments(name, args, locals, source, line)
		return "Table"
	case "option_map":
		return checker.checkOptionMap(args, locals, source, line)
	case "option_unwrap_or":
		return checker.checkOptionUnwrapOr(args, locals, source, line)
	case "option_and_then":
		return checker.checkOptionAndThen(args, locals, source, line)
	case "result_map":
		return checker.checkResultMap(args, locals, source, line)
	case "result_map_err":
		return checker.checkResultMapErr(args, locals, source, line)
	case "result_unwrap_or":
		return checker.checkResultUnwrapOr(args, locals, source, line)
	case "result_and_then":
		return checker.checkResultAndThen(args, locals, source, line)
	case "copy", "clone":
		if len(args) != 1 {
			checker.addError(source, line, fmt.Sprintf("%s expects 1 argument", name))
			return anyType
		}
		return checker.inferExpression(args[0], locals, source, line)
	case "print":
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return anyType
	case "format", "printf":
		if len(args) != 2 {
			checker.addError(source, line, name+" expects 2 arguments")
			if name == "format" {
				return "String"
			}
			return "Int"
		}
		patternType := checker.inferExpression(args[0], locals, source, line)
		valuesType := checker.inferExpression(args[1], locals, source, line)
		if !isAssignable("String", patternType) {
			checker.addError(source, line, fmt.Sprintf("%s pattern expects String, got %s", name, patternType))
		}
		if !strings.HasPrefix(normalizeType(valuesType), "List[") && valuesType != anyType {
			checker.addError(source, line, fmt.Sprintf("%s values expect List[T], got %s", name, valuesType))
		}
		if name == "format" {
			return "String"
		}
		return "Int"
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
	case "read_int":
		if len(args) != 0 {
			checker.addError(source, line, "read_int expects 0 arguments")
		}
		return "Int"
	case "read_ints":
		if len(args) != 1 {
			checker.addError(source, line, "read_ints expects 1 argument")
		} else if argumentType := checker.inferExpression(args[0], locals, source, line); !isIntegerIndexType(argumentType) {
			checker.addError(source, line, fmt.Sprintf("read_ints count expects Int, got %s", argumentType))
		}
		return "List[Int]"
	case "print_ints":
		if len(args) != 1 {
			checker.addError(source, line, "print_ints expects 1 argument")
		} else if argumentType := normalizeType(checker.inferExpression(args[0], locals, source, line)); argumentType != "List[Int]" {
			checker.addError(source, line, fmt.Sprintf("print_ints expects List[Int], got %s", argumentType))
		}
		return anyType
	case "interval_walk_max_scores":
		if len(args) != 3 {
			checker.addError(source, line, "interval_walk_max_scores expects 3 arguments")
			return "List[Int]"
		}
		for index := 0; index < 2; index++ {
			argumentType := checker.inferExpression(args[index], locals, source, line)
			if !isIntegerIndexType(argumentType) {
				checker.addError(source, line, fmt.Sprintf(
					"interval_walk_max_scores argument %d expects Int, got %s", index+1, argumentType,
				))
			}
		}
		dataType := normalizeType(checker.inferExpression(args[2], locals, source, line))
		if dataType != "List[Int]" {
			checker.addError(source, line, fmt.Sprintf(
				"interval_walk_max_scores data expects List[Int], got %s", dataType,
			))
		}
		return "List[Int]"
	case "len":
		if len(args) != 1 {
			checker.addError(source, line, "len expects 1 argument")
		}
		return "Int"
	case "JSON":
		if len(args) != 1 {
			checker.addError(source, line, "JSON expects 1 argument")
			return "JSON"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !checker.isJSONSerializableType(argType) {
			checker.addError(source, line, fmt.Sprintf("JSON expects a serializable value, got %s", argType))
		}
		return "JSON"
	case "json_parse":
		if len(args) != 1 {
			checker.addError(source, line, "json_parse expects 1 argument")
			return "Result[JSON,String]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("String", argType) {
			checker.addError(source, line, fmt.Sprintf("json_parse expects String, got %s", argType))
		}
		return "Result[JSON,String]"
	case "json_decode":
		if len(args) != 1 {
			checker.addError(source, line, "json_decode expects 1 argument")
			return "Result[T,String]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !isAssignable("String", argType) {
			checker.addError(source, line, fmt.Sprintf("json_decode expects String, got %s", argType))
		}
		return "Result[T,String]"
	case "json_encode":
		if len(args) != 1 {
			checker.addError(source, line, "json_encode expects 1 argument")
			return "Result[String,String]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if !checker.isJSONSerializableType(argType) {
			checker.addError(source, line, fmt.Sprintf("json_encode expects a serializable value, got %s", argType))
		}
		return "Result[String,String]"
	case "json_stringify":
		if len(args) != 1 {
			checker.addError(source, line, "json_stringify expects 1 argument")
		} else {
			argType := checker.inferExpression(args[0], locals, source, line)
			if !checker.isJSONSerializableType(argType) {
				checker.addError(source, line, fmt.Sprintf("json_stringify expects a serializable value, got %s", argType))
			}
		}
		return "String"
	case "json_kind", "json_string", "json_int", "json_float", "json_bool", "json_is_null":
		if len(args) != 1 {
			checker.addError(source, line, name+" expects 1 argument")
		} else {
			argType := checker.inferExpression(args[0], locals, source, line)
			if !isAssignable("JSON", argType) {
				checker.addError(source, line, fmt.Sprintf("%s expects JSON, got %s", name, argType))
			}
		}
		switch name {
		case "json_kind":
			return "String"
		case "json_string":
			return "Option[String]"
		case "json_int":
			return "Option[Int]"
		case "json_float":
			return "Option[Float]"
		case "json_bool":
			return "Option[Bool]"
		default:
			return "Bool"
		}
	case "json_get":
		if len(args) != 2 {
			checker.addError(source, line, "json_get expects 2 arguments")
			return "Option[JSON]"
		}
		jsonType := checker.inferExpression(args[0], locals, source, line)
		indexType := checker.inferExpression(args[1], locals, source, line)
		if !isAssignable("JSON", jsonType) {
			checker.addError(source, line, fmt.Sprintf("json_get expects JSON as first argument, got %s", jsonType))
		}
		if indexType != anyType && indexType != "String" && !isIntegerIndexType(indexType) {
			checker.addError(source, line, fmt.Sprintf("json_get index expects String or Int, got %s", indexType))
		}
		return "Option[JSON]"
	case "debug_state":
		if len(args) != 0 {
			checker.addError(source, line, "debug_state expects 0 arguments")
		}
		return "List[Table]"
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
	case "Set":
		if len(args) > 1 {
			checker.addError(source, line, "Set expects 0 to 1 argument(s)")
			return "Set[T]"
		}
		if len(args) == 0 {
			return "Set[T]"
		}
		argType := checker.inferExpression(args[0], locals, source, line)
		if strings.HasPrefix(argType, "Set[") && strings.HasSuffix(argType, "]") {
			return argType
		}
		if strings.HasPrefix(argType, "List[") && strings.HasSuffix(argType, "]") {
			elementType := argType[len("List[") : len(argType)-1]
			if !isSetElementType(elementType) {
				checker.addError(source, line, fmt.Sprintf("Set item expects String, Int, UInt, Float, Bool, or Char, got %s", elementType))
			}
			return "Set[" + elementType + "]"
		}
		checker.addError(source, line, fmt.Sprintf("Set expects List[T] or Set[T], got %s", argType))
		return "Set[T]"
	case "Table":
		if len(args) > 1 {
			checker.addError(source, line, "Table expects 0 to 1 argument(s)")
		}
		for _, arg := range args {
			checker.inferExpression(arg, locals, source, line)
		}
		return "Table"
	case "table_has", "has_key":
		if len(args) != 2 {
			checker.addError(source, line, name+" expects 2 arguments")
			return "Bool"
		}
		tableType := checker.inferExpression(args[0], locals, source, line)
		keyType := checker.inferExpression(args[1], locals, source, line)
		if normalizeType(tableType) != "Table" && tableType != anyType {
			checker.addError(source, line, fmt.Sprintf("%s expects Table as first argument, got %s", name, tableType))
		}
		if !isTableKeyType(keyType) {
			checker.addError(source, line, fmt.Sprintf("%s key expects String, Int, UInt, Float, Bool, or Char, got %s", name, keyType))
		}
		return "Bool"
	case "set_has":
		if len(args) != 2 {
			checker.addError(source, line, "set_has expects 2 arguments")
			return "Bool"
		}
		setType := checker.inferExpression(args[0], locals, source, line)
		valueType := checker.inferExpression(args[1], locals, source, line)
		if elementType, ok := setElementType(setType); ok {
			if !isAssignable(elementType, valueType) {
				checker.addError(source, line, fmt.Sprintf("set_has value expects %s, got %s", elementType, valueType))
			}
		} else if setType != anyType {
			checker.addError(source, line, fmt.Sprintf("set_has expects Set as first argument, got %s", setType))
		}
		if !isSetElementType(valueType) {
			checker.addError(source, line, fmt.Sprintf("set_has value expects String, Int, UInt, Float, Bool, or Char, got %s", valueType))
		}
		return "Bool"
	case "table_delete":
		if len(args) != 2 {
			checker.addError(source, line, "table_delete expects 2 arguments")
			return "Table"
		}
		tableType := checker.inferExpression(args[0], locals, source, line)
		keyType := checker.inferExpression(args[1], locals, source, line)
		if normalizeType(tableType) != "Table" && tableType != anyType {
			checker.addError(source, line, fmt.Sprintf("table_delete expects Table as first argument, got %s", tableType))
		}
		if !isTableKeyType(keyType) {
			checker.addError(source, line, fmt.Sprintf("table_delete key expects String, Int, UInt, Float, Bool, or Char, got %s", keyType))
		}
		return "Table"
	case "table_keys", "table_values":
		if len(args) != 1 {
			checker.addError(source, line, name+" expects 1 argument")
			return "List[T]"
		}
		tableType := checker.inferExpression(args[0], locals, source, line)
		if normalizeType(tableType) != "Table" && tableType != anyType {
			checker.addError(source, line, fmt.Sprintf("%s expects Table, got %s", name, tableType))
		}
		return "List[T]"
	case "table_entries":
		if len(args) != 1 {
			checker.addError(source, line, "table_entries expects 1 argument")
			return "List[Table]"
		}
		tableType := checker.inferExpression(args[0], locals, source, line)
		if normalizeType(tableType) != "Table" && tableType != anyType {
			checker.addError(source, line, fmt.Sprintf("table_entries expects Table, got %s", tableType))
		}
		return "List[Table]"
	case "table_sequence_count":
		if len(args) != 1 {
			checker.addError(source, line, "table_sequence_count expects 1 argument")
			return "Int"
		}
		tableType := checker.inferExpression(args[0], locals, source, line)
		if normalizeType(tableType) != "Table" && tableType != anyType {
			checker.addError(source, line, fmt.Sprintf("table_sequence_count expects Table, got %s", tableType))
		}
		return "Int"
	case "table_set_fallback":
		if len(args) != 2 {
			checker.addError(source, line, "table_set_fallback expects 2 arguments")
			return "Table"
		}
		leftType := checker.inferExpression(args[0], locals, source, line)
		rightType := checker.inferExpression(args[1], locals, source, line)
		if normalizeType(leftType) != "Table" && leftType != anyType {
			checker.addError(source, line, fmt.Sprintf("table_set_fallback expects Table as first argument, got %s", leftType))
		}
		if normalizeType(rightType) != "Table" && rightType != anyType {
			checker.addError(source, line, fmt.Sprintf("table_set_fallback expects Table as second argument, got %s", rightType))
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
		workerName := normalizeNamespaceAccess(strings.TrimSpace(args[0]))
		worker, namedWorker := checker.lookupFunction(workerName)
		if !namedWorker {
			checker.addError(source, line, "spawn target must be a named workspace function without captured local state")
		} else {
			if worker.Async {
				checker.addError(source, line, "spawn target cannot be async; await async work on its owning thread")
			}
			checker.checkSpawnWorkerGlobals(worker, source, line)
			for index, param := range worker.Params {
				if !checker.isThreadTransferSafeType(param.Type) {
					checker.addError(source, line, fmt.Sprintf(
						"spawn parameter %d type %s is not thread-transfer-safe", index+1, normalizeType(param.Type),
					))
				}
			}
			if !checker.isThreadTransferSafeType(worker.ReturnType) {
				checker.addError(source, line, fmt.Sprintf(
					"spawn return type %s is not thread-transfer-safe", normalizeType(worker.ReturnType),
				))
			}
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
		valueType := checker.inferExpression(args[0], locals, source, line)
		if !checker.isThreadTransferSafeType(valueType) {
			checker.addError(source, line, fmt.Sprintf("Atomic value type %s is not thread-transfer-safe", normalizeType(valueType)))
		}
		return "Atomic[" + valueType + "]"
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
			return checker.aliasConstructedType(alias, nil)
		}
		solver := checker.newConstraintSolver()
		for index, arg := range args {
			argType := checker.inferExpression(arg, locals, source, line)
			param := alias.Params[index]
			if !checker.isAssignable(param.Type, argType) {
				checker.addError(source, line, fmt.Sprintf("alias function %s argument %d expects %s, got %s", name, index+1, param.Type, argType))
			}
			solver.unify(param.Type, argType)
		}
		return checker.aliasConstructedType(alias, solver.substitutions)
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
			if typeName := normalizeType(targetExpr); isKnownType(typeName) && methodName == "get_runtime_type_info" {
				if len(args) != 0 {
					checker.addError(source, line, fmt.Sprintf("%s expects 0 arguments", name))
				}
				return "Type"
			}
			targetSymbol, targetOK := checker.lookupVariable(targetExpr, locals)
			if !targetOK {
				if targetExpr == "True" || targetExpr == "False" {
					if method, methodOK := checker.lookupExtensionMethod("Bool", methodName); methodOK {
						return checker.checkExtensionCall(name, "Bool", method, args, locals, source, line)
					}
				}
				checker.addError(source, line, fmt.Sprintf("unknown function %q", name))
				return anyType
			}
			targetType := targetSymbol.Type
			if targetSymbol.InferredType != "" {
				targetType = targetSymbol.InferredType
			}
			if methodName == "cast_as" {
				if len(args) != 1 {
					checker.addError(source, line, fmt.Sprintf("cast_as expects exactly 1 target type, got %d", len(args)))
					return anyType
				}
				targetTypeName := checker.resolveTypeAlias(normalizeType(strings.TrimSpace(args[0])))
				switch targetTypeName {
				case "Table", "JSON", "String":
				default:
					if !isIdentifier(strings.TrimSpace(args[0])) || !checker.isStructAliasType(targetTypeName) {
						checker.addError(source, line, fmt.Sprintf("unknown or unsupported cast_as target %q", strings.TrimSpace(args[0])))
						return anyType
					}
				}
				return checker.checkStructCast(targetType, targetTypeName, source, line)
			}
			if resultType, handled := checker.checkPipelineMethodCall(targetType, methodName, args, locals, source, line); handled {
				return resultType
			}
			if methodType, methodOK := checker.aliasMethodType(targetType, methodName); methodOK {
				if method, ok := checker.aliasMethod(targetType, methodName); ok {
					checker.warnDeprecatedMethod(source, line, targetType, method)
				}
				paramTypes, returnType, _ := functionValueType(methodType)
				return checker.checkCallbackCall(name, paramTypes, returnType, args, locals, source, line)
			}
			if method, methodOK := checker.lookupExtensionMethod(targetType, methodName); methodOK {
				checker.warnDeprecatedMethod(source, line, targetType, method.Method)
				return checker.checkExtensionCall(name, targetType, method, args, locals, source, line)
			}
			if methodType, methodOK := builtinProtocolMethodType(targetType, methodName); methodOK {
				paramTypes, returnType, _ := functionValueType(methodType)
				return checker.checkCallbackCall(name, paramTypes, returnType, args, locals, source, line)
			}
		}
		checker.addError(source, line, fmt.Sprintf("unknown function %q", name))
		return anyType
	}
	if fn.Deprecated && !isStdlibImplementationSource(source) {
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

	callSolver := checker.newConstraintSolver()
	for index, arg := range args {
		argType := checker.inferExpression(arg, locals, source, line)
		param := fn.Params[index]
		if param.ByRef {
			identifier := strings.TrimSpace(arg)
			if isIdentifier(identifier) {
				variable, exists := checker.lookupVariableNoUse(identifier, locals)
				if !exists {
					checker.addError(source, line, fmt.Sprintf("function %s reference argument %d expects a variable", name, index+1))
				} else if !variable.Mutable {
					checker.addError(source, line, fmt.Sprintf("function %s reference argument %d requires mutable variable %q", name, index+1, identifier))
				}
			} else {
				checker.addError(source, line, fmt.Sprintf("function %s reference argument %d expects a variable", name, index+1))
			}
		}
		assignable := checker.isAssignable(param.Type, argType)
		if len(fn.TypeRestrictions) > 0 {
			assignable = callSolver.unify(param.Type, argType) || checker.isAssignable(callSolver.apply(param.Type), argType)
		}
		if !assignable {
			checker.addError(source, line, fmt.Sprintf("function %s argument %d expects %s, got %s", name, index+1, param.Type, argType))
		}
	}

	returnType := checker.inferGenericCallReturn(fn, args, locals, source, line)
	if fn.Async {
		return "Awaitable[" + returnType + "]"
	}
	return returnType
}

func (checker *TypeChecker) isStructAliasType(typeName string) bool {
	alias, _, ok := checker.aliasTypeInfo(typeName)
	return ok && alias.Struct
}

func (checker *TypeChecker) isThreadTransferSafeType(typeName string) bool {
	return checker.isThreadTransferSafeTypeSeen(checker.resolveTypeAlias(normalizeType(typeName)), map[string]bool{})
}

func (checker *TypeChecker) checkSpawnWorkerGlobals(worker functionSymbol, source string, line int) {
	locals := map[string]bool{}
	for _, param := range worker.Params {
		locals[param.Name] = true
	}
	if parsed, ok := checker.parseFunctionBodyForSemanticCheck(worker); ok {
		collectThreadWorkerLocals(parsed.Body, locals)
	}

	reported := map[string]bool{}
	tokens := lexer.New(worker.Body).Tokenize()
	for index, token := range tokens {
		if token.Type != lexer.TokenIdentifier || locals[token.Literal] || reported[token.Literal] {
			continue
		}
		if index > 0 && (tokens[index-1].Type == lexer.TokenDot || tokens[index-1].Type == lexer.TokenNamespaceAccess) {
			continue
		}
		global, ok := checker.globals[token.Literal]
		if !ok {
			continue
		}
		if global.Mutable {
			reported[token.Literal] = true
			checker.addError(source, line, fmt.Sprintf(
				"spawn worker %s accesses mutable global %s; use an immutable Atomic[T] binding",
				worker.Name, token.Literal,
			))
		} else if !checker.isThreadTransferSafeType(global.Type) {
			reported[token.Literal] = true
			checker.addError(source, line, fmt.Sprintf(
				"spawn worker %s accesses non-transferable global %s of type %s",
				worker.Name, token.Literal, normalizeType(global.Type),
			))
		}
	}
}

func collectThreadWorkerLocals(statements []parser.Statement, locals map[string]bool) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.VariableStatement:
			locals[current.Name] = true
		case parser.MultiVariableStatement:
			for _, binding := range current.Bindings {
				locals[binding.Name] = true
			}
		case parser.DestructuringStatement:
			collectThreadWorkerPatternLocals(current.Pattern, locals)
		case parser.FunctionStatement:
			locals[current.Name] = true
		case parser.IfStatement:
			collectThreadWorkerLocals(current.Consequence, locals)
			collectThreadWorkerLocals(current.Alternative, locals)
			if current.ElseIf != nil {
				collectThreadWorkerLocals([]parser.Statement{*current.ElseIf}, locals)
			}
		case parser.MatchStatement:
			for _, matchCase := range current.Cases {
				collectThreadWorkerLocals(matchCase.Body, locals)
			}
		case parser.LoopStatement:
			if len(current.Header.Tokens) > 0 && current.Header.Tokens[0].Type == lexer.TokenIdentifier {
				locals[current.Header.Tokens[0].Literal] = true
			}
			collectThreadWorkerLocals(current.Body, locals)
		case parser.TryCatchStatement:
			locals[current.ErrorName] = true
			collectThreadWorkerLocals(current.TryBody, locals)
			collectThreadWorkerLocals(current.CatchBody, locals)
		case parser.TransactionStatement:
			collectThreadWorkerLocals(current.Body, locals)
		case parser.DeferStatement:
			if current.Stmt != nil {
				collectThreadWorkerLocals([]parser.Statement{current.Stmt}, locals)
			}
			collectThreadWorkerLocals(current.Body, locals)
		case parser.RunStatement:
			if current.Stmt != nil {
				collectThreadWorkerLocals([]parser.Statement{current.Stmt}, locals)
			}
			collectThreadWorkerLocals(current.Body, locals)
		case parser.PrivateBlockStatement:
			collectThreadWorkerLocals(current.Body, locals)
		case parser.ScopeStatement:
			collectThreadWorkerLocals(current.Body, locals)
		}
	}
}

func collectThreadWorkerPatternLocals(pattern parser.DestructuringPattern, locals map[string]bool) {
	switch current := pattern.(type) {
	case parser.DestructuringBinding:
		locals[current.Name] = true
	case parser.DestructuringListPattern:
		for _, item := range current.Items {
			collectThreadWorkerPatternLocals(item, locals)
		}
	case parser.DestructuringObjectPattern:
		for _, field := range current.Fields {
			collectThreadWorkerPatternLocals(field.Pattern, locals)
		}
	}
}

func (checker *TypeChecker) isThreadTransferSafeTypeSeen(typeName string, seen map[string]bool) bool {
	typeName = checker.resolveTypeAlias(normalizeType(typeName))
	if _, allowed, ok := restrictedGenericType(typeName); ok {
		for _, option := range allowed {
			if normalizeConstraintName(option) == "transferable" ||
				checker.isThreadTransferSafeTypeSeen(option, seen) {
				continue
			}
			return false
		}
		return true
	}
	switch typeName {
	case "Null", "Int", "UInt", "Float", "Complex", "Bool", "Char", "String", "Atom", "JSON", "File", "OS":
		return true
	case anyType, dynamicAnyType, "Table", "Box", "Ref", "RefMut", "RefCell",
		"HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator",
		"Awaitable", "Iterator", "Coroutine", "Thread", "Function", "Parsable":
		return false
	}
	if isNumeric(typeName) || checker.enums[typeName].Name != "" {
		return true
	}
	if strings.HasPrefix(typeName, "Int.child(") || strings.HasPrefix(typeName, "UInt.child(") ||
		strings.HasPrefix(typeName, "Float.child(") || strings.HasPrefix(typeName, "Complex.child(") {
		return true
	}
	if base, args, ok := splitGenericType(typeName); ok {
		switch base {
		case "List", "Set", "Option", "SIMD", "Atomic":
			return len(args) == 1 && checker.isThreadTransferSafeTypeSeen(args[0], seen)
		case "Map", "Result":
			return len(args) == 2 &&
				checker.isThreadTransferSafeTypeSeen(args[0], seen) &&
				checker.isThreadTransferSafeTypeSeen(args[1], seen)
		case "Awaitable", "Iterator", "Coroutine", "Thread", "Function", "Parsable":
			return false
		}
	}
	alias, substitutions, ok := checker.aliasTypeInfo(typeName)
	if !ok || !alias.Struct || seen[typeName] {
		return false
	}
	seen[typeName] = true
	defer delete(seen, typeName)
	for _, field := range alias.Params {
		fieldType := applyAliasTypeSubstitutions(normalizeType(field.Type), substitutions)
		if !checker.isThreadTransferSafeTypeSeen(fieldType, seen) {
			return false
		}
	}
	return true
}

func (checker *TypeChecker) checkStructCast(sourceType string, targetType string, source string, line int) string {
	sourceType = checker.resolveTypeAlias(normalizeType(sourceType))
	targetType = checker.resolveTypeAlias(normalizeType(targetType))
	if !checker.isStructAliasType(sourceType) {
		checker.addError(source, line, fmt.Sprintf("cast_as requires a struct alias receiver, got %s", sourceType))
		return targetType
	}
	switch targetType {
	case "Table", "String":
		return targetType
	case "JSON":
		if !checker.isJSONSerializableType(sourceType) {
			checker.addError(source, line, fmt.Sprintf("%s cannot be serialized as JSON", sourceType))
		}
		return targetType
	}
	targetAlias, targetSubstitutions, ok := checker.aliasTypeInfo(targetType)
	if !ok || !targetAlias.Struct {
		checker.addError(source, line, fmt.Sprintf("cast_as target %s must be Table, JSON, String, or a struct alias", targetType))
		return targetType
	}
	for _, targetField := range targetAlias.Params {
		expected := applyAliasTypeSubstitutions(normalizeType(targetField.Type), targetSubstitutions)
		actual, exists := checker.aliasFieldType(sourceType, targetField.Name)
		if !exists {
			if targetField.Default.Node == nil {
				checker.addError(source, line, fmt.Sprintf("cannot cast %s to %s: required field %q is missing", sourceType, targetType, targetField.Name))
			}
			continue
		}
		if !checker.isAssignable(expected, actual) {
			checker.addError(source, line, fmt.Sprintf("cannot cast %s to %s: field %q expects %s, got %s", sourceType, targetType, targetField.Name, expected, actual))
		}
	}
	return targetType
}

func (checker *TypeChecker) checkPipelineMethodCall(targetType string, methodName string, args []string, locals map[string]variableSymbol, source string, line int) (string, bool) {
	itemType, supported := pipelineItemType(targetType)
	if !supported || !pipelineMethodTypeName(methodName) {
		return "", false
	}
	expectCount := func(count int) bool {
		if len(args) == count {
			return true
		}
		checker.addError(source, line, fmt.Sprintf("%s expects %d argument(s), got %d", methodName, count, len(args)))
		return false
	}
	callback := func(index int, expectedReturn string) string {
		if index >= len(args) {
			return anyType
		}
		callbackType := checker.inferExpression(args[index], locals, source, line)
		params, returnType, ok := functionValueType(callbackType)
		if !ok || len(params) != 1 {
			checker.addError(source, line, fmt.Sprintf("%s callback expects Function[%s,%s], got %s", methodName, itemType, expectedReturn, callbackType))
			return anyType
		}
		if !checker.isAssignable(params[0], itemType) && !checker.isAssignable(itemType, params[0]) {
			checker.addError(source, line, fmt.Sprintf("%s callback argument expects %s, got %s", methodName, itemType, params[0]))
		}
		if expectedReturn != anyType && !checker.isAssignable(expectedReturn, returnType) {
			checker.addError(source, line, fmt.Sprintf("%s callback must return %s, got %s", methodName, expectedReturn, returnType))
		}
		return returnType
	}
	switch methodName {
	case "iter":
		expectCount(0)
		return "Iterator[" + itemType + "]", true
	case "filter":
		if expectCount(1) {
			callback(0, "Bool")
		}
		return "Iterator[" + itemType + "]", true
	case "map":
		mappedType := anyType
		if expectCount(1) {
			mappedType = callback(0, anyType)
		}
		return "Iterator[" + mappedType + "]", true
	case "skip", "limit":
		if expectCount(1) {
			countType := checker.inferExpression(args[0], locals, source, line)
			if !checker.isAssignable("Int", countType) {
				checker.addError(source, line, fmt.Sprintf("%s expects Int, got %s", methodName, countType))
			}
		}
		return "Iterator[" + itemType + "]", true
	case "collect":
		expectCount(0)
		return "List[" + itemType + "]", true
	case "sort":
		expectCount(0)
		if !isComparableConstraintType(itemType) && itemType != anyType {
			checker.addError(source, line, fmt.Sprintf("sort requires comparable items, got %s", itemType))
		}
		return "List[" + itemType + "]", true
	case "first":
		expectCount(0)
		return "Option[" + itemType + "]", true
	case "any", "all":
		if expectCount(1) {
			callback(0, "Bool")
		}
		return "Bool", true
	case "for_each":
		if expectCount(1) {
			callback(0, anyType)
		}
		return "Null", true
	case "fold":
		if !expectCount(2) {
			return anyType, true
		}
		accumulatorType := checker.inferExpression(args[0], locals, source, line)
		callbackType := checker.inferExpression(args[1], locals, source, line)
		params, returnType, ok := functionValueType(callbackType)
		if !ok || len(params) != 2 {
			checker.addError(source, line, fmt.Sprintf("fold callback expects Function[%s,%s,%s], got %s", accumulatorType, itemType, accumulatorType, callbackType))
			return accumulatorType, true
		}
		if !checker.isAssignable(params[0], accumulatorType) || !checker.isAssignable(params[1], itemType) || !checker.isAssignable(accumulatorType, returnType) {
			checker.addError(source, line, fmt.Sprintf("fold callback expects Function[%s,%s,%s], got %s", accumulatorType, itemType, accumulatorType, callbackType))
		}
		return accumulatorType, true
	default:
		return "", false
	}
}

func pipelineMethodTypeName(name string) bool {
	switch name {
	case "iter", "filter", "map", "skip", "limit", "collect", "sort", "fold", "first", "any", "all", "for_each":
		return true
	default:
		return false
	}
}

func (checker *TypeChecker) checkKeywordMacroCall(macro parser.AliasStatement, args []string, locals map[string]variableSymbol, source string, line int) string {
	argumentTypes := make([]string, 0, len(args))
	for _, argument := range args {
		argumentTypes = append(argumentTypes, checker.inferExpression(argument, locals, source, line))
	}
	_, parameters, ok := splitGenericType(normalizeType(macro.Target))
	if !ok || len(parameters) != 1 {
		checker.addError(source, line, fmt.Sprintf("keyword macro %s must declare exactly one Parsable type parameter", macro.Name))
		return anyType
	}
	_, constraints, restricted := restrictedGenericType(parameters[0])
	if restricted {
		for _, argumentType := range argumentTypes {
			allowed := false
			for _, constraint := range constraints {
				if checker.constraintAllows(constraint, argumentType) {
					allowed = true
					break
				}
			}
			if !allowed {
				checker.addError(source, line, fmt.Sprintf("keyword macro %s requires %s, got %s", macro.Name, strings.Join(constraints, " or "), argumentType))
			}
		}
	}
	if keywordMacroReturnsExpansion(macro) {
		return anyType
	}
	if len(argumentTypes) == 0 {
		return anyType
	}
	return argumentTypes[0]
}

func keywordMacroReturnsExpansion(macro parser.AliasStatement) bool {
	for _, stmt := range macro.Body {
		returnStmt, ok := stmt.(parser.ReturnStatement)
		if !ok {
			continue
		}
		call, ok := returnStmt.Expression.Node.(parser.CallExpression)
		if !ok {
			continue
		}
		callee, ok := call.Callee.(parser.IdentifierExpression)
		if ok && callee.Name == "macro_expand" {
			return true
		}
	}
	return false
}

func (checker *TypeChecker) checkParsableArguments(name string, args []string, expected int, locals map[string]variableSymbol, source string, line int) {
	if len(args) != expected {
		checker.addError(source, line, fmt.Sprintf("%s expects %d argument(s)", name, expected))
		return
	}
	parsableType := checker.inferExpression(args[0], locals, source, line)
	if parsableType != anyType && parsableType != "Parsable" && !strings.HasPrefix(normalizeType(parsableType), "Parsable[") {
		checker.addError(source, line, fmt.Sprintf("%s expects Parsable as its first argument, got %s", name, parsableType))
	}
	for _, argument := range args[1:] {
		argumentType := checker.inferExpression(argument, locals, source, line)
		if !isAssignable("String", argumentType) {
			checker.addError(source, line, fmt.Sprintf("%s source arguments expect String, got %s", name, argumentType))
		}
	}
}

func (checker *TypeChecker) checkOSArguments(name string, args []string, expected []string, locals map[string]variableSymbol, source string, line int) {
	if len(args) != len(expected) {
		checker.addError(source, line, fmt.Sprintf("%s expects %d argument(s), got %d", name, len(expected), len(args)))
		return
	}
	for index, expectedType := range expected {
		actualType := checker.inferExpression(args[index], locals, source, line)
		if !isAssignable(expectedType, actualType) {
			checker.addError(source, line, fmt.Sprintf("%s argument %d expects %s, got %s", name, index+1, expectedType, actualType))
		}
	}
}

func (checker *TypeChecker) checkParsableMessageArguments(name string, args []string, locals map[string]variableSymbol, source string, line int) {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("%s expects 2 argument(s)", name))
		return
	}
	parsableType := checker.inferExpression(args[0], locals, source, line)
	if parsableType != anyType && parsableType != "Parsable" && !strings.HasPrefix(normalizeType(parsableType), "Parsable[") {
		checker.addError(source, line, fmt.Sprintf("%s expects Parsable as its first argument, got %s", name, parsableType))
	}
	messageType := checker.inferExpression(args[1], locals, source, line)
	if messageType != anyType && messageType != "Table" && !strings.HasPrefix(normalizeType(messageType), "Map[") {
		checker.addError(source, line, fmt.Sprintf("%s expects Table as its message argument, got %s", name, messageType))
	}
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
		if !checker.isAssignable(paramTypes[index], argType) {
			checker.addError(source, line, fmt.Sprintf("callback %s argument %d expects %s, got %s", name, index+1, paramTypes[index], argType))
		}
	}
	return returnType
}

func (checker *TypeChecker) checkExtensionCall(name string, receiverType string, symbol extensionMethodSymbol, args []string, locals map[string]variableSymbol, source string, line int) string {
	paramTypes, returnType := extensionMethodSignature(receiverType, symbol)
	required := requiredAliasParamCount(symbol.Method.Params)
	if len(args) < required || len(args) > len(symbol.Method.Params) {
		checker.addError(source, line, fmt.Sprintf("method %s expects %d to %d argument(s), got %d", name, required, len(symbol.Method.Params), len(args)))
		return returnType
	}
	for index, arg := range args {
		argType := checker.inferExpression(arg, locals, source, line)
		paramType := paramTypes[index]
		if !checker.isAssignable(paramType, argType) {
			checker.addError(source, line, fmt.Sprintf("method %s argument %d expects %s, got %s", name, index+1, paramType, argType))
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
	solver := checker.newConstraintSolver()
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
		variable.Used = true
		locals[name] = variable
		return variable, true
	}
	if variable, ok := checker.globals[name]; ok {
		return variable, true
	}
	return variableSymbol{}, false
}

func (checker *TypeChecker) lookupVariableNoUse(name string, locals map[string]variableSymbol) (variableSymbol, bool) {
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
	seen := map[string]bool{}
	for !seen[name] {
		seen[name] = true
		alias, target, ok := longestAliasPath(name, checker.aliases)
		if !ok {
			break
		}
		name = target + strings.TrimPrefix(name, alias)
	}
	return name
}

func longestAliasPath(name string, aliases map[string]string) (string, string, bool) {
	best := ""
	for alias := range aliases {
		if name != alias && !strings.HasPrefix(name, alias+".") {
			continue
		}
		if len(alias) > len(best) {
			best = alias
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, aliases[best], true
}

func (checker *TypeChecker) addError(source string, line int, message string) {
	checker.addStructuredError(source, line, 0, 0, "", "", message, "", "", "")
}

func (checker *TypeChecker) addStructuredError(
	source string,
	line int,
	column int,
	endColumn int,
	code string,
	rule string,
	message string,
	hint string,
	expectedType string,
	foundType string,
) {
	checker.errors = append(checker.errors, Error{
		Code:         code,
		Severity:     diagnostic.SeverityError,
		Phase:        diagnostic.PhaseType,
		File:         filepath.Clean(source),
		Line:         line,
		Column:       column,
		EndLine:      line,
		EndColumn:    endColumn,
		Message:      message,
		Rule:         rule,
		Hint:         hint,
		ExpectedType: expectedType,
		FoundType:    foundType,
	})
}

func (checker *TypeChecker) addWarning(source string, line int, message string) {
	checker.warnings = append(checker.warnings, Warning{
		Severity: diagnostic.SeverityWarning,
		Phase:    diagnostic.PhaseType,
		File:     filepath.Clean(source),
		Line:     line,
		Message:  message,
	})
}

func stripComments(input string) string {
	var output strings.Builder
	inString := false
	inChar := false
	blockDepth := 0
	for index := 0; index < len(input); index++ {
		current := input[index]
		next := byte(0)
		if index+1 < len(input) {
			next = input[index+1]
		}
		if blockDepth > 0 {
			if current == '(' && next == '*' {
				blockDepth++
				index++
				continue
			}
			if current == '*' && next == ')' {
				blockDepth--
				index++
				continue
			}
			if current == '\n' {
				output.WriteByte('\n')
			}
			continue
		}
		switch current {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		}
		if !inString && !inChar {
			if current == '-' && next == '-' {
				for index < len(input) && input[index] != '\n' {
					index++
				}
				if index < len(input) {
					output.WriteByte('\n')
				}
				continue
			}
			if current == '(' && next == '*' {
				blockDepth = 1
				index++
				continue
			}
		}
		output.WriteByte(current)
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

func enclosingExtensionEnd(input string, functionIndex int) (int, bool) {
	start := strings.LastIndex(input[:functionIndex], "#extend")
	if start == -1 {
		return 0, false
	}
	openBrace := findChar(input, '{', start+len("#extend"))
	if openBrace == -1 || openBrace > functionIndex {
		return 0, false
	}
	closeBrace := matchBrace(input, openBrace)
	if closeBrace == -1 || closeBrace < functionIndex {
		return 0, false
	}
	return closeBrace + 1, true
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
	if canonical, ok := canonicalRestrictedType(input); ok {
		return canonical
	}
	input = strings.ReplaceAll(input, " ", "")
	if alias, ok := builtinChildTypeAliases[input]; ok {
		return alias
	}
	if strings.HasPrefix(input, "types.") {
		if alias, ok := builtinChildTypeAliases[strings.TrimPrefix(input, "types.")]; ok {
			return alias
		}
	}
	if child, ok := canonicalChildType(input); ok {
		return child
	}
	switch input {
	case "int", "size":
		return "Int"
	case "uint":
		return "UInt"
	case "bool":
		return "Bool"
	case "string":
		return "String"
	case "float":
		return "Float"
	case "Any":
		return dynamicAnyType
	}
	return input
}

func normalizeGenericArgumentSeparators(input string) string {
	if !strings.Contains(input, ":") || !strings.Contains(input, "[") {
		return input
	}
	var builder strings.Builder
	depth := 0
	for _, char := range input {
		switch char {
		case '[':
			depth++
			builder.WriteRune(char)
		case ']':
			if depth > 0 {
				depth--
			}
			builder.WriteRune(char)
		case ':':
			if depth > 0 {
				builder.WriteRune(',')
			} else {
				builder.WriteRune(char)
			}
		default:
			builder.WriteRune(char)
		}
	}
	return builder.String()
}

var builtinChildTypeAliases = map[string]string{
	"i8":         "Int.child(8)",
	"i16":        "Int.child(16)",
	"i32":        "Int.child(32)",
	"i64":        "Int.child(64)",
	"u8":         "UInt.child(8)",
	"u16":        "UInt.child(16)",
	"u32":        "UInt.child(32)",
	"u64":        "UInt.child(64)",
	"float32":    "Float.child(32)",
	"float64":    "Float.child(64)",
	"complex64":  "Complex.child(64)",
	"complex128": "Complex.child(128)",
}

type childTypeSpec struct {
	Parent string
	Bits   int
}

func canonicalChildType(input string) (string, bool) {
	parent, bits, ok := parseChildType(input)
	if !ok || !isAllowedChildType(parent, bits) {
		return "", false
	}
	return fmt.Sprintf("%s.child(%d)", parent, bits), true
}

func parseChildType(input string) (string, int, bool) {
	input = strings.TrimSpace(input)
	open := strings.Index(input, ".child(")
	if open == -1 || !strings.HasSuffix(input, ")") {
		return "", 0, false
	}
	parent := normalizeType(input[:open])
	bitsText := strings.TrimSpace(input[open+len(".child(") : len(input)-1])
	bits, err := strconv.Atoi(bitsText)
	if err != nil {
		return "", 0, false
	}
	return parent, bits, true
}

func childType(input string) (childTypeSpec, bool) {
	parent, bits, ok := parseChildType(normalizeType(input))
	if !ok || !isAllowedChildType(parent, bits) {
		return childTypeSpec{}, false
	}
	return childTypeSpec{Parent: parent, Bits: bits}, true
}

func isAllowedChildType(parent string, bits int) bool {
	switch parent {
	case "Int", "UInt":
		return bits == 8 || bits == 16 || bits == 32 || bits == 64
	case "Float":
		return bits == 32 || bits == 64
	case "Complex":
		return bits == 64 || bits == 128
	default:
		return false
	}
}

func (checker *TypeChecker) checkOptionMap(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("option_map expects Option[T] and Function[T,U], got %d argument(s)", len(args)))
		return "Option[T]"
	}
	optionType := checker.inferExpression(args[0], locals, source, line)
	itemType, ok := optionElementType(optionType)
	if !ok && optionType != anyType {
		checker.addError(source, line, fmt.Sprintf("option_map first argument expects Option[T], got %s", optionType))
		itemType = anyType
	}
	callbackType := checker.inferExpression(args[1], locals, source, line)
	params, returnType, callbackOK := functionValueType(callbackType)
	if !callbackOK || len(params) != 1 {
		checker.addError(source, line, fmt.Sprintf("option_map callback expects Function[%s,U], got %s", itemType, callbackType))
		return "Option[T]"
	}
	if !checker.isAssignable(params[0], itemType) {
		checker.addError(source, line, fmt.Sprintf("option_map callback argument expects %s, got %s", itemType, params[0]))
	}
	return "Option[" + returnType + "]"
}

func (checker *TypeChecker) checkOptionUnwrapOr(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("option_unwrap_or expects Option[T] and fallback T, got %d argument(s)", len(args)))
		return anyType
	}
	optionType := checker.inferExpression(args[0], locals, source, line)
	itemType, ok := optionElementType(optionType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("option_unwrap_or first argument expects Option[T], got %s", optionType))
		itemType = anyType
	}
	fallbackType := checker.inferExpression(args[1], locals, source, line)
	if !checker.isAssignable(itemType, fallbackType) {
		checker.addError(source, line, fmt.Sprintf("option_unwrap_or fallback expects %s, got %s", itemType, fallbackType))
	}
	return itemType
}

func (checker *TypeChecker) checkOptionAndThen(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("option_and_then expects Option[T] and Function[T,Option[U]], got %d argument(s)", len(args)))
		return "Option[T]"
	}
	optionType := checker.inferExpression(args[0], locals, source, line)
	itemType, ok := optionElementType(optionType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("option_and_then first argument expects Option[T], got %s", optionType))
		itemType = anyType
	}
	callbackType := checker.inferExpression(args[1], locals, source, line)
	params, returnType, callbackOK := functionValueType(callbackType)
	if !callbackOK || len(params) != 1 {
		checker.addError(source, line, fmt.Sprintf("option_and_then callback expects Function[%s,Option[U]], got %s", itemType, callbackType))
		return "Option[T]"
	}
	if !checker.isAssignable(params[0], itemType) {
		checker.addError(source, line, fmt.Sprintf("option_and_then callback argument expects %s, got %s", itemType, params[0]))
	}
	if _, ok := optionElementType(returnType); !ok {
		checker.addError(source, line, fmt.Sprintf("option_and_then callback must return Option[U], got %s", returnType))
		return "Option[T]"
	}
	return returnType
}

func (checker *TypeChecker) checkResultMap(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("result_map expects Result[T,E] and Function[T,U], got %d argument(s)", len(args)))
		return "Result[T,T]"
	}
	resultType := checker.inferExpression(args[0], locals, source, line)
	okType, errType, ok := resultValueTypes(resultType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("result_map first argument expects Result[T,E], got %s", resultType))
		okType, errType = anyType, anyType
	}
	callbackType := checker.inferExpression(args[1], locals, source, line)
	params, returnType, callbackOK := functionValueType(callbackType)
	if !callbackOK || len(params) != 1 {
		checker.addError(source, line, fmt.Sprintf("result_map callback expects Function[%s,U], got %s", okType, callbackType))
		return "Result[T,T]"
	}
	if !checker.isAssignable(params[0], okType) {
		checker.addError(source, line, fmt.Sprintf("result_map callback argument expects %s, got %s", okType, params[0]))
	}
	return "Result[" + returnType + "," + errType + "]"
}

func (checker *TypeChecker) checkResultMapErr(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("result_map_err expects Result[T,E] and Function[E,F], got %d argument(s)", len(args)))
		return "Result[T,T]"
	}
	resultType := checker.inferExpression(args[0], locals, source, line)
	okType, errType, ok := resultValueTypes(resultType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("result_map_err first argument expects Result[T,E], got %s", resultType))
		okType, errType = anyType, anyType
	}
	callbackType := checker.inferExpression(args[1], locals, source, line)
	params, returnType, callbackOK := functionValueType(callbackType)
	if !callbackOK || len(params) != 1 {
		checker.addError(source, line, fmt.Sprintf("result_map_err callback expects Function[%s,F], got %s", errType, callbackType))
		return "Result[T,T]"
	}
	if !checker.isAssignable(params[0], errType) {
		checker.addError(source, line, fmt.Sprintf("result_map_err callback argument expects %s, got %s", errType, params[0]))
	}
	return "Result[" + okType + "," + returnType + "]"
}

func (checker *TypeChecker) checkResultUnwrapOr(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("result_unwrap_or expects Result[T,E] and fallback T, got %d argument(s)", len(args)))
		return anyType
	}
	resultType := checker.inferExpression(args[0], locals, source, line)
	okType, _, ok := resultValueTypes(resultType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("result_unwrap_or first argument expects Result[T,E], got %s", resultType))
		okType = anyType
	}
	fallbackType := checker.inferExpression(args[1], locals, source, line)
	if !checker.isAssignable(okType, fallbackType) {
		checker.addError(source, line, fmt.Sprintf("result_unwrap_or fallback expects %s, got %s", okType, fallbackType))
	}
	return okType
}

func (checker *TypeChecker) checkResultAndThen(args []string, locals map[string]variableSymbol, source string, line int) string {
	if len(args) != 2 {
		checker.addError(source, line, fmt.Sprintf("result_and_then expects Result[T,E] and Function[T,Result[U,E]], got %d argument(s)", len(args)))
		return "Result[T,T]"
	}
	resultType := checker.inferExpression(args[0], locals, source, line)
	okType, errType, ok := resultValueTypes(resultType)
	if !ok {
		checker.addError(source, line, fmt.Sprintf("result_and_then first argument expects Result[T,E], got %s", resultType))
		okType, errType = anyType, anyType
	}
	callbackType := checker.inferExpression(args[1], locals, source, line)
	params, returnType, callbackOK := functionValueType(callbackType)
	if !callbackOK || len(params) != 1 {
		checker.addError(source, line, fmt.Sprintf("result_and_then callback expects Function[%s,Result[U,%s]], got %s", okType, errType, callbackType))
		return "Result[T,T]"
	}
	if !checker.isAssignable(params[0], okType) {
		checker.addError(source, line, fmt.Sprintf("result_and_then callback argument expects %s, got %s", okType, params[0]))
	}
	_, callbackErrType, returnOK := resultValueTypes(returnType)
	if !returnOK {
		checker.addError(source, line, fmt.Sprintf("result_and_then callback must return Result[U,%s], got %s", errType, returnType))
		return "Result[T,T]"
	}
	if !checker.isAssignable(errType, callbackErrType) {
		checker.addError(source, line, fmt.Sprintf("result_and_then callback error type expects %s, got %s", errType, callbackErrType))
	}
	return returnType
}

func canonicalRestrictedType(input string) (string, bool) {
	input = strings.TrimSpace(input)
	if name, constraint, ok := splitNamedGenericConstraint(input); ok {
		return name + ":" + normalizeConstraintName(constraint), true
	}
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

func splitNamedGenericConstraint(input string) (string, string, bool) {
	input = strings.TrimSpace(input)
	fields := strings.Fields(input)
	if len(fields) != 2 || fields[0] == "" || fields[1] == "" || fields[1] == "restrict" {
		return "", "", false
	}
	if strings.ContainsAny(fields[0], "[],:|.") || strings.ContainsAny(fields[1], "[],:|.") {
		return "", "", false
	}
	if !isGenericConstraintName(fields[1]) && !isTraitConstraintName(fields[1]) {
		return "", "", false
	}
	return normalizeType(fields[0]), fields[1], true
}

func normalizeConstraintName(name string) string {
	name = strings.TrimSpace(name)
	if name == "allocator-like" {
		return "allocator_like"
	}
	return name
}

func isGenericConstraintName(name string) bool {
	switch normalizeConstraintName(name) {
	case "numeric", "comparable", "hashable", "iterable", "allocator_like", "transferable":
		return true
	default:
		return false
	}
}

func isTraitConstraintName(name string) bool {
	if name == "" {
		return false
	}
	return unicode.IsUpper(rune(name[0]))
}

func parseTypeRestrictions(input string) (map[string]string, error) {
	restrictions := map[string]string{}
	if strings.TrimSpace(input) == "" {
		return restrictions, nil
	}
	for _, part := range splitTopLevel(input, ',') {
		canonical, ok := canonicalRestrictedType(part)
		if !ok {
			return nil, fmt.Errorf("generic restriction %q must be written as T restrict[Type,...] or T constraint", strings.TrimSpace(part))
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
	if parts, ok := tupleTypeParts(typeName); ok {
		for index := range parts {
			parts[index] = applyFunctionTypeRestrictions(parts[index], restrictions)
		}
		return "(" + strings.Join(parts, ",") + ")"
	}
	name, args, ok := splitGenericType(typeName)
	if !ok {
		return typeName
	}
	for index := range args {
		args[index] = applyFunctionTypeRestrictions(args[index], restrictions)
	}
	return name + "[" + strings.Join(args, ",") + "]"
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
			if isGenericConstraintName(option) || isTraitConstraintName(option) {
				continue
			}
			if !isKnownType(option) {
				return false
			}
		}
		return true
	}
	if _, ok := childType(typeName); ok {
		return true
	}
	if typeName == anyType || typeName == dynamicAnyType || typeName == "Int" || typeName == "UInt" || typeName == "String" || typeName == "Atom" || typeName == "JSON" || typeName == "File" || typeName == "OS" || typeName == "Parsable" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" || typeName == "Complex" || typeName == "Type" ||
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
	if strings.HasPrefix(typeName, "Set[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Set[") : len(typeName)-1])
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
	if strings.HasPrefix(typeName, "Parsable[") && strings.HasSuffix(typeName, "]") {
		return isKnownType(typeName[len("Parsable[") : len(typeName)-1])
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
		!strings.HasPrefix(typeName, "List[") && !strings.HasPrefix(typeName, "Set[") && !strings.HasPrefix(typeName, "Map[") &&
		!strings.HasPrefix(typeName, "Option[") && !strings.HasPrefix(typeName, "Result[") &&
		!strings.HasPrefix(typeName, "SIMD[") && !strings.HasPrefix(typeName, "Function[") &&
		!strings.HasPrefix(typeName, "Awaitable[") && !strings.HasPrefix(typeName, "Iterator[") &&
		!strings.HasPrefix(typeName, "Coroutine[") && !strings.HasPrefix(typeName, "Thread[") &&
		!strings.HasPrefix(typeName, "Atomic[") && !strings.HasPrefix(typeName, "Parsable[")
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

func setElementType(typeName string) (string, bool) {
	typeName = normalizeType(typeName)
	if !strings.HasPrefix(typeName, "Set[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	inner := normalizeType(typeName[len("Set[") : len(typeName)-1])
	return inner, inner != ""
}

func isSetElementType(typeName string) bool {
	return isTableKeyType(typeName)
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

func isAtomLiteral(expr string) bool {
	tokens := lexer.New(strings.TrimSpace(expr)).Tokenize()
	return len(tokens) == 2 && tokens[0].Type == lexer.TokenAtom && tokens[1].Type == lexer.TokenEOFDescriptor
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

func isTableKeyType(typeName string) bool {
	typeName = normalizeType(typeName)
	if _, allowed, ok := restrictedGenericType(typeName); ok {
		for _, option := range allowed {
			if normalizeConstraintName(option) == "hashable" || isTableKeyType(option) {
				continue
			}
			return false
		}
		return true
	}
	return typeName == anyType || typeName == "String" || typeName == "Atom" || typeName == "Int" || typeName == "UInt" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" ||
		strings.HasPrefix(typeName, "Int.child(") || strings.HasPrefix(typeName, "UInt.child(") ||
		strings.HasPrefix(typeName, "Float.child(")
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
			if isGenericConstraintName(option) && builtinConstraintAllows(option, source) {
				return true
			}
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
	if targetChild, ok := childType(target); ok {
		if sourceChild, sourceOK := childType(source); sourceOK {
			return targetChild.Parent == sourceChild.Parent && targetChild.Bits >= sourceChild.Bits
		}
		return source == targetChild.Parent ||
			(targetChild.Parent == "UInt" && source == "Int") ||
			(targetChild.Parent == "Float" && (source == "Int" || source == "UInt"))
	}
	if sourceChild, ok := childType(source); ok {
		return target == sourceChild.Parent ||
			(target == "Float" && (sourceChild.Parent == "Int" || sourceChild.Parent == "UInt")) ||
			(target == "Complex" && sourceChild.Parent == "Complex")
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
	if strings.HasPrefix(target, "Set[") && source == "Set[T]" {
		return true
	}
	if strings.HasPrefix(target, "Set[") && strings.HasPrefix(source, "Set[") {
		return isAssignable(target[len("Set["):len(target)-1], source[len("Set["):len(source)-1])
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

func (checker *TypeChecker) isAssignable(target string, source string) bool {
	target = normalizeType(target)
	source = normalizeType(source)
	if checker.aliasTypesAssignable(target, source) {
		return true
	}
	if solver := checker.newConstraintSolver(); solver.unify(target, source) {
		return true
	}
	if _, allowed, ok := restrictedGenericType(target); ok {
		for _, option := range allowed {
			if checker.constraintAllows(option, source) {
				return true
			}
		}
		return false
	}
	return isAssignable(target, source)
}

func (checker *TypeChecker) aliasTypesAssignable(target string, source string) bool {
	target = normalizeType(target)
	source = normalizeType(source)
	targetBase, targetArgs := checker.aliasBaseAndArgs(target)
	sourceBase, sourceArgs := checker.aliasBaseAndArgs(source)
	if targetBase == "" || sourceBase == "" || targetBase != sourceBase {
		return false
	}
	if len(targetArgs) == 0 || len(sourceArgs) == 0 {
		return true
	}
	if len(targetArgs) != len(sourceArgs) {
		return false
	}
	for index := range targetArgs {
		if !checker.isAssignable(targetArgs[index], sourceArgs[index]) {
			return false
		}
	}
	return true
}

func (checker *TypeChecker) aliasBaseAndArgs(typeName string) (string, []string) {
	typeName = normalizeType(typeName)
	if _, ok := checker.aliasFunctions[typeName]; ok {
		return typeName, nil
	}
	base, args, ok := splitGenericType(typeName)
	if !ok {
		return "", nil
	}
	if _, exists := checker.aliasFunctions[base]; !exists {
		return "", nil
	}
	for index := range args {
		args[index] = normalizeType(args[index])
	}
	return base, args
}

func (checker *TypeChecker) constraintAllows(constraint string, typeName string) bool {
	constraint = normalizeConstraintName(normalizeType(constraint))
	typeName = normalizeType(typeName)
	if constraint == dynamicAnyType {
		return true
	}
	if constraint == "transferable" {
		return checker.isThreadTransferSafeType(typeName)
	}
	if isGenericConstraintName(constraint) {
		return builtinConstraintAllows(constraint, typeName)
	}
	if _, ok := checker.traits[constraint]; ok {
		return checker.typeImplementsTrait(typeName, constraint)
	}
	return isAssignable(constraint, typeName)
}

func (checker *TypeChecker) typeImplementsTrait(typeName string, traitName string) bool {
	typeName = normalizeType(typeName)
	traitName = normalizeConstraintName(traitName)
	if impls, ok := checker.traitImpls[traitName]; ok && impls[typeName] {
		return true
	}
	return false
}

func builtinConstraintAllows(constraint string, typeName string) bool {
	constraint = normalizeConstraintName(constraint)
	typeName = normalizeType(typeName)
	switch constraint {
	case "numeric":
		return isNumericConstraintType(typeName)
	case "comparable":
		return isComparableConstraintType(typeName)
	case "hashable":
		return isHashableConstraintType(typeName)
	case "iterable":
		_, ok := iterableItemType(typeName)
		return ok
	case "allocator_like":
		return isAllocatorType(typeName)
	case "transferable":
		return isBuiltinThreadTransferSafeType(typeName)
	default:
		return false
	}
}

func isNumericConstraintType(typeName string) bool {
	typeName = normalizeType(typeName)
	if child, ok := childType(typeName); ok {
		return child.Parent == "Int" || child.Parent == "UInt" || child.Parent == "Float" || child.Parent == "Complex"
	}
	switch typeName {
	case "Int", "UInt", "Float", "Complex":
		return true
	default:
		return false
	}
}

func isBuiltinThreadTransferSafeType(typeName string) bool {
	typeName = normalizeType(typeName)
	switch typeName {
	case "Null", "Int", "UInt", "Float", "Complex", "Bool", "Char", "String", "Atom", "JSON", "File", "OS":
		return true
	}
	if child, ok := childType(typeName); ok {
		return child.Parent == "Int" || child.Parent == "UInt" || child.Parent == "Float" || child.Parent == "Complex"
	}
	if _, allowed, ok := restrictedGenericType(typeName); ok {
		for _, option := range allowed {
			if normalizeConstraintName(option) == "transferable" || isBuiltinThreadTransferSafeType(option) {
				continue
			}
			return false
		}
		return true
	}
	if base, args, ok := splitGenericType(typeName); ok {
		switch base {
		case "List", "Set", "Option", "SIMD", "Atomic":
			return len(args) == 1 && isBuiltinThreadTransferSafeType(args[0])
		case "Map", "Result":
			return len(args) == 2 &&
				isBuiltinThreadTransferSafeType(args[0]) &&
				isBuiltinThreadTransferSafeType(args[1])
		}
	}
	return false
}

func isComparableConstraintType(typeName string) bool {
	typeName = normalizeType(typeName)
	if isHashableConstraintType(typeName) || typeName == "Complex" {
		return true
	}
	if child, ok := childType(typeName); ok && child.Parent == "Complex" {
		return true
	}
	return false
}

func isHashableConstraintType(typeName string) bool {
	typeName = normalizeType(typeName)
	return isTableKeyType(typeName)
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
	checker       *TypeChecker
}

func newConstraintSolver() *constraintSolver {
	return &constraintSolver{substitutions: map[string]string{}}
}

func (checker *TypeChecker) newConstraintSolver() *constraintSolver {
	return &constraintSolver{substitutions: map[string]string{}, checker: checker}
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
	if key == normalizeType(typeName) {
		return true
	}
	if _, allowed, ok := restrictedGenericType(name); ok {
		allowedMatch := false
		for _, option := range allowed {
			if solver.constraintAllows(option, typeName) {
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

func (solver *constraintSolver) constraintAllows(constraint string, typeName string) bool {
	constraint = normalizeConstraintName(normalizeType(constraint))
	typeName = normalizeType(typeName)
	if constraint == dynamicAnyType {
		return true
	}
	if constraint == "transferable" && solver.checker != nil {
		return solver.checker.isThreadTransferSafeType(typeName)
	}
	if isGenericConstraintName(constraint) {
		return builtinConstraintAllows(constraint, typeName)
	}
	if solver.checker != nil {
		if _, ok := solver.checker.traits[constraint]; ok {
			return solver.checker.typeImplementsTrait(typeName, constraint)
		}
	}
	return isAssignable(constraint, typeName)
}

func (solver *constraintSolver) apply(typeName string) string {
	return solver.applyWithSeen(typeName, map[string]bool{})
}

func (solver *constraintSolver) applyWithSeen(typeName string, seen map[string]bool) string {
	typeName = normalizeType(typeName)
	if seen[typeName] {
		return typeName
	}
	seen[typeName] = true
	if resolved, exists := solver.substitutions[typeName]; exists {
		return solver.applyWithSeen(resolved, seen)
	}
	if name, _, ok := restrictedGenericType(typeName); ok {
		if resolved, exists := solver.substitutions[name]; exists {
			return solver.applyWithSeen(resolved, seen)
		}
	}
	name, args, ok := splitGenericType(typeName)
	if !ok {
		return typeName
	}
	for index, arg := range args {
		args[index] = solver.applyWithSeen(arg, copySeenTypes(seen))
	}
	return name + "[" + strings.Join(args, ",") + "]"
}

func copySeenTypes(seen map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(seen))
	for key, value := range seen {
		copied[key] = value
	}
	return copied
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
	colon := strings.Index(typeName, ":")
	if colon <= 0 || colon+1 >= len(typeName) {
		return "", nil, false
	}
	name := strings.TrimSpace(typeName[:colon])
	if name == "" || strings.ContainsAny(name, "[]|,") {
		return "", nil, false
	}
	parts := strings.Split(typeName[colon+1:], "|")
	if len(parts) == 0 {
		return "", nil, false
	}
	for index, part := range parts {
		parts[index] = normalizeConstraintName(normalizeType(part))
		if parts[index] == "" {
			return "", nil, false
		}
	}
	return name, parts, true
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
	if !isBuiltinCastTarget(target) {
		return false
	}
	if source == anyType || target == anyType || source == target {
		return true
	}
	if source == "String" && target == "JSON" || source == "JSON" && target == "String" {
		return true
	}
	if source == "String" && target == "File" || source == "File" && target == "String" {
		return true
	}
	if source == "String" && target == "Atom" || source == "Atom" && target == "String" {
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

func isBuiltinCastTarget(typeName string) bool {
	typeName = normalizeType(typeName)
	if typeName == "" {
		return false
	}
	if _, ok := childType(typeName); ok {
		return true
	}
	if _, _, ok := restrictedGenericType(typeName); ok {
		return true
	}
	if isGenericCastTargetVariable(typeName) {
		return true
	}
	if isAllocatorType(typeName) {
		return true
	}
	switch typeName {
	case anyType, dynamicAnyType,
		"Int", "UInt", "String", "Atom", "JSON", "File", "OS", "Parsable",
		"Float", "Bool", "Char", "Complex", "Type",
		"Table", "Program", "BuildSystem", "WorkSpace",
		"JSModule", "JSCall", "Context", "ErrorContext":
		return true
	}
	name, _, ok := splitGenericType(typeName)
	if !ok {
		return false
	}
	switch name {
	case "List", "Set", "Map", "Option", "Result", "SIMD",
		"Awaitable", "Iterator", "Coroutine", "Thread", "Atomic",
		"Parsable", "Function":
		return true
	default:
		return false
	}
}

func isGenericCastTargetVariable(typeName string) bool {
	runes := []rune(typeName)
	return len(runes) == 1 && unicode.IsUpper(runes[0])
}

func isScalarType(typeName string) bool {
	typeName = normalizeType(typeName)
	if _, ok := childType(typeName); ok {
		return true
	}
	return typeName == "Int" || typeName == "UInt" || typeName == "String" ||
		typeName == "Float" || typeName == "Bool" || typeName == "Char" || typeName == "Complex"
}

func isNumeric(typeName string) bool {
	typeName = normalizeType(typeName)
	if _, ok := childType(typeName); ok {
		return true
	}
	return typeName == "Int" || typeName == "UInt" || typeName == "Float" || typeName == "Complex" ||
		typeName == anyType || strings.HasPrefix(typeName, "SIMD[")
}

func numericResult(left string, right string) string {
	left = normalizeType(left)
	right = normalizeType(right)
	if child, ok := childType(left); ok {
		left = child.Parent
	}
	if child, ok := childType(right); ok {
		right = child.Parent
	}
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
		return "Table", true
	case strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]"):
		return typeName[5 : len(typeName)-1], true
	case strings.HasPrefix(typeName, "Set[") && strings.HasSuffix(typeName, "]"):
		return typeName[len("Set[") : len(typeName)-1], true
	case strings.HasPrefix(typeName, "Iterator[") && strings.HasSuffix(typeName, "]"):
		return typeName[len("Iterator[") : len(typeName)-1], true
	case strings.HasPrefix(typeName, "Map[") && strings.HasSuffix(typeName, "]"):
		return "Table", true
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
			if unicode.IsDigit(char) || !isIdentifierRune(char) {
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
	if target == "" {
		return "", false
	}
	typeName := normalizeType(target)
	if !isKnownType(typeName) {
		return "", false
	}
	return typeName, true
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
	return unicode.IsLetter(char) || unicode.IsDigit(char) || unicode.IsMark(char) || unicode.IsSymbol(char) || char == '_'
}

func isIntegerLiteral(input string) bool {
	input = normalizeNumericLiteral(input)
	_, err := strconv.ParseInt(input, 0, 64)
	return err == nil
}

func isFloatLiteral(input string) bool {
	input = normalizeNumericLiteral(input)
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

func childTypeLiteralFits(typeName string, expr string) bool {
	spec, ok := childType(typeName)
	if !ok {
		return true
	}
	expr = strings.TrimSpace(expr)
	switch spec.Parent {
	case "Int":
		if !isIntegerLiteral(expr) {
			return true
		}
		value, err := strconv.ParseInt(normalizeNumericLiteral(expr), 0, 64)
		if err != nil {
			return false
		}
		min, max := signedBitRange(spec.Bits)
		return value >= min && value <= max
	case "UInt":
		if !isIntegerLiteral(expr) {
			return true
		}
		if strings.HasPrefix(expr, "-") {
			return false
		}
		value, err := strconv.ParseUint(normalizeNumericLiteral(expr), 0, 64)
		if err != nil {
			return false
		}
		return value <= unsignedBitMax(spec.Bits)
	case "Float":
		return isIntegerLiteral(expr) || isFloatLiteral(expr)
	case "Complex":
		return true
	default:
		return true
	}
}

func normalizeNumericLiteral(input string) string {
	return strings.ReplaceAll(input, "_", "")
}

func signedBitRange(bits int) (int64, int64) {
	if bits >= 64 {
		return -1 << 63, 1<<63 - 1
	}
	max := int64(1)<<(bits-1) - 1
	return -int64(1) << (bits - 1), max
}

func unsignedBitMax(bits int) uint64 {
	if bits >= 64 {
		return ^uint64(0)
	}
	return uint64(1)<<bits - 1
}
