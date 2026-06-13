package runtime

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/lexer"
	"kLang/src/parser"
)

type ValueKind string

const (
	ValueNull        ValueKind = "Null"
	ValueInt         ValueKind = "Int"
	ValueFloat       ValueKind = "Float"
	ValueString      ValueKind = "String"
	ValueBool        ValueKind = "Bool"
	ValueChar        ValueKind = "Char"
	ValueList        ValueKind = "List"
	ValueMap         ValueKind = "Map"
	ValueOption      ValueKind = "Option"
	ValueResult      ValueKind = "Result"
	ValueComplex     ValueKind = "Complex"
	ValueSIMD        ValueKind = "SIMD"
	ValueTable       ValueKind = "Table"
	ValueAwaitable   ValueKind = "Awaitable"
	ValueIterator    ValueKind = "Iterator"
	ValueCoroutine   ValueKind = "Coroutine"
	ValueFunction    ValueKind = "Function"
	ValueObject      ValueKind = "Object"
	ValueBoundMethod ValueKind = "BoundMethod"
	ValueThunk       ValueKind = "Thunk"
)

type Value struct {
	Kind ValueKind
	Data any
}

type OptionData struct {
	Some  bool
	Value Value
}

type ResultData struct {
	Ok    bool
	Value Value
}

type ComplexData struct {
	Real float64
	Imag float64
}

type SIMDData struct {
	Lanes []Value
}

type AwaitableData struct {
	Function string
	Args     []Value
	Done     bool
	Value    Value
}

type IteratorData struct {
	Items []Value
	Index int
}

type CoroutineData struct {
	Function string
	Done     bool
	Value    Value
}

type ThunkData struct {
	Expr      parser.ExpressionNode
	Env       *Environment
	Evaluated bool
	Value     Value
}

type ObjectData struct {
	Type   string
	Fields map[string]Value
}

type BoundMethodData struct {
	Type     string
	Name     string
	Receiver Value
}

type RegionData struct {
	Name     string
	TypeName string
	Size     Value
	Count    Value
}

type Result struct {
	Value  Value
	Output []string
	Memory MemoryStats
}

type Error struct {
	Message string
	Line    int
	Column  int
}

func (err Error) Error() string {
	if err.Line > 0 {
		return fmt.Sprintf("line %d:%d: %s", err.Line, err.Column, err.Message)
	}
	return err.Message
}

func errorAt(pos parser.Position, message string) error {
	if pos.Line > 0 {
		return Error{Line: pos.Line, Column: pos.Column, Message: message}
	}
	return Error{Message: message}
}

type thrownError struct {
	Value Value
}

func (err thrownError) Error() string {
	return "thrown " + valueString(err.Value)
}

func thrownValue(err error) (Value, bool) {
	thrown, ok := err.(thrownError)
	if !ok {
		return NullValue(), false
	}
	return thrown.Value, true
}

type Runtime struct {
	memory         *Memory
	global         *Environment
	functions      map[string]parser.FunctionStatement
	aliasFunctions map[string]parser.AliasFunctionStatement
	regions        map[string]RegionData
	groups         map[string][]string
	closures       map[string]*Environment
	aliases        map[string]string
	output         []string
	callDepth      int
	maxDepth       int
	callStack      []string
	nextFunc       int
	innerSets      []map[string]Value
	args           []string
}

const defaultMaxCallDepth = 1024

func New() *Runtime {
	return &Runtime{
		memory:         NewMemory(),
		global:         NewEnvironment(nil),
		functions:      map[string]parser.FunctionStatement{},
		aliasFunctions: map[string]parser.AliasFunctionStatement{},
		regions:        map[string]RegionData{},
		groups:         map[string][]string{},
		closures:       map[string]*Environment{},
		aliases:        map[string]string{},
		maxDepth:       defaultMaxCallDepth,
	}
}

func NewWithArgs(args []string) *Runtime {
	runtime := New()
	runtime.args = append([]string(nil), args...)
	return runtime
}

func RunProgram(program file.Program) (Result, error) {
	return RunProgramWithArgs(program, nil)
}

func RunProgramWithArgs(program file.Program, args []string) (Result, error) {
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

	return NewWithArgs(args).Run(parsed)
}

func (runtime *Runtime) Run(program parser.ParsedProgram) (Result, error) {
	if err := runtime.defineArgs(); err != nil {
		return Result{}, err
	}
	for _, source := range program.Sources {
		for _, stmt := range source.Program.Statements {
			if err := runtime.collectFunctions(stmt, ""); err != nil {
				return Result{}, err
			}
		}
	}

	for _, source := range program.Sources {
		signal, err := runtime.executeBlock(source.Program.Statements, runtime.global, false)
		if err != nil {
			return Result{}, err
		}
		if signal.kind != signalNone {
			if signal.kind == signalThrow {
				return Result{}, Error{Message: "uncaught exception: " + valueString(signal.value)}
			}
			return Result{}, Error{Message: "top-level return, break, or continue is not allowed"}
		}
	}

	mainName, err := runtime.resolveFunctionName("Main")
	if err != nil {
		return Result{}, err
	}
	if mainName == "" {
		return Result{Value: NullValue(), Output: runtime.output, Memory: runtime.memory.Stats()}, nil
	}

	value, err := runtime.callFunction(mainName, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Value: value, Output: runtime.output, Memory: runtime.memory.Stats()}, nil
}

func (runtime *Runtime) defineArgs() error {
	values := make([]Value, 0, len(runtime.args))
	for _, arg := range runtime.args {
		values = append(values, StringValue(arg))
	}
	return runtime.defineValueInRegion(runtime.global, "Args", false, "List[String]", Value{Kind: ValueList, Data: values}, MemoryHeap)
}

