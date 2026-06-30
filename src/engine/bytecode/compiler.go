package bytecode

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"kLang/src/engine/ir"
)

type compiler struct {
	program       Program
	functionIndex map[string]int
	stringIndex   map[string]int
	diagnostics   []Diagnostic
}

type functionCompiler struct {
	owner     *compiler
	source    ir.Function
	function  Function
	scopes    []map[string]int
	nextLocal int
	loopStack []loopContext
	hasReturn bool
}

type loopContext struct {
	start     int
	breaks    []int
	continues []int
}

func Compile(source ir.Program) (Program, []Diagnostic) {
	current := &compiler{
		program:       Program{Version: Version},
		functionIndex: map[string]int{},
		stringIndex:   map[string]int{},
	}
	if len(source.Globals) != 0 {
		current.addDiagnostic(ir.Position{}, "global variables are not supported by the WASM bytecode subset", "Move state into Main or pass it through function parameters.")
	}
	for _, structure := range source.Structs {
		current.addDiagnostic(structure.Pos, fmt.Sprintf("struct alias %s is not supported by the WASM bytecode subset", structure.Name), "The browser interpreter fallback preserves full struct semantics.")
	}
	for _, extension := range source.Extensions {
		current.addDiagnostic(ir.Position{}, fmt.Sprintf("extension method %s.%s is not supported by the WASM bytecode subset", extension.Target, extension.Name), "The browser interpreter fallback preserves extension methods.")
	}
	for _, function := range source.Functions {
		if _, exists := current.functionIndex[function.Name]; exists {
			current.addDiagnostic(function.Pos, fmt.Sprintf("duplicate bytecode function %q", function.Name), "")
			continue
		}
		current.functionIndex[function.Name] = len(current.program.Functions)
		current.program.Functions = append(current.program.Functions, Function{Name: function.Name})
	}
	entry, ok := current.functionIndex[source.EntryPoint]
	if !ok {
		current.addDiagnostic(ir.Position{}, fmt.Sprintf("bytecode entry function %q was not found", source.EntryPoint), "Define Main() : Int or a valid custom entry point.")
	} else {
		current.program.Entry = entry
	}
	for _, function := range source.Functions {
		index, exists := current.functionIndex[function.Name]
		if !exists {
			continue
		}
		compiled := current.compileFunction(function)
		current.program.Functions[index] = compiled
	}
	return current.program, current.diagnostics
}

func (current *compiler) compileFunction(source ir.Function) Function {
	result := functionCompiler{
		owner:    current,
		source:   source,
		function: Function{Name: source.Name, Parameters: len(source.Params)},
		scopes:   []map[string]int{{}},
	}
	if !bytecodeTypeSupported(source.ReturnType) {
		current.addDiagnostic(source.Pos, fmt.Sprintf("function %s returns unsupported bytecode type %s", source.Name, source.ReturnType), "Use Int, UInt, Float, Bool, String, or T in the initial bytecode subset.")
	}
	for _, parameter := range source.Params {
		if !bytecodeTypeSupported(parameter.Type) {
			current.addDiagnostic(source.Pos, fmt.Sprintf("function %s parameter %s uses unsupported bytecode type %s", source.Name, parameter.Name, parameter.Type), "")
		}
		result.define(parameter.Name)
	}
	result.compileStatements(source.Body, false)
	if !result.hasReturn {
		result.emit(OpConstNull, 0, 0, source.Pos.Line)
		result.emit(OpReturn, 0, 0, source.Pos.Line)
	}
	result.function.Locals = result.nextLocal
	return result.function
}

func (current *functionCompiler) compileStatements(statements []ir.Statement, scoped bool) {
	if scoped {
		current.scopes = append(current.scopes, map[string]int{})
		defer func() { current.scopes = current.scopes[:len(current.scopes)-1] }()
	}
	for _, statement := range statements {
		current.compileStatement(statement)
	}
}

