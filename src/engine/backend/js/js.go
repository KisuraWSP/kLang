package jsbackend

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"kLang/src/engine/backend"
	"kLang/src/engine/file"
	"kLang/src/engine/ir"
	"kLang/src/lexer"
	"kLang/src/parser"
)

type Compiler struct{}

func New() *Compiler {
	return &Compiler{}
}

func (compiler *Compiler) Name() string {
	return "JS"
}

func (compiler *Compiler) Check(request backend.Request) []backend.Diagnostic {
	_, diagnostics := lowerProgram(request)
	return diagnostics
}

// LowerIR exposes the typed-core lowering shared by native backends.
func LowerIR(request backend.Request) (ir.Program, []backend.Diagnostic) {
	return lowerProgram(request)
}

func (compiler *Compiler) Emit(request backend.Request) (backend.Output, error) {
	program, diagnostics := lowerProgram(request)
	if len(diagnostics) != 0 {
		return backend.Output{}, fmt.Errorf("JS backend rejected %d unsupported construct(s)", len(diagnostics))
	}
	source, sourceMap := emitProgram(program)
	packageJSON := "{\n  \"private\": true,\n  \"scripts\": {\n    \"start\": \"node --enable-source-maps program.js\"\n  }\n}\n"
	readme := "# kLang JavaScript Build\n\nThis experimental artifact is generated from kLang's typed core subset. Run it with `npm start` for native source-map support, or `node program.js` for the built-in kLang runtime diagnostic.\n"
	return backend.Output{
		Entry: "program.js",
		Artifacts: []backend.Artifact{
			{Path: "program.js", Content: []byte(source)},
			{Path: "program.js.map", Content: sourceMap},
			{Path: "package.json", Content: []byte(packageJSON)},
			{Path: "README.md", Content: []byte(readme)},
		},
	}, nil
}

func (compiler *Compiler) Package(output backend.Output, bundleDir string) error {
	for _, artifact := range output.Artifacts {
		clean := filepath.Clean(artifact.Path)
		if clean == "." || filepath.IsAbs(clean) || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid JS artifact path %q", artifact.Path)
		}
		target := filepath.Join(bundleDir, clean)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(target, artifact.Content, 0644); err != nil {
			return err
		}
	}
	return nil
}

type lowerer struct {
	diagnostics     []backend.Diagnostic
	functions       map[string]bool
	globalFunctions map[string][]string
	structs         map[string]parser.AliasFunctionStatement
	aliases         map[string]string
	file            string
	namespace       string
	returnType      string
}

func lowerProgram(request backend.Request) (ir.Program, []backend.Diagnostic) {
	entry, entryDiagnostics := parser.ResolveEntryPoint(request.Parsed)
	lower := &lowerer{
		functions:       map[string]bool{},
		globalFunctions: map[string][]string{},
		structs:         map[string]parser.AliasFunctionStatement{},
		aliases:         map[string]string{},
	}
	for _, diagnostic := range entryDiagnostics {
		lower.diagnostics = append(lower.diagnostics, backend.Diagnostic{
			File: diagnostic.File, Line: diagnostic.Line, Column: diagnostic.Column,
			EndColumn: diagnostic.Column + 1,
			Rule:      "JS_ENTRY_POINT",
			Message:   diagnostic.Message,
			Hint:      "Define function Main() : Int or mark one () : Int function with #set_entry_point_to_here.",
		})
	}
	for _, source := range request.Parsed.Sources {
		lower.collectSymbols(source.Program.Statements, "", source.ModuleFunctionFilter, false)
	}
	program := ir.Program{Name: request.Program.Name, EntryPoint: entry}
	sourceFiles := map[string]file.SourceFile{}
	for _, source := range request.Program.Files {
		sourceFiles[filepath.Clean(source.Path)] = source
	}
	for _, source := range request.Parsed.Sources {
		loaded := sourceFiles[filepath.Clean(source.Path)]
		program.Sources = append(program.Sources, ir.Source{
			Path: source.Path, MapPath: jsSourceMapPath(request.Program.Root, source.Path),
			Content: strings.Join(loaded.DisplayLines(), "\n"),
		})
	}
	for _, source := range request.Parsed.Sources {
		lower.file = source.Path
		lower.lowerTopLevel(source.Program.Statements, "", source.ModuleFunctionFilter, &program)
	}
	return program, lower.diagnostics
}

func (lower *lowerer) collectSymbols(statements []parser.Statement, namespace string, filter map[string]bool, globalNamespace bool) {
	for _, statement := range statements {
		switch current := statement.(type) {
		case parser.NamespaceStatement:
			lower.collectSymbols(current.Body, namespace+current.Name+".", filter, globalNamespace || current.Global)
		case parser.ScopeStatement:
			lower.collectSymbols(current.Body, namespace, filter, globalNamespace)
		case parser.FunctionStatement:
			name := namespace + current.Name
			if filter != nil && !filter[name] {
				continue
			}
			lower.functions[name] = true
			if globalNamespace {
				lower.globalFunctions[current.Name] = append(lower.globalFunctions[current.Name], name)
			}
		case parser.AliasFunctionStatement:
			lower.structs[namespace+current.Name] = current
		case parser.AliasStatement:
			if !current.KeywordMacro {
				lower.aliases[current.Name] = strings.ReplaceAll(current.Target, "::", ".")
			}
		}
	}
}

func (lower *lowerer) lowerTopLevel(statements []parser.Statement, namespace string, filter map[string]bool, program *ir.Program) {
	previousNamespace := lower.namespace
	lower.namespace = namespace
	defer func() { lower.namespace = previousNamespace }()
	for _, statement := range statements {
		switch current := statement.(type) {
		case parser.ImportStatement, parser.ModuleDirectiveStatement, parser.EntryPointStatement,
			parser.TypeAliasStatement, parser.TraitStatement, parser.ImplStatement, parser.AliasStatement:
			continue
		case parser.NamespaceStatement:
			lower.lowerTopLevel(current.Body, namespace+current.Name+".", filter, program)
		case parser.ScopeStatement:
			lower.lowerTopLevel(current.Body, namespace, filter, program)
		case parser.FunctionStatement:
			name := namespace + current.Name
			if filter != nil && !filter[name] {
				continue
			}
			if function, ok := lower.lowerFunction(current, name, namespace); ok {
				program.Functions = append(program.Functions, function)
			}
		case parser.AliasFunctionStatement:
			name := namespace + current.Name
			structure, methods, ok := lower.lowerStruct(current, name, namespace)
			if ok {
				program.Structs = append(program.Structs, structure)
				program.Functions = append(program.Functions, methods...)
			}
		case parser.ExtensionStatement:
			for _, method := range current.Methods {
				name := current.Target + "." + method.Name
				function, ok := lower.lowerFunction(method, name, namespace)
				function.Params = append([]ir.Binding{{Name: "this", Type: current.Target}}, function.Params...)
				if !ok {
					continue
				}
				program.Extensions = append(program.Extensions, ir.Extension{
					Target: current.Target, Name: method.Name, Function: function.Name,
				})
				program.Functions = append(program.Functions, function)
			}
		case parser.VariableStatement:
			if namespace != "" {
				lower.unsupported(current.Pos, "namespace-scoped global variables")
				continue
			}
			if current.Scope != "global" && current.Scope != "const" && !current.Exported {
				lower.unsupported(current.Pos, "top-level local variables")
				continue
			}
			if statement, ok := lower.lowerVariable(current); ok {
				program.Globals = append(program.Globals, statement)
			}
		default:
			lower.unsupported(statement.Position(), fmt.Sprintf("top-level %T", statement))
		}
	}
}

func (lower *lowerer) lowerFunction(function parser.FunctionStatement, name string, namespace string) (ir.Function, bool) {
	previousNamespace := lower.namespace
	previousReturnType := lower.returnType
	lower.namespace = namespace
	lower.returnType = function.ReturnType
	defer func() {
		lower.namespace = previousNamespace
		lower.returnType = previousReturnType
	}()
	valid := true
	if function.Async || function.Lazy || function.Inner {
		lower.unsupported(function.Pos, "async, lazy, and inner functions")
		valid = false
	}
	if len(function.ReturnValues) != 0 {
		lower.unsupported(function.Pos, "multiple return values")
		valid = false
	}
	if !lower.supportedType(function.ReturnType, true) {
		lower.unsupported(function.Pos, "function return type "+function.ReturnType)
		valid = false
	}
	result := ir.Function{Pos: lower.position(function.Pos), Name: name, ReturnType: function.ReturnType}
	for _, param := range function.Params {
		if param.ByRef || param.Default.Node != nil || !lower.supportedType(param.Type, true) {
			lower.unsupported(function.Pos, fmt.Sprintf("parameter %s with type %s, ref, or default semantics", param.Name, param.Type))
			valid = false
		}
		result.Params = append(result.Params, ir.Binding{Name: param.Name, Type: param.Type, Mutable: param.Mutable})
	}
	body, bodyOK := lower.lowerStatements(function.Body, false)
	result.Body = body
	return result, valid && bodyOK
}

