package bytecode

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

type ValueKind byte

const (
	ValueNull ValueKind = iota
	ValueInt
	ValueFloat
	ValueBool
	ValueString
	ValueList
	ValueFunction
	ValueIterator
)

type Value struct {
	Kind   ValueKind
	Int    int64
	Float  float64
	Bool   bool
	String string
	List   []Value
	Index  int
	Iter   *IteratorData
}

type IteratorData struct {
	Items     []Value
	Position  int
	Stages    []PipelineStage
	Exhausted bool
}

type PipelineStage struct {
	Method   PipelineMethod
	Function int
	Count    int64
	Seen     int64
}

func NullValue() Value           { return Value{Kind: ValueNull} }
func IntValue(value int64) Value { return Value{Kind: ValueInt, Int: value} }
func FloatValue(value float64) Value {
	return Value{Kind: ValueFloat, Float: value}
}
func BoolValue(value bool) Value     { return Value{Kind: ValueBool, Bool: value} }
func StringValue(value string) Value { return Value{Kind: ValueString, String: value} }
func ListValue(value []Value) Value {
	items := make([]Value, len(value))
	for index, item := range value {
		items[index] = cloneValue(item)
	}
	return Value{Kind: ValueList, List: items}
}
func FunctionValue(index int) Value { return Value{Kind: ValueFunction, Index: index} }
func IteratorValue(data *IteratorData) Value {
	return Value{Kind: ValueIterator, Iter: data}
}

func (value Value) Interface() any {
	switch value.Kind {
	case ValueNull:
		return nil
	case ValueInt:
		return value.Int
	case ValueFloat:
		return value.Float
	case ValueBool:
		return value.Bool
	case ValueString:
		return value.String
	case ValueList:
		items := make([]any, len(value.List))
		for index, item := range value.List {
			items[index] = item.Interface()
		}
		return items
	case ValueFunction:
		return value.Index
	case ValueIterator:
		return map[string]any{"index": value.Iter.Position, "items": len(value.Iter.Items), "stages": len(value.Iter.Stages)}
	default:
		return nil
	}
}

func (value Value) StringValue() string {
	switch value.Kind {
	case ValueNull:
		return "Null"
	case ValueInt:
		return strconv.FormatInt(value.Int, 10)
	case ValueFloat:
		return strconv.FormatFloat(value.Float, 'g', -1, 64)
	case ValueBool:
		if value.Bool {
			return "True"
		}
		return "False"
	case ValueString:
		return value.String
	case ValueList:
		items := make([]string, len(value.List))
		for index, item := range value.List {
			items[index] = item.StringValue()
		}
		return "[" + strings.Join(items, ", ") + "]"
	case ValueFunction:
		return fmt.Sprintf("Function(%d)", value.Index)
	case ValueIterator:
		return fmt.Sprintf("Iterator(%d/%d, stages=%d)", value.Iter.Position, len(value.Iter.Items), len(value.Iter.Stages))
	default:
		return "<invalid>"
	}
}

type Result struct {
	Value        Value
	Output       []string
	Instructions uint64
}

type RuntimeError struct {
	Function string
	Line     int
	IP       int
	Message  string
}

func (err RuntimeError) Error() string {
	location := err.Function
	if err.Line > 0 {
		location += fmt.Sprintf(":%d", err.Line)
	}
	return fmt.Sprintf("bytecode runtime error at %s [ip=%d]: %s", location, err.IP, err.Message)
}

type VM struct {
	InstructionLimit uint64
	CallDepthLimit   int
	output           []string
	instructions     uint64
	callDepth        int
}

func NewVM() *VM {
	return &VM{InstructionLimit: 10_000_000, CallDepthLimit: 1024}
}

func (vm *VM) Execute(program Program) (Result, error) {
	if program.Version != Version {
		return Result{}, fmt.Errorf("unsupported bytecode version %d", program.Version)
	}
	if program.Entry < 0 || program.Entry >= len(program.Functions) {
		return Result{}, fmt.Errorf("entry function index %d is out of range", program.Entry)
	}
	vm.output = nil
	vm.instructions = 0
	vm.callDepth = 0
	value, err := vm.call(program, program.Entry, nil)
	if err != nil {
		return Result{}, err
	}
	return Result{Value: value, Output: append([]string(nil), vm.output...), Instructions: vm.instructions}, nil
}