func (current *functionCompiler) compileStatement(statement ir.Statement) {
	line := statement.Pos.Line
	switch statement.Kind {
	case ir.StatementVariable:
		if !bytecodeTypeSupported(statement.Binding.Type) {
			current.owner.addDiagnostic(statement.Pos, fmt.Sprintf("variable %s uses unsupported bytecode type %s", statement.Binding.Name, statement.Binding.Type), "")
		}
		slot := current.define(statement.Binding.Name)
		if statement.Value.Kind == "" {
			current.emitZero(statement.Binding.Type, line)
		} else {
			current.compileExpression(statement.Value, statement.Pos)
		}
		current.emit(OpStoreLocal, int64(slot), 0, line)
	case ir.StatementAssignment:
		if statement.Target.Kind == ir.ExpressionIndex {
			current.compileIndexAssignment(statement)
			return
		}
		if statement.Target.Kind != ir.ExpressionIdentifier {
			current.owner.addDiagnostic(statement.Pos, "bytecode assignments require a local variable or indexed local List target", "")
			return
		}
		slot, ok := current.lookup(statement.Target.Name)
		if !ok {
			current.owner.addDiagnostic(statement.Pos, fmt.Sprintf("unknown bytecode local %q", statement.Target.Name), "")
			return
		}
		if statement.Operator != "=" {
			current.emit(OpLoadLocal, int64(slot), 0, line)
		}
		current.compileExpression(statement.Value, statement.Pos)
		if statement.Operator != "=" {
			current.emitBinary(strings.TrimSuffix(statement.Operator, "="), statement.Pos)
		}
		current.emit(OpStoreLocal, int64(slot), 0, line)
	case ir.StatementExpression:
		current.compileExpression(statement.Value, statement.Pos)
		current.emit(OpPop, 0, 0, line)
	case ir.StatementReturn:
		if statement.Value.Kind == "" {
			current.emit(OpConstNull, 0, 0, line)
		} else {
			current.compileExpression(statement.Value, statement.Pos)
		}
		current.emit(OpReturn, 0, 0, line)
		current.hasReturn = true
	case ir.StatementIf:
		current.compileExpression(statement.Condition, statement.Pos)
		falseJump := current.emit(OpJumpIfFalse, 0, 0, line)
		current.compileStatements(statement.Body, true)
		endJump := current.emit(OpJump, 0, 0, line)
		current.patch(falseJump, len(current.function.Code))
		current.compileStatements(statement.Else, true)
		current.patch(endJump, len(current.function.Code))
	case ir.StatementWhile:
		start := len(current.function.Code)
		current.compileExpression(statement.Condition, statement.Pos)
		exitJump := current.emit(OpJumpIfFalse, 0, 0, line)
		current.loopStack = append(current.loopStack, loopContext{start: start})
		current.compileStatements(statement.Body, true)
		loop := current.loopStack[len(current.loopStack)-1]
		current.emit(OpJump, int64(start), 0, line)
		end := len(current.function.Code)
		current.patch(exitJump, end)
		for _, jump := range loop.breaks {
			current.patch(jump, end)
		}
		for _, jump := range loop.continues {
			current.patch(jump, start)
		}
		current.loopStack = current.loopStack[:len(current.loopStack)-1]
	case ir.StatementRange:
		current.compileRange(statement)
	case ir.StatementForEach:
		current.compileForEach(statement)
	case ir.StatementBreak:
		if len(current.loopStack) == 0 {
			current.owner.addDiagnostic(statement.Pos, "break is outside a bytecode loop", "")
			return
		}
		jump := current.emit(OpJump, 0, 0, line)
		index := len(current.loopStack) - 1
		current.loopStack[index].breaks = append(current.loopStack[index].breaks, jump)
	case ir.StatementContinue:
		if len(current.loopStack) == 0 {
			current.owner.addDiagnostic(statement.Pos, "continue is outside a bytecode loop", "")
			return
		}
		jump := current.emit(OpJump, 0, 0, line)
		index := len(current.loopStack) - 1
		current.loopStack[index].continues = append(current.loopStack[index].continues, jump)
	case ir.StatementAssert:
		current.compileExpression(statement.Value, statement.Pos)
		current.emit(OpAssert, 0, 0, line)
	case ir.StatementBlock:
		current.compileStatements(statement.Body, true)
	default:
		current.owner.addDiagnostic(statement.Pos, fmt.Sprintf("statement %s is not supported by the WASM bytecode subset", statement.Kind), "")
	}
}

