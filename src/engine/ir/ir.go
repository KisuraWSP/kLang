package ir

type Position struct {
	File   string
	Line   int
	Column int
}

type Program struct {
	Name       string
	EntryPoint string
	Sources    []Source
	Globals    []Statement
	Structs    []Struct
	Functions  []Function
}

type Source struct {
	Path    string
	MapPath string
	Content string
}

type Struct struct {
	Pos      Position
	Name     string
	Fields   []StructField
	Methods  []StructMethod
	Required int
}

type StructField struct {
	Binding    Binding
	Default    Expression
	HasDefault bool
	JSONName   string
}

type StructMethod struct {
	Name     string
	Function string
}

type Function struct {
	Pos        Position
	Name       string
	Params     []Binding
	ReturnType string
	Body       []Statement
}

type Binding struct {
	Name    string
	Type    string
	Mutable bool
}

type StatementKind string

const (
	StatementVariable   StatementKind = "variable"
	StatementAssignment StatementKind = "assignment"
	StatementExpression StatementKind = "expression"
	StatementReturn     StatementKind = "return"
	StatementIf         StatementKind = "if"
	StatementWhile      StatementKind = "while"
	StatementRange      StatementKind = "range"
	StatementBreak      StatementKind = "break"
	StatementContinue   StatementKind = "continue"
	StatementThrow      StatementKind = "throw"
	StatementAssert     StatementKind = "assert"
)

type Statement struct {
	Pos       Position
	Kind      StatementKind
	Binding   Binding
	Operator  string
	Target    Expression
	Value     Expression
	Condition Expression
	Body      []Statement
	Else      []Statement
}

type ExpressionKind string

const (
	ExpressionLiteral       ExpressionKind = "literal"
	ExpressionIdentifier    ExpressionKind = "identifier"
	ExpressionUnary         ExpressionKind = "unary"
	ExpressionBinary        ExpressionKind = "binary"
	ExpressionCall          ExpressionKind = "call"
	ExpressionIndex         ExpressionKind = "index"
	ExpressionSelector      ExpressionKind = "selector"
	ExpressionList          ExpressionKind = "list"
	ExpressionComprehension ExpressionKind = "comprehension"
	ExpressionConditional   ExpressionKind = "conditional"
	ExpressionCast          ExpressionKind = "cast"
)

type Expression struct {
	Kind        ExpressionKind
	Type        string
	Value       string
	Name        string
	Operator    string
	Left        *Expression
	Right       *Expression
	Condition   *Expression
	Consequence *Expression
	Alternative *Expression
	Arguments   []Expression
}