func (runtime *Runtime) collectFunctions(stmt parser.Statement, namespace string) error {
	switch current := stmt.(type) {
	case parser.RegionStatement:
		runtime.regions[current.Name] = RegionData{Name: current.Name, TypeName: current.TypeName}
	case parser.AliasFunctionStatement:
		if _, exists := runtime.aliasFunctions[current.Name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("alias function %q is already defined", current.Name))
		}
		runtime.aliasFunctions[current.Name] = current
	case parser.FunctionStatement:
		name := namespace + current.Name
		if _, exists := runtime.functions[name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("function %q is already defined", name))
		}
		runtime.functions[name] = current
	case parser.FunctionGroupStatement:
		name := namespace + current.Name
		if _, exists := runtime.groups[name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("function_group %q is already defined", name))
		}
		runtime.groups[name] = append([]string(nil), current.Functions...)
	case parser.TraitStatement:
		return nil
	case parser.ImplStatement:
		return nil
	case parser.AliasStatement:
		if current.Target == "" {
			return errorAt(current.Pos, fmt.Sprintf("alias %q is missing a namespace target", current.Name))
		}
		if _, exists := runtime.aliases[current.Name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("alias %q is already defined", current.Name))
		}
		runtime.aliases[current.Name] = current.Target
	case parser.NamespaceStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace+current.Name+"."); err != nil {
				return err
			}
		}
	case parser.MatchStatement:
		for _, matchCase := range current.Cases {
			for _, nested := range matchCase.Body {
				if err := runtime.collectFunctions(nested, namespace); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

type signalKind int

const (
	signalNone signalKind = iota
	signalReturn
	signalBreak
	signalContinue
	signalTailCall
	signalThrow
)

type signal struct {
	kind     signalKind
	value    Value
	tailName string
	tailArgs []Value
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
	case parser.AliasStatement:
		return signal{kind: signalNone}, nil
	case parser.NamespaceStatement:
		return signal{kind: signalNone}, nil
	case parser.RegionStatement:
		size, err := runtime.evalExpression(current.Size.Node, env)
		if err != nil {
			size = NullValue()
		}
		count, err := runtime.evalExpression(current.Count.Node, env)
		if err != nil {
			count = NullValue()
		}
		runtime.regions[current.Name] = RegionData{Name: current.Name, TypeName: current.TypeName, Size: size, Count: count}
		return signal{kind: signalNone}, nil
	case parser.AliasFunctionStatement:
		return signal{kind: signalNone}, nil
	case parser.FunctionGroupStatement:
		return signal{kind: signalNone}, nil
	case parser.FunctionStatement:
		name, err := runtime.defineLocalFunction(current, env)
		if err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		value := FunctionValue(name)
		if current.Inner && len(runtime.innerSets) > 0 {
			runtime.innerSets[len(runtime.innerSets)-1][current.Name] = value
		}
		if err := runtime.defineValue(env, current.Name, false, functionTypeName(current), value); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.TraitStatement:
		return signal{kind: signalNone}, nil
	case parser.ImplStatement:
		return signal{kind: signalNone}, nil
	case parser.VariableStatement:
		value := zeroValue(current.Type)
		if current.Expression.Node != nil {
			var err error
			value, err = runtime.evalExpression(current.Expression.Node, env)
			if err != nil {
				if thrown, ok := thrownValue(err); ok {
					return signal{kind: signalThrow, value: thrown}, nil
				}
				return signal{}, err
			}
		}
		if current.Type == "Table" && value.Kind == ValueMap {
			value = TableValue(value.Data.(map[string]Value))
		}
		typeName := current.Type
		if current.Inferred && typeName == "T" {
			typeName = runtimeTypeName(value)
		}
		if !valueMatchesType(value, typeName) {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("cannot assign %s to %s variable %q", value.Kind, typeName, current.Name))
		}
		targetEnv := env
		region := MemoryStack
		if current.Scope == "global" || current.Exported {
			targetEnv = runtime.global
			region = MemoryHeap
		} else if preferred := preferredMemoryRegion(value); preferred != "" {
			region = preferred
		}
		if err := runtime.defineValueInRegion(targetEnv, current.Name, current.Mutable, typeName, value, region); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.ReturnStatement:
		if tailSignal, ok, err := runtime.tailCallSignal(current.Expression.Node, env); ok || err != nil {
			if err != nil {
				return signal{}, err
			}
			return tailSignal, nil
		}
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			if thrown, ok := thrownValue(err); ok {
				return signal{kind: signalThrow, value: thrown}, nil
			}
			return signal{}, err
		}
		return signal{kind: signalReturn, value: value}, nil
	case parser.ThrowStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		return signal{kind: signalThrow, value: value}, nil
	case parser.BreakStatement:
		if !inLoop {
			return signal{}, errorAt(current.Pos, "break is only allowed inside a loop")
		}
		return signal{kind: signalBreak}, nil
	case parser.ContinueStatement:
		if !inLoop {
			return signal{}, errorAt(current.Pos, "continue is only allowed inside a loop or pattern match case")
		}
		return signal{kind: signalContinue}, nil
	case parser.ExpressionStatement:
		_, err := runtime.evalExpression(current.Expression.Node, env)
		if thrown, ok := thrownValue(err); ok {
			return signal{kind: signalThrow, value: thrown}, nil
		}
		return signal{kind: signalNone}, err
	case parser.AssignmentStatement:
		if err := runtime.executeAssignment(current, env); err != nil {
			if thrown, ok := thrownValue(err); ok {
				return signal{kind: signalThrow, value: thrown}, nil
			}
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.IfStatement:
		return runtime.executeIf(current, env, inLoop)
	case parser.MatchStatement:
		return runtime.executeMatch(current, env, inLoop)
	case parser.LoopStatement:
		return runtime.executeLoop(current, env)
	case parser.TryCatchStatement:
		return runtime.executeTryCatch(current, env, inLoop)
	default:
		return signal{}, Error{Message: fmt.Sprintf("unsupported statement %T", stmt)}
	}
}

