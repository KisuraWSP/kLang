package parser

import (
	"fmt"
	"strings"

	"kLang/src/lexer"
)

type Error struct {
	Line    int
	Column  int
	Message string
}

type Parser struct {
	tokens []lexer.Token
	pos    int
	errors []Error
}

func New(tokens []lexer.Token) *Parser {
	return &Parser{tokens: tokens}
}

func Parse(input string) (*Program, []Error) {
	tokens := lexer.New(input).Tokenize()
	parser := New(tokens)
	program := parser.ParseProgram()
	return program, parser.Errors()
}

func (parser *Parser) ParseProgram() *Program {
	program := &Program{}
	for !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}

		stmt := parser.parseStatement()
		if stmt == nil {
			parser.synchronize()
			continue
		}
		program.Statements = append(program.Statements, stmt)
	}
	return program
}

func (parser *Parser) Errors() []Error {
	return parser.errors
}

func (parser *Parser) parseStatement() Statement {
	token := parser.current()
	switch token.Type {
	case lexer.TokenIllegal:
		parser.addError(token, fmt.Sprintf("illegal token %q", token.Literal))
		parser.advance()
		return nil
	case lexer.TokenImport:
		return parser.parseImport()
	case lexer.TokenNameSpace:
		return parser.parseNamespace()
	case lexer.TokenFunc:
		return parser.parseFunction()
	case lexer.TokenGlobal:
		return parser.parseVariable("global")
	case lexer.TokenLocal:
		return parser.parseVariable("local")
	case lexer.TokenReturn:
		return parser.parseReturn()
	case lexer.TokenBreak:
		return parser.parseBreak()
	case lexer.TokenIf:
		return parser.parseCondition("if")
	case lexer.TokenUnless:
		return parser.parseCondition("unless")
	case lexer.TokenWhile:
		return parser.parseLoop("while")
	case lexer.TokenFor:
		return parser.parseLoop("for")
	case lexer.TokenDoWhile, lexer.TokenDo:
		return parser.parseLoop(token.Literal)
	default:
		return parser.parseExpressionOrAssignment()
	}
}

func (parser *Parser) parseImport() Statement {
	start := parser.consume(lexer.TokenImport, "expected import")
	path := parser.consume(lexer.TokenString, "expected import path string")
	parser.consumeOptionalSemicolon()
	return ImportStatement{
		Pos:  positionFromToken(start),
		Path: path.Literal,
	}
}

func (parser *Parser) parseNamespace() Statement {
	start := parser.consume(lexer.TokenNameSpace, "expected namespace")
	name := parser.consume(lexer.TokenIdentifier, "expected namespace name")
	body := parser.parseBlock()
	return NamespaceStatement{
		Pos:  positionFromToken(start),
		Name: name.Literal,
		Body: body,
	}
}

func (parser *Parser) parseFunction() Statement {
	start := parser.consume(lexer.TokenFunc, "expected function")
	name := parser.consume(lexer.TokenIdentifier, "expected function name")
	parser.consume(lexer.TokenLeftBrace, "expected '(' after function name")
	params := parser.parseParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after function parameters")
	parser.consume(lexer.TokenInferReturn, "expected ':' before function return type")
	returnType := parser.parseType()
	body := parser.parseBlock()

	return FunctionStatement{
		Pos:        positionFromToken(start),
		Name:       name.Literal,
		Params:     params,
		ReturnType: returnType,
		Body:       body,
	}
}

func (parser *Parser) parseParameters() []Parameter {
	var params []Parameter
	if parser.check(lexer.TokenRightBrace) || parser.atEnd() {
		return params
	}

	for {
		name := parser.consume(lexer.TokenIdentifier, "expected parameter name")
		parser.consume(lexer.TokenInferReturn, "expected ':' after parameter name")
		typeName := parser.parseType()
		params = append(params, Parameter{Name: name.Literal, Type: typeName})

		if !parser.match(lexer.TokenComma) {
			break
		}
	}

	return params
}

func (parser *Parser) parseVariable(scope string) Statement {
	start := parser.advance()
	mutable := parser.match(lexer.TokenMut)
	typeName := parser.parseType()
	name := parser.consume(lexer.TokenIdentifier, "expected variable name")
	parser.consume(lexer.TokenAssign, "expected '=' in variable declaration")
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()

	return VariableStatement{
		Pos:        positionFromToken(start),
		Scope:      scope,
		Mutable:    mutable,
		Type:       typeName,
		Name:       name.Literal,
		Expression: expr,
	}
}

func (parser *Parser) parseReturn() Statement {
	start := parser.consume(lexer.TokenReturn, "expected return")
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	return ReturnStatement{
		Pos:        positionFromToken(start),
		Expression: expr,
	}
}

