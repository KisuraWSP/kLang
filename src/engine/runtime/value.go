package runtime

import (
	"fmt"
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
		if strings.HasPrefix(typeName, "List[") || strings.HasPrefix(typeName, "Map[") {
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
	return numericBinary(left, right, op)
}

func numericBinary(left Value, right Value, op func(float64, float64) float64) (Value, error) {
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("numeric operation requires Int or Float, got %s and %s", left.Kind, right.Kind)}
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
	if !isNumeric(left) || !isNumeric(right) {
		return NullValue(), Error{Message: fmt.Sprintf("division requires Int or Float, got %s and %s", left.Kind, right.Kind)}
	}
	rightFloat, _ := asFloat(right)
	if rightFloat == 0 {
		return NullValue(), Error{Message: "division by zero"}
	}
	return numericBinary(left, right, func(a, b float64) float64 { return a / b })
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
	default:
		return fmt.Sprintf("%v", value.Data)
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
	switch {
	case typeName == "" || typeName == "T":
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
	case strings.HasPrefix(typeName, "List["):
		return value.Kind == ValueList
	case strings.HasPrefix(typeName, "Map["):
		return value.Kind == ValueMap
	default:
		return true
	}
}

func listElementType(typeName string) (string, bool) {
	typeName = strings.TrimSpace(typeName)
	if !strings.HasPrefix(typeName, "List[") || !strings.HasSuffix(typeName, "]") {
		return "", false
	}
	elementType := strings.TrimSpace(typeName[len("List[") : len(typeName)-1])
	return elementType, elementType != "" && elementType != "...$Items"
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
