package parser

import (
	"fmt"

	"kLang/src/lexer"
)

type destructuringLowerer struct {
	nextTemp int
}

func lowerDestructuringStatements(statements []Statement) []Statement {
	lowerer := &destructuringLowerer{}
	return lowerer.lowerStatements(statements)
}

func (lowerer *destructuringLowerer) lowerStatements(statements []Statement) []Statement {
	lowered := make([]Statement, 0, len(statements))
	for _, stmt := range statements {
		lowered = append(lowered, lowerer.lowerStatement(stmt)...)
	}
	return lowered
}

func (lowerer *destructuringLowerer) lowerStatement(stmt Statement) []Statement {
	switch current := stmt.(type) {
	case DestructuringStatement:
		return lowerer.lowerDestructuringStatement(current)
	case FunctionStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	case AliasFunctionStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		current.Methods = lowerer.lowerFunctions(current.Methods)
		return []Statement{current}
	case ExtensionStatement:
		current.Methods = lowerer.lowerFunctions(current.Methods)
		return []Statement{current}
	case NamespaceStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	case ImplStatement:
		current.Methods = lowerer.lowerFunctions(current.Methods)
		return []Statement{current}
	case IfStatement:
		current.Consequence = lowerer.lowerStatements(current.Consequence)
		current.Alternative = lowerer.lowerStatements(current.Alternative)
		if current.ElseIf != nil {
			lowered := lowerer.lowerStatement(*current.ElseIf)
			if len(lowered) == 1 {
				if elseIf, ok := lowered[0].(IfStatement); ok {
					current.ElseIf = &elseIf
				}
			}
		}
		return []Statement{current}
	case MatchStatement:
		for index := range current.Cases {
			current.Cases[index].Body = lowerer.lowerStatements(current.Cases[index].Body)
		}
		return []Statement{current}
	case LoopStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	case TryCatchStatement:
		current.TryBody = lowerer.lowerStatements(current.TryBody)
		current.CatchBody = lowerer.lowerStatements(current.CatchBody)
		return []Statement{current}
	case TransactionStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	case DeferStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		if current.Stmt != nil {
			lowered := lowerer.lowerStatement(current.Stmt)
			if len(lowered) == 1 {
				current.Stmt = lowered[0]
			} else if len(lowered) > 1 {
				current.Stmt = nil
				current.Body = lowered
			}
		}
		return []Statement{current}
	case RunStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		if current.Stmt != nil {
			lowered := lowerer.lowerStatement(current.Stmt)
			if len(lowered) == 1 {
				current.Stmt = lowered[0]
			} else if len(lowered) > 1 {
				current.Stmt = nil
				current.Body = lowered
			}
		}
		return []Statement{current}
	case PrivateBlockStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	case ScopeStatement:
		current.Body = lowerer.lowerStatements(current.Body)
		return []Statement{current}
	default:
		return []Statement{stmt}
	}
}

func (lowerer *destructuringLowerer) lowerFunctions(functions []FunctionStatement) []FunctionStatement {
	for index := range functions {
		functions[index].Body = lowerer.lowerStatements(functions[index].Body)
	}
	return functions
}

func (lowerer *destructuringLowerer) lowerDestructuringStatement(stmt DestructuringStatement) []Statement {
	tempName := lowerer.tempName()
	lowered := []Statement{
		VariableStatement{
			Pos:        stmt.Pos,
			Scope:      "local",
			Inferred:   true,
			Mutable:    false,
			Lazy:       stmt.Lazy,
			Temporary:  true,
			Type:       "T",
			Name:       tempName,
			Expression: stmt.Expression,
		},
	}
	lowerer.lowerPattern(stmt, stmt.Pattern, identifierExpression(tempName), &lowered)
	return lowered
}

func (lowerer *destructuringLowerer) lowerPattern(stmt DestructuringStatement, pattern DestructuringPattern, value Expression, lowered *[]Statement) {
	switch current := pattern.(type) {
	case DestructuringBinding:
		binding := VariableStatement{
			Pos:        stmt.Pos,
			Scope:      stmt.Scope,
			Inferred:   true,
			Exported:   stmt.Exported,
			Mutable:    stmt.Mutable,
			Lazy:       stmt.Lazy,
			Temporary:  stmt.Temporary,
			Type:       "T",
			Name:       current.Name,
			Expression: value,
		}
		if isDiscardName(current.Name) {
			binding.Scope = "local"
			binding.Exported = false
			binding.Mutable = false
			binding.Lazy = false
		}
		*lowered = append(*lowered, binding)
	case DestructuringListPattern:
		for index, item := range current.Items {
			access := indexExpression(value, index)
			if _, ok := item.(DestructuringBinding); ok {
				lowerer.lowerPattern(stmt, item, access, lowered)
				continue
			}
			nestedTemp := lowerer.tempName()
			*lowered = append(*lowered, tempVariable(stmt.Pos, nestedTemp, access, stmt.Lazy))
			lowerer.lowerPattern(stmt, item, identifierExpression(nestedTemp), lowered)
		}
	case DestructuringObjectPattern:
		for _, field := range current.Fields {
			access := selectorExpression(value, field.Field)
			if _, ok := field.Pattern.(DestructuringBinding); ok {
				lowerer.lowerPattern(stmt, field.Pattern, access, lowered)
				continue
			}
			nestedTemp := lowerer.tempName()
			*lowered = append(*lowered, tempVariable(stmt.Pos, nestedTemp, access, stmt.Lazy))
			lowerer.lowerPattern(stmt, field.Pattern, identifierExpression(nestedTemp), lowered)
		}
	}
}

func isDiscardName(name string) bool {
	return name == "_"
}

func (lowerer *destructuringLowerer) tempName() string {
	name := fmt.Sprintf("__klang_destructure_%d", lowerer.nextTemp)
	lowerer.nextTemp++
	return name
}

func tempVariable(pos Position, name string, value Expression, lazy bool) VariableStatement {
	return VariableStatement{
		Pos:        pos,
		Scope:      "local",
		Inferred:   true,
		Mutable:    false,
		Lazy:       lazy,
		Temporary:  true,
		Type:       "T",
		Name:       name,
		Expression: value,
	}
}

func identifierExpression(name string) Expression {
	return expressionFromTokens(expressionTokens(name))
}

func indexExpression(target Expression, index int) Expression {
	source := fmt.Sprintf("%s[%d]", target.Literal(), index)
	return expressionFromTokens(expressionTokens(source))
}

func selectorExpression(target Expression, field string) Expression {
	source := target.Literal() + "." + field
	return expressionFromTokens(expressionTokens(source))
}

func expressionTokens(source string) []lexer.Token {
	tokens := lexer.New(source).Tokenize()
	if len(tokens) > 0 && tokens[len(tokens)-1].Type == lexer.TokenEOFDescriptor {
		return tokens[:len(tokens)-1]
	}
	return tokens
}