func (parser *Parser) parseBreak() Statement {
	start := parser.consume(lexer.TokenBreak, "expected break")
	parser.consumeOptionalSemicolon()
	return BreakStatement{Pos: positionFromToken(start)}
}

func (parser *Parser) parseCondition(kind string) Statement {
	start := parser.advance()
	condition := parser.parseExpressionUntil(lexer.TokenScopeBegin, lexer.TokenSemicolon)
	var consequence []Statement
	if parser.check(lexer.TokenScopeBegin) {
		consequence = parser.parseBlock()
	} else {
		parser.consumeOptionalSemicolon()
		condition, consequence = parseInlineCondition(condition)
	}

	stmt := IfStatement{
		Pos:         positionFromToken(start),
		Kind:        kind,
		Condition:   condition,
		Consequence: consequence,
	}

	if parser.match(lexer.TokenElse) {
		if parser.check(lexer.TokenIf) {
			elseIf := parser.parseCondition("if")
			if parsed, ok := elseIf.(IfStatement); ok {
				stmt.ElseIf = &parsed
			}
		} else if parser.check(lexer.TokenScopeBegin) {
			stmt.Alternative = parser.parseBlock()
		} else {
			expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
			parser.consumeOptionalSemicolon()
			stmt.Alternative = parseInlineStatements(expr.Tokens)
		}
	}

	return stmt
}

func parseInlineCondition(expr Expression) (Expression, []Statement) {
	index := inlineStatementStart(expr.Tokens)
	if index == -1 {
		return expr, nil
	}
	return Expression{Tokens: expr.Tokens[:index]}, parseInlineStatements(expr.Tokens[index:])
}

func parseInlineStatements(tokens []lexer.Token) []Statement {
	if len(tokens) == 0 {
		return nil
	}
	inlineTokens := append([]lexer.Token{}, tokens...)
	inlineTokens = append(inlineTokens, lexer.Token{Type: lexer.TokenEOFDescriptor})
	inlineParser := New(inlineTokens)
	stmt := inlineParser.parseStatement()
	if stmt == nil {
		return nil
	}
	return []Statement{stmt}
}

func inlineStatementStart(tokens []lexer.Token) int {
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenBreak, lexer.TokenReturn, lexer.TokenLocal, lexer.TokenGlobal, lexer.TokenCall:
			return index
		}
	}
	return -1
}

func (parser *Parser) parseLoop(kind string) Statement {
	start := parser.advance()
	header := parser.parseExpressionUntil(lexer.TokenScopeBegin)
	body := parser.parseBlock()
	return LoopStatement{
		Pos:    positionFromToken(start),
		Kind:   kind,
		Header: header,
		Body:   body,
	}
}

func (parser *Parser) parseExpressionOrAssignment() Statement {
	start := parser.current()
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	if len(expr.Tokens) == 0 {
		return nil
	}

	if index := assignmentOperatorIndex(expr.Tokens); index != -1 {
		target := Expression{Tokens: expr.Tokens[:index]}
		target.Node = parseExpressionNode(target.Tokens)
		value := Expression{Tokens: expr.Tokens[index+1:]}
		value.Node = parseExpressionNode(value.Tokens)
		return AssignmentStatement{
			Pos:        positionFromToken(start),
			Target:     target,
			Operator:   expr.Tokens[index].Literal,
			Expression: value,
		}
	}

	return ExpressionStatement{
		Pos:        positionFromToken(start),
		Expression: expr,
	}
}

func (parser *Parser) parseBlock() []Statement {
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start block")
	var statements []Statement
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		stmt := parser.parseStatement()
		if stmt == nil {
			parser.synchronize()
			continue
		}
		statements = append(statements, stmt)
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close block")
	return statements
}

func (parser *Parser) parseType() string {
	if parser.atEnd() {
		parser.addError(parser.previous(), "expected type")
		return ""
	}

	start := parser.current()
	var parts []string
	depth := 0
	for !parser.atEnd() {
		token := parser.current()
		if depth == 0 && (token.Type == lexer.TokenIdentifier || token.Type == lexer.TokenBool) && len(parts) > 0 {
			break
		}
		if depth == 0 && (token.Type == lexer.TokenComma || token.Type == lexer.TokenRightBrace ||
			token.Type == lexer.TokenAssign || token.Type == lexer.TokenScopeBegin) {
			break
		}

		switch token.Type {
		case lexer.TokenIdentifier:
			parts = append(parts, token.Literal)
		case lexer.TokenLeftSquareBrace:
			depth++
			parts = append(parts, token.Literal)
		case lexer.TokenRightSquareBrace:
			if depth == 0 {
				parser.addError(token, "unexpected ']' in type")
				return strings.Join(parts, "")
			}
			depth--
			parts = append(parts, token.Literal)
		case lexer.TokenComma:
			parts = append(parts, token.Literal)
		default:
			if len(parts) == 0 {
				parser.addError(token, "expected type")
				return ""
			}
			if depth != 0 {
				parser.addError(token, "expected ']' to close type")
			}
			return strings.Join(parts, "")
		}
		parser.advance()
		if depth == 0 && len(parts) > 0 && !parser.check(lexer.TokenLeftSquareBrace) {
			break
		}
	}

	if len(parts) == 0 {
		parser.addError(start, "expected type")
		return ""
	}
	if depth != 0 {
		parser.addError(start, "expected ']' to close type")
	}
	return strings.Join(parts, "")
}

