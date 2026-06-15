package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"kLang/src/lexer"
	"kLang/src/parser"
)

func NullValue() Value {
	return Value{Kind: ValueNull}
}

func IntValue(value int) Value {
	return Value{Kind: ValueInt, Data: value}
}

func FloatValue(value float64) Value {
	return Value{Kind: ValueFloat, Data: value}
}

func StringValue(value string) Value {
	return Value{Kind: ValueString, Data: value}
}

func BoolValue(value bool) Value {
	return Value{Kind: ValueBool, Data: value}
}

func CharValue(value string) Value {
	return Value{Kind: ValueChar, Data: value}
}

func FunctionValue(name string) Value {
	return Value{Kind: ValueFunction, Data: name}
}

func ThunkValue(expr parser.ExpressionNode, env *Environment) Value {
	return Value{Kind: ValueThunk, Data: &ThunkData{Expr: expr, Env: env}}
}

func OptionSomeValue(value Value) Value {
	return Value{Kind: ValueOption, Data: OptionData{Some: true, Value: value}}
}

func OptionNoneValue() Value {
	return Value{Kind: ValueOption, Data: OptionData{Some: false, Value: NullValue()}}
}

func ResultOkValue(value Value) Value {
	return Value{Kind: ValueResult, Data: ResultData{Ok: true, Value: value}}
}

func ResultErrValue(value Value) Value {
	return Value{Kind: ValueResult, Data: ResultData{Ok: false, Value: value}}
}

func ComplexValue(real float64, imag float64) Value {
	return Value{Kind: ValueComplex, Data: ComplexData{Real: real, Imag: imag}}
}

func SIMDValue(lanes []Value) Value {
	copied := make([]Value, 0, len(lanes))
	for _, lane := range lanes {
		copied = append(copied, cloneValue(lane))
	}
	return Value{Kind: ValueSIMD, Data: SIMDData{Lanes: copied}}
}

func TableValue(items map[string]Value) Value {
	copied := make(map[string]Value, len(items))
	for key, item := range items {
		copied[key] = cloneValue(item)
	}
	return Value{Kind: ValueTable, Data: copied}
}

func AwaitableValue(function string, args []Value) Value {
	copied := make([]Value, 0, len(args))
	for _, arg := range args {
		copied = append(copied, cloneValue(arg))
	}
	return Value{Kind: ValueAwaitable, Data: &AwaitableData{Function: function, Args: copied}}
}

func IteratorValue(items []Value) Value {
	copied := make([]Value, 0, len(items))
	for _, item := range items {
		copied = append(copied, cloneValue(item))
	}
	return Value{Kind: ValueIterator, Data: &IteratorData{Items: copied}}
}

func CoroutineValue(function string) Value {
	return Value{Kind: ValueCoroutine, Data: &CoroutineData{Function: function}}
}

func zeroValue(typeName string) Value {
	typeName = strings.TrimSpace(typeName)
	switch typeName {
	case "Int", "UInt":
		return IntValue(0)
	case "Float":
		return FloatValue(0)
	case "String":
		return StringValue("")
	case "Bool":
		return BoolValue(false)
	case "Char":
		return CharValue("")
	case "Complex":
		return ComplexValue(0, 0)
	case "T":
		return NullValue()
	}
	if strings.HasPrefix(typeName, "Function[") {
		return NullValue()
	}
	if strings.HasPrefix(typeName, "List[") {
		return Value{Kind: ValueList, Data: []Value{}}
	}
	if strings.HasPrefix(typeName, "Map[") {
		return Value{Kind: ValueMap, Data: map[string]Value{}}
	}
	if typeName == "Table" {
		return TableValue(map[string]Value{})
	}
	if strings.HasPrefix(typeName, "Option[") {
		return OptionNoneValue()
	}
	if strings.HasPrefix(typeName, "Result[") {
		okType, _, ok := resultTypes(typeName)
		if ok {
			return ResultOkValue(zeroValue(okType))
		}
		return ResultOkValue(NullValue())
	}
	if strings.HasPrefix(typeName, "SIMD[") {
		return SIMDValue(nil)
	}
	if isRuntimeArrayType(typeName) {
		return Value{Kind: ValueList, Data: []Value{}}
	}
	if _, allowed, ok := restrictedGenericRuntimeType(typeName); ok && len(allowed) > 0 {
		return zeroValue(allowed[0])
	}
	return NullValue()
}

