package runtime

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"

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

func SetValue(items []Value) (Value, error) {
	set := newSetData()
	for _, item := range items {
		if err := setAdd(&set, item); err != nil {
			return NullValue(), err
		}
	}
	return Value{Kind: ValueSet, Data: set}, nil
}

func newSetData() SetData {
	return SetData{Entries: map[TableKey]Value{}, Order: []TableKey{}}
}

func cloneSetData(set SetData) SetData {
	cloned := SetData{
		Entries: map[TableKey]Value{},
		Order:   append([]TableKey(nil), set.Order...),
	}
	for key, item := range set.Entries {
		cloned.Entries[key] = cloneValue(item)
	}
	return cloned
}

func shareSetData(set SetData) SetData {
	shared := SetData{
		Entries: map[TableKey]Value{},
		Order:   append([]TableKey(nil), set.Order...),
	}
	for key, item := range set.Entries {
		shared.Entries[key] = shareValue(item)
	}
	return shared
}

func setAdd(set *SetData, value Value) error {
	key, err := setKey(value)
	if err != nil {
		return err
	}
	if _, ok := set.Entries[key]; !ok {
		set.Order = append(set.Order, key)
	}
	set.Entries[key] = cloneValue(value)
	return nil
}

func setHas(set SetData, value Value) (bool, error) {
	key, err := setKey(value)
	if err != nil {
		return false, err
	}
	_, ok := set.Entries[key]
	return ok, nil
}

func setValues(set SetData) []Value {
	values := make([]Value, 0, len(set.Order))
	for _, key := range set.Order {
		if value, ok := set.Entries[key]; ok {
			values = append(values, cloneValue(value))
		}
	}
	return values
}

func TableValue(items map[string]Value) Value {
	table := newTableData()
	for key, item := range items {
		tableSet(&table, TableKey{Kind: ValueString, Repr: key}, cloneValue(item))
	}
	return Value{Kind: ValueTable, Data: table}
}

func TableValueFromEntries(entries []TableEntryData) Value {
	table := newTableData()
	for _, entry := range entries {
		key, err := tableKey(entry.Key)
		if err != nil {
			continue
		}
		tableSet(&table, key, cloneValue(entry.Value))
	}
	return Value{Kind: ValueTable, Data: table}
}

func newTableData() TableData {
	return TableData{Entries: map[TableKey]Value{}, Order: []TableKey{}}
}

func cloneTableData(table TableData) TableData {
	cloned := TableData{
		Entries: map[TableKey]Value{},
		Order:   append([]TableKey(nil), table.Order...),
	}
	for key, item := range table.Entries {
		cloned.Entries[key] = cloneValue(item)
	}
	if table.Fallback != nil {
		fallback := cloneTableData(*table.Fallback)
		cloned.Fallback = &fallback
	}
	return cloned
}

func shareTableData(table TableData) TableData {
	shared := TableData{
		Entries: map[TableKey]Value{},
		Order:   append([]TableKey(nil), table.Order...),
	}
	for key, item := range table.Entries {
		shared.Entries[key] = shareValue(item)
	}
	if table.Fallback != nil {
		fallback := shareTableData(*table.Fallback)
		shared.Fallback = &fallback
	}
	return shared
}

func tableSet(table *TableData, key TableKey, value Value) {
	if _, ok := table.Entries[key]; !ok {
		table.Order = append(table.Order, key)
	}
	table.Entries[key] = value
}

func tableDelete(table *TableData, key TableKey) bool {
	if _, ok := table.Entries[key]; !ok {
		return false
	}
	delete(table.Entries, key)
	nextOrder := table.Order[:0]
	for _, existing := range table.Order {
		if existing != key {
			nextOrder = append(nextOrder, existing)
		}
	}
	table.Order = nextOrder
	return true
}

func tableGet(table TableData, key TableKey) (Value, bool) {
	if value, ok := table.Entries[key]; ok {
		return value, true
	}
	if table.Fallback != nil {
		return tableGet(*table.Fallback, key)
	}
	return NullValue(), false
}

func tableHas(table TableData, key TableKey) bool {
	_, ok := tableGet(table, key)
	return ok
}