func (lower *lowerer) lowerStruct(alias parser.AliasFunctionStatement, name string, namespace string) (ir.Struct, []ir.Function, bool) {
	result := ir.Struct{Pos: lower.position(alias.Pos), Name: name}
	valid := alias.Struct
	if !alias.Struct {
		lower.unsupported(alias.Pos, "non-struct alias function "+name)
	}
	if len(alias.Hooks) != 0 || len(alias.Body) != 0 {
		lower.unsupported(alias.Pos, "struct alias hooks, nested traits, or implementations")
		valid = false
	}
	tags := map[string]string{}
	for _, tag := range alias.FieldTags {
		if tag.Kind == "json" {
			tags[tag.Field] = tag.Name
		}
	}
	seenDefault := false
	for _, param := range alias.Params {
		if param.ByRef || !lower.supportedType(param.Type, true) {
			lower.unsupported(alias.Pos, fmt.Sprintf("struct field %s with type %s or ref semantics", param.Name, param.Type))
			valid = false
		}
		field := ir.StructField{
			Binding:    ir.Binding{Name: param.Name, Type: param.Type, Mutable: param.Mutable},
			HasDefault: param.Default.Node != nil,
			JSONName:   tags[param.Name],
		}
		if field.HasDefault {
			seenDefault = true
			value, ok := lower.lowerExpression(param.Default.Node, alias.Pos)
			field.Default = value
			valid = valid && ok
		} else if !seenDefault {
			result.Required++
		}
		result.Fields = append(result.Fields, field)
	}
	var methods []ir.Function
	for _, method := range alias.Methods {
		function, ok := lower.lowerFunction(method, name+"."+method.Name, namespace)
		function.Params = append([]ir.Binding{{Name: "this", Type: name}}, function.Params...)
		result.Methods = append(result.Methods, ir.StructMethod{Name: method.Name, Function: function.Name})
		if ok {
			methods = append(methods, function)
		} else {
			valid = false
		}
	}
	return result, methods, valid
}

func (lower *lowerer) lowerStatements(statements []parser.Statement, inLoop bool) ([]ir.Statement, bool) {
	result := make([]ir.Statement, 0, len(statements))
	valid := true
	for _, statement := range statements {
		lowered, ok := lower.lowerStatement(statement, inLoop)
		if !ok {
			valid = false
			continue
		}
		if lowered.Kind != "" {
			result = append(result, lowered)
		}
	}
	return result, valid
}

func (lower *lowerer) lowerStatement(statement parser.Statement, inLoop bool) (ir.Statement, bool) {
	position := lower.position(statement.Position())
	switch current := statement.(type) {
	case parser.VariableStatement:
		return lower.lowerVariable(current)
	case parser.AssignmentStatement:
		switch current.Target.Node.(type) {
		case parser.IdentifierExpression, parser.IndexExpression:
		default:
			lower.unsupported(current.Pos, "selected or dynamic assignment")
			return ir.Statement{}, false
		}
		target, targetOK := lower.lowerExpression(current.Target.Node, current.Pos)
		value, valueOK := lower.lowerExpression(current.Expression.Node, current.Pos)
		return ir.Statement{Pos: position, Kind: ir.StatementAssignment, Operator: current.Operator, Target: target, Value: value}, targetOK && valueOK
	case parser.ExpressionStatement:
		value, ok := lower.lowerExpression(current.Expression.Node, current.Pos)
		return ir.Statement{Pos: position, Kind: ir.StatementExpression, Value: value}, ok
	case parser.ReturnStatement:
		if len(current.Values) != 0 {
			lower.unsupported(current.Pos, "multiple return values")
			return ir.Statement{}, false
		}
		value, ok := lower.lowerExpression(current.Expression.Node, current.Pos)
		return ir.Statement{Pos: position, Kind: ir.StatementReturn, Binding: ir.Binding{Type: lower.returnType}, Value: value}, ok
	case parser.IfStatement:
		condition, conditionOK := lower.lowerExpression(current.Condition.Node, current.Pos)
		if current.Kind == "unless" {
			condition = ir.Expression{Kind: ir.ExpressionUnary, Operator: "not", Right: expressionPointer(condition)}
		}
		body, bodyOK := lower.lowerStatements(current.Consequence, inLoop)
		alternative, alternativeOK := lower.lowerStatements(current.Alternative, inLoop)
		if current.ElseIf != nil {
			elseIf, ok := lower.lowerStatement(*current.ElseIf, inLoop)
			alternative = []ir.Statement{elseIf}
			alternativeOK = alternativeOK && ok
		}
		return ir.Statement{Pos: position, Kind: ir.StatementIf, Condition: condition, Body: body, Else: alternative}, conditionOK && bodyOK && alternativeOK
	case parser.LoopStatement:
		if current.Kind == "for_each" {
			iterator, iterableNode, ok := parseForEachLoopHeader(current.Header)
			if !ok {
				lower.unsupported(current.Pos, "for_each loops without 'name in iterable'")
				return ir.Statement{}, false
			}
			iterable, iterableOK := lower.lowerExpression(iterableNode, current.Pos)
			body, bodyOK := lower.lowerStatements(current.Body, true)
			return ir.Statement{Pos: position, Kind: ir.StatementForEach, Binding: ir.Binding{Name: iterator, Type: "T"}, Value: iterable, Body: body}, iterableOK && bodyOK
		}
		if current.Kind == "for" {
			iterator, countNode, ok := parseRangeLoopHeader(current.Header)
			if !ok {
				lower.unsupported(current.Pos, "for loops other than range(count)")
				return ir.Statement{}, false
			}
			count, countOK := lower.lowerExpression(countNode, current.Pos)
			body, bodyOK := lower.lowerStatements(current.Body, true)
			return ir.Statement{Pos: position, Kind: ir.StatementRange, Binding: ir.Binding{Name: iterator, Type: "Int"}, Value: count, Body: body}, countOK && bodyOK
		}
		if current.Kind != "while" {
			lower.unsupported(current.Pos, current.Kind+" loops")
			return ir.Statement{}, false
		}
		condition, conditionOK := lower.lowerExpression(current.Header.Node, current.Pos)
		body, bodyOK := lower.lowerStatements(current.Body, true)
		return ir.Statement{Pos: position, Kind: ir.StatementWhile, Condition: condition, Body: body}, conditionOK && bodyOK
	case parser.BreakStatement:
		if !inLoop {
			lower.unsupported(current.Pos, "break outside a loop")
			return ir.Statement{}, false
		}
		return ir.Statement{Pos: position, Kind: ir.StatementBreak}, true
	case parser.ContinueStatement:
		if !inLoop {
			lower.unsupported(current.Pos, "continue outside a loop")
			return ir.Statement{}, false
		}
		return ir.Statement{Pos: position, Kind: ir.StatementContinue}, true
	case parser.ThrowStatement:
		value, ok := lower.lowerExpression(current.Expression.Node, current.Pos)
		return ir.Statement{Pos: position, Kind: ir.StatementThrow, Value: value}, ok
	case parser.AssertStatement:
		value, ok := lower.lowerExpression(current.Expression.Node, current.Pos)
		return ir.Statement{Pos: position, Kind: ir.StatementAssert, Value: value}, ok
	case parser.ScopeStatement:
		body, ok := lower.lowerStatements(current.Body, inLoop)
		return ir.Statement{Pos: position, Kind: ir.StatementBlock, Body: body}, ok
	case parser.PrivateBlockStatement:
		body, ok := lower.lowerStatements(current.Body, inLoop)
		return ir.Statement{Pos: position, Kind: ir.StatementBlock, Body: body}, ok
	case parser.TypeAliasStatement:
		return ir.Statement{}, true
	default:
		lower.unsupported(statement.Position(), fmt.Sprintf("statement %T", statement))
		return ir.Statement{}, false
	}
}

func (lower *lowerer) lowerVariable(variable parser.VariableStatement) (ir.Statement, bool) {
	if variable.Lazy || variable.Temporary || !lower.supportedType(variable.Type, true) {
		lower.unsupported(variable.Pos, fmt.Sprintf("variable %s with type %s, lazy, or temporary semantics", variable.Name, variable.Type))
		return ir.Statement{}, false
	}
	value, ok := lower.lowerExpression(variable.Expression.Node, variable.Pos)
	return ir.Statement{Pos: lower.position(variable.Pos), Kind: ir.StatementVariable, Binding: ir.Binding{Name: variable.Name, Type: variable.Type, Mutable: variable.Mutable}, Value: value}, ok
}