func (runtime *Runtime) executeMatch(stmt parser.MatchStatement, env *Environment, inLoop bool) (signal, error) {
	value, err := runtime.evalExpression(stmt.Value.Node, env)
	if err != nil {
		return signal{}, err
	}
	if !isRuntimePatternMatchValue(value) {
		return signal{}, errorAt(stmt.Pos, fmt.Sprintf("pattern match value must be Bool, String, Int, or Float, got %s", value.Kind))
	}

	matched := false
	for _, matchCase := range stmt.Cases {
		if !matched {
			if matchCase.Default {
				matched = true
			} else {
				pattern, err := runtime.evalExpression(matchCase.Pattern.Node, env)
				if err != nil {
					return signal{}, err
				}
				if !isRuntimePatternMatchValue(pattern) {
					return signal{}, errorAt(matchCase.Pos, fmt.Sprintf("case pattern must be Bool, String, Int, or Float, got %s", pattern.Kind))
				}
				if value.Kind != pattern.Kind {
					return signal{}, errorAt(matchCase.Pos, fmt.Sprintf("case pattern type %s does not match %s", pattern.Kind, value.Kind))
				}
				matched = valuesEqual(value, pattern)
			}
		}
		if !matched {
			continue
		}

		currentSignal, err := runtime.executeBlock(matchCase.Body, NewEnvironment(env), true)
		if err != nil {
			return signal{}, err
		}
		switch currentSignal.kind {
		case signalNone, signalBreak:
			return signal{kind: signalNone}, nil
		case signalContinue:
			continue
		default:
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeTryCatch(stmt parser.TryCatchStatement, env *Environment, inLoop bool) (signal, error) {
	currentSignal, err := runtime.executeBlock(stmt.TryBody, NewEnvironment(env), inLoop)
	if err != nil {
		return signal{}, err
	}
	if currentSignal.kind != signalThrow {
		return currentSignal, nil
	}
	catchEnv := NewEnvironment(env)
	if err := runtime.defineValueInRegion(catchEnv, stmt.ErrorName, false, "T", currentSignal.value, MemoryStack); err != nil {
		return signal{}, errorAt(stmt.Pos, err.Error())
	}
	return runtime.executeBlock(stmt.CatchBody, catchEnv, inLoop)
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

func (runtime *Runtime) tailCallSignal(expr parser.ExpressionNode, env *Environment) (signal, bool, error) {
	call, ok := expr.(parser.CallExpression)
	if !ok || len(call.Arguments) == 0 || len(runtime.callStack) == 0 {
		return signal{}, false, nil
	}
	callee, err := runtime.evalExpression(call.Callee, env)
	if err != nil {
		return signal{}, false, err
	}
	if callee.Kind != ValueFunction {
		return signal{}, false, nil
	}
	name, err := runtime.resolveFunctionName(callee.Data.(string))
	if err != nil {
		return signal{}, false, err
	}
	if name == "" || name != runtime.callStack[len(runtime.callStack)-1] {
		return signal{}, false, nil
	}
	args := make([]Value, 0, len(call.Arguments))
	if runtime.isLazyFunction(name) {
		for _, arg := range call.Arguments {
			args = append(args, ThunkValue(arg, env))
		}
	} else {
		for _, arg := range call.Arguments {
			value, err := runtime.evalExpression(arg, env)
			if err != nil {
				return signal{}, false, err
			}
			args = append(args, value)
		}
	}
	return signal{kind: signalTailCall, tailName: name, tailArgs: args}, true, nil
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
			if currentSignal.kind == signalContinue {
				if len(post.Tokens) != 0 {
					if err := runtime.executeLoopHeaderAssignment(post, loopEnv); err != nil {
						return signal{}, errorAt(stmt.Pos, err.Error())
					}
				}
				continue
			}
			if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
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
			if err := runtime.defineValue(loopEnv, iterator, false, "Int", value); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, loopEnv, true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				return signal{kind: signalNone}, nil
			}
			if currentSignal.kind == signalContinue {
				continue
			}
			if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
				return currentSignal, nil
			}
		}
		return signal{kind: signalNone}, nil
	}

	loopEnv := env
	headerName, headerExpr, hasHeaderBinding := parseEvaluationHeader(stmt.Header)
	if hasHeaderBinding {
		loopEnv = NewEnvironment(env)
	}
	first := true
	for {
		if stmt.Kind != "do_while" && stmt.Kind != "do" || !first {
			conditionExpression := loopCondition(stmt.Header)
			if hasHeaderBinding {
				conditionExpression = headerExpr
			}
			condition, err := runtime.evalExpression(conditionExpression.Node, loopEnv)
			if err != nil {
				return signal{}, err
			}
			if hasHeaderBinding {
				if err := runtime.storeLoopHeaderBinding(headerName, condition, loopEnv); err != nil {
					return signal{}, errorAt(stmt.Pos, err.Error())
				}
			}
			if !isTruthy(condition) {
				break
			}
		}
		first = false
		currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(loopEnv), true)
		if err != nil {
			return signal{}, err
		}
		if currentSignal.kind == signalBreak {
			break
		}
		if currentSignal.kind == signalContinue {
			continue
		}
		if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func isRuntimePatternMatchValue(value Value) bool {
	switch value.Kind {
	case ValueBool, ValueString, ValueInt, ValueFloat:
		return true
	default:
		return false
	}
}

func valuesEqual(left Value, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueBool:
		return left.Data.(bool) == right.Data.(bool)
	case ValueString:
		return left.Data.(string) == right.Data.(string)
	case ValueInt:
		return left.Data.(int) == right.Data.(int)
	case ValueFloat:
		return left.Data.(float64) == right.Data.(float64)
	default:
		return false
	}
}

func typeSizeof(typeName string) (int, bool) {
	switch typeName {
	case "Bool", "Char":
		return 1, true
	case "Int", "UInt", "Float", "Complex":
		return 8, true
	case "String", "List", "Map", "Table", "T", "Function", "Option", "Result", "SIMD", "Awaitable", "Iterator", "Coroutine":
		return 16, true
	default:
		return 0, false
	}
}

func (runtime *Runtime) storeLoopHeaderBinding(name string, value Value, env *Environment) error {
	if binding, ok := env.bindings[name]; ok {
		runtime.storeBindingValue(binding, value)
		return nil
	}
	return runtime.defineValue(env, name, true, runtimeTypeName(value), value)
}

func (runtime *Runtime) defineValue(env *Environment, name string, mutable bool, typeName string, value Value) error {
	return runtime.defineValueInRegion(env, name, mutable, typeName, value, MemoryStack)
}

func (runtime *Runtime) defineValueInRegion(env *Environment, name string, mutable bool, typeName string, value Value, region MemoryRegion) error {
	snapshot := cloneValue(value)
	return env.Define(name, mutable, typeName, snapshot, runtime.memory.Allocate(snapshot, region))
}

func (runtime *Runtime) storeBindingValue(binding *Binding, value Value) {
	snapshot := cloneValue(value)
	binding.Value = snapshot
	runtime.memory.Store(binding.ObjectID, snapshot)
}

func (runtime *Runtime) defineLocalFunction(fn parser.FunctionStatement, env *Environment) (string, error) {
	runtime.nextFunc++
	name := fmt.Sprintf("<local:%s:%d>", fn.Name, runtime.nextFunc)
	if _, exists := runtime.functions[name]; exists {
		return "", Error{Message: fmt.Sprintf("function %q is already defined", name)}
	}
	runtime.functions[name] = fn
	runtime.closures[name] = env
	return name, nil
}