func cloneValue(value Value) Value {
	switch value.Kind {
	case ValueList:
		items := value.Data.([]Value)
		cloned := make([]Value, 0, len(items))
		for _, item := range items {
			cloned = append(cloned, cloneValue(item))
		}
		return Value{Kind: ValueList, Data: cloned}
	case ValueMap:
		items := value.Data.(map[string]Value)
		cloned := make(map[string]Value, len(items))
		for key, item := range items {
			cloned[key] = cloneValue(item)
		}
		return Value{Kind: ValueMap, Data: cloned}
	case ValueTable:
		return TableValue(value.Data.(map[string]Value))
	case ValueOption:
		option := value.Data.(OptionData)
		option.Value = cloneValue(option.Value)
		return Value{Kind: ValueOption, Data: option}
	case ValueResult:
		result := value.Data.(ResultData)
		result.Value = cloneValue(result.Value)
		return Value{Kind: ValueResult, Data: result}
	case ValueSIMD:
		return SIMDValue(value.Data.(SIMDData).Lanes)
	case ValueAwaitable, ValueIterator, ValueCoroutine, ValueThread:
		return value
	case ValueAtomic:
		return value
	case ValueObject:
		object := value.Data.(ObjectData)
		fields := make(map[string]Value, len(object.Fields))
		for key, item := range object.Fields {
			fields[key] = cloneValue(item)
		}
		return Value{Kind: ValueObject, Data: ObjectData{Type: object.Type, Struct: object.Struct, Fields: fields}}
	case ValueThunk:
		return value
	default:
		return value
	}
}

func runtimeTypeName(value Value) string {
	switch value.Kind {
	case ValueInt:
		return "Int"
	case ValueFloat:
		return "Float"
	case ValueString:
		return "String"
	case ValueBool:
		return "Bool"
	case ValueChar:
		return "Char"
	case ValueList:
		return "List[T]"
	case ValueMap:
		return "Map[T,T]"
	case ValueTable:
		return "Table"
	case ValueOption:
		return "Option[T]"
	case ValueResult:
		return "Result[T,T]"
	case ValueComplex:
		return "Complex"
	case ValueSIMD:
		return "SIMD[T]"
	case ValueAwaitable:
		return "Awaitable[T]"
	case ValueIterator:
		return "Iterator[T]"
	case ValueCoroutine:
		return "Coroutine[T]"
	case ValueThread:
		return "Thread[T]"
	case ValueAtomic:
		return "Atomic[T]"
	case ValueEnum:
		return value.Data.(EnumData).Type
	case ValueFunction:
		return "Function[T]"
	case ValueObject:
		return value.Data.(ObjectData).Type
	case ValueBoundMethod:
		return "Function[T]"
	case ValueThunk:
		return "T"
	default:
		return "T"
	}
}

func literalValue(expr parser.LiteralExpression) (Value, error) {
	switch expr.Kind {
	case "Int":
		value, err := strconv.Atoi(expr.Value)
		if err != nil {
			return NullValue(), err
		}
		return IntValue(value), nil
	case "Float":
		value, err := strconv.ParseFloat(expr.Value, 64)
		if err != nil {
			return NullValue(), err
		}
		return FloatValue(value), nil
	case "String":
		return StringValue(expr.Value), nil
	case "Bool":
		return BoolValue(expr.Value == "True"), nil
	case "Char":
		return CharValue(expr.Value), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported literal kind %q", expr.Kind)}
	}
}

func castValue(value Value, typeName string) (Value, error) {
	typeName = strings.TrimSpace(typeName)
	if typeName == "" || typeName == "T" {
		return value, nil
	}
	if valueMatchesType(value, typeName) {
		return value, nil
	}

	switch typeName {
	case "Int", "UInt":
		return castToInt(value, typeName)
	case "Float":
		return castToFloat(value)
	case "String":
		return StringValue(valueString(value)), nil
	case "Bool":
		return castToBool(value), nil
	case "Char":
		return castToChar(value)
	default:
		if strings.HasPrefix(typeName, "List[") || strings.HasPrefix(typeName, "Map[") ||
			strings.HasPrefix(typeName, "Option[") || strings.HasPrefix(typeName, "Result[") ||
			strings.HasPrefix(typeName, "SIMD[") {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s", value.Kind, typeName)}
		}
		return NullValue(), Error{Message: fmt.Sprintf("unknown cast target type %q", typeName)}
	}
}