func (lower *lowerer) lowerExpression(node parser.ExpressionNode, position parser.Position) (ir.Expression, bool) {
	switch current := node.(type) {
	case nil:
		return ir.Expression{}, true
	case parser.LiteralExpression:
		if !supportedType(current.Kind, false) {
			lower.unsupported(position, "literal type "+current.Kind)
			return ir.Expression{}, false
		}
		return ir.Expression{Kind: ir.ExpressionLiteral, Type: current.Kind, Value: current.Value}, true
	case parser.IdentifierExpression:
		return ir.Expression{Kind: ir.ExpressionIdentifier, Name: current.Name}, true
	case parser.GroupExpression:
		return lower.lowerExpression(current.Inner, position)
	case parser.UnaryExpression:
		if current.Operator != "-" && current.Operator != "not" {
			lower.unsupported(position, "unary operator "+current.Operator)
			return ir.Expression{}, false
		}
		right, ok := lower.lowerExpression(current.Right, position)
		return ir.Expression{Kind: ir.ExpressionUnary, Operator: current.Operator, Right: expressionPointer(right)}, ok
	case parser.BinaryExpression:
		if !supportedBinaryOperator(current.Operator) {
			lower.unsupported(position, "binary operator "+current.Operator)
			return ir.Expression{}, false
		}
		left, leftOK := lower.lowerExpression(current.Left, position)
		right, rightOK := lower.lowerExpression(current.Right, position)
		return ir.Expression{Kind: ir.ExpressionBinary, Operator: current.Operator, Left: expressionPointer(left), Right: expressionPointer(right)}, leftOK && rightOK
	case parser.IndexExpression:
		target, targetOK := lower.lowerExpression(current.Target, position)
		index, indexOK := lower.lowerExpression(current.Index, position)
		return ir.Expression{Kind: ir.ExpressionIndex, Left: expressionPointer(target), Right: expressionPointer(index)}, targetOK && indexOK
	case parser.ListExpression:
		items := make([]ir.Expression, 0, len(current.Items))
		valid := true
		for _, item := range current.Items {
			value, ok := lower.lowerExpression(item, position)
			items = append(items, value)
			valid = valid && ok
		}
		return ir.Expression{Kind: ir.ExpressionList, Arguments: items}, valid
	case parser.MapExpression:
		entries := make([]ir.MapEntry, 0, len(current.Entries))
		valid := true
		for _, entry := range current.Entries {
			key, keyOK := lower.lowerExpression(entry.Key, position)
			value, valueOK := lower.lowerExpression(entry.Value, position)
			entries = append(entries, ir.MapEntry{Key: key, Value: value})
			valid = valid && keyOK && valueOK
		}
		return ir.Expression{Kind: ir.ExpressionMap, Entries: entries}, valid
	case parser.ListComprehensionExpression:
		iterable, iterableOK := lower.lowerExpression(current.Iterable, position)
		value, valueOK := lower.lowerExpression(current.Value, position)
		result := ir.Expression{Kind: ir.ExpressionComprehension, Name: current.Iterator, Left: expressionPointer(iterable), Right: expressionPointer(value)}
		conditionOK := true
		if current.Condition != nil {
			condition, ok := lower.lowerExpression(current.Condition, position)
			result.Condition = expressionPointer(condition)
			conditionOK = ok
		}
		return result, iterableOK && valueOK && conditionOK
	case parser.SelectorExpression:
		target, ok := lower.lowerExpression(current.Target, position)
		return ir.Expression{Kind: ir.ExpressionSelector, Name: current.Field, Left: expressionPointer(target)}, ok
	case parser.CallExpression:
		if selector, ok := current.Callee.(parser.SelectorExpression); ok && jsPipelineMethod(selector.Field) {
			receiver, receiverOK := lower.lowerExpression(selector.Target, position)
			arguments := []ir.Expression{receiver}
			valid := receiverOK
			for _, argument := range current.Arguments {
				value, argumentOK := lower.lowerExpression(argument, position)
				arguments = append(arguments, value)
				valid = valid && argumentOK
			}
			return ir.Expression{Kind: ir.ExpressionCall, Name: "__pipeline:" + selector.Field, Arguments: arguments}, valid
		}
		if selector, ok := current.Callee.(parser.SelectorExpression); ok && selector.Field == "cast_as" {
			if len(current.Arguments) != 1 {
				lower.unsupported(position, "cast_as calls without exactly one target type")
				return ir.Expression{}, false
			}
			target, ok := current.Arguments[0].(parser.IdentifierExpression)
			if !ok {
				lower.unsupported(position, "non-identifier cast_as target")
				return ir.Expression{}, false
			}
			targetType := lower.resolveAliasPath(target.Name)
			if targetType != "Table" && targetType != "JSON" && targetType != "String" && !lower.structs[targetType].Struct {
				lower.unsupported(position, "cast_as target "+targetType)
				return ir.Expression{}, false
			}
			receiver, receiverOK := lower.lowerExpression(selector.Target, position)
			return ir.Expression{Kind: ir.ExpressionCall, Name: "__struct_cast:" + targetType, Arguments: []ir.Expression{receiver}}, receiverOK
		}
		if selector, ok := current.Callee.(parser.SelectorExpression); ok && (selector.Field == "uppercase" || selector.Field == "lowercase") {
			if len(current.Arguments) != 0 {
				lower.unsupported(position, "arguments to String."+selector.Field)
				return ir.Expression{}, false
			}
			receiver, receiverOK := lower.lowerExpression(selector.Target, position)
			return ir.Expression{Kind: ir.ExpressionCall, Name: "__string_" + selector.Field, Arguments: []ir.Expression{receiver}}, receiverOK
		}
		calleePath, ok := expressionPath(current.Callee)
		calleeName, resolved := lower.resolveFunction(calleePath)
		if calleePath == "len" {
			calleeName, resolved = "__len", len(current.Arguments) == 1
		} else if calleePath == "JSON" {
			calleeName, resolved = "__json", len(current.Arguments) == 1
		} else if calleePath == "json_stringify" {
			calleeName, resolved = "__json_stringify", len(current.Arguments) == 1
		} else if calleePath == "json_encode" {
			calleeName, resolved = "__json_encode", len(current.Arguments) == 1
		} else if calleePath == "json_decode" {
			calleeName, resolved = "__json_decode", len(current.Arguments) == 1
		} else if builtin, exists := jsCollectionBuiltin(calleePath); exists {
			calleeName, resolved = builtin, true
		}
		if !resolved {
			if selector, selectorOK := current.Callee.(parser.SelectorExpression); selectorOK {
				receiver, receiverOK := lower.lowerExpression(selector.Target, position)
				arguments := []ir.Expression{receiver}
				valid := receiverOK
				for _, argument := range current.Arguments {
					value, argOK := lower.lowerExpression(argument, position)
					arguments = append(arguments, value)
					valid = valid && argOK
				}
				return ir.Expression{Kind: ir.ExpressionCall, Name: "__method:" + selector.Field, Arguments: arguments}, valid
			}
		}
		if !ok || !resolved {
			lower.unsupported(position, "builtin, method, or dynamic call")
			return ir.Expression{}, false
		}
		arguments := make([]ir.Expression, 0, len(current.Arguments))
		valid := true
		for _, argument := range current.Arguments {
			value, argOK := lower.lowerExpression(argument, position)
			arguments = append(arguments, value)
			valid = valid && argOK
		}
		return ir.Expression{Kind: ir.ExpressionCall, Name: calleeName, Arguments: arguments}, valid
	case parser.ConditionalExpression:
		condition, conditionOK := lower.lowerExpression(current.Condition, position)
		consequence, consequenceOK := lower.lowerExpression(current.Consequence, position)
		alternative, alternativeOK := lower.lowerExpression(current.Alternative, position)
		return ir.Expression{Kind: ir.ExpressionConditional, Condition: expressionPointer(condition), Consequence: expressionPointer(consequence), Alternative: expressionPointer(alternative)}, conditionOK && consequenceOK && alternativeOK
	case parser.CastExpression:
		if !lower.supportedType(current.Type, true) {
			lower.unsupported(position, "cast target "+current.Type)
			return ir.Expression{}, false
		}
		value, ok := lower.lowerExpression(current.Value, position)
		return ir.Expression{Kind: ir.ExpressionCast, Type: current.Type, Left: expressionPointer(value)}, ok
	default:
		lower.unsupported(position, fmt.Sprintf("expression %T", node))
		return ir.Expression{}, false
	}
}