func (vm *VM) call(program Program, functionIndex int, args []Value) (Value, error) {
	if functionIndex < 0 || functionIndex >= len(program.Functions) {
		return NullValue(), fmt.Errorf("function index %d is out of range", functionIndex)
	}
	function := program.Functions[functionIndex]
	if vm.CallDepthLimit != 0 && vm.callDepth >= vm.CallDepthLimit {
		return NullValue(), RuntimeError{Function: function.Name, IP: 0, Message: "call depth limit exceeded"}
	}
	vm.callDepth++
	defer func() { vm.callDepth-- }()
	if len(args) != function.Parameters {
		return NullValue(), RuntimeError{Function: function.Name, IP: 0, Message: fmt.Sprintf("expects %d argument(s), got %d", function.Parameters, len(args))}
	}
	if function.Locals < function.Parameters {
		return NullValue(), RuntimeError{Function: function.Name, IP: 0, Message: "local count is smaller than parameter count"}
	}
	locals := make([]Value, function.Locals)
	for index, arg := range args {
		locals[index] = cloneValue(arg)
	}
	stack := make([]Value, 0, 16)
	pop := func(ip int) (Value, error) {
		if len(stack) == 0 {
			return NullValue(), RuntimeError{Function: function.Name, IP: ip, Message: "stack underflow"}
		}
		value := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		return value, nil
	}
	for ip := 0; ip < len(function.Code); ip++ {
		instruction := function.Code[ip]
		vm.instructions++
		if vm.InstructionLimit != 0 && vm.instructions > vm.InstructionLimit {
			return NullValue(), RuntimeError{Function: function.Name, Line: instruction.Line, IP: ip, Message: "instruction limit exceeded"}
		}
		fail := func(message string) (Value, error) {
			return NullValue(), RuntimeError{Function: function.Name, Line: instruction.Line, IP: ip, Message: message}
		}
		switch instruction.Opcode {
		case OpConstNull:
			stack = append(stack, NullValue())
		case OpConstInt:
			stack = append(stack, IntValue(instruction.A))
		case OpConstFloat:
			stack = append(stack, FloatValue(math.Float64frombits(uint64(instruction.A))))
		case OpConstBool:
			stack = append(stack, BoolValue(instruction.A != 0))
		case OpConstString:
			if instruction.A < 0 || int(instruction.A) >= len(program.Strings) {
				return fail(fmt.Sprintf("string index %d is out of range", instruction.A))
			}
			stack = append(stack, StringValue(program.Strings[instruction.A]))
		case OpConstFunction:
			if instruction.A < 0 || int(instruction.A) >= len(program.Functions) {
				return fail(fmt.Sprintf("function index %d is out of range", instruction.A))
			}
			stack = append(stack, FunctionValue(int(instruction.A)))
		case OpLoadLocal:
			if instruction.A < 0 || int(instruction.A) >= len(locals) {
				return fail(fmt.Sprintf("local index %d is out of range", instruction.A))
			}
			stack = append(stack, cloneValue(locals[instruction.A]))
		case OpStoreLocal:
			if instruction.A < 0 || int(instruction.A) >= len(locals) {
				return fail(fmt.Sprintf("local index %d is out of range", instruction.A))
			}
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			locals[instruction.A] = cloneValue(value)
		case OpPop:
			if _, err := pop(ip); err != nil {
				return NullValue(), err
			}
		case OpNegate:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			switch value.Kind {
			case ValueInt:
				stack = append(stack, IntValue(-value.Int))
			case ValueFloat:
				stack = append(stack, FloatValue(-value.Float))
			default:
				return fail("negation expects Int or Float")
			}
		case OpNot:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			stack = append(stack, BoolValue(!truthy(value)))
		case OpAdd, OpSubtract, OpMultiply, OpDivide, OpFloorDivide, OpModulo, OpPower,
			OpEqual, OpNotEqual, OpGreater, OpGreaterEqual, OpLess, OpLessEqual:
			right, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			left, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			value, operationErr := applyBinary(instruction.Opcode, left, right)
			if operationErr != nil {
				return fail(operationErr.Error())
			}
			stack = append(stack, value)
		case OpJump:
			if instruction.A < 0 || int(instruction.A) > len(function.Code) {
				return fail(fmt.Sprintf("jump target %d is out of range", instruction.A))
			}
			ip = int(instruction.A) - 1
		case OpJumpIfFalse, OpJumpIfTrue:
			condition, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			jump := instruction.Opcode == OpJumpIfFalse && !truthy(condition) ||
				instruction.Opcode == OpJumpIfTrue && truthy(condition)
			if jump {
				if instruction.A < 0 || int(instruction.A) > len(function.Code) {
					return fail(fmt.Sprintf("jump target %d is out of range", instruction.A))
				}
				ip = int(instruction.A) - 1
			}
		case OpCall:
			count := int(instruction.B)
			if count < 0 || count > len(stack) {
				return fail(fmt.Sprintf("call argument count %d exceeds stack", count))
			}
			callArgs := make([]Value, count)
			for index, value := range stack[len(stack)-count:] {
				callArgs[index] = cloneValue(value)
			}
			stack = stack[:len(stack)-count]
			value, err := vm.call(program, int(instruction.A), callArgs)
			if err != nil {
				return NullValue(), err
			}
			stack = append(stack, value)
		case OpPrint:
			count := int(instruction.A)
			if count < 0 || count > len(stack) {
				return fail(fmt.Sprintf("print argument count %d exceeds stack", count))
			}
			values := stack[len(stack)-count:]
			stack = stack[:len(stack)-count]
			parts := make([]string, 0, len(values))
			for _, value := range values {
				parts = append(parts, value.StringValue())
			}
			vm.output = append(vm.output, strings.Join(parts, " "))
			stack = append(stack, NullValue())
		case OpAssert:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			if !truthy(value) {
				return fail("assertion failed")
			}
		case OpReturn:
			if len(stack) == 0 {
				return NullValue(), nil
			}
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			return cloneValue(value), nil
		case OpMakeList:
			count := int(instruction.A)
			if count < 0 || count > len(stack) {
				return fail(fmt.Sprintf("list item count %d exceeds stack", count))
			}
			items := make([]Value, count)
			for index, value := range stack[len(stack)-count:] {
				items[index] = cloneValue(value)
			}
			stack = stack[:len(stack)-count]
			stack = append(stack, ListValue(items))
		case OpIndex:
			indexValue, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			target, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			value, indexErr := indexValueAt(target, indexValue)
			if indexErr != nil {
				return fail(indexErr.Error())
			}
			stack = append(stack, value)
		case OpStoreIndexLocal:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			indexValue, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			if instruction.A < 0 || int(instruction.A) >= len(locals) {
				return fail(fmt.Sprintf("local index %d is out of range", instruction.A))
			}
			if indexValue.Kind != ValueInt {
				return fail("List mutation index must be Int")
			}
			index := int(indexValue.Int)
			if index < 0 {
				return fail(fmt.Sprintf("List index %d is out of bounds", index))
			}
			target := locals[instruction.A]
			if target.Kind != ValueList {
				return fail("indexed mutation expects List")
			}
			if instruction.B >= 0 {
				if index >= len(target.List) {
					return fail(fmt.Sprintf("compound assignment requires existing List index %d", index))
				}
				value, err = applyBinary(Opcode(instruction.B), target.List[index], value)
				if err != nil {
					return fail(err.Error())
				}
			}
			for len(target.List) <= index {
				target.List = append(target.List, NullValue())
			}
			target.List[index] = cloneValue(value)
			locals[instruction.A] = target
		case OpLength:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			switch value.Kind {
			case ValueList:
				stack = append(stack, IntValue(int64(len(value.List))))
			case ValueString:
				stack = append(stack, IntValue(int64(len([]rune(value.String)))))
			case ValueInt:
				if value.Int < 0 {
					return fail("iteration count cannot be negative")
				}
				stack = append(stack, value)
			case ValueIterator:
				iterator := cloneValue(value)
				items, collectErr := vm.collectPipeline(program, iterator.Iter)
				if collectErr != nil {
					return fail(collectErr.Error())
				}
				stack = append(stack, IntValue(int64(len(items))))
			default:
				return fail("length expects List, String, or iterable Int")
			}
		case OpValidateRange:
			value, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			if value.Kind != ValueInt {
				return fail("range expects an Int count")
			}
			if value.Int < 0 {
				return fail("range count cannot be negative")
			}
			stack = append(stack, value)
		case OpIndexLocal:
			if instruction.A < 0 || int(instruction.A) >= len(locals) {
				return fail(fmt.Sprintf("local index %d is out of range", instruction.A))
			}
			indexValue, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			value, indexErr := indexValueAt(locals[instruction.A], indexValue)
			if indexErr != nil {
				return fail(indexErr.Error())
			}
			stack = append(stack, value)
		case OpLengthLocal:
			if instruction.A < 0 || int(instruction.A) >= len(locals) {
				return fail(fmt.Sprintf("local index %d is out of range", instruction.A))
			}
			switch value := locals[instruction.A]; value.Kind {
			case ValueList:
				stack = append(stack, IntValue(int64(len(value.List))))
			case ValueString:
				stack = append(stack, IntValue(int64(len([]rune(value.String)))))
			case ValueInt:
				if value.Int < 0 {
					return fail("iteration count cannot be negative")
				}
				stack = append(stack, value)
			case ValueIterator:
				iterator := cloneValue(value)
				items, collectErr := vm.collectPipeline(program, iterator.Iter)
				if collectErr != nil {
					return fail(collectErr.Error())
				}
				stack = append(stack, IntValue(int64(len(items))))
			default:
				return fail("length expects List, String, or iterable Int")
			}
		case OpPipeline:
			count := int(instruction.B)
			if count < 0 || count+1 > len(stack) {
				return fail(fmt.Sprintf("pipeline argument count %d exceeds stack", count))
			}
			arguments := make([]Value, count)
			for index, value := range stack[len(stack)-count:] {
				arguments[index] = cloneValue(value)
			}
			stack = stack[:len(stack)-count]
			receiver, err := pop(ip)
			if err != nil {
				return NullValue(), err
			}
			result, pipelineErr := vm.applyPipeline(program, receiver, PipelineMethod(instruction.A), arguments)
			if pipelineErr != nil {
				return fail(pipelineErr.Error())
			}
			stack = append(stack, result)
		default:
			return fail(fmt.Sprintf("unknown opcode %d", instruction.Opcode))
		}
	}
	return NullValue(), nil
}