func castToInt(value Value, typeName string) (Value, error) {
	var result int
	switch value.Kind {
	case ValueInt:
		result = value.Data.(int)
	case ValueFloat:
		result = int(value.Data.(float64))
	case ValueBool:
		if value.Data.(bool) {
			result = 1
		}
	case ValueString, ValueChar:
		parsed, err := strconv.Atoi(value.Data.(string))
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s %q to %s", value.Kind, value.Data.(string), typeName)}
		}
		result = parsed
	default:
		return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s", value.Kind, typeName)}
	}
	if typeName == "UInt" && result < 0 {
		return NullValue(), Error{Message: "cannot cast negative Int to UInt"}
	}
	return IntValue(result), nil
}

func castToFloat(value Value) (Value, error) {
	switch value.Kind {
	case ValueFloat:
		return value, nil
	case ValueInt:
		return FloatValue(float64(value.Data.(int))), nil
	case ValueBool:
		if value.Data.(bool) {
			return FloatValue(1), nil
		}
		return FloatValue(0), nil
	case ValueString, ValueChar:
		parsed, err := strconv.ParseFloat(value.Data.(string), 64)
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s %q to Float", value.Kind, value.Data.(string))}
		}
		return FloatValue(parsed), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to Float", value.Kind)}
	}
}

func castToBool(value Value) Value {
	if value.Kind == ValueString {
		switch value.Data.(string) {
		case "True", "true", "1":
			return BoolValue(true)
		case "False", "false", "0", "":
			return BoolValue(false)
		}
	}
	return BoolValue(isTruthy(value))
}

func castToChar(value Value) (Value, error) {
	switch value.Kind {
	case ValueChar:
		return value, nil
	case ValueString:
		runes := []rune(value.Data.(string))
		if len(runes) != 1 {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast String %q to Char", value.Data.(string))}
		}
		return CharValue(string(runes[0])), nil
	case ValueInt:
		return CharValue(string(rune(value.Data.(int)))), nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to Char", value.Kind)}
	}
}

func applyAssignmentOperator(left Value, operator string, right Value) (Value, error) {
	switch operator {
	case "=":
		return right, nil
	case "+=":
		return numericOrString(left, right, func(a, b float64) float64 { return a + b })
	case "-=":
		return numericBinary(left, right, func(a, b float64) float64 { return a - b })
	case "*=":
		return numericBinary(left, right, func(a, b float64) float64 { return a * b })
	case "/=":
		return divideValues(left, right)
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported assignment operator %q", operator)}
	}
}

func numericOrString(left Value, right Value, op func(float64, float64) float64) (Value, error) {
	if left.Kind == ValueString || right.Kind == ValueString {
		return StringValue(valueString(left) + valueString(right)), nil
	}
	if left.Kind == ValueSIMD || right.Kind == ValueSIMD {
		return simdBinary(left, right, op)
	}
	if left.Kind == ValueComplex || right.Kind == ValueComplex {
		return complexBinary(left, right, op, "+")
	}
	return numericBinary(left, right, op)
}

func numericBinary(left Value, right Value, op func(float64, float64) float64) (Value, error) {
	if left.Kind == ValueSIMD || right.Kind == ValueSIMD {
		return simdBinary(left, right, op)
	}
	if left.Kind == ValueComplex || right.Kind == ValueComplex {
		return complexBinary(left, right, op, "")
	}
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("numeric operation requires Int, Float, Complex, or SIMD, got %s and %s", left.Kind, right.Kind)}
	}
	if left.Kind == ValueFloat || right.Kind == ValueFloat {
		leftFloat, _ := asFloat(left)
		rightFloat, _ := asFloat(right)
		return FloatValue(op(leftFloat, rightFloat)), nil
	}
	leftInt, _ := asInt(left)
	rightInt, _ := asInt(right)
	return IntValue(int(op(float64(leftInt), float64(rightInt)))), nil
}

func divideValues(left Value, right Value) (Value, error) {
	if left.Kind == ValueSIMD || right.Kind == ValueSIMD {
		return simdBinary(left, right, func(a, b float64) float64 { return a / b })
	}
	if left.Kind == ValueComplex || right.Kind == ValueComplex {
		return divideComplexValues(left, right)
	}
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("division requires Int or Float, got %s and %s", left.Kind, right.Kind)}
	}
	rightFloat, _ := asFloat(right)
	if rightFloat == 0 {
		return NullValue(), Error{Message: "division by zero"}
	}
	return numericBinary(left, right, func(a, b float64) float64 { return a / b })
}

