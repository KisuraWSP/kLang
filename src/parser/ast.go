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
	Pos        Position
	Name       string
	Params     []Parameter
	ReturnType string
	Body       []Statement
}

type Parameter struct {
	Name string
	Type string
}

type VariableStatement struct {
	Pos        Position
	Scope      string
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

func (expr Expression) Literal() string {
	parts := make([]string, 0, len(expr.Tokens))
	for _, token := range expr.Tokens {
		parts = append(parts, token.Literal)
	}
	return strings.Join(parts, " ")
}