func applyBinary(opcode Opcode, left Value, right Value) (Value, error) {
	switch opcode {
	case OpEqual, OpNotEqual:
		equal := valuesEqual(left, right)
		if opcode == OpNotEqual {
			equal = !equal
		}
		return BoolValue(equal), nil
	case OpAdd:
		if left.Kind == ValueString || right.Kind == ValueString {
			return StringValue(left.StringValue() + right.StringValue()), nil
		}
	}
	leftNumber, leftFloat, ok := number(left)
	if !ok {
		return NullValue(), fmt.Errorf("%s expects numeric operands", opcode)
	}
	rightNumber, rightFloat, ok := number(right)
	if !ok {
		return NullValue(), fmt.Errorf("%s expects numeric operands", opcode)
	}
	switch opcode {
	case OpAdd:
		return numericResult(leftNumber+rightNumber, leftFloat || rightFloat), nil
	case OpSubtract:
		return numericResult(leftNumber-rightNumber, leftFloat || rightFloat), nil
	case OpMultiply:
		return numericResult(leftNumber*rightNumber, leftFloat || rightFloat), nil
	case OpDivide:
		if rightNumber == 0 {
			return NullValue(), fmt.Errorf("division by zero")
		}
		return numericResult(leftNumber/rightNumber, leftFloat || rightFloat), nil
	case OpFloorDivide:
		if rightNumber == 0 {
			return NullValue(), fmt.Errorf("division by zero")
		}
		return numericResult(math.Floor(leftNumber/rightNumber), leftFloat || rightFloat), nil
	case OpModulo:
		if rightNumber == 0 {
			return NullValue(), fmt.Errorf("modulo by zero")
		}
		return numericResult(math.Mod(leftNumber, rightNumber), leftFloat || rightFloat), nil
	case OpPower:
		return numericResult(math.Pow(leftNumber, rightNumber), leftFloat || rightFloat || rightNumber < 0), nil
	case OpGreater:
		return BoolValue(leftNumber > rightNumber), nil
	case OpGreaterEqual:
		return BoolValue(leftNumber >= rightNumber), nil
	case OpLess:
		return BoolValue(leftNumber < rightNumber), nil
	case OpLessEqual:
		return BoolValue(leftNumber <= rightNumber), nil
	default:
		return NullValue(), fmt.Errorf("unsupported binary opcode %s", opcode)
	}
}

