package runtime

import (
	"fmt"
	"strconv"

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

func applyAssignmentOperator(left Value, operator string, right Value) Value {
	switch operator {
	case "=":
		return right
	case "+=":
		return mustValue(numericOrString(left, right, func(a, b float64) float64 { return a + b }))
	case "-=":
		return numericBinary(left, right, func(a, b float64) float64 { return a - b })
	case "*=":
		return numericBinary(left, right, func(a, b float64) float64 { return a * b })
	case "/=":
		return numericBinary(left, right, func(a, b float64) float64 { return a / b })
	default:
		return right
	}
}

func numericOrString(left Value, right Value, op func(float64, float64) float64) (Value, error) {
	if left.Kind == ValueString || right.Kind == ValueString {
		return StringValue(valueString(left) + valueString(right)), nil
	}
	return numericBinary(left, right, op), nil
}

func numericBinary(left Value, right Value, op func(float64, float64) float64) Value {
	if left.Kind == ValueFloat || right.Kind == ValueFloat {
		return FloatValue(op(asFloat(left), asFloat(right)))
	}
	return IntValue(int(op(float64(asInt(left)), float64(asInt(right)))))
}

func mustValue(value Value, _ error) Value {
	return value
}

func asInt(value Value) int {
	switch value.Kind {
	case ValueInt:
		return value.Data.(int)
	case ValueFloat:
		return int(value.Data.(float64))
	case ValueBool:
		if value.Data.(bool) {
			return 1
		}
		return 0
	default:
		return 0
	}
}

func asFloat(value Value) float64 {
	switch value.Kind {
	case ValueFloat:
		return value.Data.(float64)
	case ValueInt:
		return float64(value.Data.(int))
	case ValueBool:
		if value.Data.(bool) {
			return 1
		}
		return 0
	default:
		return 0
	}
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

func valueLen(value Value) int {
	switch value.Kind {
	case ValueString:
		return len(value.Data.(string))
	case ValueList:
		return len(value.Data.([]Value))
	case ValueMap:
		return len(value.Data.(map[string]Value))
	default:
		return 0
	}
}

func mapKey(value Value) string {
	return valueString(value)
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