func floorDivideValues(left Value, right Value) (Value, error) {
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("floor division requires Int or Float, got %s and %s", left.Kind, right.Kind)}
	}
	rightFloat, _ := asFloat(right)
	if rightFloat == 0 {
		return NullValue(), Error{Message: "division by zero"}
	}
	leftFloat, _ := asFloat(left)
	result := math.Floor(leftFloat / rightFloat)
	if left.Kind == ValueFloat || right.Kind == ValueFloat {
		return FloatValue(result), nil
	}
	return IntValue(int(result)), nil
}

func exponentValues(left Value, right Value) (Value, error) {
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("exponent requires Int or Float, got %s and %s", left.Kind, right.Kind)}
	}
	leftFloat, _ := asFloat(left)
	rightFloat, _ := asFloat(right)
	result := math.Pow(leftFloat, rightFloat)
	if left.Kind == ValueFloat || right.Kind == ValueFloat || rightFloat < 0 {
		return FloatValue(result), nil
	}
	return IntValue(int(result)), nil
}

func moduloValues(left Value, right Value) (Value, error) {
	leftInt, err := asInt(left)
	if err != nil {
		return NullValue(), err
	}
	rightInt, err := asInt(right)
	if err != nil {
		return NullValue(), err
	}
	if rightInt == 0 {
		return NullValue(), Error{Message: "modulo by zero"}
	}
	return IntValue(leftInt % rightInt), nil
}

func complexComponents(value Value) (float64, float64, error) {
	switch value.Kind {
	case ValueComplex:
		data := value.Data.(ComplexData)
		return data.Real, data.Imag, nil
	case ValueInt, ValueFloat:
		real, _ := asFloat(value)
		return real, 0, nil
	default:
		return 0, 0, Error{Message: fmt.Sprintf("expected Complex-compatible value, got %s", value.Kind)}
	}
}

func complexBinary(left Value, right Value, op func(float64, float64) float64, operator string) (Value, error) {
	leftReal, leftImag, err := complexComponents(left)
	if err != nil {
		return NullValue(), err
	}
	rightReal, rightImag, err := complexComponents(right)
	if err != nil {
		return NullValue(), err
	}
	if operator == "*" {
		return ComplexValue(leftReal*rightReal-leftImag*rightImag, leftReal*rightImag+leftImag*rightReal), nil
	}
	return ComplexValue(op(leftReal, rightReal), op(leftImag, rightImag)), nil
}

func divideComplexValues(left Value, right Value) (Value, error) {
	leftReal, leftImag, err := complexComponents(left)
	if err != nil {
		return NullValue(), err
	}
	rightReal, rightImag, err := complexComponents(right)
	if err != nil {
		return NullValue(), err
	}
	denominator := rightReal*rightReal + rightImag*rightImag
	if denominator == 0 {
		return NullValue(), Error{Message: "division by zero"}
	}
	return ComplexValue((leftReal*rightReal+leftImag*rightImag)/denominator, (leftImag*rightReal-leftReal*rightImag)/denominator), nil
}

func simdBinary(left Value, right Value, op func(float64, float64) float64) (Value, error) {
	leftLanes, rightLanes, err := simdLanePairs(left, right)
	if err != nil {
		return NullValue(), err
	}
	result := make([]Value, 0, len(leftLanes))
	for index := range leftLanes {
		if !isNumeric(leftLanes[index]) || !isNumeric(rightLanes[index]) {
			return NullValue(), Error{Message: "SIMD operations require numeric lanes"}
		}
		if leftLanes[index].Kind == ValueFloat || rightLanes[index].Kind == ValueFloat {
			leftFloat, _ := asFloat(leftLanes[index])
			rightFloat, _ := asFloat(rightLanes[index])
			result = append(result, FloatValue(op(leftFloat, rightFloat)))
			continue
		}
		leftInt, _ := asInt(leftLanes[index])
		rightInt, _ := asInt(rightLanes[index])
		result = append(result, IntValue(int(op(float64(leftInt), float64(rightInt)))))
	}
	return SIMDValue(result), nil
}

func simdLanePairs(left Value, right Value) ([]Value, []Value, error) {
	if left.Kind != ValueSIMD && right.Kind != ValueSIMD {
		return nil, nil, Error{Message: "SIMD operation requires a SIMD value"}
	}
	if left.Kind == ValueSIMD && right.Kind == ValueSIMD {
		leftLanes := left.Data.(SIMDData).Lanes
		rightLanes := right.Data.(SIMDData).Lanes
		if len(leftLanes) != len(rightLanes) {
			return nil, nil, Error{Message: "SIMD lane counts must match"}
		}
		return leftLanes, rightLanes, nil
	}
	if left.Kind == ValueSIMD {
		leftLanes := left.Data.(SIMDData).Lanes
		rightLanes := make([]Value, 0, len(leftLanes))
		for range leftLanes {
			rightLanes = append(rightLanes, right)
		}
		return leftLanes, rightLanes, nil
	}
	rightLanes := right.Data.(SIMDData).Lanes
	leftLanes := make([]Value, 0, len(rightLanes))
	for range rightLanes {
		leftLanes = append(leftLanes, left)
	}
	return leftLanes, rightLanes, nil
}