func functionTypeName(fn parser.FunctionStatement) string {
	parts := make([]string, 0, len(fn.Params)+1)
	for _, param := range fn.Params {
		parts = append(parts, param.Type)
	}
	parts = append(parts, fn.ReturnType)
	return "Function[" + strings.Join(parts, ",") + "]"
}

func (runtime *Runtime) forceBindingValue(binding *Binding) (Value, error) {
	value, err := runtime.forceValue(binding.Value)
	if err != nil {
		return NullValue(), err
	}
	if binding.Value.Kind == ValueThunk {
		if !valueMatchesType(value, binding.Type) {
			return NullValue(), Error{Message: fmt.Sprintf("lazy value expects %s, got %s", binding.Type, value.Kind)}
		}
	}
	return value, nil
}

func (runtime *Runtime) forceValue(value Value) (Value, error) {
	if value.Kind != ValueThunk {
		return value, nil
	}
	thunk := value.Data.(*ThunkData)
	if thunk.Evaluated {
		return thunk.Value, nil
	}
	result, err := runtime.evalExpression(thunk.Expr, thunk.Env)
	if err != nil {
		return NullValue(), err
	}
	thunk.Value = cloneValue(result)
	thunk.Evaluated = true
	return thunk.Value, nil
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
		return Error{Message: "assignment target must be an lvalue"}
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
	if !valueMatchesType(next, binding.Type) {
		return Error{Message: fmt.Sprintf("cannot assign %s to %s variable %q", next.Kind, binding.Type, identifier.Name)}
	}
	runtime.storeBindingValue(binding, next)
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
		elementType, hasElementType := listElementType(binding.Type)
		if !hasElementType {
			elementType, hasElementType = arrayElementRuntimeType(binding.Type)
		}
		items := append([]Value(nil), binding.Value.Data.([]Value)...)
		position, err := asIndex(index)
		if err != nil {
			return err
		}
		if position < 0 {
			return Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
		}
		if capacity, ok := runtime.regionArrayCapacity(binding.Type); ok && position >= capacity {
			return Error{Message: fmt.Sprintf("array index %d exceeds region %s capacity %d", position, regionNameFromRuntimeArrayType(binding.Type), capacity)}
		}
		for len(items) <= position {
			items = append(items, NullValue())
		}
		current := items[position]
		next, err := applyAssignmentOperator(current, operator, value)
		if err != nil {
			return err
		}
		if hasElementType && !valueMatchesType(next, elementType) {
			return Error{Message: fmt.Sprintf("cannot assign %s to list element type %s", next.Kind, elementType)}
		}
		items[position] = next
		runtime.storeBindingValue(binding, Value{Kind: ValueList, Data: items})
	case ValueMap, ValueTable:
		keyType, valueType, hasMapTypes := mapTypes(binding.Type)
		items := make(map[string]Value, len(binding.Value.Data.(map[string]Value)))
		for existingKey, existingValue := range binding.Value.Data.(map[string]Value) {
			items[existingKey] = cloneValue(existingValue)
		}
		if binding.Value.Kind == ValueMap && hasMapTypes && !valueMatchesType(index, keyType) {
			return Error{Message: fmt.Sprintf("cannot use %s as map key type %s", index.Kind, keyType)}
		}
		key, err := mapKey(index)
		if err != nil {
			return err
		}
		current := items[key]
		next, err := applyAssignmentOperator(current, operator, value)
		if err != nil {
			return err
		}
		if binding.Value.Kind == ValueMap && hasMapTypes && !valueMatchesType(next, valueType) {
			return Error{Message: fmt.Sprintf("cannot assign %s to map value type %s", next.Kind, valueType)}
		}
		items[key] = next
		runtime.storeBindingValue(binding, Value{Kind: binding.Value.Kind, Data: items})
	default:
		return Error{Message: fmt.Sprintf("%s is not index-assignable", binding.Value.Kind)}
	}
	return nil
}

