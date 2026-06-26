package parser

import (
	"strings"
	"unicode"

	"kLang/src/lexer"
)

func discoverTypeAliases(tokens []lexer.Token) map[string]string {
	aliases := map[string]string{}
	for index := 0; index+3 < len(tokens); index++ {
		if tokens[index].Type != lexer.TokenIdentifier || tokens[index].Literal != "type" ||
			tokens[index+1].Type != lexer.TokenIdentifier || tokens[index+2].Type != lexer.TokenAssign {
			continue
		}
		var target strings.Builder
		for cursor := index + 3; cursor < len(tokens) && tokens[cursor].Type != lexer.TokenSemicolon && tokens[cursor].Type != lexer.TokenEOFDescriptor; cursor++ {
			target.WriteString(tokens[cursor].Literal)
		}
		if target.Len() != 0 {
			aliases[tokens[index+1].Literal] = target.String()
		}
	}
	return aliases
}

func cloneTypeAliases(aliases map[string]string) map[string]string {
	cloned := make(map[string]string, len(aliases))
	for name, target := range aliases {
		cloned[name] = target
	}
	return cloned
}

func resolveTypeAlias(typeName string, aliases map[string]string, stack map[string]bool) (string, bool) {
	if len(aliases) == 0 || strings.TrimSpace(typeName) == "" {
		return typeName, false
	}
	if stack == nil {
		stack = map[string]bool{}
	}
	var output strings.Builder
	cycle := false
	for index := 0; index < len(typeName); {
		current := rune(typeName[index])
		if !isTypeAliasIdentifierStart(current) {
			output.WriteByte(typeName[index])
			index++
			continue
		}
		end := index + 1
		for end < len(typeName) && isTypeAliasIdentifierPart(rune(typeName[end])) {
			end++
		}
		name := typeName[index:end]
		target, exists := aliases[name]
		if !exists {
			output.WriteString(name)
			index = end
			continue
		}
		if stack[name] {
			output.WriteString(name)
			cycle = true
			index = end
			continue
		}
		nextStack := make(map[string]bool, len(stack)+1)
		for item, active := range stack {
			nextStack[item] = active
		}
		nextStack[name] = true
		resolved, nestedCycle := resolveTypeAlias(target, aliases, nextStack)
		output.WriteString(resolved)
		cycle = cycle || nestedCycle
		index = end
	}
	return output.String(), cycle
}

func ResolveTypeAlias(typeName string, aliases map[string]string) (string, bool) {
	return resolveTypeAlias(typeName, aliases, nil)
}

func isTypeAliasIdentifierStart(value rune) bool {
	return value == '_' || unicode.IsLetter(value)
}

func isTypeAliasIdentifierPart(value rune) bool {
	return isTypeAliasIdentifierStart(value) || unicode.IsDigit(value)
}