func tableKeyValue(key TableKey) Value {
	switch key.Kind {
	case ValueInt:
		parsed, _ := strconv.Atoi(key.Repr)
		return IntValue(parsed)
	case ValueFloat:
		parsed, _ := strconv.ParseFloat(key.Repr, 64)
		return FloatValue(parsed)
	case ValueBool:
		return BoolValue(key.Repr == "true")
	case ValueChar:
		return CharValue(key.Repr)
	default:
		return StringValue(key.Repr)
	}
}

func tableKeys(table TableData) []Value {
	values := make([]Value, 0, len(table.Order))
	for _, key := range table.Order {
		if _, ok := table.Entries[key]; ok {
			values = append(values, tableKeyValue(key))
		}
	}
	return values
}

func tableValues(table TableData) []Value {
	values := make([]Value, 0, len(table.Order))
	for _, key := range table.Order {
		if value, ok := table.Entries[key]; ok {
			values = append(values, cloneValue(value))
		}
	}
	return values
}

func tableEntries(table TableData) []Value {
	entries := make([]Value, 0, len(table.Order))
	for _, key := range table.Order {
		value, ok := table.Entries[key]
		if !ok {
			continue
		}
		entries = append(entries, TableValue(map[string]Value{
			"key":   tableKeyValue(key),
			"value": cloneValue(value),
		}))
	}
	return entries
}

func tableSequenceCount(table TableData) int {
	count := 0
	for {
		if _, ok := table.Entries[TableKey{Kind: ValueInt, Repr: strconv.Itoa(count)}]; !ok {
			return count
		}
		count++
	}
}

func tableToStringMap(table TableData) (map[string]Value, error) {
	items := make(map[string]Value, len(table.Entries))
	for _, key := range table.Order {
		value, ok := table.Entries[key]
		if !ok {
			continue
		}
		if key.Kind != ValueString {
			return nil, Error{Message: fmt.Sprintf("%s table key cannot be used as a Map key", key.Kind)}
		}
		items[key.Repr] = cloneValue(value)
	}
	return items, nil
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
	typeName = normalizeRuntimeType(typeName)
	if spec, ok := runtimeChildType(typeName); ok {
		switch spec.Parent {
		case "Int", "UInt":
			return IntValue(0)
		case "Float":
			return FloatValue(0)
		case "Complex":
			return ComplexValue(0, 0)
		}
	}
	switch typeName {
	case "Type":
		return typeInfoValue("T")
	case "Int", "UInt":
		return IntValue(0)
	case "Float":
		return FloatValue(0)
	case "String":
		return StringValue("")
	case "JSON":
		return JSONValue(nil)
	case "Bool":
		return BoolValue(false)
	case "Char":
		return CharValue("")
	case "Complex":
		return ComplexValue(0, 0)
	case "T":
		return NullValue()
	}
	if typeName == "Parsable" || strings.HasPrefix(typeName, "Parsable[") {
		return objectValue("Parsable", map[string]Value{})
	}
	if strings.HasPrefix(typeName, "Function[") {
		return NullValue()
	}
	if strings.HasPrefix(typeName, "List[") {
		return Value{Kind: ValueList, Data: []Value{}}
	}
	if strings.HasPrefix(typeName, "Set[") {
		return Value{Kind: ValueSet, Data: newSetData()}
	}
	if strings.HasPrefix(typeName, "Map[") {
		return Value{Kind: ValueMap, Data: map[string]Value{}}
	}
	if typeName == "Table" {
		return Value{Kind: ValueTable, Data: newTableData()}
	}
	if typeName == "Context" || typeName == "ErrorContext" {
		return Value{Kind: ValueObject, Data: ObjectData{Type: typeName, Fields: map[string]Value{}}}
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
	case ValueSet:
		return Value{Kind: ValueSet, Data: cloneSetData(value.Data.(SetData))}
	case ValueMap:
		items := value.Data.(map[string]Value)
		cloned := make(map[string]Value, len(items))
		for key, item := range items {
			cloned[key] = cloneValue(item)
		}
		return Value{Kind: ValueMap, Data: cloned}
	case ValueTable:
		return Value{Kind: ValueTable, Data: cloneTableData(value.Data.(TableData))}
	case ValueJSON:
		return value
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
		return Value{Kind: ValueObject, Data: ObjectData{Type: object.Type, Struct: object.Struct, Fields: fields, JSONTags: cloneStringMap(object.JSONTags)}}
	case ValueThunk:
		return value
	default:
		return value
	}
}