func (runtime *Runtime) evalExpression(expr parser.ExpressionNode, env *Environment) (Value, error) {
	switch current := expr.(type) {
	case nil:
		return NullValue(), nil
	case parser.IdentifierExpression:
		binding, ok := env.Get(current.Name)
		if ok {
			if binding.Moved {
				return NullValue(), Error{Message: fmt.Sprintf("variable %q was moved", current.Name)}
			}
			if err := runtime.memory.BorrowImmutable(binding.ObjectID); err != nil {
				return NullValue(), err
			}
			runtime.memory.ReleaseImmutable(binding.ObjectID)
			return runtime.forceBindingValue(binding)
		}
		if isBuiltinFunction(current.Name) {
			return FunctionValue(current.Name), nil
		}
		if target, ok := runtime.aliases[current.Name]; ok {
			return FunctionValue(target), nil
		}
		if _, ok := runtime.aliasFunctions[current.Name]; ok {
			return FunctionValue(current.Name), nil
		}
		if _, ok := runtime.groups[current.Name]; ok {
			return FunctionValue(current.Name), nil
		}
		name, err := runtime.resolveFunctionName(current.Name)
		if err != nil {
			return NullValue(), err
		}
		if name != "" {
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
		if target, ok := current.Target.(parser.IdentifierExpression); ok && current.Field == "sizeof" {
			if size, ok := typeSizeof(target.Name); ok {
				return IntValue(size), nil
			}
		}
		value, err := runtime.evalExpression(current.Target, env)
		if err == nil && value.Kind == ValueFunction {
			return FunctionValue(runtime.resolveAliasPath(value.Data.(string)) + "." + current.Field), nil
		}
		if err == nil && (value.Kind == ValueMap || value.Kind == ValueTable) {
			fields := value.Data.(map[string]Value)
			field, ok := fields[current.Field]
			if !ok {
				return NullValue(), Error{Message: fmt.Sprintf("unknown field %q", current.Field)}
			}
			return field, nil
		}
		if err == nil && value.Kind == ValueOption {
			option := value.Data.(OptionData)
			switch current.Field {
			case "value":
				return option.Value, nil
			case "some":
				return BoolValue(option.Some), nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown Option field %q", current.Field)}
		}
		if err == nil && value.Kind == ValueResult {
			result := value.Data.(ResultData)
			switch current.Field {
			case "value":
				return result.Value, nil
			case "ok":
				return BoolValue(result.Ok), nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown Result field %q", current.Field)}
		}
		if err == nil && value.Kind == ValueObject {
			object := value.Data.(ObjectData)
			if field, ok := object.Fields[current.Field]; ok {
				return field, nil
			}
			if runtime.aliasMethodExists(object.Type, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: object.Type, Name: current.Field, Receiver: value}}, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown field or method %q", current.Field)}
		}
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			return FunctionValue(runtime.resolveAliasPath(target.Name) + "." + current.Field), nil
		}
		return NullValue(), Error{Message: "unsupported selector target"}
	case parser.CastExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		return castValue(value, current.Type)
	case parser.NullCheckExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(value.Kind != ValueNull), nil
	case parser.PropagateExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		if value.Kind != ValueResult {
			return NullValue(), Error{Message: fmt.Sprintf("! expects Result, got %s", value.Kind)}
		}
		result := value.Data.(ResultData)
		if result.Ok {
			return result.Value, nil
		}
		return NullValue(), thrownError{Value: result.Value}
	case parser.ConditionalExpression:
		condition, err := runtime.evalExpression(current.Condition, env)
		if err != nil {
			return NullValue(), err
		}
		if isTruthy(condition) {
			return runtime.evalExpression(current.Consequence, env)
		}
		return runtime.evalExpression(current.Alternative, env)
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
	case parser.ListComprehensionExpression:
		return runtime.evalListComprehension(current, env)
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
	case parser.LambdaExpression:
		name, err := runtime.defineLocalFunction(parser.FunctionStatement{
			Name:       "lambda",
			Params:     current.Params,
			ReturnType: current.ReturnType,
			Body:       current.Body,
		}, env)
		if err != nil {
			return NullValue(), err
		}
		return FunctionValue(name), nil
	case parser.RawExpression:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %q", parser.Expression{Tokens: current.Tokens}.Literal())}
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %T", expr)}
	}
}

func (runtime *Runtime) evalListComprehension(expr parser.ListComprehensionExpression, env *Environment) (Value, error) {
	iterable, err := runtime.evalExpression(expr.Iterable, env)
	if err != nil {
		return NullValue(), err
	}

	values, err := iterableValues(iterable)
	if err != nil {
		return NullValue(), err
	}

	items := make([]Value, 0, len(values))
	for _, value := range values {
		itemEnv := NewEnvironment(env)
		if err := runtime.defineValue(itemEnv, expr.Iterator, false, runtimeTypeName(value), value); err != nil {
			return NullValue(), err
		}

		if expr.Condition != nil {
			condition, err := runtime.evalExpression(expr.Condition, itemEnv)
			if err != nil {
				return NullValue(), err
			}
			if !isTruthy(condition) {
				continue
			}
		}

		item, err := runtime.evalExpression(expr.Value, itemEnv)
		if err != nil {
			return NullValue(), err
		}
		items = append(items, item)
	}
	return Value{Kind: ValueList, Data: items}, nil
}

func iterableValues(value Value) ([]Value, error) {
	switch value.Kind {
	case ValueList:
		return value.Data.([]Value), nil
	case ValueIterator:
		iterator := value.Data.(*IteratorData)
		return iterator.Items, nil
	case ValueString:
		runes := []rune(value.Data.(string))
		values := make([]Value, 0, len(runes))
		for _, current := range runes {
			values = append(values, CharValue(string(current)))
		}
		return values, nil
	case ValueInt:
		count := value.Data.(int)
		if count < 0 {
			return nil, Error{Message: "list comprehension count cannot be negative"}
		}
		values := make([]Value, 0, count)
		for index := 0; index < count; index++ {
			values = append(values, IntValue(index))
		}
		return values, nil
	case ValueTable:
		fields := value.Data.(map[string]Value)
		values := make([]Value, 0, len(fields))
		for _, item := range fields {
			values = append(values, item)
		}
		return values, nil
	default:
		return nil, Error{Message: fmt.Sprintf("list comprehension cannot iterate over %s", value.Kind)}
	}
}

func (runtime *Runtime) evalUnary(expr parser.UnaryExpression, env *Environment) (Value, error) {
	if expr.Operator == "move" {
		return runtime.evalMove(expr.Right, env)
	}
	value, err := runtime.evalExpression(expr.Right, env)
	if err != nil {
		return NullValue(), err
	}
	switch expr.Operator {
	case "copy", "clone":
		return cloneValue(value), nil
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
	case "await":
		return runtime.awaitValue(value)
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported unary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) awaitValue(value Value) (Value, error) {
	if value.Kind != ValueAwaitable {
		return NullValue(), Error{Message: fmt.Sprintf("await expects Awaitable, got %s", value.Kind)}
	}
	data := value.Data.(*AwaitableData)
	if data.Done {
		return data.Value, nil
	}
	result, err := runtime.callFunctionMode(data.Function, data.Args, false)
	if err != nil {
		return NullValue(), err
	}
	data.Done = true
	data.Value = result
	return result, nil
}

func (runtime *Runtime) evalMove(expr parser.ExpressionNode, env *Environment) (Value, error) {
	identifier, ok := expr.(parser.IdentifierExpression)
	if !ok {
		return NullValue(), Error{Message: "move expects a variable"}
	}
	binding, ok := env.Get(identifier.Name)
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown variable %q", identifier.Name)}
	}
	if binding.Moved {
		return NullValue(), Error{Message: fmt.Sprintf("variable %q was moved", identifier.Name)}
	}
	value, err := runtime.forceBindingValue(binding)
	if err != nil {
		return NullValue(), err
	}
	binding.Moved = true
	binding.Value = NullValue()
	runtime.memory.Store(binding.ObjectID, NullValue())
	return value, nil
}

func (runtime *Runtime) evalBinary(expr parser.BinaryExpression, env *Environment) (Value, error) {
	left, err := runtime.evalExpression(expr.Left, env)
	if err != nil {
		return NullValue(), err
	}

	switch expr.Operator {
	case "|>":
		return runtime.evalPipe(left, expr.Right, env)
	case "and":
		if !isTruthy(left) {
			return BoolValue(false), nil
		}
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(right)), nil
	case "or":
		if isTruthy(left) {
			return BoolValue(true), nil
		}
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(right)), nil
	case "xor":
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(left) != isTruthy(right)), nil
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
		if left.Kind == ValueComplex || right.Kind == ValueComplex {
			return complexBinary(left, right, func(a, b float64) float64 { return a * b }, "*")
		}
		return numericBinary(left, right, func(a, b float64) float64 { return a * b })
	case "/":
		return divideValues(left, right)
	case "//":
		return floorDivideValues(left, right)
	case "%":
		return moduloValues(left, right)
	case "**":
		return exponentValues(left, right)
	case "==":
		return BoolValue(valueString(left) == valueString(right)), nil
	case "!=":
		return BoolValue(valueString(left) != valueString(right)), nil
	case ">":
		return compareOrdered(left, right, func(compare int) bool { return compare > 0 })
	case ">=":
		return compareOrdered(left, right, func(compare int) bool { return compare >= 0 })
	case "<":
		return compareOrdered(left, right, func(compare int) bool { return compare < 0 })
	case "<=":
		return compareOrdered(left, right, func(compare int) bool { return compare <= 0 })
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported binary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) evalPipe(value Value, target parser.ExpressionNode, env *Environment) (Value, error) {
	switch current := target.(type) {
	case parser.CallExpression:
		callee, err := runtime.evalExpression(current.Callee, env)
		if err != nil {
			return NullValue(), err
		}
		if callee.Kind != ValueFunction {
			return NullValue(), Error{Message: "pipe target is not a function"}
		}
		args := []Value{value}
		for _, arg := range current.Arguments {
			argValue, err := runtime.evalExpression(arg, env)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, argValue)
		}
		return runtime.callFunction(callee.Data.(string), args)
	case parser.UnaryExpression:
		if current.Operator == "call" {
			return runtime.evalPipe(value, current.Right, env)
		}
	case parser.IdentifierExpression, parser.SelectorExpression:
		callee, err := runtime.evalExpression(current, env)
		if err != nil {
			return NullValue(), err
		}
		if callee.Kind != ValueFunction {
			return NullValue(), Error{Message: "pipe target is not a function"}
		}
		return runtime.callFunction(callee.Data.(string), []Value{value})
	}
	return NullValue(), Error{Message: "pipe target must be a function or function call"}
}