func resolveExpressionTypeAliases(node ExpressionNode, aliases map[string]string) ExpressionNode {
	resolve := func(typeName string) string {
		resolved, _ := resolveTypeAlias(typeName, aliases, nil)
		return resolved
	}
	switch current := node.(type) {
	case UnaryExpression:
		current.Right = resolveExpressionTypeAliases(current.Right, aliases)
		return current
	case BinaryExpression:
		current.Left = resolveExpressionTypeAliases(current.Left, aliases)
		current.Right = resolveExpressionTypeAliases(current.Right, aliases)
		return current
	case CallExpression:
		current.Callee = resolveExpressionTypeAliases(current.Callee, aliases)
		for index := range current.Arguments {
			current.Arguments[index] = resolveExpressionTypeAliases(current.Arguments[index], aliases)
		}
		return current
	case IndexExpression:
		current.Target = resolveExpressionTypeAliases(current.Target, aliases)
		current.Index = resolveExpressionTypeAliases(current.Index, aliases)
		return current
	case SelectorExpression:
		current.Target = resolveExpressionTypeAliases(current.Target, aliases)
		return current
	case CastExpression:
		current.Value = resolveExpressionTypeAliases(current.Value, aliases)
		current.Type = resolve(current.Type)
		return current
	case NullCheckExpression:
		current.Value = resolveExpressionTypeAliases(current.Value, aliases)
		return current
	case PropagateExpression:
		current.Value = resolveExpressionTypeAliases(current.Value, aliases)
		return current
	case ConditionalExpression:
		current.Condition = resolveExpressionTypeAliases(current.Condition, aliases)
		current.Consequence = resolveExpressionTypeAliases(current.Consequence, aliases)
		current.Alternative = resolveExpressionTypeAliases(current.Alternative, aliases)
		return current
	case ListExpression:
		for index := range current.Items {
			current.Items[index] = resolveExpressionTypeAliases(current.Items[index], aliases)
		}
		return current
	case ListComprehensionExpression:
		current.Value = resolveExpressionTypeAliases(current.Value, aliases)
		current.Iterable = resolveExpressionTypeAliases(current.Iterable, aliases)
		current.Condition = resolveExpressionTypeAliases(current.Condition, aliases)
		return current
	case MapExpression:
		for index := range current.Entries {
			current.Entries[index].Key = resolveExpressionTypeAliases(current.Entries[index].Key, aliases)
			current.Entries[index].Value = resolveExpressionTypeAliases(current.Entries[index].Value, aliases)
		}
		return current
	case GroupExpression:
		current.Inner = resolveExpressionTypeAliases(current.Inner, aliases)
		return current
	case LambdaExpression:
		for index := range current.TypeParams {
			current.TypeParams[index].Type = resolve(current.TypeParams[index].Type)
		}
		for index := range current.Params {
			current.Params[index].Type = resolve(current.Params[index].Type)
			current.Params[index].Default.Node = resolveExpressionTypeAliases(current.Params[index].Default.Node, aliases)
		}
		current.ReturnType = resolve(current.ReturnType)
		current.Body = resolveStatementTypeAliases(current.Body, aliases)
		return current
	default:
		return node
	}
}

func resolveStatementTypeAliases(statements []Statement, aliases map[string]string) []Statement {
	resolve := func(typeName string) string {
		resolved, _ := resolveTypeAlias(typeName, aliases, nil)
		return resolved
	}
	for index, stmt := range statements {
		switch current := stmt.(type) {
		case VariableStatement:
			current.Type = resolve(current.Type)
			current.Expression.Node = resolveExpressionTypeAliases(current.Expression.Node, aliases)
			statements[index] = current
		case MultiVariableStatement:
			for binding := range current.Bindings {
				current.Bindings[binding].Type = resolve(current.Bindings[binding].Type)
			}
			current.Expression.Node = resolveExpressionTypeAliases(current.Expression.Node, aliases)
			statements[index] = current
		case ReturnStatement:
			current.Expression.Node = resolveExpressionTypeAliases(current.Expression.Node, aliases)
			for value := range current.Values {
				current.Values[value].Node = resolveExpressionTypeAliases(current.Values[value].Node, aliases)
			}
			statements[index] = current
		case ExpressionStatement:
			current.Expression.Node = resolveExpressionTypeAliases(current.Expression.Node, aliases)
			statements[index] = current
		case AssignmentStatement:
			current.Target.Node = resolveExpressionTypeAliases(current.Target.Node, aliases)
			current.Expression.Node = resolveExpressionTypeAliases(current.Expression.Node, aliases)
			statements[index] = current
		case IfStatement:
			current.Condition.Node = resolveExpressionTypeAliases(current.Condition.Node, aliases)
			current.Consequence = resolveStatementTypeAliases(current.Consequence, aliases)
			current.Alternative = resolveStatementTypeAliases(current.Alternative, aliases)
			if current.ElseIf != nil {
				resolved := resolveStatementTypeAliases([]Statement{*current.ElseIf}, aliases)[0].(IfStatement)
				current.ElseIf = &resolved
			}
			statements[index] = current
		case LoopStatement:
			current.Header.Node = resolveExpressionTypeAliases(current.Header.Node, aliases)
			current.Body = resolveStatementTypeAliases(current.Body, aliases)
			statements[index] = current
		case PrivateBlockStatement:
			current.Body = resolveStatementTypeAliases(current.Body, aliases)
			statements[index] = current
		case ScopeStatement:
			current.Body = resolveStatementTypeAliases(current.Body, aliases)
			statements[index] = current
		}
	}
	return statements
}