func (current *functionCompiler) compileExpression(expression ir.Expression, position ir.Position) {
	line := position.Line
	switch expression.Kind {
	case ir.ExpressionLiteral:
		switch expression.Type {
		case "Int", "UInt":
			value, err := strconv.ParseInt(strings.ReplaceAll(expression.Value, "_", ""), 0, 64)
			if err != nil {
				current.owner.addDiagnostic(position, fmt.Sprintf("invalid bytecode integer %q", expression.Value), "")
				value = 0
			}
			current.emit(OpConstInt, value, 0, line)
		case "Float":
			value, err := strconv.ParseFloat(strings.ReplaceAll(expression.Value, "_", ""), 64)
			if err != nil {
				current.owner.addDiagnostic(position, fmt.Sprintf("invalid bytecode float %q", expression.Value), "")
				value = 0
			}
			current.emit(OpConstFloat, int64(math.Float64bits(value)), 0, line)
		case "Bool":
			flag := int64(0)
			if expression.Value == "True" {
				flag = 1
			}
			current.emit(OpConstBool, flag, 0, line)
		case "String":
			current.emit(OpConstString, int64(current.owner.intern(expression.Value)), 0, line)
		default:
			current.owner.addDiagnostic(position, fmt.Sprintf("literal type %s is not supported by bytecode", expression.Type), "")
			current.emit(OpConstNull, 0, 0, line)
		}
	case ir.ExpressionIdentifier:
		slot, ok := current.lookup(expression.Name)
		if ok {
			current.emit(OpLoadLocal, int64(slot), 0, line)
			return
		}
		if function, exists := current.owner.functionIndex[expression.Name]; exists {
			current.emit(OpConstFunction, int64(function), 0, line)
			return
		}
		current.owner.addDiagnostic(position, fmt.Sprintf("unknown bytecode local or function %q", expression.Name), "")
		current.emit(OpConstNull, 0, 0, line)
	case ir.ExpressionUnary:
		current.compileExpression(*expression.Right, position)
		switch expression.Operator {
		case "-":
			current.emit(OpNegate, 0, 0, line)
		case "not":
			current.emit(OpNot, 0, 0, line)
		default:
			current.owner.addDiagnostic(position, fmt.Sprintf("unsupported bytecode unary operator %s", expression.Operator), "")
		}
	case ir.ExpressionBinary:
		if expression.Operator == "and" || expression.Operator == "or" {
			current.compileShortCircuit(expression, position)
			return
		}
		current.compileExpression(*expression.Left, position)
		current.compileExpression(*expression.Right, position)
		current.emitBinary(expression.Operator, position)
	case ir.ExpressionCall:
		if strings.HasPrefix(expression.Name, "__pipeline:") {
			methodName := strings.TrimPrefix(expression.Name, "__pipeline:")
			method, ok := bytecodePipelineMethod(methodName)
			if !ok {
				current.owner.addDiagnostic(position, fmt.Sprintf("pipeline method %s is not supported by bytecode", methodName), "Use collect, sort, fold, any, all, or for_each as the terminal operation.")
				current.emit(OpConstNull, 0, 0, line)
				return
			}
			for _, argument := range expression.Arguments {
				current.compileExpression(argument, position)
			}
			current.emit(OpPipeline, int64(method), int64(len(expression.Arguments)-1), line)
			return
		}
		if expression.Name == "__len" || expression.Name == "len" {
			if len(expression.Arguments) != 1 {
				current.owner.addDiagnostic(position, "len expects exactly one bytecode argument", "")
				current.emit(OpConstInt, 0, 0, line)
				return
			}
			current.compileLength(expression.Arguments[0], position)
			return
		}
		if expression.Name == "print" {
			for _, argument := range expression.Arguments {
				current.compileExpression(argument, position)
			}
			current.emit(OpPrint, int64(len(expression.Arguments)), 0, line)
			return
		}
		target, ok := current.owner.functionIndex[expression.Name]
		if !ok {
			current.owner.addDiagnostic(position, fmt.Sprintf("bytecode cannot call unknown or builtin function %q", expression.Name), "The initial bytecode VM supports direct Klang calls and print.")
			current.emit(OpConstNull, 0, 0, line)
			return
		}
		for _, argument := range expression.Arguments {
			current.compileExpression(argument, position)
		}
		current.emit(OpCall, int64(target), int64(len(expression.Arguments)), line)
	case ir.ExpressionConditional:
		current.compileExpression(*expression.Condition, position)
		falseJump := current.emit(OpJumpIfFalse, 0, 0, line)
		current.compileExpression(*expression.Consequence, position)
		endJump := current.emit(OpJump, 0, 0, line)
		current.patch(falseJump, len(current.function.Code))
		current.compileExpression(*expression.Alternative, position)
		current.patch(endJump, len(current.function.Code))
	case ir.ExpressionCast:
		current.compileExpression(*expression.Left, position)
	case ir.ExpressionList:
		for _, item := range expression.Arguments {
			current.compileExpression(item, position)
		}
		current.emit(OpMakeList, int64(len(expression.Arguments)), 0, line)
	case ir.ExpressionIndex:
		current.compileIndex(expression, position)
	case ir.ExpressionSelector:
		if expression.Name != "count" {
			current.owner.addDiagnostic(position, fmt.Sprintf("selector .%s is not supported by the WASM bytecode subset", expression.Name), "")
			current.emit(OpConstNull, 0, 0, line)
			return
		}
		current.compileLength(*expression.Left, position)
	default:
		current.owner.addDiagnostic(position, fmt.Sprintf("expression %s is not supported by the WASM bytecode subset", expression.Kind), "")
		current.emit(OpConstNull, 0, 0, line)
	}
}