func compareNumeric(left Value, right Value, op func(float64, float64) bool) (Value, error) {
	leftFloat, err := asFloat(left)
	if err != nil {
		return NullValue(), err
	}
	rightFloat, err := asFloat(right)
	if err != nil {
		return NullValue(), err
	}
	return BoolValue(op(leftFloat, rightFloat)), nil
}

func compareOrdered(left Value, right Value, op func(int) bool) (Value, error) {
	if isNumeric(left) && isNumeric(right) {
		leftFloat, _ := asFloat(left)
		rightFloat, _ := asFloat(right)
		if leftFloat < rightFloat {
			return BoolValue(op(-1)), nil
		}
		if leftFloat > rightFloat {
			return BoolValue(op(1)), nil
		}
		return BoolValue(op(0)), nil
	}
	if left.Kind == ValueString && right.Kind == ValueString || left.Kind == ValueChar && right.Kind == ValueChar {
		leftString := valueString(left)
		rightString := valueString(right)
		if leftString < rightString {
			return BoolValue(op(-1)), nil
		}
		if leftString > rightString {
			return BoolValue(op(1)), nil
		}
		return BoolValue(op(0)), nil
	}
	return NullValue(), Error{Message: fmt.Sprintf("ordered comparison requires matching numeric, String, or Char values, got %s and %s", left.Kind, right.Kind)}
}

func asInt(value Value) (int, error) {
	switch value.Kind {
	case ValueInt:
		return value.Data.(int), nil
	case ValueFloat:
		return int(value.Data.(float64)), nil
	default:
		return 0, Error{Message: fmt.Sprintf("expected Int-compatible value, got %s", value.Kind)}
	}
}

func asIndex(value Value) (int, error) {
	if value.Kind != ValueInt {
		return 0, Error{Message: fmt.Sprintf("list index must be Int, got %s", value.Kind)}
	}
	return value.Data.(int), nil
}

func asFloat(value Value) (float64, error) {
	switch value.Kind {
	case ValueFloat:
		return value.Data.(float64), nil
	case ValueInt:
		return float64(value.Data.(int)), nil
	default:
		return 0, Error{Message: fmt.Sprintf("expected numeric value, got %s", value.Kind)}
	}
}

func isNumeric(value Value) bool {
	return value.Kind == ValueInt || value.Kind == ValueFloat
}

func isTruthy(value Value) bool {
	switch value.Kind {
	case ValueBool:
		return value.Data.(bool)
	case ValueInt:
		return value.Data.(int) != 0
	case ValueFloat:
		return value.Data.(float64) != 0
	case ValueString:
		return value.Data.(string) != ""
	case ValueNull:
		return false
	case ValueOption:
		return value.Data.(OptionData).Some
	case ValueResult:
		return value.Data.(ResultData).Ok
	default:
		return true
	}
}

