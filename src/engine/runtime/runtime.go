package runtime

import (
	"fmt"
	"strings"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/lexer"
	"kLang/src/parser"
)

type ValueKind string

const (
	ValueNull     ValueKind = "Null"
	ValueInt      ValueKind = "Int"
	ValueFloat    ValueKind = "Float"
	ValueString   ValueKind = "String"
	ValueBool     ValueKind = "Bool"
	ValueChar     ValueKind = "Char"
	ValueList     ValueKind = "List"
	ValueMap      ValueKind = "Map"
	ValueFunction ValueKind = "Function"
)

type Value struct {
	Kind ValueKind
	Data any
}

type Result struct {
	Value  Value
	Output []string
}

type Error struct {
	Message string
}

func (err Error) Error() string {
	return err.Message
}

func errorAt(pos parser.Position, message string) error {
	if pos.Line > 0 {
		return Error{Message: fmt.Sprintf("line %d:%d: %s", pos.Line, pos.Column, message)}
	}
	return Error{Message: message}
}

type Runtime struct {
	memory    *Memory
	global    *Environment
	functions map[string]parser.FunctionStatement
	output    []string
}

func New() *Runtime {
	return &Runtime{
		memory:    NewMemory(),
		global:    NewEnvironment(nil),
		functions: map[string]parser.FunctionStatement{},
	}
}

func RunProgram(program file.Program) (Result, error) {
	resolvedProgram, moduleReport := modulesystem.ResolveProgram(program)
	if !moduleReport.Passed() {
		return Result{}, Error{Message: fmt.Sprintf("module resolution failed: %v", moduleReport.Errors)}
	}

	typeReport := typechecker.CheckProgram(resolvedProgram)
	if !typeReport.Passed() {
		return Result{}, Error{Message: fmt.Sprintf("type check failed: %v", typeReport.Errors)}
	}

	parsed := parser.ParseLoadedProgram(resolvedProgram)
	if !parsed.Passed() {
		return Result{}, Error{Message: fmt.Sprintf("parse failed: %v", parsed.Errors())}
	}

	return New().Run(parsed)
}

func (runtime *Runtime) Run(program parser.ParsedProgram) (Result, error) {
	for _, source := range program.Sources {
		for _, stmt := range source.Program.Statements {
			runtime.collectFunctions(stmt, "")
		}
	}

	for _, source := range program.Sources {
		signal, err := runtime.executeBlock(source.Program.Statements, runtime.global, false)
		if err != nil {
			return Result{}, err
		}
		if signal.kind != signalNone {
			return Result{}, Error{Message: "top-level return or break is not allowed"}
		}
	}

	mainName := runtime.resolveFunctionName("Main")
	if mainName == "" {
		return Result{Value: NullValue(), Output: runtime.output}, nil
	}

	value, err := runtime.callFunction(mainName, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Value: value, Output: runtime.output}, nil
}

func (runtime *Runtime) collectFunctions(stmt parser.Statement, namespace string) {
	switch current := stmt.(type) {
	case parser.FunctionStatement:
		name := namespace + current.Name
		runtime.functions[name] = current
	case parser.NamespaceStatement:
		for _, nested := range current.Body {
			runtime.collectFunctions(nested, namespace+current.Name+".")
		}
	}
}

type signalKind int

const (
	signalNone signalKind = iota
	signalReturn
	signalBreak
)

type signal struct {
	kind  signalKind
	value Value
}