func (current *functionCompiler) compileShortCircuit(expression ir.Expression, position ir.Position) {
	line := position.Line
	current.compileExpression(*expression.Left, position)
	var branch int
	if expression.Operator == "and" {
		branch = current.emit(OpJumpIfFalse, 0, 0, line)
	} else {
		branch = current.emit(OpJumpIfTrue, 0, 0, line)
	}
	current.compileExpression(*expression.Right, position)
	end := current.emit(OpJump, 0, 0, line)
	current.patch(branch, len(current.function.Code))
	flag := int64(0)
	if expression.Operator == "or" {
		flag = 1
	}
	current.emit(OpConstBool, flag, 0, line)
	current.patch(end, len(current.function.Code))
}

func (current *functionCompiler) emitBinary(operator string, position ir.Position) {
	opcodes := map[string]Opcode{
		"+": OpAdd, "-": OpSubtract, "*": OpMultiply, "/": OpDivide, "//": OpFloorDivide,
		"%": OpModulo, "**": OpPower, "==": OpEqual, "!=": OpNotEqual, ">": OpGreater,
		">=": OpGreaterEqual, "<": OpLess, "<=": OpLessEqual,
	}
	opcode, ok := opcodes[operator]
	if !ok {
		current.owner.addDiagnostic(position, fmt.Sprintf("unsupported bytecode binary operator %s", operator), "")
		return
	}
	current.emit(opcode, 0, 0, position.Line)
}

func (current *functionCompiler) emitZero(typeName string, line int) {
	switch typeName {
	case "Int", "UInt":
		current.emit(OpConstInt, 0, 0, line)
	case "Float":
		current.emit(OpConstFloat, int64(math.Float64bits(0)), 0, line)
	case "Bool":
		current.emit(OpConstBool, 0, 0, line)
	case "String":
		current.emit(OpConstString, int64(current.owner.intern("")), 0, line)
	default:
		if isBytecodeListType(typeName) {
			current.emit(OpMakeList, 0, 0, line)
			return
		}
		current.emit(OpConstNull, 0, 0, line)
	}
}