func valueString(value Value) string {
	switch value.Kind {
	case ValueNull:
		return "Null"
	case ValueInt:
		return strconv.Itoa(value.Data.(int))
	case ValueFloat:
		return strconv.FormatFloat(value.Data.(float64), 'f', -1, 64)
	case ValueString, ValueChar:
		return value.Data.(string)
	case ValueBool:
		if value.Data.(bool) {
			return "True"
		}
		return "False"
	case ValueOption:
		option := value.Data.(OptionData)
		if !option.Some {
			return "None"
		}
		return "Some(" + valueString(option.Value) + ")"
	case ValueResult:
		result := value.Data.(ResultData)
		if result.Ok {
			return "Ok(" + valueString(result.Value) + ")"
		}
		return "Err(" + valueString(result.Value) + ")"
	case ValueComplex:
		data := value.Data.(ComplexData)
		sign := "+"
		imag := data.Imag
		if imag < 0 {
			sign = "-"
			imag = -imag
		}
		return fmt.Sprintf("%g%s%gi", data.Real, sign, imag)
	case ValueSIMD:
		lanes := value.Data.(SIMDData).Lanes
		parts := make([]string, 0, len(lanes))
		for _, lane := range lanes {
			parts = append(parts, valueString(lane))
		}
		return "SIMD[" + strings.Join(parts, ", ") + "]"
	case ValueTable:
		parts := make([]string, 0, len(value.Data.(map[string]Value)))
		for key, item := range value.Data.(map[string]Value) {
			parts = append(parts, key+": "+valueString(item))
		}
		return "Table{" + strings.Join(parts, ", ") + "}"
	case ValueAwaitable:
		data := value.Data.(*AwaitableData)
		if data.Done {
			return "Awaitable(" + valueString(data.Value) + ")"
		}
		return "Awaitable(" + data.Function + ")"
	case ValueIterator:
		data := value.Data.(*IteratorData)
		return fmt.Sprintf("Iterator(%d/%d)", data.Index, len(data.Items))
	case ValueCoroutine:
		data := value.Data.(*CoroutineData)
		if data.Done {
			return "Coroutine(done)"
		}
		return "Coroutine(" + data.Function + ")"
	case ValueThread:
		thread := value.Data.(*ThreadData)
		select {
		case <-thread.Done:
			return "Thread(done)"
		default:
			return "Thread(running)"
		}
	case ValueEnum:
		data := value.Data.(EnumData)
		return data.Type + "." + data.Variant
	case ValueObject:
		object := value.Data.(ObjectData)
		parts := make([]string, 0, len(object.Fields))
		for key, field := range object.Fields {
			if strings.HasPrefix(key, "__") {
				continue
			}
			parts = append(parts, key+": "+valueString(field))
		}
		return object.Type + "{" + strings.Join(parts, ", ") + "}"
	case ValueBoundMethod:
		method := value.Data.(BoundMethodData)
		return method.Type + "." + method.Name
	default:
		return fmt.Sprintf("%v", value.Data)
	}
}

func valueSize(value Value) int {
	switch value.Kind {
	case ValueNull:
		return 0
	case ValueInt, ValueFloat, ValueBool, ValueChar, ValueFunction, ValueBoundMethod:
		return 8
	case ValueEnum:
		return 16 + len(value.Data.(EnumData).Type) + len(value.Data.(EnumData).Variant)
	case ValueString:
		return len(value.Data.(string))
	case ValueList:
		size := 24
		for _, item := range value.Data.([]Value) {
			size += valueSize(item)
		}
		return size
	case ValueMap:
		size := 48
		for key, item := range value.Data.(map[string]Value) {
			size += len(key) + valueSize(item)
		}
		return size
	case ValueTable:
		size := 48
		for key, item := range value.Data.(map[string]Value) {
			size += len(key) + valueSize(item)
		}
		return size
	case ValueOption:
		return 8 + valueSize(value.Data.(OptionData).Value)
	case ValueResult:
		return 8 + valueSize(value.Data.(ResultData).Value)
	case ValueComplex:
		return 16
	case ValueSIMD:
		size := 16
		for _, lane := range value.Data.(SIMDData).Lanes {
			size += valueSize(lane)
		}
		return size
	case ValueThunk:
		return 16
	case ValueAwaitable:
		data := value.Data.(*AwaitableData)
		size := 16 + len(data.Function)
		for _, arg := range data.Args {
			size += valueSize(arg)
		}
		return size + valueSize(data.Value)
	case ValueIterator:
		data := value.Data.(*IteratorData)
		size := 16
		for _, item := range data.Items {
			size += valueSize(item)
		}
		return size
	case ValueCoroutine:
		data := value.Data.(*CoroutineData)
		return 16 + len(data.Function) + valueSize(data.Value)
	case ValueObject:
		object := value.Data.(ObjectData)
		size := 32 + len(object.Type)
		for key, field := range object.Fields {
			size += len(key) + valueSize(field)
		}
		return size
	default:
		return 8
	}
}

func valueLen(value Value) (int, error) {
	switch value.Kind {
	case ValueString:
		return len([]rune(value.Data.(string))), nil
	case ValueList:
		return len(value.Data.([]Value)), nil
	case ValueMap:
		return len(value.Data.(map[string]Value)), nil
	case ValueTable:
		return len(value.Data.(map[string]Value)), nil
	case ValueSIMD:
		return len(value.Data.(SIMDData).Lanes), nil
	default:
		return 0, Error{Message: fmt.Sprintf("len does not support %s", value.Kind)}
	}
}

func mapKey(value Value) (string, error) {
	switch value.Kind {
	case ValueInt, ValueFloat, ValueString, ValueBool, ValueChar:
		return valueString(value), nil
	default:
		return "", Error{Message: fmt.Sprintf("%s cannot be used as a map key", value.Kind)}
	}
}