func (runtime *Runtime) evalCall(expr parser.CallExpression, env *Environment) (Value, error) {
	callee, err := runtime.evalExpression(expr.Callee, env)
	if err != nil {
		return NullValue(), err
	}
	if callee.Kind != ValueFunction {
		if callee.Kind == ValueBoundMethod {
			return runtime.callBoundMethod(callee.Data.(BoundMethodData), expr.Arguments, env)
		}
		return NullValue(), Error{Message: "callee is not a function"}
	}

	args := make([]Value, 0, len(expr.Arguments))
	if runtime.isLazyFunction(callee.Data.(string)) {
		for _, arg := range expr.Arguments {
			args = append(args, ThunkValue(arg, env))
		}
	} else {
		for _, arg := range expr.Arguments {
			value, err := runtime.evalExpression(arg, env)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, value)
		}
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
	case ValueString:
		position, err := asIndex(index)
		if err != nil {
			return NullValue(), err
		}
		runes := []rune(target.Data.(string))
		if position < 0 || position >= len(runes) {
			return NullValue(), Error{Message: fmt.Sprintf("string index %d is out of bounds", position)}
		}
		return CharValue(string(runes[position])), nil
	case ValueList:
		items := target.Data.([]Value)
		position, err := asIndex(index)
		if err != nil {
			return NullValue(), err
		}
		if position < 0 || position >= len(items) {
			return NullValue(), Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
		}
		return items[position], nil
	case ValueMap, ValueTable:
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
	return runtime.callFunctionMode(name, args, true)
}