func shareValue(value Value) Value {
	switch value.Kind {
	case ValueOption:
		option := value.Data.(OptionData)
		option.Value = shareValue(option.Value)
		return Value{Kind: ValueOption, Data: option}
	case ValueResult:
		result := value.Data.(ResultData)
		result.Value = shareValue(result.Value)
		return Value{Kind: ValueResult, Data: result}
	case ValueTable:
		return Value{Kind: ValueTable, Data: shareTableData(value.Data.(TableData))}
	case ValueJSON:
		return value
	case ValueSet:
		return Value{Kind: ValueSet, Data: shareSetData(value.Data.(SetData))}
	case ValueAwaitable:
		data := value.Data.(*AwaitableData)
		args := make([]Value, 0, len(data.Args))
		for _, arg := range data.Args {
			args = append(args, shareValue(arg))
		}
		return Value{Kind: ValueAwaitable, Data: &AwaitableData{Function: data.Function, Args: args, Done: data.Done, Value: shareValue(data.Value)}}
	case ValueObject:
		object := value.Data.(ObjectData)
		fields := make(map[string]Value, len(object.Fields))
		for key, field := range object.Fields {
			fields[key] = shareValue(field)
		}
		return Value{Kind: ValueObject, Data: ObjectData{Type: object.Type, Struct: object.Struct, Fields: fields, JSONTags: cloneStringMap(object.JSONTags)}}
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
	case ValueSet:
		return "Set[T]"
	case ValueMap:
		return "Map[T,T]"
	case ValueTable:
		return "Table"
	case ValueJSON:
		return "JSON"
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
		parsed, err := strconv.ParseInt(normalizeRuntimeNumberLiteral(expr.Value), 0, 0)
		if err != nil {
			return NullValue(), err
		}
		return IntValue(int(parsed)), nil
	case "Float":
		value, err := strconv.ParseFloat(normalizeRuntimeNumberLiteral(expr.Value), 64)
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

func normalizeRuntimeNumberLiteral(literal string) string {
	return strings.ReplaceAll(literal, "_", "")
}

func castValue(value Value, typeName string) (Value, error) {
	typeName = normalizeRuntimeType(typeName)
	if typeName == "" || typeName == "T" {
		return value, nil
	}
	if !isRuntimeBuiltinCastTarget(typeName) {
		return NullValue(), Error{Message: fmt.Sprintf("cast target %s is not a builtin type", typeName)}
	}
	if valueMatchesType(value, typeName) {
		return value, nil
	}

	if spec, ok := runtimeChildType(typeName); ok {
		switch spec.Parent {
		case "Int", "UInt":
			cast, err := castToInt(value, spec.Parent)
			if err != nil {
				return NullValue(), err
			}
			if !valueMatchesType(cast, typeName) {
				return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s: value out of range", value.Kind, typeName)}
			}
			return cast, nil
		case "Float":
			return castToFloat(value)
		case "Complex":
			if value.Kind == ValueComplex {
				return value, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s", value.Kind, typeName)}
		}
	}

	switch typeName {
	case "Int", "UInt":
		return castToInt(value, typeName)
	case "Float":
		return castToFloat(value)
	case "String":
		return StringValue(valueString(value)), nil
	case "JSON":
		if value.Kind != ValueString {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to JSON", value.Kind)}
		}
		return parseJSONValue(value.Data.(string))
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

func isRuntimeBuiltinCastTarget(typeName string) bool {
	typeName = normalizeRuntimeType(typeName)
	if typeName == "" {
		return false
	}
	if _, ok := runtimeChildType(typeName); ok {
		return true
	}
	if _, _, ok := restrictedGenericRuntimeType(typeName); ok {
		return true
	}
	if isRuntimeGenericCastTargetVariable(typeName) {
		return true
	}
	switch typeName {
	case "T", "Any",
		"Int", "UInt", "String", "JSON", "Parsable",
		"Float", "Bool", "Char", "Complex", "Type",
		"Table", "Program", "BuildSystem", "WorkSpace",
		"JSModule", "JSCall", "Context", "ErrorContext",
		"Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return true
	}
	name, _, ok := splitRuntimeGenericType(typeName)
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

func isRuntimeGenericCastTargetVariable(typeName string) bool {
	runes := []rune(typeName)
	return len(runes) == 1 && unicode.IsUpper(runes[0])
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
		parsed, err := strconv.ParseInt(value.Data.(string), 0, 0)
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s %q to %s", value.Kind, value.Data.(string), typeName)}
		}
		result = int(parsed)
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
	case ValueSet:
		set := value.Data.(SetData)
		parts := make([]string, 0, len(set.Order))
		for _, key := range set.Order {
			if item, ok := set.Entries[key]; ok {
				parts = append(parts, valueString(item))
			}
		}
		return "Set{" + strings.Join(parts, ", ") + "}"
	case ValueTable:
		table := value.Data.(TableData)
		parts := make([]string, 0, len(table.Entries))
		for _, key := range table.Order {
			item, ok := table.Entries[key]
			if ok {
				parts = append(parts, valueString(tableKeyValue(key))+": "+valueString(item))
			}
		}
		return "Table{" + strings.Join(parts, ", ") + "}"
	case ValueJSON:
		encoded, err := stringifyJSONValue(value)
		if err != nil {
			return "<invalid JSON>"
		}
		return encoded
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
	case ValueSet:
		size := 48
		set := value.Data.(SetData)
		for key, item := range set.Entries {
			size += len(key.Repr) + valueSize(item)
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
		table := value.Data.(TableData)
		for key, item := range table.Entries {
			size += len(key.Repr) + valueSize(item)
		}
		if table.Fallback != nil {
			size += valueSize(Value{Kind: ValueTable, Data: *table.Fallback})
		}
		return size
	case ValueJSON:
		encoded, err := stringifyJSONValue(value)
		if err != nil {
			return 16
		}
		return 16 + len(encoded)
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
	case ValueSet:
		return len(value.Data.(SetData).Entries), nil
	case ValueMap:
		return len(value.Data.(map[string]Value)), nil
	case ValueTable:
		return len(value.Data.(TableData).Entries), nil
	case ValueJSON:
		return jsonValueLength(value)
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

func tableKey(value Value) (TableKey, error) {
	switch value.Kind {
	case ValueString:
		return TableKey{Kind: ValueString, Repr: value.Data.(string)}, nil
	case ValueInt:
		return TableKey{Kind: ValueInt, Repr: strconv.Itoa(value.Data.(int))}, nil
	case ValueFloat:
		return TableKey{Kind: ValueFloat, Repr: strconv.FormatFloat(value.Data.(float64), 'g', -1, 64)}, nil
	case ValueBool:
		if value.Data.(bool) {
			return TableKey{Kind: ValueBool, Repr: "true"}, nil
		}
		return TableKey{Kind: ValueBool, Repr: "false"}, nil
	case ValueChar:
		return TableKey{Kind: ValueChar, Repr: value.Data.(string)}, nil
	default:
		return TableKey{}, Error{Message: fmt.Sprintf("%s cannot be used as a table key", value.Kind)}
	}
}

func setKey(value Value) (TableKey, error) {
	key, err := tableKey(value)
	if err != nil {
		return TableKey{}, Error{Message: fmt.Sprintf("%s cannot be used as a set item", value.Kind)}
	}
	return key, nil
}

func valueMatchesType(value Value, typeName string) bool {
	typeName = normalizeRuntimeType(typeName)
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
	if spec, ok := runtimeChildType(typeName); ok {
		return valueMatchesRuntimeChildType(value, spec)
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
	case typeName == "JSON":
		return value.Kind == ValueJSON
	case typeName == "Parsable" || strings.HasPrefix(typeName, "Parsable["):
		return isObjectType(value, "Parsable")
	case typeName == "Bool":
		return value.Kind == ValueBool
	case typeName == "Char":
		return value.Kind == ValueChar
	case typeName == "Complex":
		return value.Kind == ValueComplex
	case typeName == "Type":
		return value.Kind == ValueObject && value.Data.(ObjectData).Type == "Type"
	case value.Kind == ValueEnum:
		return value.Data.(EnumData).Type == typeName
	case typeName == "Table":
		return value.Kind == ValueTable || value.Kind == ValueMap
	case typeName == "Context" || typeName == "ErrorContext":
		return value.Kind == ValueObject && value.Data.(ObjectData).Type == typeName
	case strings.HasPrefix(typeName, "List["):
		return value.Kind == ValueList
	case strings.HasPrefix(typeName, "Set["):
		elementType, ok := setElementRuntimeType(typeName)
		if !ok || value.Kind != ValueSet {
			return false
		}
		for _, item := range value.Data.(SetData).Entries {
			if !valueMatchesType(item, elementType) {
				return false
			}
		}
		return true
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
			objectType := value.Data.(ObjectData).Type
			if objectType == typeName {
				return true
			}
			if base, _, ok := splitRuntimeGenericType(typeName); ok {
				return objectType == base
			}
			return false
		}
		return true
	}
}

func splitRuntimeGenericType(typeName string) (string, []string, bool) {
	typeName = normalizeRuntimeType(typeName)
	open := strings.Index(typeName, "[")
	if open == -1 || !strings.HasSuffix(typeName, "]") {
		return "", nil, false
	}
	base := typeName[:open]
	if base == "" {
		return "", nil, false
	}
	inner := typeName[open+1 : len(typeName)-1]
	return base, splitTopLevelType(inner, ','), true
}

func setElementRuntimeType(typeName string) (string, bool) {
	typeName = normalizeRuntimeType(typeName)
	if !strings.HasPrefix(typeName, "Set[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := normalizeRuntimeType(typeName[len("Set[") : len(typeName)-1])
	return elementType, elementType != ""
}

type runtimeChildTypeSpec struct {
	Parent string
	Bits   int
}

var runtimeChildTypeAliases = map[string]string{
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

func normalizeRuntimeType(typeName string) string {
	typeName = strings.ReplaceAll(strings.TrimSpace(typeName), " ", "")
	if alias, ok := runtimeChildTypeAliases[typeName]; ok {
		return alias
	}
	if strings.HasPrefix(typeName, "types.") {
		if alias, ok := runtimeChildTypeAliases[strings.TrimPrefix(typeName, "types.")]; ok {
			return alias
		}
	}
	if parent, bits, ok := parseRuntimeChildType(typeName); ok && isAllowedRuntimeChildType(parent, bits) {
		return fmt.Sprintf("%s.child(%d)", parent, bits)
	}
	return typeName
}

func runtimeChildType(typeName string) (runtimeChildTypeSpec, bool) {
	parent, bits, ok := parseRuntimeChildType(normalizeRuntimeType(typeName))
	if !ok || !isAllowedRuntimeChildType(parent, bits) {
		return runtimeChildTypeSpec{}, false
	}
	return runtimeChildTypeSpec{Parent: parent, Bits: bits}, true
}

func parseRuntimeChildType(typeName string) (string, int, bool) {
	open := strings.Index(typeName, ".child(")
	if open == -1 || !strings.HasSuffix(typeName, ")") {
		return "", 0, false
	}
	parent := typeName[:open]
	switch parent {
	case "int":
		parent = "Int"
	case "uint":
		parent = "UInt"
	case "float":
		parent = "Float"
	case "complex":
		parent = "Complex"
	}
	bits, err := strconv.Atoi(typeName[open+len(".child(") : len(typeName)-1])
	if err != nil {
		return "", 0, false
	}
	return parent, bits, true
}

func isAllowedRuntimeChildType(parent string, bits int) bool {
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

func valueMatchesRuntimeChildType(value Value, spec runtimeChildTypeSpec) bool {
	switch spec.Parent {
	case "Int":
		if value.Kind != ValueInt {
			return false
		}
		min, max := runtimeSignedBitRange(spec.Bits)
		current := int64(value.Data.(int))
		return current >= min && current <= max
	case "UInt":
		if value.Kind != ValueInt || value.Data.(int) < 0 {
			return false
		}
		return uint64(value.Data.(int)) <= runtimeUnsignedBitMax(spec.Bits)
	case "Float":
		return value.Kind == ValueFloat || value.Kind == ValueInt
	case "Complex":
		return value.Kind == ValueComplex
	default:
		return false
	}
}

func runtimeSignedBitRange(bits int) (int64, int64) {
	if bits >= 64 {
		return math.MinInt64, math.MaxInt64
	}
	max := int64(1)<<(bits-1) - 1
	return -int64(1) << (bits - 1), max
}

func runtimeUnsignedBitMax(bits int) uint64 {
	if bits >= 64 {
		return math.MaxUint64
	}
	return uint64(1)<<bits - 1
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

func parseForEachHeader(expr parser.Expression) (string, parser.Expression, bool) {
	tokens := expr.Tokens
	if len(tokens) < 3 || tokens[0].Type != lexer.TokenIdentifier || tokens[1].Type != lexer.TokenIn {
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
