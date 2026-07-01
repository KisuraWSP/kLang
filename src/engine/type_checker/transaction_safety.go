package typechecker

import (
	"fmt"

	"kLang/src/diagnostic"
	"kLang/src/lexer"
	"kLang/src/parser"
)

// transactionRetrySafeCalls contains builtins whose evaluation is deterministic
// and has no externally visible effect, plus the Atomic operations interpreted
// by the STM runtime. User functions stay excluded until kLang has effect
// annotations that can prove a function retry-safe.
var transactionRetrySafeCalls = map[string]bool{
	"atomic_load": true, "atomic_store": true, "atomic_add": true,
	"len": true, "range": true, "Some": true, "None": true, "Ok": true, "Err": true,
	"Atom": true, "Set": true, "Table": true,
	"option_unwrap_or": true, "result_unwrap_or": true,
	"set_has": true, "table_has": true, "has_key": true,
}

func (checker *TypeChecker) addTransactionSafetyError(fn functionSymbol, line int, message string) {
	checker.addStructuredError(
		fn.File, line, 0, 0,
		diagnostic.CodeTransactionSafety,
		"transaction retry safety",
		message,
		"A transaction may execute more than once after a conflict. Move observable effects after the transaction and communicate through Atomic values.",
		"", "",
	)
}

func (checker *TypeChecker) checkTransactionSafety(fn functionSymbol, statements []parser.Statement) {
	for _, stmt := range statements {
		line := semanticLine(fn, stmt.Position())
		switch current := stmt.(type) {
		case parser.VariableStatement:
			if current.Mutable || current.Scope == "global" || current.Exported {
				checker.addTransactionSafetyError(fn, line, "transaction local declarations must be immutable and cannot define global state")
			}
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.MultiVariableStatement:
			if current.Mutable || current.Scope == "global" || current.Exported {
				checker.addTransactionSafetyError(fn, line, "transaction local declarations must be immutable and cannot define global state")
			}
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.AssignmentStatement:
			checker.addTransactionSafetyError(fn, line, "transaction cannot mutate ordinary bindings; use atomic_store or atomic_add")
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.ReturnStatement:
			checker.addTransactionSafetyError(fn, line, "return is not allowed inside transaction")
		case parser.DeferStatement:
			checker.addTransactionSafetyError(fn, line, "defer is not retry-safe and is not allowed inside transaction")
		case parser.RunStatement:
			checker.addTransactionSafetyError(fn, line, "run is not retry-safe and is not allowed inside transaction")
		case parser.ReportStatement:
			checker.addTransactionSafetyError(fn, line, "report is not retry-safe and is not allowed inside transaction")
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.FunctionStatement, parser.AliasFunctionStatement, parser.ExtensionStatement:
			checker.addTransactionSafetyError(fn, line, "declarations with executable bodies are not allowed inside transaction")
		case parser.ExpressionStatement:
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.ThrowStatement:
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.AssertStatement:
			checker.checkTransactionExpression(fn, current.Expression.Node, line)
		case parser.IfStatement:
			checker.checkTransactionExpression(fn, current.Condition.Node, line)
			checker.checkTransactionSafety(fn, current.Consequence)
			if current.ElseIf != nil {
				checker.checkTransactionSafety(fn, []parser.Statement{*current.ElseIf})
			}
			checker.checkTransactionSafety(fn, current.Alternative)
		case parser.MatchStatement:
			checker.checkTransactionExpression(fn, current.Value.Node, line)
			for _, matchCase := range current.Cases {
				checker.checkTransactionSafety(fn, matchCase.Body)
			}
		case parser.LoopStatement:
			checker.checkTransactionExpression(fn, current.Header.Node, line)
			checker.checkTransactionSafety(fn, current.Body)
		case parser.TryCatchStatement:
			checker.checkTransactionSafety(fn, current.TryBody)
			checker.checkTransactionSafety(fn, current.CatchBody)
		case parser.TransactionStatement:
			// The scope walk checks every nested transaction independently.
		case parser.PrivateBlockStatement:
			checker.checkTransactionSafety(fn, current.Body)
		case parser.ScopeStatement:
			checker.checkTransactionSafety(fn, current.Body)
		}
	}
}

func (checker *TypeChecker) checkTransactionExpression(fn functionSymbol, node parser.ExpressionNode, line int) {
	switch current := node.(type) {
	case nil, parser.IdentifierExpression, parser.LiteralExpression:
		return
	case parser.RawExpression:
		for index, token := range current.Tokens {
			if token.Type == lexer.TokenAwait {
				checker.addTransactionSafetyError(fn, line, "await is not retry-safe and is not allowed inside transaction")
			}
			if token.Type == lexer.TokenIdentifier && index+1 < len(current.Tokens) &&
				current.Tokens[index+1].Type == lexer.TokenLeftBrace &&
				!transactionRetrySafeCalls[token.Literal] {
				checker.addTransactionSafetyError(fn, line, fmt.Sprintf("%s call is not proven retry-safe inside transaction", token.Literal))
			}
		}
	case parser.UnaryExpression:
		if current.Operator == "await" {
			checker.addTransactionSafetyError(fn, line, "await is not retry-safe and is not allowed inside transaction")
		}
		checker.checkTransactionExpression(fn, current.Right, line)
	case parser.BinaryExpression:
		checker.checkTransactionExpression(fn, current.Left, line)
		checker.checkTransactionExpression(fn, current.Right, line)
	case parser.CallExpression:
		name := ""
		if callee, ok := current.Callee.(parser.IdentifierExpression); ok {
			name = callee.Name
		}
		if !transactionRetrySafeCalls[name] {
			if name == "" {
				name = "method or computed function"
			}
			checker.addTransactionSafetyError(fn, line, fmt.Sprintf("%s call is not proven retry-safe inside transaction", name))
		}
		checker.checkTransactionExpression(fn, current.Callee, line)
		for _, argument := range current.Arguments {
			checker.checkTransactionExpression(fn, argument, line)
		}
	case parser.IndexExpression:
		checker.checkTransactionExpression(fn, current.Target, line)
		checker.checkTransactionExpression(fn, current.Index, line)
	case parser.SelectorExpression:
		checker.checkTransactionExpression(fn, current.Target, line)
	case parser.CastExpression:
		checker.checkTransactionExpression(fn, current.Value, line)
	case parser.NullCheckExpression:
		checker.checkTransactionExpression(fn, current.Value, line)
	case parser.PropagateExpression:
		checker.checkTransactionExpression(fn, current.Value, line)
	case parser.ConditionalExpression:
		checker.checkTransactionExpression(fn, current.Condition, line)
		checker.checkTransactionExpression(fn, current.Consequence, line)
		checker.checkTransactionExpression(fn, current.Alternative, line)
	case parser.ListExpression:
		for _, item := range current.Items {
			checker.checkTransactionExpression(fn, item, line)
		}
	case parser.ListComprehensionExpression:
		checker.checkTransactionExpression(fn, current.Value, line)
		checker.checkTransactionExpression(fn, current.Iterable, line)
		checker.checkTransactionExpression(fn, current.Condition, line)
	case parser.MapExpression:
		for _, entry := range current.Entries {
			checker.checkTransactionExpression(fn, entry.Key, line)
			checker.checkTransactionExpression(fn, entry.Value, line)
		}
	case parser.GroupExpression:
		checker.checkTransactionExpression(fn, current.Inner, line)
	case parser.LambdaExpression:
		checker.addTransactionSafetyError(fn, line, "lambda creation is not allowed inside transaction")
	}
}