func (current *functionCompiler) compileIndexAssignment(statement ir.Statement) {
	target := statement.Target
	if target.Left == nil || target.Right == nil || target.Left.Kind != ir.ExpressionIdentifier {
		current.owner.addDiagnostic(statement.Pos, "indexed bytecode mutation requires a local List binding", "")
		return
	}
	slot, ok := current.lookup(target.Left.Name)
	if !ok {
		current.owner.addDiagnostic(statement.Pos, fmt.Sprintf("unknown bytecode local %q", target.Left.Name), "")
		return
	}
	current.compileExpression(*target.Right, statement.Pos)
	current.compileExpression(statement.Value, statement.Pos)
	operator := int64(-1)
	if statement.Operator != "=" {
		opcode, exists := bytecodeBinaryOpcode(strings.TrimSuffix(statement.Operator, "="))
		if !exists {
			current.owner.addDiagnostic(statement.Pos, fmt.Sprintf("unsupported indexed assignment operator %s", statement.Operator), "")
			return
		}
		operator = int64(opcode)
	}
	current.emit(OpStoreIndexLocal, int64(slot), operator, statement.Pos.Line)
}

func (current *functionCompiler) compileRange(statement ir.Statement) {
	line := statement.Pos.Line
	current.scopes = append(current.scopes, map[string]int{})
	defer func() { current.scopes = current.scopes[:len(current.scopes)-1] }()
	iterator := current.define(statement.Binding.Name)
	limit := current.defineHidden()
	current.compileExpression(statement.Value, statement.Pos)
	current.emit(OpValidateRange, 0, 0, line)
	current.emit(OpStoreLocal, int64(limit), 0, line)
	current.emit(OpConstInt, 0, 0, line)
	current.emit(OpStoreLocal, int64(iterator), 0, line)
	start := len(current.function.Code)
	current.emit(OpLoadLocal, int64(iterator), 0, line)
	current.emit(OpLoadLocal, int64(limit), 0, line)
	current.emit(OpLess, 0, 0, line)
	exitJump := current.emit(OpJumpIfFalse, 0, 0, line)
	current.loopStack = append(current.loopStack, loopContext{start: start})
	current.compileStatements(statement.Body, true)
	continueTarget := len(current.function.Code)
	current.emitIncrement(iterator, line)
	current.emit(OpJump, int64(start), 0, line)
	current.finishLoop(exitJump, continueTarget)
}

func (current *functionCompiler) compileForEach(statement ir.Statement) {
	line := statement.Pos.Line
	current.scopes = append(current.scopes, map[string]int{})
	defer func() { current.scopes = current.scopes[:len(current.scopes)-1] }()
	item := current.define(statement.Binding.Name)
	iterable := current.defineHidden()
	index := current.defineHidden()
	current.compileExpression(statement.Value, statement.Pos)
	current.emit(OpStoreLocal, int64(iterable), 0, line)
	current.emit(OpConstInt, 0, 0, line)
	current.emit(OpStoreLocal, int64(index), 0, line)
	start := len(current.function.Code)
	current.emit(OpLoadLocal, int64(index), 0, line)
	current.emit(OpLengthLocal, int64(iterable), 0, line)
	current.emit(OpLess, 0, 0, line)
	exitJump := current.emit(OpJumpIfFalse, 0, 0, line)
	current.emit(OpLoadLocal, int64(index), 0, line)
	current.emit(OpIndexLocal, int64(iterable), 0, line)
	current.emit(OpStoreLocal, int64(item), 0, line)
	current.loopStack = append(current.loopStack, loopContext{start: start})
	current.compileStatements(statement.Body, true)
	continueTarget := len(current.function.Code)
	current.emitIncrement(index, line)
	current.emit(OpJump, int64(start), 0, line)
	current.finishLoop(exitJump, continueTarget)
}

func (current *functionCompiler) emitIncrement(slot int, line int) {
	current.emit(OpLoadLocal, int64(slot), 0, line)
	current.emit(OpConstInt, 1, 0, line)
	current.emit(OpAdd, 0, 0, line)
	current.emit(OpStoreLocal, int64(slot), 0, line)
}

func (current *functionCompiler) finishLoop(exitJump int, continueTarget int) {
	loop := current.loopStack[len(current.loopStack)-1]
	end := len(current.function.Code)
	current.patch(exitJump, end)
	for _, jump := range loop.breaks {
		current.patch(jump, end)
	}
	for _, jump := range loop.continues {
		current.patch(jump, continueTarget)
	}
	current.loopStack = current.loopStack[:len(current.loopStack)-1]
}

func (current *functionCompiler) defineHidden() int {
	slot := current.nextLocal
	current.nextLocal++
	return slot
}