func valueMatchesType(value Value, typeName string) bool {
	typeName = strings.TrimSpace(typeName)
	if tupleTypes, ok := tupleRuntimeTypes(typeName); ok {
		if value.Kind != ValueList {
			return false
		}
		items := value.Data.([]Value)
		if len(items) != len(tupleTypes) {
			return false
		}
		for index, item := range items {
			if !valueMatchesType(item, tupleTypes[index]) {
				return false
			}
		}
		return true
	}
	if _, allowed, ok := restrictedGenericRuntimeType(typeName); ok {
		for _, option := range allowed {
			if valueMatchesType(value, option) {
				return true
			}
		}
		return false
	}
	switch {
	case typeName == "" || typeName == "T" || typeName == "Any":
		return true
	case value.Kind == ValueThunk:
		return true
	case typeName == "Int":
		return value.Kind == ValueInt
	case typeName == "UInt":
		return value.Kind == ValueInt && value.Data.(int) >= 0
	case typeName == "Float":
		return value.Kind == ValueFloat || value.Kind == ValueInt
	case typeName == "String":
		return value.Kind == ValueString
	case typeName == "Bool":
		return value.Kind == ValueBool
	case typeName == "Char":
		return value.Kind == ValueChar
	case typeName == "Complex":
		return value.Kind == ValueComplex
	case value.Kind == ValueEnum:
		return value.Data.(EnumData).Type == typeName
	case typeName == "Table":
		return value.Kind == ValueTable || value.Kind == ValueMap
	case strings.HasPrefix(typeName, "List["):
		return value.Kind == ValueList
	case strings.HasPrefix(typeName, "Map["):
		return value.Kind == ValueMap
	case strings.HasPrefix(typeName, "Awaitable["):
		return value.Kind == ValueAwaitable
	case strings.HasPrefix(typeName, "Iterator["):
		return value.Kind == ValueIterator
	case strings.HasPrefix(typeName, "Coroutine["):
		return value.Kind == ValueCoroutine
	case strings.HasPrefix(typeName, "Thread["):
		return value.Kind == ValueThread
	case typeName == "Thread":
		return value.Kind == ValueThread
	case strings.HasPrefix(typeName, "Atomic["):
		return value.Kind == ValueAtomic
	case strings.HasPrefix(typeName, "SIMD["):
		elementType, ok := simdType(typeName)
		if !ok || value.Kind != ValueSIMD {
			return false
		}
		for _, lane := range value.Data.(SIMDData).Lanes {
			if !valueMatchesType(lane, elementType) {
				return false
			}
		}
		return true
	case isRuntimeArrayType(typeName):
		return value.Kind == ValueList
	case strings.HasPrefix(typeName, "Function["):
		return value.Kind == ValueFunction || value.Kind == ValueNull
	case strings.HasPrefix(typeName, "Option["):
		elementType, ok := optionType(typeName)
		if !ok || value.Kind != ValueOption {
			return false
		}
		option := value.Data.(OptionData)
		return !option.Some || valueMatchesType(option.Value, elementType)
	case strings.HasPrefix(typeName, "Result["):
		okType, errType, ok := resultTypes(typeName)
		if !ok || value.Kind != ValueResult {
			return false
		}
		result := value.Data.(ResultData)
		if result.Ok {
			return valueMatchesType(result.Value, okType)
		}
		return valueMatchesType(result.Value, errType)
	default:
		if value.Kind == ValueObject {
			return value.Data.(ObjectData).Type == typeName
		}
		return true
	}
}

func tupleRuntimeTypes(typeName string) ([]string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "(") || !strings.HasSuffix(typeName, ")") {
		return nil, false
	}
	inner := strings.TrimSpace(typeName[1 : len(typeName)-1])
	if inner == "" {
		return nil, false
	}
	return splitTopLevelType(inner, ','), true
}

func isRuntimeArrayType(typeName string) bool {
	return strings.Contains(typeName, "[") && strings.HasSuffix(typeName, "]") &&
		!strings.HasPrefix(typeName, "List[") && !strings.HasPrefix(typeName, "Map[") &&
		!strings.HasPrefix(typeName, "Option[") && !strings.HasPrefix(typeName, "Result[") &&
		!strings.HasPrefix(typeName, "SIMD[") && !strings.HasPrefix(typeName, "Function[") &&
		!strings.HasPrefix(typeName, "Awaitable[") && !strings.HasPrefix(typeName, "Iterator[") &&
		!strings.HasPrefix(typeName, "Coroutine[")
}