func (lower *lowerer) resolveFunction(name string) (string, bool) {
	name = lower.resolveAliasPath(name)
	if name == "print" {
		return name, true
	}
	if lower.structs[name].Struct {
		return name, true
	}
	if lower.namespace != "" && lower.structs[lower.namespace+name].Struct {
		return lower.namespace + name, true
	}
	if lower.functions[name] {
		return name, true
	}
	if lower.namespace != "" && lower.functions[lower.namespace+name] {
		return lower.namespace + name, true
	}
	if matches := lower.globalFunctions[name]; len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func (lower *lowerer) resolveAliasPath(name string) string {
	for iteration := 0; iteration <= len(lower.aliases); iteration++ {
		best := ""
		for alias := range lower.aliases {
			if name == alias || strings.HasPrefix(name, alias+".") {
				if len(alias) > len(best) {
					best = alias
				}
			}
		}
		if best == "" {
			return name
		}
		next := lower.aliases[best] + strings.TrimPrefix(name, best)
		if next == name {
			return name
		}
		name = next
	}
	return name
}

func expressionPath(expression parser.ExpressionNode) (string, bool) {
	switch current := expression.(type) {
	case parser.IdentifierExpression:
		return current.Name, true
	case parser.SelectorExpression:
		target, ok := expressionPath(current.Target)
		if !ok {
			return "", false
		}
		return target + "." + current.Field, true
	default:
		return "", false
	}
}

func jsPipelineMethod(name string) bool {
	switch name {
	case "iter", "filter", "map", "skip", "limit", "collect", "sort", "fold", "first", "any", "all", "for_each":
		return true
	default:
		return false
	}
}

func (lower *lowerer) unsupported(position parser.Position, feature string) {
	lower.diagnostics = append(lower.diagnostics, backend.Diagnostic{
		File: lower.file, Line: position.Line, Column: position.Column, EndColumn: position.Column + 1,
		Rule:    "js-backend/unsupported-feature",
		Message: "JS backend does not yet support " + feature,
		Hint:    "Use the typed JS core subset or package with Standalone/WASM runtime mode for this feature.",
	})
}

func jsSourceMapPath(root string, source string) string {
	relative := source
	if root != "" {
		if candidate, err := filepath.Rel(root, source); err == nil && candidate != ".." && !strings.HasPrefix(candidate, ".."+string(filepath.Separator)) {
			relative = candidate
		}
	}
	if filepath.IsAbs(relative) {
		relative = filepath.Base(relative)
	}
	return filepath.ToSlash(filepath.Join("src", relative))
}

func (lower *lowerer) position(position parser.Position) ir.Position {
	return ir.Position{File: lower.file, Line: position.Line, Column: position.Column}
}

func supportedType(typeName string, allowAny bool) bool {
	typeName = strings.TrimSpace(typeName)
	if strings.HasPrefix(typeName, "List[") && strings.HasSuffix(typeName, "]") {
		return supportedType(typeName[len("List["):len(typeName)-1], allowAny)
	}
	if keyType, valueType, ok := jsMapTypes(typeName); ok {
		return supportedType(keyType, allowAny) && supportedType(valueType, allowAny)
	}
	if okType, errType, ok := jsResultTypes(typeName); ok {
		return supportedType(okType, allowAny) && supportedType(errType, allowAny)
	}
	if strings.HasPrefix(typeName, "Iterator[") && strings.HasSuffix(typeName, "]") {
		return supportedType(typeName[len("Iterator["):len(typeName)-1], allowAny)
	}
	if strings.HasPrefix(typeName, "Option[") && strings.HasSuffix(typeName, "]") {
		return supportedType(typeName[len("Option["):len(typeName)-1], allowAny)
	}
	switch typeName {
	case "Int", "UInt", "Float", "Bool", "String", "Char", "JSON", "Table":
		return true
	case "T":
		return allowAny
	default:
		return false
	}
}

func jsResultTypes(typeName string) (string, string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "Result[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	inner := typeName[len("Result[") : len(typeName)-1]
	depth := 0
	for index, current := range inner {
		switch current {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				left := strings.TrimSpace(inner[:index])
				right := strings.TrimSpace(inner[index+1:])
				return left, right, left != "" && right != ""
			}
		}
	}
	return "", "", false
}

func jsMapTypes(typeName string) (string, string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "Map[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	inner := typeName[len("Map[") : len(typeName)-1]
	depth := 0
	for index, current := range inner {
		switch current {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				left := strings.TrimSpace(inner[:index])
				right := strings.TrimSpace(inner[index+1:])
				return left, right, left != "" && right != ""
			}
		}
	}
	return "", "", false
}

func jsCollectionBuiltin(name string) (string, bool) {
	switch name {
	case "Table":
		return "__table", true
	case "table_has", "has_key":
		return "__table_has", true
	case "table_delete":
		return "__table_delete", true
	case "table_keys":
		return "__table_keys", true
	case "table_values":
		return "__table_values", true
	case "table_entries":
		return "__table_entries", true
	case "table_sequence_count":
		return "__table_sequence_count", true
	case "table_set_fallback":
		return "__table_set_fallback", true
	default:
		return "", false
	}
}

func (lower *lowerer) supportedType(typeName string, allowAny bool) bool {
	if supportedType(typeName, allowAny) {
		return true
	}
	base := strings.TrimSpace(typeName)
	if index := strings.IndexByte(base, '['); index >= 0 {
		base = base[:index]
	}
	base = lower.resolveAliasPath(base)
	if lower.structs[base].Struct {
		return true
	}
	return lower.namespace != "" && lower.structs[lower.namespace+base].Struct
}

func parseRangeLoopHeader(expression parser.Expression) (string, parser.ExpressionNode, bool) {
	tokens := expression.Tokens
	if len(tokens) < 5 || tokens[0].Type != lexer.TokenIdentifier || tokens[1].Type != lexer.TokenEvaluationAssign || tokens[2].Literal != "range" {
		return "", nil, false
	}
	call, ok := parser.ParseExpressionTokens(tokens[2:]).(parser.CallExpression)
	if !ok || len(call.Arguments) != 1 {
		return "", nil, false
	}
	return tokens[0].Literal, call.Arguments[0], true
}

func parseForEachLoopHeader(expression parser.Expression) (string, parser.ExpressionNode, bool) {
	tokens := expression.Tokens
	if len(tokens) < 3 || tokens[0].Type != lexer.TokenIdentifier || tokens[1].Type != lexer.TokenIn {
		return "", nil, false
	}
	return tokens[0].Literal, parser.ParseExpressionTokens(tokens[2:]), true
}

func supportedBinaryOperator(operator string) bool {
	switch operator {
	case "+", "-", "*", "/", "//", "%", "**", "==", "!=", ">", "<", ">=", "<=", "and", "or":
		return true
	default:
		return false
	}
}

func expressionPointer(expression ir.Expression) *ir.Expression {
	return &expression
}

func emitProgram(program ir.Program) (string, []byte) {
	var output strings.Builder
	sourceMap := newSourceMapBuilder(program.Sources)
	output.WriteString("\"use strict\";\n\n")
	output.WriteString("const __klang_struct_tags = {\n")
	for _, structure := range program.Structs {
		fmt.Fprintf(&output, "  %s: {", strconv.Quote(structure.Name))
		wroteTag := false
		for _, field := range structure.Fields {
			if field.JSONName == "" {
				continue
			}
			if wroteTag {
				output.WriteString(", ")
			}
			fmt.Fprintf(&output, "%s: %s", strconv.Quote(field.Binding.Name), strconv.Quote(field.JSONName))
			wroteTag = true
		}
		output.WriteString("},\n")
	}
	output.WriteString("};\n")
	output.WriteString("const __klang_struct_definitions = {\n")
	for _, structure := range program.Structs {
		fmt.Fprintf(&output, "  %s: { constructor: %s, fields: [", strconv.Quote(structure.Name), jsIdentifier(structure.Name))
		for index, field := range structure.Fields {
			if index != 0 {
				output.WriteString(", ")
			}
			fmt.Fprintf(&output, "{ name: %s, required: %t }", strconv.Quote(field.Binding.Name), !field.HasDefault)
		}
		output.WriteString("] },\n")
	}
	output.WriteString("};\n")
	output.WriteString("const __klang_struct_methods = {\n")
	methodsByType := map[string][]ir.StructMethod{}
	for _, structure := range program.Structs {
		methodsByType[structure.Name] = append(methodsByType[structure.Name], structure.Methods...)
	}
	for _, extension := range program.Extensions {
		methodsByType[extension.Target] = append(methodsByType[extension.Target], ir.StructMethod{Name: extension.Name, Function: extension.Function})
	}
	methodTypes := make([]string, 0, len(methodsByType))
	for typeName := range methodsByType {
		methodTypes = append(methodTypes, typeName)
	}
	sort.Strings(methodTypes)
	for _, typeName := range methodTypes {
		fmt.Fprintf(&output, "  %s: {", strconv.Quote(typeName))
		for index, method := range methodsByType[typeName] {
			if index != 0 {
				output.WriteString(", ")
			}
			fmt.Fprintf(&output, "%s: %s", strconv.Quote(method.Name), jsIdentifier(method.Function))
		}
		output.WriteString("},\n")
	}
	output.WriteString("};\n")
	output.WriteString("const __klang_is_struct = (value) => value !== null && typeof value === \"object\" && typeof value.__klang_struct === \"string\";\n")
	output.WriteString("const __klang_result = (ok, value) => ({ __klang_result: true, ok, value });\n")
	output.WriteString("const __klang_is_result = (value) => value !== null && typeof value === \"object\" && value.__klang_result === true;\n")
	output.WriteString("const __klang_copy = (value) => { if (Array.isArray(value)) return value.map(__klang_copy); if (value?.__klang_pipeline === true) return __klang_pipeline_clone(value); if (value?.__klang_option === true) return __klang_option(value.some, __klang_copy(value.value)); if (__klang_is_result(value)) return __klang_result(value.ok, __klang_copy(value.value)); if (__klang_is_collection(value)) return __klang_collection_copy(value); if (__klang_is_char(value)) return value; if (!__klang_is_struct(value)) return value; const copied = { __klang_struct: value.__klang_struct }; for (const field of Object.keys(value)) if (!field.startsWith(\"__\")) copied[field] = __klang_copy(value[field]); return copied; };\n")
	output.WriteString("const __klang_format = (value) => Array.isArray(value) ? `[${value.map(__klang_format).join(\", \")}]` : value?.__klang_option === true ? (value.some ? `Some(${__klang_format(value.value)})` : \"None\") : value?.__klang_pipeline === true ? `Iterator(${value.index}/${value.items.length})` : __klang_is_result(value) ? `${value.ok ? \"Ok\" : \"Err\"}(${__klang_format(value.value)})` : __klang_is_collection(value) ? __klang_collection_format(value) : __klang_is_char(value) ? value.__klang_char : value === null ? \"Null\" : typeof value === \"boolean\" ? (value ? \"True\" : \"False\") : String(value);\n")
	output.WriteString("const __klang_print = (...values) => console.log(values.map(__klang_format).join(\" \"));\n\n")
	output.WriteString("const __klang_add_frame = (thrown, frame) => { const error = thrown instanceof Error ? thrown : new Error(__klang_format(thrown)); if (!Object.prototype.hasOwnProperty.call(error, \"klangFrames\")) Object.defineProperty(error, \"klangFrames\", { value: [], enumerable: false }); error.klangFrames.push(frame); return error; };\n")
	output.WriteString("const __klang_render_error = (thrown) => { const error = thrown instanceof Error ? thrown : new Error(__klang_format(thrown)); const lines = [\"\", \"-- JS RUNTIME ERROR --------------------------------------------------------\", \"\", `${error.name}: ${error.message}`]; for (const frame of error.klangFrames || []) { lines.push(\"\", `at ${frame.function} (${frame.file}:${frame.line}:${frame.column})`); if (frame.source) { lines.push(`${frame.line} | ${frame.source}`); lines.push(`  | ${\" \".repeat(Math.max(0, frame.column - 1))}^`); } } return lines.join(\"\\n\"); };\n\n")
	output.WriteString("const __klang_add = (left, right) => typeof left === \"string\" || typeof right === \"string\" ? __klang_format(left) + __klang_format(right) : left + right;\n")
	output.WriteString(collectionRuntime)
	output.WriteString("const __klang_len = (value) => { if (value?.__klang_pipeline === true) { const pipeline = __klang_pipeline_clone(value); let count = 0; for (;;) { const next = __klang_pipeline_next(pipeline); if (!next.some) return count; count++; } } if (typeof value === \"string\") return Array.from(value).length; if (Array.isArray(value)) return value.length; if (__klang_is_collection(value)) return value.entries.size; throw new TypeError(\"len expects String, List, Map, Table, or Iterator in the JS backend\"); };\n")
	output.WriteString("const __klang_index = (value, index) => { if (__klang_is_collection(value)) return __klang_collection_get(value, index); if (!Number.isInteger(index)) throw new TypeError(\"index must be an Int\"); const items = typeof value === \"string\" ? Array.from(value) : value; const kind = typeof value === \"string\" ? \"string\" : \"list\"; if (!Array.isArray(items)) throw new TypeError(\"indexing expects String, List, Map, or Table\"); if (index < 0 || index >= items.length) throw new RangeError(`${kind} index ${index} is out of bounds`); return typeof value === \"string\" ? __klang_char(items[index]) : __klang_copy(items[index]); };\n")
	output.WriteString("const __klang_list_assign = (list, index, operator, value) => { if (!Array.isArray(list)) throw new TypeError(\"indexed mutation expects List\"); if (!Number.isInteger(index)) throw new TypeError(\"list index must be an Int\"); if (index < 0) throw new RangeError(`list index ${index} is out of bounds`); if (operator !== \"=\" && index >= list.length) throw new RangeError(`compound assignment requires existing list index ${index}`); while (list.length <= index) list.push(null); const right = __klang_copy(value); if (operator === \"=\") list[index] = right; else if (operator === \"+=\") list[index] = __klang_add(list[index], right); else if (operator === \"-=\") list[index] -= right; else if (operator === \"*=\") list[index] *= right; else if (operator === \"/=\") list[index] /= right; else throw new TypeError(`unsupported assignment operator ${operator}`); };\n")
	output.WriteString("const __klang_assign_index = (target, index, operator, value) => { if (__klang_is_collection(target)) return __klang_collection_put(target, index, value, operator); return __klang_list_assign(target, index, operator, value); };\n")
	output.WriteString("const __klang_iter = (value) => { if (value?.__klang_pipeline === true) { const result = []; for (;;) { const next = __klang_pipeline_next(value); if (!next.some) return result; result.push(next.value); } } if (Array.isArray(value)) return value.map(__klang_copy); if (typeof value === \"string\") return Array.from(value).map(__klang_char); if (typeof value === \"number\") { if (value < 0) throw new TypeError(\"for_each count cannot be negative\"); return Array.from({ length: value }, (_, index) => index); } if (__klang_is_collection(value)) return value.order.map((token) => { const entry = value.entries.get(token); return __klang_table_from_pairs([[\"key\", entry.key], [\"value\", entry.value]]); }); throw new TypeError(\"for_each expects List, String, Map, Table, Iterator, or Int\"); };\n")
	output.WriteString("const __klang_option = (some, value = null) => ({ __klang_option: true, some, value });\n")
	output.WriteString("const __klang_pipeline = (value) => value?.__klang_pipeline === true ? __klang_pipeline_clone(value) : ({ __klang_pipeline: true, items: __klang_iter(value), index: 0, stages: [], exhausted: false });\n")
	output.WriteString("const __klang_pipeline_clone = (value) => ({ __klang_pipeline: true, items: value.items, index: value.index, stages: value.stages.map((stage) => ({ ...stage })), exhausted: value.exhausted });\n")
	output.WriteString("const __klang_pipeline_add = (value, kind, argument) => { const pipeline = __klang_pipeline(value); if (kind === \"limit\") kind = \"take\"; pipeline.stages.push(kind === \"filter\" || kind === \"map\" ? { kind, callback: argument } : { kind, count: argument, seen: 0 }); return pipeline; };\n")
	output.WriteString("const __klang_pipeline_next = (pipeline) => { if (pipeline.exhausted) return __klang_option(false); if (pipeline.stages.some((stage) => stage.kind === \"take\" && stage.seen >= stage.count)) { pipeline.exhausted = true; return __klang_option(false); } source: while (pipeline.index < pipeline.items.length) { let current = __klang_copy(pipeline.items[pipeline.index++]); for (const stage of pipeline.stages) { if (stage.kind === \"filter\") { if (!stage.callback(__klang_copy(current))) continue source; } else if (stage.kind === \"map\") current = stage.callback(__klang_copy(current)); else if (stage.kind === \"skip\") { if (stage.seen < stage.count) { stage.seen++; continue source; } } else if (stage.kind === \"take\") { if (stage.seen >= stage.count) { pipeline.exhausted = true; return __klang_option(false); } stage.seen++; } } return __klang_option(true, __klang_copy(current)); } pipeline.exhausted = true; return __klang_option(false); };\n")
	output.WriteString("const __klang_pipeline_method = (value, name, args) => { if (name === \"iter\") return __klang_pipeline(value); if (name === \"filter\" || name === \"map\") return __klang_pipeline_add(value, name, args[0]); if (name === \"skip\" || name === \"limit\") { if (!Number.isInteger(args[0]) || args[0] < 0) throw new TypeError(`${name} expects a non-negative Int`); return __klang_pipeline_add(value, name, args[0]); } const pipeline = __klang_pipeline(value); if (name === \"collect\" || name === \"sort\") { const result = []; for (;;) { const next = __klang_pipeline_next(pipeline); if (!next.some) break; result.push(next.value); } if (name === \"sort\") result.sort((left, right) => left < right ? -1 : left > right ? 1 : 0); return result; } if (name === \"first\") return __klang_pipeline_next(pipeline); if (name === \"fold\") { let result = __klang_copy(args[0]); for (;;) { const next = __klang_pipeline_next(pipeline); if (!next.some) return result; result = args[1](__klang_copy(result), __klang_copy(next.value)); } } if (name === \"any\" || name === \"all\") { for (;;) { const next = __klang_pipeline_next(pipeline); if (!next.some) return name === \"all\"; const matched = !!args[0](__klang_copy(next.value)); if (name === \"any\" && matched) return true; if (name === \"all\" && !matched) return false; } } if (name === \"for_each\") { for (;;) { const next = __klang_pipeline_next(pipeline); if (!next.some) return null; args[0](__klang_copy(next.value)); } } throw new TypeError(`unknown pipeline method ${name}`); };\n")
	output.WriteString("const __klang_list_iter = (value) => { if (!Array.isArray(value)) throw new TypeError(\"list comprehension expects List\"); return value.map(__klang_copy); };\n")
	output.WriteString("const __klang_range_count = (value) => { if (!Number.isInteger(value)) throw new TypeError(\"range expects an Int count\"); if (value < 0) throw new RangeError(\"range count cannot be negative\"); return value; };\n")
	output.WriteString("const __klang_string_uppercase = (value) => __klang_is_char(value) ? __klang_char(value.__klang_char.toUpperCase()) : value.toUpperCase();\n")
	output.WriteString("const __klang_string_lowercase = (value) => __klang_is_char(value) ? __klang_char(value.__klang_char.toLowerCase()) : value.toLowerCase();\n\n")
	output.WriteString("const __klang_select = (value, field) => { if ((value?.__klang_option === true || __klang_is_result(value)) && (field === \"some\" || field === \"ok\" || field === \"value\")) return __klang_copy(value[field]); if (__klang_is_struct(value)) { if (!Object.prototype.hasOwnProperty.call(value, field)) throw new TypeError(`unknown field ${value.__klang_struct}.${field}`); return __klang_copy(value[field]); } if (field === \"count\") return __klang_len(value); if (__klang_is_collection(value) && value.__klang_collection === \"Table\") return __klang_collection_get(value, field); throw new TypeError(`selector .${field} is not supported for this value`); };\n")
	output.WriteString("const __klang_runtime_type = (value) => value?.__klang_pipeline === true ? \"Iterator[T]\" : value?.__klang_option === true ? \"Option[T]\" : __klang_is_struct(value) ? value.__klang_struct : __klang_is_char(value) ? \"Char\" : Array.isArray(value) ? \"List[T]\" : __klang_is_collection(value) ? value.__klang_collection + (value.__klang_collection === \"Table\" ? \"\" : \"[T,T]\") : typeof value === \"string\" ? \"String\" : typeof value === \"boolean\" ? \"Bool\" : typeof value === \"number\" ? (Number.isInteger(value) ? \"Int\" : \"Float\") : \"T\";\n")
	output.WriteString("const __klang_call_method = (value, name, args) => { const type = __klang_runtime_type(value); const method = __klang_struct_methods[type]?.[name]; if (typeof method !== \"function\") throw new TypeError(`unknown method ${type}.${name}`); return method(__klang_copy(value), ...args.map(__klang_copy)); };\n")
	output.WriteString("const __klang_operator_methods = { \"+\": \"__operator_add\", \"-\": \"__operator_subtract\", \"*\": \"__operator_multiply\", \"/\": \"__operator_divide\", \"//\": \"__operator_floor_divide\", \"%\": \"__operator_modulo\", \"**\": \"__operator_power\", \"==\": \"__operator_equal\", \"!=\": \"__operator_not_equal\", \">\": \"__operator_greater\", \">=\": \"__operator_greater_equal\", \"<\": \"__operator_less\", \"<=\": \"__operator_less_equal\" };\n")
	output.WriteString("const __klang_binary = (left, operator, right) => { const type = __klang_runtime_type(left); const methodName = __klang_operator_methods[operator]; const method = methodName === undefined ? undefined : __klang_struct_methods[type]?.[methodName]; if (typeof method === \"function\") return method(__klang_copy(left), __klang_copy(right)); if (operator === \"+\") return __klang_add(left, right); if (operator === \"-\") return left - right; if (operator === \"*\") return left * right; if (operator === \"/\") return left / right; if (operator === \"//\") return Math.floor(left / right); if (operator === \"%\") return left % right; if (operator === \"**\") return left ** right; if (operator === \"==\") return __klang_equal(left, right); if (operator === \"!=\") return !__klang_equal(left, right); if (operator === \">\") return left > right; if (operator === \">=\") return left >= right; if (operator === \"<\") return left < right; if (operator === \"<=\") return left <= right; throw new TypeError(`unsupported binary operator ${operator}`); };\n")
	output.WriteString("const __klang_to_json = (value) => { if (__klang_is_char(value)) return value.__klang_char; if (value === null || typeof value === \"string\" || typeof value === \"number\" || typeof value === \"boolean\") return value; if (Array.isArray(value)) return value.map(__klang_to_json); if (__klang_is_collection(value)) return __klang_collection_json(value); if (typeof value === \"object\") { const tags = __klang_is_struct(value) ? (__klang_struct_tags[value.__klang_struct] || {}) : {}; const entries = Object.keys(value).filter((field) => !field.startsWith(\"__\")).map((field) => [tags[field] || field, __klang_to_json(value[field])]).sort((left, right) => left[0].localeCompare(right[0])); return Object.fromEntries(entries); } throw new TypeError(\"cannot serialize value as JSON\"); };\n")
	output.WriteString("const __klang_json = (value) => typeof value === \"string\" ? JSON.parse(value) : __klang_to_json(value);\n")
	output.WriteString("const __klang_json_stringify = (value) => JSON.stringify(__klang_to_json(value));\n\n")
	output.WriteString("const __klang_struct_cast = (value, target) => { if (!__klang_is_struct(value)) throw new TypeError(`cast_as requires a struct alias receiver, got ${__klang_runtime_type(value)}`); if (target === value.__klang_struct) return __klang_copy(value); if (target === \"Table\") return __klang_table_from_pairs(Object.keys(value).filter((field) => !field.startsWith(\"__\")).map((field) => [field, __klang_copy(value[field])])); if (target === \"JSON\") return __klang_to_json(value); if (target === \"String\") return __klang_json_stringify(value); const definition = __klang_struct_definitions[target]; if (definition === undefined) throw new TypeError(`unknown cast_as target ${target}`); const args = definition.fields.map((field) => { if (Object.prototype.hasOwnProperty.call(value, field.name)) return __klang_copy(value[field.name]); if (field.required) throw new TypeError(`cannot cast ${value.__klang_struct} to ${target}: required field ${field.name} is missing`); return undefined; }); return definition.constructor(...args); };\n\n")
	output.WriteString("const __klang_json_to_native = (value) => { if (value === null || typeof value === \"string\" || typeof value === \"number\" || typeof value === \"boolean\") return value; if (Array.isArray(value)) return value.map(__klang_json_to_native); const pairs = Object.keys(value).sort().map((key) => [key, __klang_json_to_native(value[key])]); return __klang_table_from_pairs(pairs); };\n")
	output.WriteString("const __klang_json_encode = (value) => { try { return __klang_result(true, __klang_json_stringify(value)); } catch (error) { return __klang_result(false, error.message); } };\n")
	output.WriteString("const __klang_json_decode = (source) => { try { return __klang_result(true, __klang_json_to_native(JSON.parse(source))); } catch (error) { return __klang_result(false, error.message); } };\n\n")
	for _, structure := range program.Structs {
		emitStruct(&output, structure, sourceMap)
	}
	for _, global := range program.Globals {
		emitStatement(&output, global, 0, sourceMap)
	}
	if len(program.Globals) != 0 {
		output.WriteByte('\n')
	}
	for _, function := range program.Functions {
		sourceMap.mark(&output, function.Pos)
		output.WriteString("function ")
		output.WriteString(jsIdentifier(function.Name))
		output.WriteByte('(')
		for index, param := range function.Params {
			if index != 0 {
				output.WriteString(", ")
			}
			output.WriteString(jsIdentifier(param.Name))
		}
		output.WriteString(") {\n")
		output.WriteString("    try {\n")
		for _, statement := range function.Body {
			emitStatement(&output, statement, 2, sourceMap)
		}
		output.WriteString("    } catch (__klang_error) {\n")
		output.WriteString("        throw __klang_add_frame(__klang_error, ")
		emitRuntimeFrame(&output, function.Pos, function.Name, sourceMap)
		output.WriteString(");\n")
		output.WriteString("    }\n")
		output.WriteString("}\n\n")
	}
	output.WriteString("const __klang_exports = {\n")
	for _, function := range program.Functions {
		fmt.Fprintf(&output, "  %s: %s,\n", strconv.Quote(function.Name), jsIdentifier(function.Name))
	}
	output.WriteString("};\n")
	output.WriteString("if (typeof globalThis !== \"undefined\") globalThis.KlangProgram = __klang_exports;\n")
	if hasIRFunction(program.Functions, program.EntryPoint) {
		output.WriteString("if (typeof require !== \"undefined\" && typeof module !== \"undefined\" && require.main === module) {\n")
		output.WriteString("    try {\n")
		fmt.Fprintf(&output, "        %s();\n", jsIdentifier(program.EntryPoint))
		output.WriteString("    } catch (__klang_error) {\n")
		output.WriteString("        console.error(__klang_render_error(__klang_error));\n")
		output.WriteString("        if (typeof process !== \"undefined\") process.exitCode = 1;\n")
		output.WriteString("    }\n")
		output.WriteString("}\n")
	}
	output.WriteString("//# sourceMappingURL=program.js.map\n")
	return output.String(), sourceMap.encode()
}

func emitStruct(output *strings.Builder, structure ir.Struct, sourceMap *sourceMapBuilder) {
	sourceMap.mark(output, structure.Pos)
	output.WriteString("function ")
	output.WriteString(jsIdentifier(structure.Name))
	output.WriteByte('(')
	for index, field := range structure.Fields {
		if index != 0 {
			output.WriteString(", ")
		}
		output.WriteString(jsIdentifier(field.Binding.Name))
	}
	output.WriteString(") {\n")
	output.WriteString("    try {\n")
	fmt.Fprintf(output, "        if (arguments.length < %d || arguments.length > %d) throw new TypeError(%s);\n",
		structure.Required, len(structure.Fields),
		strconv.Quote(fmt.Sprintf("struct alias %s expects %d to %d argument(s)", structure.Name, structure.Required, len(structure.Fields))))
	for _, field := range structure.Fields {
		if !field.HasDefault {
			continue
		}
		output.WriteString("        if (")
		output.WriteString(jsIdentifier(field.Binding.Name))
		output.WriteString(" === undefined) ")
		output.WriteString(jsIdentifier(field.Binding.Name))
		output.WriteString(" = ")
		emitExpression(output, field.Default)
		output.WriteString(";\n")
	}
	fmt.Fprintf(output, "        return { __klang_struct: %s", strconv.Quote(structure.Name))
	for _, field := range structure.Fields {
		fmt.Fprintf(output, ", %s: __klang_copy(%s)", strconv.Quote(field.Binding.Name), jsIdentifier(field.Binding.Name))
	}
	output.WriteString(" };\n")
	output.WriteString("    } catch (__klang_error) {\n")
	output.WriteString("        throw __klang_add_frame(__klang_error, ")
	emitRuntimeFrame(output, structure.Pos, structure.Name, sourceMap)
	output.WriteString(");\n")
	output.WriteString("    }\n")
	output.WriteString("}\n\n")
}

func emitRuntimeFrame(output *strings.Builder, position ir.Position, function string, sourceMap *sourceMapBuilder) {
	fmt.Fprintf(output, "{ file: %s, line: %d, column: %d, function: %s, source: %s }",
		strconv.Quote(sourceMap.sourcePath(position.File)), position.Line, max(position.Column, 1),
		strconv.Quote(function), strconv.Quote(sourceMap.sourceLine(position)))
}

func emitStatement(output *strings.Builder, statement ir.Statement, indent int, sourceMap *sourceMapBuilder) {
	writeIndent(output, indent)
	sourceMap.mark(output, statement.Pos)
	switch statement.Kind {
	case ir.StatementVariable:
		if statement.Binding.Mutable {
			output.WriteString("let ")
		} else {
			output.WriteString("const ")
		}
		output.WriteString(jsIdentifier(statement.Binding.Name))
		output.WriteString(" = ")
		if statement.Value.Kind == "" {
			output.WriteString(jsZeroValue(statement.Binding.Type))
		} else {
			emitTypedValue(output, statement.Value, statement.Binding.Type)
		}
		output.WriteString(";\n")
	case ir.StatementAssignment:
		if statement.Target.Kind == ir.ExpressionIndex {
			output.WriteString("__klang_assign_index(")
			emitExpression(output, *statement.Target.Left)
			output.WriteString(", ")
			emitExpression(output, *statement.Target.Right)
			fmt.Fprintf(output, ", %s, ", strconv.Quote(statement.Operator))
			emitExpression(output, statement.Value)
			output.WriteString(");\n")
			return
		}
		if statement.Operator != "=" {
			emitExpression(output, statement.Target)
			output.WriteString(" = __klang_binary(")
			emitExpression(output, statement.Target)
			fmt.Fprintf(output, ", %s, ", strconv.Quote(strings.TrimSuffix(statement.Operator, "=")))
			emitExpression(output, statement.Value)
			output.WriteByte(')')
		} else {
			emitExpression(output, statement.Target)
			fmt.Fprintf(output, " %s ", statement.Operator)
			output.WriteString("__klang_copy(")
			emitExpression(output, statement.Value)
			output.WriteByte(')')
		}
		output.WriteString(";\n")
	case ir.StatementExpression:
		emitExpression(output, statement.Value)
		output.WriteString(";\n")
	case ir.StatementReturn:
		output.WriteString("return")
		if statement.Value.Kind != "" {
			output.WriteByte(' ')
			emitTypedValue(output, statement.Value, statement.Binding.Type)
		}
		output.WriteString(";\n")
	case ir.StatementIf:
		output.WriteString("if (")
		emitExpression(output, statement.Condition)
		output.WriteString(") {\n")
		for _, child := range statement.Body {
			emitStatement(output, child, indent+1, sourceMap)
		}
		writeIndent(output, indent)
		if len(statement.Else) == 0 {
			output.WriteString("}\n")
		} else {
			output.WriteString("} else {\n")
			for _, child := range statement.Else {
				emitStatement(output, child, indent+1, sourceMap)
			}
			writeIndent(output, indent)
			output.WriteString("}\n")
		}
	case ir.StatementWhile:
		output.WriteString("while (")
		emitExpression(output, statement.Condition)
		output.WriteString(") {\n")
		for _, child := range statement.Body {
			emitStatement(output, child, indent+1, sourceMap)
		}
		writeIndent(output, indent)
		output.WriteString("}\n")
	case ir.StatementRange:
		output.WriteString("for (let ")
		output.WriteString(jsIdentifier(statement.Binding.Name))
		output.WriteString(" = 0, __klang_count = ")
		output.WriteString("__klang_range_count(")
		emitExpression(output, statement.Value)
		output.WriteByte(')')
		output.WriteString("; ")
		output.WriteString(jsIdentifier(statement.Binding.Name))
		output.WriteString(" < __klang_count; ")
		output.WriteString(jsIdentifier(statement.Binding.Name))
		output.WriteString(" += 1) {\n")
		for _, child := range statement.Body {
			emitStatement(output, child, indent+1, sourceMap)
		}
		writeIndent(output, indent)
		output.WriteString("}\n")
	case ir.StatementForEach:
		output.WriteString("for (const ")
		output.WriteString(jsIdentifier(statement.Binding.Name))
		output.WriteString(" of __klang_iter(")
		emitExpression(output, statement.Value)
		output.WriteString(")) {\n")
		for _, child := range statement.Body {
			emitStatement(output, child, indent+1, sourceMap)
		}
		writeIndent(output, indent)
		output.WriteString("}\n")
	case ir.StatementBreak:
		output.WriteString("break;\n")
	case ir.StatementContinue:
		output.WriteString("continue;\n")
	case ir.StatementThrow:
		output.WriteString("throw ")
		emitExpression(output, statement.Value)
		output.WriteString(";\n")
	case ir.StatementAssert:
		output.WriteString("if (!(")
		emitExpression(output, statement.Value)
		output.WriteString(")) throw new Error(\"assertion failed\");\n")
	case ir.StatementBlock:
		output.WriteString("{\n")
		for _, child := range statement.Body {
			emitStatement(output, child, indent+1, sourceMap)
		}
		writeIndent(output, indent)
		output.WriteString("}\n")
	}
}

func emitExpression(output *strings.Builder, expression ir.Expression) {
	switch expression.Kind {
	case ir.ExpressionLiteral:
		switch expression.Type {
		case "String":
			output.WriteString(strconv.Quote(expression.Value))
		case "Char":
			output.WriteString("__klang_char(")
			output.WriteString(strconv.Quote(expression.Value))
			output.WriteByte(')')
		case "Bool":
			output.WriteString(strings.ToLower(expression.Value))
		default:
			output.WriteString(strings.ReplaceAll(expression.Value, "_", ""))
		}
	case ir.ExpressionIdentifier:
		output.WriteString(jsIdentifier(expression.Name))
	case ir.ExpressionUnary:
		if expression.Operator == "not" {
			output.WriteByte('!')
		} else {
			output.WriteString(expression.Operator)
		}
		output.WriteByte('(')
		emitExpression(output, *expression.Right)
		output.WriteByte(')')
	case ir.ExpressionBinary:
		if expression.Operator != "and" && expression.Operator != "or" {
			output.WriteString("__klang_binary(")
			emitExpression(output, *expression.Left)
			fmt.Fprintf(output, ", %s, ", strconv.Quote(expression.Operator))
			emitExpression(output, *expression.Right)
			output.WriteByte(')')
			return
		}
		operator := expression.Operator
		switch operator {
		case "and":
			operator = "&&"
		case "or":
			operator = "||"
		}
		output.WriteByte('(')
		emitExpression(output, *expression.Left)
		fmt.Fprintf(output, " %s ", operator)
		emitExpression(output, *expression.Right)
		output.WriteByte(')')
	case ir.ExpressionCall:
		if strings.HasPrefix(expression.Name, "__pipeline:") {
			output.WriteString("__klang_pipeline_method(")
			emitExpression(output, expression.Arguments[0])
			fmt.Fprintf(output, ", %s, [", strconv.Quote(strings.TrimPrefix(expression.Name, "__pipeline:")))
			for index, argument := range expression.Arguments[1:] {
				if index != 0 {
					output.WriteString(", ")
				}
				emitExpression(output, argument)
			}
			output.WriteString("])")
			return
		}
		if strings.HasPrefix(expression.Name, "__struct_cast:") {
			output.WriteString("__klang_struct_cast(")
			emitExpression(output, expression.Arguments[0])
			fmt.Fprintf(output, ", %s)", strconv.Quote(strings.TrimPrefix(expression.Name, "__struct_cast:")))
			return
		}
		if strings.HasPrefix(expression.Name, "__method:") {
			output.WriteString("__klang_call_method(")
			emitExpression(output, expression.Arguments[0])
			fmt.Fprintf(output, ", %s, [", strconv.Quote(strings.TrimPrefix(expression.Name, "__method:")))
			for index, argument := range expression.Arguments[1:] {
				if index != 0 {
					output.WriteString(", ")
				}
				emitExpression(output, argument)
			}
			output.WriteString("])")
			return
		}
		copyArguments := false
		switch expression.Name {
		case "print":
			output.WriteString("__klang_print")
		case "__len":
			output.WriteString("__klang_len")
		case "__string_uppercase":
			output.WriteString("__klang_string_uppercase")
		case "__string_lowercase":
			output.WriteString("__klang_string_lowercase")
		case "__json":
			output.WriteString("__klang_json")
		case "__json_stringify":
			output.WriteString("__klang_json_stringify")
		case "__json_encode":
			output.WriteString("__klang_json_encode")
		case "__json_decode":
			output.WriteString("__klang_json_decode")
		case "__table":
			output.WriteString("__klang_table")
		case "__table_has":
			output.WriteString("__klang_table_has")
		case "__table_delete":
			output.WriteString("__klang_table_delete")
		case "__table_keys":
			output.WriteString("__klang_table_keys")
		case "__table_values":
			output.WriteString("__klang_table_values")
		case "__table_entries":
			output.WriteString("__klang_table_entries")
		case "__table_sequence_count":
			output.WriteString("__klang_table_sequence_count")
		case "__table_set_fallback":
			output.WriteString("__klang_table_set_fallback")
		default:
			output.WriteString(jsIdentifier(expression.Name))
			copyArguments = true
		}
		output.WriteByte('(')
		for index, argument := range expression.Arguments {
			if index != 0 {
				output.WriteString(", ")
			}
			if copyArguments {
				output.WriteString("__klang_copy(")
			}
			emitExpression(output, argument)
			if copyArguments {
				output.WriteByte(')')
			}
		}
		output.WriteByte(')')
	case ir.ExpressionSelector:
		output.WriteString("__klang_select(")
		emitExpression(output, *expression.Left)
		fmt.Fprintf(output, ", %s)", strconv.Quote(expression.Name))
	case ir.ExpressionIndex:
		output.WriteString("__klang_index(")
		emitExpression(output, *expression.Left)
		output.WriteString(", ")
		emitExpression(output, *expression.Right)
		output.WriteByte(')')
	case ir.ExpressionList:
		output.WriteByte('[')
		for index, item := range expression.Arguments {
			if index != 0 {
				output.WriteString(", ")
			}
			emitExpression(output, item)
		}
		output.WriteByte(']')
	case ir.ExpressionMap:
		output.WriteString("__klang_table_from_pairs([")
		for index, entry := range expression.Entries {
			if index != 0 {
				output.WriteString(", ")
			}
			output.WriteByte('[')
			emitExpression(output, entry.Key)
			output.WriteString(", ")
			emitExpression(output, entry.Value)
			output.WriteByte(']')
		}
		output.WriteString("])")
	case ir.ExpressionComprehension:
		output.WriteString("(() => { const __klang_result = []; for (const ")
		output.WriteString(jsIdentifier(expression.Name))
		output.WriteString(" of __klang_list_iter(")
		emitExpression(output, *expression.Left)
		output.WriteString(")) { ")
		if expression.Condition != nil {
			output.WriteString("if (")
			emitExpression(output, *expression.Condition)
			output.WriteString(") ")
		}
		output.WriteString("__klang_result.push(__klang_copy(")
		emitExpression(output, *expression.Right)
		output.WriteString(")); } return __klang_result; })()")
	case ir.ExpressionConditional:
		output.WriteByte('(')
		emitExpression(output, *expression.Condition)
		output.WriteString(" ? ")
		emitExpression(output, *expression.Consequence)
		output.WriteString(" : ")
		emitExpression(output, *expression.Alternative)
		output.WriteByte(')')
	case ir.ExpressionCast:
		if expression.Type == "String" {
			output.WriteString("__klang_format(")
			emitExpression(output, *expression.Left)
			output.WriteByte(')')
		} else if expression.Type == "JSON" {
			output.WriteString("__klang_json(")
			emitExpression(output, *expression.Left)
			output.WriteByte(')')
		} else {
			emitExpression(output, *expression.Left)
		}
	}
}

func emitTypedValue(output *strings.Builder, expression ir.Expression, typeName string) {
	if keyType, valueType, ok := jsMapTypes(typeName); ok {
		output.WriteString("__klang_as_map(")
		emitExpression(output, expression)
		fmt.Fprintf(output, ", %s, %s)", strconv.Quote(keyType), strconv.Quote(valueType))
		return
	}
	output.WriteString("__klang_copy(")
	emitExpression(output, expression)
	output.WriteByte(')')
}

func jsZeroValue(typeName string) string {
	if strings.HasPrefix(strings.TrimSpace(typeName), "List[") {
		return "[]"
	}
	if keyType, valueType, ok := jsMapTypes(typeName); ok {
		return fmt.Sprintf("__klang_collection_new(\"Map\", %s, %s)", strconv.Quote(keyType), strconv.Quote(valueType))
	}
	switch typeName {
	case "String":
		return `""`
	case "Char":
		return `__klang_char("")`
	case "Bool":
		return "false"
	case "JSON":
		return "null"
	case "Table":
		return `__klang_table()`
	default:
		return "0"
	}
}

func jsIdentifier(name string) string {
	var output strings.Builder
	output.WriteString("k_")
	for _, current := range name {
		if current == '_' || current == '$' || unicode.IsLetter(current) || (output.Len() > 2 && unicode.IsDigit(current)) {
			output.WriteRune(current)
			continue
		}
		fmt.Fprintf(&output, "_u%x_", current)
	}
	return output.String()
}

func writeIndent(output *strings.Builder, count int) {
	output.WriteString(strings.Repeat("    ", count))
}

func hasIRFunction(functions []ir.Function, name string) bool {
	for _, function := range functions {
		if function.Name == name {
			return true
		}
	}
	return false
}