func (parser *Parser) parseExpressionUntil(stopTypes ...lexer.TokenType) Expression {
	var tokens []lexer.Token
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for !parser.atEnd() {
		token := parser.current()
		if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 && tokenTypeIn(token.Type, stopTypes) {
			break
		}
		if token.Type == lexer.TokenIllegal {
			parser.addError(token, fmt.Sprintf("illegal token %q", token.Literal))
		}

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
			if braceDepth == 0 && tokenTypeIn(lexer.TokenScopeEnd, stopTypes) {
				return expressionFromTokens(tokens)
			}
			if braceDepth > 0 {
				braceDepth--
			}
		}

		tokens = append(tokens, token)
		parser.advance()
	}
	return expressionFromTokens(tokens)
}

func (parser *Parser) consume(expected lexer.TokenType, message string) lexer.Token {
	if parser.check(expected) {
		return parser.advance()
	}
	token := parser.current()
	parser.addError(token, message)
	return lexer.Token{Type: expected, Line: token.Line, Column: token.Column}
}

func (parser *Parser) consumeOptionalSemicolon() {
	parser.match(lexer.TokenSemicolon)
}

func (parser *Parser) match(tokenType lexer.TokenType) bool {
	if !parser.check(tokenType) {
		return false
	}
	parser.advance()
	return true
}

func (parser *Parser) check(tokenType lexer.TokenType) bool {
	if parser.atEnd() {
		return tokenType == lexer.TokenEOFDescriptor
	}
	return parser.current().Type == tokenType
}

func (parser *Parser) advance() lexer.Token {
	if !parser.atEnd() {
		parser.pos++
	}
	return parser.previous()
}

func (parser *Parser) current() lexer.Token {
	if parser.pos >= len(parser.tokens) {
		if len(parser.tokens) == 0 {
			return lexer.Token{Type: lexer.TokenEOFDescriptor}
		}
		return parser.tokens[len(parser.tokens)-1]
	}
	return parser.tokens[parser.pos]
}

func (parser *Parser) previous() lexer.Token {
	if parser.pos == 0 || len(parser.tokens) == 0 {
		return lexer.Token{Type: lexer.TokenEOFDescriptor}
	}
	return parser.tokens[parser.pos-1]
}

func (parser *Parser) atEnd() bool {
	return parser.current().Type == lexer.TokenEOFDescriptor
}

func (parser *Parser) addError(token lexer.Token, message string) {
	parser.errors = append(parser.errors, Error{
		Line:    token.Line,
		Column:  token.Column,
		Message: message,
	})
}

func (parser *Parser) synchronize() {
	for !parser.atEnd() {
		if parser.previous().Type == lexer.TokenSemicolon {
			return
		}
		switch parser.current().Type {
		case lexer.TokenFunc, lexer.TokenGlobal, lexer.TokenLocal, lexer.TokenReturn,
			lexer.TokenIf, lexer.TokenUnless, lexer.TokenFor, lexer.TokenWhile,
			lexer.TokenDoWhile, lexer.TokenImport, lexer.TokenNameSpace:
			return
		}
		parser.advance()
	}
}

func positionFromToken(token lexer.Token) Position {
	return Position{Line: token.Line, Column: token.Column}
}

func tokenTypeIn(tokenType lexer.TokenType, tokenTypes []lexer.TokenType) bool {
	for _, current := range tokenTypes {
		if tokenType == current {
			return true
		}
	}
	return false
}

func assignmentOperatorIndex(tokens []lexer.Token) int {
	depth := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace:
			depth++
		case lexer.TokenRightBrace, lexer.TokenRightSquareBrace:
			depth--
		case lexer.TokenAssign, lexer.TokenPlusEqual, lexer.TokenMinusEqual, lexer.TokenMultiEqual, lexer.TokenDivideEqual:
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func trimExpressionTokens(tokens []lexer.Token) []lexer.Token {
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

func expressionFromTokens(tokens []lexer.Token) Expression {
	tokens = trimExpressionTokens(tokens)
	return Expression{
		Tokens: tokens,
		Node:   parseExpressionNode(tokens),
	}
}