func restrictedGenericRuntimeType(typeName string) (string, []string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "T:") {
		return "", nil, false
	}
	parts := strings.Split(typeName[len("T:"):], "|")
	for index, part := range parts {
		parts[index] = strings.TrimSpace(part)
		if parts[index] == "" {
			return "", nil, false
		}
	}
	return "T", parts, true
}

func listElementType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "List[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := strings.TrimSpace(typeName[len("List[") : len(typeName)-1])
	return elementType, elementType != "" && elementType != "...$Items"
}

func arrayElementRuntimeType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	index := strings.Index(typeName, "[")
	if index <= 0 || !strings.HasSuffix(typeName, "]") || !isRuntimeArrayType(typeName) {
		return "", false
	}
	return strings.TrimSpace(typeName[:index]), true
}

func regionNameFromRuntimeArrayType(typeName string) string {
	typeName = strings.TrimSpace(typeName)
	index := strings.Index(typeName, "[")
	if index <= 0 || !strings.HasSuffix(typeName, "]") || !isRuntimeArrayType(typeName) {
		return ""
	}
	return strings.TrimSpace(typeName[index+1 : len(typeName)-1])
}

func mapTypes(typeName string) (string, string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "Map[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	inner := typeName[len("Map[") : len(typeName)-1]
	parts := splitTopLevelType(inner, ',')
	if len(parts) != 2 {
		return "", "", false
	}
	keyType := strings.TrimSpace(parts[0])
	valueType := strings.TrimSpace(parts[1])
	return keyType, valueType, keyType != "" && valueType != ""
}

func optionType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "Option[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := strings.TrimSpace(typeName[len("Option[") : len(typeName)-1])
	return elementType, elementType != ""
}

func resultTypes(typeName string) (string, string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "Result[") || !strings.HasSuffix(typeName, "]") {
		return "", "", false
	}
	inner := typeName[len("Result[") : len(typeName)-1]
	parts := splitTopLevelType(inner, ',')
	if len(parts) != 2 {
		return "", "", false
	}
	okType := strings.TrimSpace(parts[0])
	errType := strings.TrimSpace(parts[1])
	return okType, errType, okType != "" && errType != ""
}

func simdType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "SIMD[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := strings.TrimSpace(typeName[len("SIMD[") : len(typeName)-1])
	return elementType, elementType != ""
}

func splitTopLevelType(input string, separator rune) []string {
	var parts []string
	start := 0
	depth := 0
	for index, char := range input {
		switch char {
		case '[':
			depth++
		case ']':
			if depth > 0 {
				depth--
			}
		default:
			if char == separator && depth == 0 {
				parts = append(parts, input[start:index])
				start = index + len(string(char))
			}
		}
	}
	parts = append(parts, input[start:])
	return parts
}

func parseRangeHeader(expr parser.Expression) (string, parser.Expression, bool) {
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

func parseCStyleForHeader(expr parser.Expression) (parser.Expression, parser.Expression, parser.Expression, bool) {
	parts := splitTopLevel(expr.Tokens, lexer.TokenSemicolon)
	if len(parts) != 3 {
		return parser.Expression{}, parser.Expression{}, parser.Expression{}, false
	}
	return expressionFromRuntimeTokens(parts[0]), expressionFromRuntimeTokens(parts[1]), expressionFromRuntimeTokens(parts[2]), true
}

func loopCondition(expr parser.Expression) parser.Expression {
	tokens := expr.Tokens
	for index, token := range tokens {
		if token.Type == lexer.TokenEvaluationAssign && index+1 < len(tokens) {
			conditionTokens := tokens[index+1:]
			return parser.Expression{Tokens: conditionTokens, Node: parser.ParseExpressionTokens(conditionTokens)}
		}
	}
	return expr
}

func parseEvaluationHeader(expr parser.Expression) (string, parser.Expression, bool) {
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

func splitTopLevel(tokens []lexer.Token, separator lexer.TokenType) [][]lexer.Token {
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
				parts = append(parts, trimRuntimeTokens(tokens[start:index]))
				start = index + 1
			}
		}
	}
	parts = append(parts, trimRuntimeTokens(tokens[start:]))
	return parts
}

func trimRuntimeTokens(tokens []lexer.Token) []lexer.Token {
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

func expressionFromRuntimeTokens(tokens []lexer.Token) parser.Expression {
	tokens = trimRuntimeTokens(tokens)
	return parser.Expression{Tokens: tokens, Node: parser.ParseExpressionTokens(tokens)}
}