func (runtime *Runtime) callFunctionMode(name string, args []Value, wrapAsync bool) (Value, error) {
	name = runtime.resolveAliasPath(name)
	switch name {
	case "print":
		values := make([]string, 0, len(args))
		for _, arg := range args {
			value, err := runtime.forceValue(arg)
			if err != nil {
				return NullValue(), err
			}
			values = append(values, valueString(value))
		}
		runtime.output = append(runtime.output, strings.Join(values, " "))
		return NullValue(), nil
	case "input":
		if len(args) > 1 {
			return NullValue(), Error{Message: "input expects zero or one argument"}
		}
		if len(args) == 1 {
			value, err := runtime.forceValue(args[0])
			if err != nil {
				return NullValue(), err
			}
			runtime.output = append(runtime.output, valueString(value))
		}
		reader := bufio.NewReader(os.Stdin)
		text, err := reader.ReadString('\n')
		if err != nil && len(text) == 0 {
			return StringValue(""), nil
		}
		return StringValue(strings.TrimRight(text, "\r\n")), nil
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
	case "Some":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Some expects one argument"}
		}
		return OptionSomeValue(args[0]), nil
	case "None":
		if len(args) != 0 {
			return NullValue(), Error{Message: "None expects no arguments"}
		}
		return OptionNoneValue(), nil
	case "Ok":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Ok expects one argument"}
		}
		return ResultOkValue(args[0]), nil
	case "Err":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Err expects one argument"}
		}
		return ResultErrValue(args[0]), nil
	case "Result":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Result expects one argument"}
		}
		return ResultOkValue(args[0]), nil
	case "Complex":
		if len(args) != 2 {
			return NullValue(), Error{Message: "Complex expects two arguments"}
		}
		real, err := asFloat(args[0])
		if err != nil {
			return NullValue(), Error{Message: "Complex real component must be numeric"}
		}
		imag, err := asFloat(args[1])
		if err != nil {
			return NullValue(), Error{Message: "Complex imaginary component must be numeric"}
		}
		return ComplexValue(real, imag), nil
	case "SIMD":
		if len(args) != 1 {
			return NullValue(), Error{Message: "SIMD expects one list argument"}
		}
		if args[0].Kind != ValueList {
			return NullValue(), Error{Message: "SIMD expects a List argument"}
		}
		return SIMDValue(args[0].Data.([]Value)), nil
	case "Table":
		if len(args) > 1 {
			return NullValue(), Error{Message: "Table expects zero or one argument"}
		}
		if len(args) == 0 {
			return TableValue(map[string]Value{}), nil
		}
		if args[0].Kind == ValueTable {
			return args[0], nil
		}
		if args[0].Kind != ValueMap {
			return NullValue(), Error{Message: "Table expects a map literal or Table value"}
		}
		return TableValue(args[0].Data.(map[string]Value)), nil
	case "iter":
		if len(args) != 1 {
			return NullValue(), Error{Message: "iter expects one iterable value"}
		}
		values, err := iterableValues(args[0])
		if err != nil {
			return NullValue(), err
		}
		return IteratorValue(values), nil
	case "next":
		if len(args) != 1 {
			return NullValue(), Error{Message: "next expects one Iterator"}
		}
		if args[0].Kind != ValueIterator {
			return NullValue(), Error{Message: fmt.Sprintf("next expects Iterator, got %s", args[0].Kind)}
		}
		iterator := args[0].Data.(*IteratorData)
		if iterator.Index >= len(iterator.Items) {
			return OptionNoneValue(), nil
		}
		value := iterator.Items[iterator.Index]
		iterator.Index++
		return OptionSomeValue(value), nil
	case "coroutine":
		if len(args) != 1 {
			return NullValue(), Error{Message: "coroutine expects one function"}
		}
		if args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: fmt.Sprintf("coroutine expects Function, got %s", args[0].Kind)}
		}
		return CoroutineValue(args[0].Data.(string)), nil
	case "resume":
		if len(args) != 1 {
			return NullValue(), Error{Message: "resume expects one Coroutine"}
		}
		if args[0].Kind != ValueCoroutine {
			return NullValue(), Error{Message: fmt.Sprintf("resume expects Coroutine, got %s", args[0].Kind)}
		}
		coroutine := args[0].Data.(*CoroutineData)
		if coroutine.Done {
			return OptionNoneValue(), nil
		}
		value, err := runtime.callFunctionMode(coroutine.Function, nil, false)
		if err != nil {
			return NullValue(), err
		}
		coroutine.Done = true
		coroutine.Value = value
		return OptionSomeValue(value), nil
	case "Box", "Ref", "RefMut", "RefCell":
		if len(args) != 1 {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects one value", name)}
		}
		return allocatorObject(name, map[string]Value{"value": args[0]}), nil
	case "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		fields := map[string]Value{}
		if len(args) > 0 {
			fields["region"] = args[0]
		}
		return allocatorObject(name, fields), nil
	}

	if _, ok := runtime.aliasFunctions[name]; ok {
		return runtime.callAliasFunction(name, args)
	}

	resolvedName, err := runtime.resolveFunctionName(name)
	if err != nil {
		return NullValue(), err
	}
	if resolvedName == "" {
		if _, ok := runtime.groups[name]; ok {
			return runtime.callFunctionGroup(name, args)
		}
		return NullValue(), Error{Message: fmt.Sprintf("unknown function %q", name)}
	}
	if wrapAsync && runtime.functions[resolvedName].Async {
		return AwaitableValue(resolvedName, args), nil
	}
	if runtime.callDepth >= runtime.maxDepth {
		return NullValue(), Error{Message: fmt.Sprintf("maximum call depth %d exceeded while calling %s", runtime.maxDepth, name)}
	}

	runtime.callDepth++
	runtime.callStack = append(runtime.callStack, resolvedName)
	defer func() {
		runtime.callDepth--
		runtime.callStack = runtime.callStack[:len(runtime.callStack)-1]
	}()

	for {
		function := runtime.functions[resolvedName]
		required := requiredRuntimeParamCount(function.Params)
		if len(args) < required || len(args) > len(function.Params) {
			return NullValue(), Error{Message: fmt.Sprintf("function %s expects %d to %d argument(s), got %d", resolvedName, required, len(function.Params), len(args))}
		}

		parent := runtime.global
		if closureEnv, ok := runtime.closures[resolvedName]; ok {
			parent = closureEnv
		}
		env := NewEnvironment(parent)
		for index, param := range function.Params {
			var value Value
			if index < len(args) {
				value = args[index]
			} else {
				var err error
				if function.Lazy {
					value = ThunkValue(param.Default.Node, env)
				} else {
					value, err = runtime.evalExpression(param.Default.Node, env)
				}
				if err != nil {
					return NullValue(), err
				}
			}
			if !valueMatchesType(value, param.Type) {
				return NullValue(), Error{Message: fmt.Sprintf("function %s argument %q expects %s, got %s", resolvedName, param.Name, param.Type, value.Kind)}
			}
			if err := runtime.defineValue(env, param.Name, param.Mutable, param.Type, value); err != nil {
				return NullValue(), err
			}
		}
		runtime.innerSets = append(runtime.innerSets, map[string]Value{})
		currentSignal, err := runtime.executeBlock(function.Body, env, false)
		innerFields := runtime.innerSets[len(runtime.innerSets)-1]
		runtime.innerSets = runtime.innerSets[:len(runtime.innerSets)-1]
		if err != nil {
			return NullValue(), err
		}
		if currentSignal.kind == signalTailCall {
			args = currentSignal.tailArgs
			continue
		}
		if currentSignal.kind == signalThrow {
			return NullValue(), Error{Message: "uncaught exception: " + valueString(currentSignal.value)}
		}
		if currentSignal.kind == signalReturn {
			if !valueMatchesType(currentSignal.value, function.ReturnType) {
				return NullValue(), Error{Message: fmt.Sprintf("function %s returns %s, got %s", resolvedName, function.ReturnType, currentSignal.value.Kind)}
			}
			return currentSignal.value, nil
		}
		if function.ReturnType != "" && function.ReturnType != "T" {
			return NullValue(), Error{Message: fmt.Sprintf("function %s returns %s, got Null", resolvedName, function.ReturnType)}
		}
		if len(innerFields) != 0 {
			return Value{Kind: ValueMap, Data: innerFields}, nil
		}
		return NullValue(), nil
	}
}

func allocatorObject(kind string, fields map[string]Value) Value {
	copied := map[string]Value{"kind": StringValue(kind)}
	for key, value := range fields {
		copied[key] = value
	}
	return Value{Kind: ValueObject, Data: ObjectData{Type: kind, Fields: copied}}
}

func preferredMemoryRegion(value Value) MemoryRegion {
	if value.Kind != ValueObject {
		return ""
	}
	object := value.Data.(ObjectData)
	switch object.Type {
	case "Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return MemoryHeap
	default:
		return MemoryHeap
	}
}

