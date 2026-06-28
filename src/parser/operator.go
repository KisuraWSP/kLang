package parser

var operatorMethodNames = map[string]string{
	"+":  "__operator_add",
	"-":  "__operator_subtract",
	"*":  "__operator_multiply",
	"/":  "__operator_divide",
	"//": "__operator_floor_divide",
	"%":  "__operator_modulo",
	"**": "__operator_power",
	"==": "__operator_equal",
	"!=": "__operator_not_equal",
	">":  "__operator_greater",
	">=": "__operator_greater_equal",
	"<":  "__operator_less",
	"<=": "__operator_less_equal",
}

var operatorSymbols = func() map[string]string {
	result := make(map[string]string, len(operatorMethodNames))
	for symbol, name := range operatorMethodNames {
		result[name] = symbol
	}
	return result
}()

func OperatorMethodName(symbol string) (string, bool) {
	name, ok := operatorMethodNames[symbol]
	return name, ok
}

func OperatorMethodSymbol(name string) (string, bool) {
	symbol, ok := operatorSymbols[name]
	return symbol, ok
}

func IsComparisonOperator(symbol string) bool {
	switch symbol {
	case "==", "!=", ">", ">=", "<", "<=":
		return true
	default:
		return false
	}
}
