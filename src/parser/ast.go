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

type AliasStatement struct {
	Pos    Position
	Name   string
	Target string
}

type RegionStatement struct {
	Pos      Position
	Name     string
	TypeName string
	Size     Expression
	Count    Expression
}

type AliasFunctionStatement struct {
	Pos        Position
	Name       string
	TypeParams []TypeParameter
	Params     []Parameter
	ReturnType string
	Hooks      []AliasHook
	Methods    []FunctionStatement
}

type AliasHook struct {
	Name string
	Body []lexer.Token
}

type NamespaceStatement struct {
	Pos  Position
	Name string
	Body []Statement
}

type TraitStatement struct {
	Pos     Position
	Name    string
	Methods []TraitMethod
}

type TraitMethod struct {
	Pos        Position
	Name       string
	Params     []Parameter
	ReturnType string
}

type ImplStatement struct {
	Pos     Position
	Trait   string
	Type    string
	Methods []FunctionStatement
}

type FunctionGroupStatement struct {
	Pos       Position
	Name      string
	Functions []string
}

type FunctionStatement struct {
	Pos                Position
	Name               string
	TypeParams         []TypeParameter
	Params             []Parameter
	ReturnType         string
	Lazy               bool
	Async              bool
	Inner              bool
	Deprecated         bool
	DeprecationMessage string
	Body               []Statement
}

type TypeParameter struct {
	Name string
	Type string
}

type Parameter struct {
	Name    string
	Type    string
	Default Expression
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

type ThrowStatement struct {
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

type TryCatchStatement struct {
	Pos       Position
	TryBody   []Statement
	ErrorName string
	CatchBody []Statement
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

type PropagateExpression struct {
	Value ExpressionNode
}

type ConditionalExpression struct {
	Condition   ExpressionNode
	Consequence ExpressionNode
	Alternative ExpressionNode
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

type LambdaExpression struct {
	Params     []Parameter
	ReturnType string
	Body       []Statement
}

type RawExpression struct {
	Tokens []lexer.Token
}

func (stmt ImportStatement) statementNode() {}
func (stmt AliasStatement) statementNode()  {}
func (stmt RegionStatement) statementNode() {}
func (stmt AliasFunctionStatement) statementNode() {
}
func (stmt NamespaceStatement) statementNode() {}
func (stmt TraitStatement) statementNode()     {}
func (stmt ImplStatement) statementNode()      {}
func (stmt FunctionGroupStatement) statementNode() {
}
func (stmt FunctionStatement) statementNode()   {}
func (stmt VariableStatement) statementNode()   {}
func (stmt ReturnStatement) statementNode()     {}
func (stmt ThrowStatement) statementNode()      {}
func (stmt BreakStatement) statementNode()      {}
func (stmt AssignmentStatement) statementNode() {}
func (stmt ExpressionStatement) statementNode() {}
func (stmt IfStatement) statementNode()         {}
func (stmt LoopStatement) statementNode()       {}
func (stmt TryCatchStatement) statementNode()   {}

func (stmt ImportStatement) Position() Position { return stmt.Pos }
func (stmt AliasStatement) Position() Position  { return stmt.Pos }
func (stmt RegionStatement) Position() Position { return stmt.Pos }
func (stmt AliasFunctionStatement) Position() Position {
	return stmt.Pos
}
func (stmt NamespaceStatement) Position() Position { return stmt.Pos }
func (stmt TraitStatement) Position() Position     { return stmt.Pos }
func (stmt ImplStatement) Position() Position      { return stmt.Pos }
func (stmt FunctionGroupStatement) Position() Position {
	return stmt.Pos
}
func (stmt FunctionStatement) Position() Position   { return stmt.Pos }
func (stmt VariableStatement) Position() Position   { return stmt.Pos }
func (stmt ReturnStatement) Position() Position     { return stmt.Pos }
func (stmt ThrowStatement) Position() Position      { return stmt.Pos }
func (stmt BreakStatement) Position() Position      { return stmt.Pos }
func (stmt AssignmentStatement) Position() Position { return stmt.Pos }
func (stmt ExpressionStatement) Position() Position { return stmt.Pos }
func (stmt IfStatement) Position() Position         { return stmt.Pos }
func (stmt LoopStatement) Position() Position       { return stmt.Pos }
func (stmt TryCatchStatement) Position() Position   { return stmt.Pos }

func (expr IdentifierExpression) expressionNode()  {}
func (expr LiteralExpression) expressionNode()     {}
func (expr UnaryExpression) expressionNode()       {}
func (expr BinaryExpression) expressionNode()      {}
func (expr CallExpression) expressionNode()        {}
func (expr IndexExpression) expressionNode()       {}
func (expr SelectorExpression) expressionNode()    {}
func (expr CastExpression) expressionNode()        {}
func (expr NullCheckExpression) expressionNode()   {}
func (expr PropagateExpression) expressionNode()   {}
func (expr ConditionalExpression) expressionNode() {}
func (expr ListExpression) expressionNode()        {}
func (expr ListComprehensionExpression) expressionNode() {
}
func (expr MapExpression) expressionNode()   {}
func (expr GroupExpression) expressionNode() {}
func (expr LambdaExpression) expressionNode() {
}
func (expr RawExpression) expressionNode() {}

func (expr Expression) Literal() string {
	parts := make([]string, 0, len(expr.Tokens))
	for _, token := range expr.Tokens {
		parts = append(parts, token.Literal)
	}
	return strings.Join(parts, " ")
}