func number(value Value) (float64, bool, bool) {
	switch value.Kind {
	case ValueInt:
		return float64(value.Int), false, true
	case ValueFloat:
		return value.Float, true, true
	default:
		return 0, false, false
	}
}

func numericResult(value float64, floating bool) Value {
	if floating {
		return FloatValue(value)
	}
	return IntValue(int64(value))
}

func truthy(value Value) bool {
	switch value.Kind {
	case ValueNull:
		return false
	case ValueBool:
		return value.Bool
	case ValueInt:
		return value.Int != 0
	case ValueFloat:
		return value.Float != 0
	case ValueString:
		return value.String != ""
	case ValueList:
		return true
	case ValueFunction, ValueIterator:
		return true
	default:
		return false
	}
}

func cloneValue(value Value) Value {
	if value.Kind == ValueIterator {
		if value.Iter == nil {
			return value
		}
		data := *value.Iter
		data.Items = value.Iter.Items
		data.Stages = append([]PipelineStage(nil), value.Iter.Stages...)
		return IteratorValue(&data)
	}
	if value.Kind != ValueList {
		return value
	}
	return ListValue(value.List)
}

func indexValueAt(target Value, indexValue Value) (Value, error) {
	if indexValue.Kind != ValueInt {
		return NullValue(), fmt.Errorf("index must be Int")
	}
	index := int(indexValue.Int)
	switch target.Kind {
	case ValueList:
		if index < 0 || index >= len(target.List) {
			return NullValue(), fmt.Errorf("List index %d is out of bounds", index)
		}
		return cloneValue(target.List[index]), nil
	case ValueString:
		items := []rune(target.String)
		if index < 0 || index >= len(items) {
			return NullValue(), fmt.Errorf("String index %d is out of bounds", index)
		}
		return StringValue(string(items[index])), nil
	case ValueInt:
		if target.Int < 0 {
			return NullValue(), fmt.Errorf("iteration count cannot be negative")
		}
		if index < 0 || int64(index) >= target.Int {
			return NullValue(), fmt.Errorf("Int iteration index %d is out of bounds", index)
		}
		return IntValue(int64(index)), nil
	default:
		return NullValue(), fmt.Errorf("indexing expects List, String, or iterable Int")
	}
}

func valuesEqual(left Value, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	if left.Kind == ValueFunction {
		return left.Index == right.Index
	}
	if left.Kind == ValueIterator {
		return left.Iter == right.Iter
	}
	if left.Kind != ValueList {
		return left.Interface() == right.Interface()
	}
	if len(left.List) != len(right.List) {
		return false
	}
	for index := range left.List {
		if !valuesEqual(left.List[index], right.List[index]) {
			return false
		}
	}
	return true
}