func (runtime *Runtime) callAliasFunction(name string, args []Value) (Value, error) {
	alias, ok := runtime.aliasFunctions[name]
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown alias function %q", name)}
	}
	fields := map[string]Value{}
	required := requiredRuntimeParamCount(alias.Params)
	if len(args) < required || len(args) > len(alias.Params) {
		return NullValue(), Error{Message: fmt.Sprintf("alias function %s expects %d to %d argument(s), got %d", name, required, len(alias.Params), len(args))}
	}
	for index, param := range alias.Params {
		var value Value
		if index < len(args) {
			value = args[index]
		} else if isDefaultAllocator(param.Default) {
			value = allocatorObject("HeapAllocator", nil)
		} else if param.Default.Node != nil {
			evaluated, err := runtime.evalExpression(param.Default.Node, runtime.global)
			if err != nil {
				return NullValue(), err
			}
			value = evaluated
		} else {
			value = zeroValue(param.Type)
		}
		fields[param.Name] = value
	}
	fields["__type"] = StringValue(name)
	fields["__hooks"] = IntValue(len(alias.Hooks))
	return Value{Kind: ValueObject, Data: ObjectData{Type: name, Fields: fields}}, nil
}

func isDefaultAllocator(expr parser.Expression) bool {
	if len(expr.Tokens) != 2 {
		return false
	}
	return expr.Tokens[0].Type == lexer.TokenDot && expr.Tokens[1].Literal == "DEFAULT"
}

func (runtime *Runtime) aliasMethodExists(typeName string, methodName string) bool {
	alias, ok := runtime.aliasFunctions[typeName]
	if !ok {
		return false
	}
	for _, method := range alias.Methods {
		if method.Name == methodName {
			return true
		}
	}
	return false
}

func (runtime *Runtime) callBoundMethod(method BoundMethodData, argNodes []parser.ExpressionNode, env *Environment) (Value, error) {
	alias := runtime.aliasFunctions[method.Type]
	for _, fn := range alias.Methods {
		if fn.Name != method.Name {
			continue
		}
		required := requiredRuntimeParamCount(fn.Params)
		if len(argNodes) < required || len(argNodes) > len(fn.Params) {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects %d to %d argument(s), got %d", method.Type, method.Name, required, len(fn.Params), len(argNodes))}
		}
		args := make([]Value, 0, len(argNodes))
		for _, arg := range argNodes {
			value, err := runtime.evalExpression(arg, env)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, value)
		}
		methodEnv := NewEnvironment(env)
		if err := runtime.defineValue(methodEnv, "this", false, method.Type, method.Receiver); err != nil {
			return NullValue(), err
		}
		for index, param := range fn.Params {
			value := zeroValue(param.Type)
			if index < len(args) {
				value = args[index]
			} else if param.Default.Node != nil {
				evaluated, err := runtime.evalExpression(param.Default.Node, methodEnv)
				if err != nil {
					return NullValue(), err
				}
				value = evaluated
			}
			if !valueMatchesType(value, param.Type) {
				return NullValue(), Error{Message: fmt.Sprintf("method %s.%s argument %d expects %s, got %s", method.Type, method.Name, index+1, param.Type, runtimeTypeName(value))}
			}
			if err := runtime.defineValue(methodEnv, param.Name, param.Mutable, param.Type, value); err != nil {
				return NullValue(), err
			}
		}
		signal, err := runtime.executeBlock(fn.Body, methodEnv, false)
		if err != nil {
			return NullValue(), err
		}
		if signal.kind == signalReturn {
			return signal.value, nil
		}
		return NullValue(), nil
	}
	return NullValue(), Error{Message: fmt.Sprintf("unknown method %s.%s", method.Type, method.Name)}
}

func (runtime *Runtime) regionArrayCapacity(typeName string) (int, bool) {
	regionName := regionNameFromRuntimeArrayType(typeName)
	if regionName == "" {
		return 0, false
	}
	region, ok := runtime.regions[regionName]
	if !ok || region.Count.Kind != ValueInt {
		return 0, false
	}
	return region.Count.Data.(int), true
}

func (runtime *Runtime) callFunctionGroup(name string, args []Value) (Value, error) {
	name = runtime.resolveAliasPath(name)
	members, ok := runtime.groups[name]
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown function_group %q", name)}
	}

	var matches []string
	for _, member := range members {
		resolvedMember, err := runtime.resolveFunctionName(member)
		if err != nil {
			return NullValue(), err
		}
		if resolvedMember == "" {
			return NullValue(), Error{Message: fmt.Sprintf("function_group %s references unknown function %q", name, member)}
		}
		fn := runtime.functions[resolvedMember]
		required := requiredRuntimeParamCount(fn.Params)
		if len(args) < required || len(args) > len(fn.Params) {
			continue
		}
		if runtime.argumentsMatchParameters(args, fn.Params) {
			matches = append(matches, resolvedMember)
		}
	}

	if len(matches) == 0 {
		return NullValue(), Error{Message: fmt.Sprintf("no function_group %s overload matches %d argument(s)", name, len(args))}
	}
	if len(matches) > 1 {
		return NullValue(), Error{Message: fmt.Sprintf("ambiguous function_group %s call matches %s", name, strings.Join(matches, ", "))}
	}
	return runtime.callFunction(matches[0], args)
}

func (runtime *Runtime) argumentsMatchParameters(args []Value, params []parser.Parameter) bool {
	for index, arg := range args {
		if index >= len(params) || !valueMatchesType(arg, params[index].Type) {
			return false
		}
	}
	return true
}

func requiredRuntimeParamCount(params []parser.Parameter) int {
	count := len(params)
	for count > 0 && params[count-1].Default.Node != nil {
		count--
	}
	return count
}

func (runtime *Runtime) resolveFunctionName(name string) (string, error) {
	name = runtime.resolveAliasPath(name)
	if _, ok := runtime.functions[name]; ok {
		return name, nil
	}
	var matches []string
	for functionName := range runtime.functions {
		if strings.HasSuffix(functionName, "."+name) {
			matches = append(matches, functionName)
		}
	}
	if len(matches) > 1 {
		return "", Error{Message: fmt.Sprintf("ambiguous function %q matches %s", name, strings.Join(matches, ", "))}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}

func (runtime *Runtime) isLazyFunction(name string) bool {
	resolvedName, err := runtime.resolveFunctionName(name)
	if err != nil || resolvedName == "" {
		return false
	}
	return runtime.functions[resolvedName].Lazy
}

func (runtime *Runtime) resolveAliasPath(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "::", ".")
	for alias, target := range runtime.aliases {
		if name == alias {
			return target
		}
		if strings.HasPrefix(name, alias+".") {
			return target + strings.TrimPrefix(name, alias)
		}
	}
	return name
}

func isBuiltinFunction(name string) bool {
	switch name {
	case "print", "input", "len", "range", "Some", "None", "Ok", "Err", "Result", "Complex", "SIMD",
		"Table", "iter", "next", "coroutine", "resume",
		"Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return true
	default:
		return false
	}
}