func (current *functionCompiler) compileLength(expression ir.Expression, position ir.Position) {
	if expression.Kind == ir.ExpressionIdentifier {
		if slot, ok := current.lookup(expression.Name); ok {
			current.emit(OpLengthLocal, int64(slot), 0, position.Line)
			return
		}
	}
	current.compileExpression(expression, position)
	current.emit(OpLength, 0, 0, position.Line)
}

func (current *functionCompiler) compileIndex(expression ir.Expression, position ir.Position) {
	if expression.Left != nil && expression.Left.Kind == ir.ExpressionIdentifier {
		if slot, ok := current.lookup(expression.Left.Name); ok {
			current.compileExpression(*expression.Right, position)
			current.emit(OpIndexLocal, int64(slot), 0, position.Line)
			return
		}
	}
	current.compileExpression(*expression.Left, position)
	current.compileExpression(*expression.Right, position)
	current.emit(OpIndex, 0, 0, position.Line)
}

func bytecodeBinaryOpcode(operator string) (Opcode, bool) {
	opcodes := map[string]Opcode{
		"+": OpAdd, "-": OpSubtract, "*": OpMultiply, "/": OpDivide, "//": OpFloorDivide,
		"%": OpModulo, "**": OpPower, "==": OpEqual, "!=": OpNotEqual, ">": OpGreater,
		">=": OpGreaterEqual, "<": OpLess, "<=": OpLessEqual,
	}
	opcode, ok := opcodes[operator]
	return opcode, ok
}

func isBytecodeListType(typeName string) bool {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "List[") || !strings.HasSuffix(typeName, "]") {
		return false
	}
	return bytecodeTypeSupported(typeName[len("List[") : len(typeName)-1])
}

func bytecodeTypeSupported(typeName string) bool {
	typeName = strings.TrimSpace(typeName)
	if isBytecodeListType(typeName) {
		return true
	}
	if strings.HasPrefix(typeName, "Iterator[") && strings.HasSuffix(typeName, "]") {
		return bytecodeTypeSupported(typeName[len("Iterator[") : len(typeName)-1])
	}
	if strings.HasPrefix(typeName, "Function[") && strings.HasSuffix(typeName, "]") {
		return true
	}
	switch typeName {
	case "", "T", "Int", "UInt", "Float", "Bool", "String":
		return true
	default:
		return false
	}
}

func bytecodePipelineMethod(name string) (PipelineMethod, bool) {
	methods := map[string]PipelineMethod{
		"iter": PipelineIter, "filter": PipelineFilter, "map": PipelineMap,
		"skip": PipelineSkip, "limit": PipelineTake, "collect": PipelineCollect,
		"sort": PipelineSort, "fold": PipelineFold, "any": PipelineAny,
		"all": PipelineAll, "for_each": PipelineForEach,
	}
	method, ok := methods[name]
	return method, ok
}

func (current *functionCompiler) define(name string) int {
	slot := current.nextLocal
	current.nextLocal++
	current.scopes[len(current.scopes)-1][name] = slot
	return slot
}

func (current *functionCompiler) lookup(name string) (int, bool) {
	for index := len(current.scopes) - 1; index >= 0; index-- {
		if slot, ok := current.scopes[index][name]; ok {
			return slot, true
		}
	}
	return 0, false
}

func (current *functionCompiler) emit(opcode Opcode, a int64, b int64, line int) int {
	index := len(current.function.Code)
	current.function.Code = append(current.function.Code, Instruction{Opcode: opcode, A: a, B: b, Line: line})
	return index
}

func (current *functionCompiler) patch(index int, target int) {
	current.function.Code[index].A = int64(target)
}

func (current *compiler) intern(value string) int {
	if index, ok := current.stringIndex[value]; ok {
		return index
	}
	index := len(current.program.Strings)
	current.stringIndex[value] = index
	current.program.Strings = append(current.program.Strings, value)
	return index
}

func (current *compiler) addDiagnostic(position ir.Position, message string, hint string) {
	current.diagnostics = append(current.diagnostics, Diagnostic{
		File: position.File, Line: position.Line, Column: position.Column, Message: message, Hint: hint,
	})
}