func (runtime *Runtime) executeBlock(statements []parser.Statement, env *Environment, inLoop bool) (signal, error) {
	for _, stmt := range statements {
		currentSignal, err := runtime.executeStatement(stmt, env, inLoop)
		if err != nil {
			return signal{}, err
		}
		if currentSignal.kind != signalNone {
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeStatement(stmt parser.Statement, env *Environment, inLoop bool) (signal, error) {
	switch current := stmt.(type) {
	case parser.ImportStatement:
		return signal{kind: signalNone}, nil
	case parser.NamespaceStatement:
		return signal{kind: signalNone}, nil
	case parser.FunctionStatement:
		return signal{kind: signalNone}, nil
	case parser.VariableStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		if !valueMatchesType(value, current.Type) {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("cannot assign %s to %s variable %q", value.Kind, current.Type, current.Name))
		}
		targetEnv := env
		if current.Scope == "global" {
			targetEnv = runtime.global
		}
		if err := targetEnv.Define(current.Name, current.Mutable, value, runtime.memory.Allocate(value)); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.ReturnStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		return signal{kind: signalReturn, value: value}, nil
	case parser.BreakStatement:
		if !inLoop {
			return signal{}, errorAt(current.Pos, "break is only allowed inside a loop")
		}
		return signal{kind: signalBreak}, nil
	case parser.ExpressionStatement:
		_, err := runtime.evalExpression(current.Expression.Node, env)
		return signal{kind: signalNone}, err
	case parser.AssignmentStatement:
		if err := runtime.executeAssignment(current, env); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.IfStatement:
		return runtime.executeIf(current, env, inLoop)
	case parser.LoopStatement:
		return runtime.executeLoop(current, env)
	default:
		return signal{}, Error{Message: fmt.Sprintf("unsupported statement %T", stmt)}
	}
}

func (runtime *Runtime) executeIf(stmt parser.IfStatement, env *Environment, inLoop bool) (signal, error) {
	condition, err := runtime.evalExpression(stmt.Condition.Node, env)
	if err != nil {
		return signal{}, err
	}

	conditionValue := isTruthy(condition)
	shouldRun := conditionValue
	if stmt.Kind == "unless" {
		shouldRun = !conditionValue
	}

	if shouldRun {
		return runtime.executeBlock(stmt.Consequence, NewEnvironment(env), inLoop)
	}
	if stmt.ElseIf != nil {
		return runtime.executeIf(*stmt.ElseIf, env, inLoop)
	}
	if len(stmt.Alternative) != 0 {
		return runtime.executeBlock(stmt.Alternative, NewEnvironment(env), inLoop)
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeLoop(stmt parser.LoopStatement, env *Environment) (signal, error) {
	if init, condition, post, ok := parseCStyleForHeader(stmt.Header); ok {
		loopEnv := NewEnvironment(env)
		if len(init.Tokens) != 0 {
			if err := runtime.executeLoopHeaderAssignment(init, loopEnv); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
		}
		for {
			if len(condition.Tokens) != 0 {
				conditionValue, err := runtime.evalExpression(condition.Node, loopEnv)
				if err != nil {
					return signal{}, err
				}
				if !isTruthy(conditionValue) {
					break
				}
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(loopEnv), true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				break
			}
			if currentSignal.kind == signalReturn {
				return currentSignal, nil
			}
			if len(post.Tokens) != 0 {
				if err := runtime.executeLoopHeaderAssignment(post, loopEnv); err != nil {
					return signal{}, errorAt(stmt.Pos, err.Error())
				}
			}
		}
		return signal{kind: signalNone}, nil
	}

	if iterator, iterable, ok := parseRangeHeader(stmt.Header); ok {
		countValue, err := runtime.evalExpression(iterable.Node, env)
		if err != nil {
			return signal{}, err
		}
		count, err := asInt(countValue)
		if err != nil {
			return signal{}, errorAt(stmt.Pos, "range expects an Int count")
		}
		if count < 0 {
			return signal{}, errorAt(stmt.Pos, "range count cannot be negative")
		}
		for index := 0; index < count; index++ {
			loopEnv := NewEnvironment(env)
			value := IntValue(index)
			if err := loopEnv.Define(iterator, false, value, runtime.memory.Allocate(value)); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, loopEnv, true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				return signal{kind: signalNone}, nil
			}
			if currentSignal.kind == signalReturn {
				return currentSignal, nil
			}
		}
		return signal{kind: signalNone}, nil
	}

	first := true
	for {
		if stmt.Kind != "do_while" && stmt.Kind != "do" || !first {
			condition, err := runtime.evalExpression(loopCondition(stmt.Header).Node, env)
			if err != nil {
				return signal{}, err
			}
			if !isTruthy(condition) {
				break
			}
		}
		first = false
		currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(env), true)
		if err != nil {
			return signal{}, err
		}
		if currentSignal.kind == signalBreak {
			break
		}
		if currentSignal.kind == signalReturn {
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeLoopHeaderAssignment(expr parser.Expression, env *Environment) error {
	if len(expr.Tokens) == 0 {
		return nil
	}
	if stmt, ok := loopHeaderStatement(expr); ok {
		currentSignal, err := runtime.executeStatement(stmt, env, true)
		if err != nil {
			return err
		}
		if currentSignal.kind != signalNone {
			return Error{Message: "loop header cannot return or break"}
		}
		return nil
	}
	_, err := runtime.evalExpression(expr.Node, env)
	return err
}

func loopHeaderStatement(expr parser.Expression) (parser.Statement, bool) {
	tokens := expr.Tokens
	if len(tokens) < 3 {
		return nil, false
	}

	if tokens[0].Type == lexer.TokenIdentifier && tokens[1].Type == lexer.TokenEvaluationAssign {
		value := parser.Expression{Tokens: tokens[2:], Node: parser.ParseExpressionTokens(tokens[2:])}
		return parser.VariableStatement{
			Pos:        parser.Position{Line: tokens[0].Line, Column: tokens[0].Column},
			Scope:      "local",
			Mutable:    true,
			Type:       "Int",
			Name:       tokens[0].Literal,
			Expression: value,
		}, true
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

func (runtime *Runtime) executeAssignment(stmt parser.AssignmentStatement, env *Environment) error {
	value, err := runtime.evalExpression(stmt.Expression.Node, env)
	if err != nil {
		return err
	}

	if indexExpr, ok := stmt.Target.Node.(parser.IndexExpression); ok {
		return runtime.assignIndex(indexExpr, stmt.Operator, value, env)
	}

	identifier, ok := stmt.Target.Node.(parser.IdentifierExpression)
	if !ok {
		return Error{Message: "assignment target must be a variable or indexed value"}
	}

	binding, ok := env.Get(identifier.Name)
	if !ok {
		return Error{Message: fmt.Sprintf("unknown variable %q", identifier.Name)}
	}
	if err := runtime.memory.EnsureWritable(binding.ObjectID); err != nil {
		return err
	}
	if !binding.Mutable {
		return Error{Message: fmt.Sprintf("cannot mutate immutable variable %q", identifier.Name)}
	}

	next, err := applyAssignmentOperator(binding.Value, stmt.Operator, value)
	if err != nil {
		return err
	}
	binding.Value = next
	runtime.memory.Store(binding.ObjectID, next)
	return nil
}

func (runtime *Runtime) assignIndex(indexExpr parser.IndexExpression, operator string, value Value, env *Environment) error {
	targetIdentifier, ok := indexExpr.Target.(parser.IdentifierExpression)
	if !ok {
		return Error{Message: "indexed assignment target must start from a variable"}
	}
	binding, ok := env.Get(targetIdentifier.Name)
	if !ok {
		return Error{Message: fmt.Sprintf("unknown variable %q", targetIdentifier.Name)}
	}
	if !binding.Mutable {
		return Error{Message: fmt.Sprintf("cannot mutate immutable variable %q", targetIdentifier.Name)}
	}
	if err := runtime.memory.EnsureWritable(binding.ObjectID); err != nil {
		return err
	}

	index, err := runtime.evalExpression(indexExpr.Index, env)
	if err != nil {
		return err
	}

	switch binding.Value.Kind {
	case ValueList:
		items := binding.Value.Data.([]Value)
		position, err := asInt(index)
		if err != nil {
			return err
		}
		if position < 0 {
			return Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
		}
		for len(items) <= position {
			items = append(items, NullValue())
		}
		current := items[position]
		next, err := applyAssignmentOperator(current, operator, value)
		if err != nil {
			return err
		}
		items[position] = next
		binding.Value = Value{Kind: ValueList, Data: items}
	case ValueMap:
		items := binding.Value.Data.(map[string]Value)
		key, err := mapKey(index)
		if err != nil {
			return err
		}
		current := items[key]
		next, err := applyAssignmentOperator(current, operator, value)
		if err != nil {
			return err
		}
		items[key] = next
		binding.Value = Value{Kind: ValueMap, Data: items}
	default:
		return Error{Message: fmt.Sprintf("%s is not index-assignable", binding.Value.Kind)}
	}
	runtime.memory.Store(binding.ObjectID, binding.Value)
	return nil
}

func (runtime *Runtime) evalExpression(expr parser.ExpressionNode, env *Environment) (Value, error) {
	switch current := expr.(type) {
	case nil:
		return NullValue(), nil
	case parser.IdentifierExpression:
		binding, ok := env.Get(current.Name)
		if ok {
			if err := runtime.memory.BorrowImmutable(binding.ObjectID); err != nil {
				return NullValue(), err
			}
			runtime.memory.ReleaseImmutable(binding.ObjectID)
			return binding.Value, nil
		}
		if isBuiltinFunction(current.Name) {
			return FunctionValue(current.Name), nil
		}
		if name := runtime.resolveFunctionName(current.Name); name != "" {
			return FunctionValue(name), nil
		}
		return NullValue(), Error{Message: fmt.Sprintf("unknown identifier %q", current.Name)}
	case parser.LiteralExpression:
		return literalValue(current)
	case parser.GroupExpression:
		return runtime.evalExpression(current.Inner, env)
	case parser.UnaryExpression:
		return runtime.evalUnary(current, env)
	case parser.BinaryExpression:
		return runtime.evalBinary(current, env)
	case parser.CallExpression:
		return runtime.evalCall(current, env)
	case parser.SelectorExpression:
		value, err := runtime.evalExpression(current.Target, env)
		if err == nil && value.Kind == ValueFunction {
			return FunctionValue(value.Data.(string) + "." + current.Field), nil
		}
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			return FunctionValue(target.Name + "." + current.Field), nil
		}
		return NullValue(), Error{Message: "unsupported selector target"}
	case parser.IndexExpression:
		return runtime.evalIndex(current, env)
	case parser.ListExpression:
		items := make([]Value, 0, len(current.Items))
		for _, item := range current.Items {
			value, err := runtime.evalExpression(item, env)
			if err != nil {
				return NullValue(), err
			}
			items = append(items, value)
		}
		return Value{Kind: ValueList, Data: items}, nil
	case parser.MapExpression:
		items := map[string]Value{}
		for _, entry := range current.Entries {
			key, err := runtime.evalExpression(entry.Key, env)
			if err != nil {
				return NullValue(), err
			}
			value, err := runtime.evalExpression(entry.Value, env)
			if err != nil {
				return NullValue(), err
			}
			mapKeyValue, err := mapKey(key)
			if err != nil {
				return NullValue(), err
			}
			items[mapKeyValue] = value
		}
		return Value{Kind: ValueMap, Data: items}, nil
	case parser.RawExpression:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %q", parser.Expression{Tokens: current.Tokens}.Literal())}
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %T", expr)}
	}
}

func (runtime *Runtime) evalUnary(expr parser.UnaryExpression, env *Environment) (Value, error) {
	value, err := runtime.evalExpression(expr.Right, env)
	if err != nil {
		return NullValue(), err
	}
	switch expr.Operator {
	case "-":
		if value.Kind == ValueFloat {
			return FloatValue(-value.Data.(float64)), nil
		}
		intValue, err := asInt(value)
		if err != nil {
			return NullValue(), err
		}
		return IntValue(-intValue), nil
	case "not":
		return BoolValue(!isTruthy(value)), nil
	case "call":
		if call, ok := expr.Right.(parser.CallExpression); ok {
			return runtime.evalCall(call, env)
		}
		return value, nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported unary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) evalBinary(expr parser.BinaryExpression, env *Environment) (Value, error) {
	left, err := runtime.evalExpression(expr.Left, env)
	if err != nil {
		return NullValue(), err
	}
	right, err := runtime.evalExpression(expr.Right, env)
	if err != nil {
		return NullValue(), err
	}

	switch expr.Operator {
	case "+":
		if left.Kind == ValueString || right.Kind == ValueString {
			return StringValue(valueString(left) + valueString(right)), nil
		}
		return numericBinary(left, right, func(a, b float64) float64 { return a + b })
	case "-":
		return numericBinary(left, right, func(a, b float64) float64 { return a - b })
	case "*":
		return numericBinary(left, right, func(a, b float64) float64 { return a * b })
	case "/", "//":
		return divideValues(left, right)
	case "%":
		return moduloValues(left, right)
	case "==":
		return BoolValue(valueString(left) == valueString(right)), nil
	case "!=":
		return BoolValue(valueString(left) != valueString(right)), nil
	case ">":
		return compareNumeric(left, right, func(a, b float64) bool { return a > b })
	case ">=":
		return compareNumeric(left, right, func(a, b float64) bool { return a >= b })
	case "<":
		return compareNumeric(left, right, func(a, b float64) bool { return a < b })
	case "<=":
		return compareNumeric(left, right, func(a, b float64) bool { return a <= b })
	case "and":
		return BoolValue(isTruthy(left) && isTruthy(right)), nil
	case "or":
		return BoolValue(isTruthy(left) || isTruthy(right)), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported binary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) evalCall(expr parser.CallExpression, env *Environment) (Value, error) {
	callee, err := runtime.evalExpression(expr.Callee, env)
	if err != nil {
		return NullValue(), err
	}
	if callee.Kind != ValueFunction {
		return NullValue(), Error{Message: "callee is not a function"}
	}

	args := make([]Value, 0, len(expr.Arguments))
	for _, arg := range expr.Arguments {
		value, err := runtime.evalExpression(arg, env)
		if err != nil {
			return NullValue(), err
		}
		args = append(args, value)
	}
	return runtime.callFunction(callee.Data.(string), args)
}

func (runtime *Runtime) evalIndex(expr parser.IndexExpression, env *Environment) (Value, error) {
	target, err := runtime.evalExpression(expr.Target, env)
	if err != nil {
		return NullValue(), err
	}
	index, err := runtime.evalExpression(expr.Index, env)
	if err != nil {
		return NullValue(), err
	}
	switch target.Kind {
	case ValueList:
		items := target.Data.([]Value)
		position, err := asInt(index)
		if err != nil {
			return NullValue(), err
		}
		if position < 0 || position >= len(items) {
			return NullValue(), Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
		}
		return items[position], nil
	case ValueMap:
		items := target.Data.(map[string]Value)
		key, err := mapKey(index)
		if err != nil {
			return NullValue(), err
		}
		value, ok := items[key]
		if !ok {
			return NullValue(), Error{Message: fmt.Sprintf("map key %q does not exist", key)}
		}
		return value, nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("%s is not indexable", target.Kind)}
	}
}

func (runtime *Runtime) callFunction(name string, args []Value) (Value, error) {
	switch name {
	case "print":
		if len(args) > 0 {
			runtime.output = append(runtime.output, valueString(args[0]))
		}
		return NullValue(), nil
	case "len":
		if len(args) != 1 {
			return NullValue(), Error{Message: "len expects one argument"}
		}
		length, err := valueLen(args[0])
		if err != nil {
			return NullValue(), err
		}
		return IntValue(length), nil
	case "range":
		if len(args) != 1 {
			return NullValue(), Error{Message: "range expects one argument"}
		}
		return args[0], nil
	}

	resolvedName := runtime.resolveFunctionName(name)
	if resolvedName == "" {
		return NullValue(), Error{Message: fmt.Sprintf("unknown function %q", name)}
	}
	function := runtime.functions[resolvedName]
	if len(args) != len(function.Params) {
		return NullValue(), Error{Message: fmt.Sprintf("function %s expects %d argument(s), got %d", name, len(function.Params), len(args))}
	}

	env := NewEnvironment(runtime.global)
	for index, param := range function.Params {
		value := args[index]
		if !valueMatchesType(value, param.Type) {
			return NullValue(), Error{Message: fmt.Sprintf("function %s argument %q expects %s, got %s", name, param.Name, param.Type, value.Kind)}
		}
		if err := env.Define(param.Name, false, value, runtime.memory.Allocate(value)); err != nil {
			return NullValue(), err
		}
	}
	currentSignal, err := runtime.executeBlock(function.Body, env, false)
	if err != nil {
		return NullValue(), err
	}
	if currentSignal.kind == signalReturn {
		if !valueMatchesType(currentSignal.value, function.ReturnType) {
			return NullValue(), Error{Message: fmt.Sprintf("function %s returns %s, got %s", name, function.ReturnType, currentSignal.value.Kind)}
		}
		return currentSignal.value, nil
	}
	return NullValue(), nil
}

func (runtime *Runtime) resolveFunctionName(name string) string {
	if _, ok := runtime.functions[name]; ok {
		return name
	}
	for functionName := range runtime.functions {
		if strings.HasSuffix(functionName, "."+name) {
			return functionName
		}
	}
	return ""
}

func isBuiltinFunction(name string) bool {
	switch name {
	case "print", "len", "range":
		return true
	default:
		return false
	}
}
