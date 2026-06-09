package parser

import (
	"strings"

	"kLang/src/lexer"
)

type Program struct {
	Statements []Statement
}

type Position struct {
	Line   int
	Column int
}

type Node interface {
	Position() Position
}

type Statement interface {
	Node
	statementNode()
}

type ImportStatement struct {
	Pos  Position
	Path string
}

type NamespaceStatement struct {
	Pos  Position
	Name string
	Body []Statement
}

type FunctionStatement struct {
	Pos                Position
	Name               string
	Params             []Parameter
	ReturnType         string
	Deprecated         bool
	DeprecationMessage string
	Body               []Statement
}

type Parameter struct {
	Name string
	Type string
}

type VariableStatement struct {
	Pos        Position
	Scope      string
	Exported   bool
	Mutable    bool
	Type       string
	Name       string
	Expression Expression
}

type ReturnStatement struct {
	Pos        Position
	Expression Expression
}

type BreakStatement struct {
	Pos Position
}

type AssignmentStatement struct {
	Pos        Position
	Target     Expression
	Operator   string
	Expression Expression
}

type ExpressionStatement struct {
	Pos        Position
	Expression Expression
}

type IfStatement struct {
	Pos         Position
	Kind        string
	Condition   Expression
	Consequence []Statement
	Alternative []Statement
	ElseIf      *IfStatement
}

type LoopStatement struct {
	Pos    Position
	Kind   string
	Header Expression
	Body   []Statement
}

type Expression struct {
	Tokens []lexer.Token
	Node   ExpressionNode
}

type ExpressionNode interface {
	expressionNode()
}

type IdentifierExpression struct {
	Name string
}

type LiteralExpression struct {
	Kind  string
	Value string
}

type UnaryExpression struct {
	Operator string
	Right    ExpressionNode
}

type BinaryExpression struct {
	Left     ExpressionNode
	Operator string
	Right    ExpressionNode
}

type CallExpression struct {
	Callee    ExpressionNode
	Arguments []ExpressionNode
}

type IndexExpression struct {
	Target ExpressionNode
	Index  ExpressionNode
}

type SelectorExpression struct {
	Target ExpressionNode
	Field  string
}

type CastExpression struct {
	Value ExpressionNode
	Type  string
}

type NullCheckExpression struct {
	Value ExpressionNode
}

type ListExpression struct {
	Items []ExpressionNode
}

type ListComprehensionExpression struct {
	Value     ExpressionNode
	Iterator  string
	Iterable  ExpressionNode
	Condition ExpressionNode
}

type MapExpression struct {
	Entries []MapEntry
}

type MapEntry struct {
	Key   ExpressionNode
	Value ExpressionNode
}

type GroupExpression struct {
	Inner ExpressionNode
}

type RawExpression struct {
	Tokens []lexer.Token
}

func (stmt ImportStatement) statementNode()     {}
func (stmt NamespaceStatement) statementNode()  {}
func (stmt FunctionStatement) statementNode()   {}
func (stmt VariableStatement) statementNode()   {}
func (stmt ReturnStatement) statementNode()     {}
func (stmt BreakStatement) statementNode()      {}
func (stmt AssignmentStatement) statementNode() {}
func (stmt ExpressionStatement) statementNode() {}
func (stmt IfStatement) statementNode()         {}
func (stmt LoopStatement) statementNode()       {}

func (stmt ImportStatement) Position() Position     { return stmt.Pos }
func (stmt NamespaceStatement) Position() Position  { return stmt.Pos }
func (stmt FunctionStatement) Position() Position   { return stmt.Pos }
func (stmt VariableStatement) Position() Position   { return stmt.Pos }
func (stmt ReturnStatement) Position() Position     { return stmt.Pos }
func (stmt BreakStatement) Position() Position      { return stmt.Pos }
func (stmt AssignmentStatement) Position() Position { return stmt.Pos }
func (stmt ExpressionStatement) Position() Position { return stmt.Pos }
func (stmt IfStatement) Position() Position         { return stmt.Pos }
func (stmt LoopStatement) Position() Position       { return stmt.Pos }

func (expr IdentifierExpression) expressionNode() {}
func (expr LiteralExpression) expressionNode()    {}
func (expr UnaryExpression) expressionNode()      {}
func (expr BinaryExpression) expressionNode()     {}
func (expr CallExpression) expressionNode()       {}
func (expr IndexExpression) expressionNode()      {}
func (expr SelectorExpression) expressionNode()   {}
func (expr CastExpression) expressionNode()       {}
func (expr NullCheckExpression) expressionNode()  {}
func (expr ListExpression) expressionNode()       {}
func (expr ListComprehensionExpression) expressionNode() {
}
func (expr MapExpression) expressionNode()   {}
func (expr GroupExpression) expressionNode() {}
func (expr RawExpression) expressionNode()   {}

func (expr Expression) Literal() string {
	parts := make([]string, 0, len(expr.Tokens))
	for _, token := range expr.Tokens {
		parts = append(parts, token.Literal)
	}
	return strings.Join(parts, " ")
}
