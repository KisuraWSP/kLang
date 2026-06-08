package parser

import "kLang/src/lexer"

const (
	precedenceLowest = iota
	precedencePipe
	precedenceOr
	precedenceXor
	precedenceAnd
	precedenceEquality
	precedenceComparison
	precedenceTerm
	precedenceFactor
	precedencePrefix
	precedencePower
	precedenceCast
	precedenceCall
)

type expressionParser struct {
	tokens []lexer.Token
	pos    int
}

func parseExpressionNode(tokens []lexer.Token) ExpressionNode {
	tokens = trimExpressionTokens(tokens)
	if len(tokens) == 0 {
		return nil
	}

	parser := &expressionParser{tokens: tokens}
	node := parser.parseExpression(precedenceLowest)
	if node == nil || !parser.atEnd() {
		return RawExpression{Tokens: tokens}
	}
	return node
}

func ParseExpressionTokens(tokens []lexer.Token) ExpressionNode {
	return parseExpressionNode(tokens)
}

func (parser *expressionParser) parseExpression(precedence int) ExpressionNode {
	left := parser.parsePrefix()
	if left == nil {
		return nil
	}

	for !parser.atEnd() && precedence < parser.currentPrecedence() {
		token := parser.current()
		switch token.Type {
		case lexer.TokenLeftBrace:
			left = parser.parseCall(left)
		case lexer.TokenLeftSquareBrace:
			left = parser.parseIndex(left)
		case lexer.TokenDot:
			left = parser.parseSelector(left)
		case lexer.TokenAs:
			left = parser.parseCast(left)
		case lexer.TokenQuestion:
			left = parser.parseNullCheck(left)
		default:
			left = parser.parseBinary(left)
		}
		if left == nil {
			return nil
		}
	}

	return left
}

func (parser *expressionParser) parsePrefix() ExpressionNode {
	if parser.atEnd() {
		return nil
	}

	token := parser.advance()
	switch token.Type {
	case lexer.TokenIdentifier:
		return IdentifierExpression{Name: token.Literal}
	case lexer.TokenInt:
		return LiteralExpression{Kind: "Int", Value: token.Literal}
	case lexer.TokenFloat:
		return LiteralExpression{Kind: "Float", Value: token.Literal}
	case lexer.TokenString:
		return LiteralExpression{Kind: "String", Value: token.Literal}
	case lexer.TokenChar:
		return LiteralExpression{Kind: "Char", Value: token.Literal}
	case lexer.TokenBool:
		return LiteralExpression{Kind: "Bool", Value: token.Literal}
	case lexer.TokenMinus, lexer.TokenNot, lexer.TokenCall:
		return UnaryExpression{
			Operator: token.Literal,
			Right:    parser.parseExpression(precedencePrefix),
		}
	case lexer.TokenLeftBrace:
		inner := parser.parseExpression(precedenceLowest)
		if !parser.match(lexer.TokenRightBrace) {
			return nil
		}
		return GroupExpression{Inner: inner}
	case lexer.TokenLeftSquareBrace:
		return parser.parseList()
	case lexer.TokenScopeBegin:
		return parser.parseMap()
	default:
		return nil
	}
}

func (parser *expressionParser) parseBinary(left ExpressionNode) ExpressionNode {
	token := parser.advance()
	precedence := tokenPrecedence(token.Type)
	if token.Type == lexer.TokenExponent {
		precedence--
	}
	right := parser.parseExpression(precedence)
	return BinaryExpression{
		Left:     left,
		Operator: token.Literal,
		Right:    right,
	}
}

func (parser *expressionParser) parseCall(callee ExpressionNode) ExpressionNode {
	parser.advance()
	var args []ExpressionNode
	if !parser.check(lexer.TokenRightBrace) {
		for {
			args = append(args, parser.parseExpression(precedenceLowest))
			if !parser.match(lexer.TokenComma) {
				break
			}
		}
	}
	if !parser.match(lexer.TokenRightBrace) {
		return nil
	}
	return CallExpression{Callee: callee, Arguments: args}
}

func (parser *expressionParser) parseIndex(target ExpressionNode) ExpressionNode {
	parser.advance()
	index := parser.parseExpression(precedenceLowest)
	if !parser.match(lexer.TokenRightSquareBrace) {
		return nil
	}
	return IndexExpression{Target: target, Index: index}
}

func (parser *expressionParser) parseSelector(target ExpressionNode) ExpressionNode {
	parser.advance()
	field := parser.advance()
	if field.Type != lexer.TokenIdentifier {
		return nil
	}
	return SelectorExpression{Target: target, Field: field.Literal}
}

func (parser *expressionParser) parseCast(value ExpressionNode) ExpressionNode {
	parser.advance()
	typeName := parser.parseTypeName()
	if typeName == "" {
		return nil
	}
	return CastExpression{Value: value, Type: typeName}
}

func (parser *expressionParser) parseNullCheck(value ExpressionNode) ExpressionNode {
	parser.advance()
	return NullCheckExpression{Value: value}
}

func (parser *expressionParser) parseList() ExpressionNode {
	var items []ExpressionNode
	if !parser.check(lexer.TokenRightSquareBrace) {
		for {
			items = append(items, parser.parseExpression(precedenceLowest))
			if !parser.match(lexer.TokenComma) {
				break
			}
		}
	}
	if !parser.match(lexer.TokenRightSquareBrace) {
		return nil
	}
	return ListExpression{Items: items}
}

func (parser *expressionParser) parseTypeName() string {
	if parser.atEnd() || parser.current().Type != lexer.TokenIdentifier {
		return ""
	}
	parts := []string{parser.advance().Literal}
	if !parser.match(lexer.TokenLeftSquareBrace) {
		return parts[0]
	}

	parts = append(parts, "[")
	depth := 1
	for !parser.atEnd() && depth > 0 {
		token := parser.advance()
		switch token.Type {
		case lexer.TokenIdentifier:
			parts = append(parts, token.Literal)
		case lexer.TokenComma:
			parts = append(parts, ",")
		case lexer.TokenLeftSquareBrace:
			depth++
			parts = append(parts, "[")
		case lexer.TokenRightSquareBrace:
			depth--
			parts = append(parts, "]")
		default:
			return ""
		}
	}
	if depth != 0 {
		return ""
	}
	return joinTypeParts(parts)
}

func (parser *expressionParser) parseMap() ExpressionNode {
	var entries []MapEntry
	if !parser.check(lexer.TokenScopeEnd) {
		for {
			key := parser.parseExpression(precedenceLowest)
			if !parser.match(lexer.TokenInferReturn) {
				return RawExpression{Tokens: parser.tokens}
			}
			value := parser.parseExpression(precedenceLowest)
			entries = append(entries, MapEntry{Key: key, Value: value})
			if !parser.match(lexer.TokenComma) {
				break
			}
		}
	}
	if !parser.match(lexer.TokenScopeEnd) {
		return nil
	}
	return MapExpression{Entries: entries}
}

func (parser *expressionParser) currentPrecedence() int {
	if parser.atEnd() {
		return precedenceLowest
	}
	return tokenPrecedence(parser.current().Type)
}

func tokenPrecedence(tokenType lexer.TokenType) int {
	switch tokenType {
	case lexer.TokenPipe:
		return precedencePipe
	case lexer.TokenOr:
		return precedenceOr
	case lexer.TokenXor:
		return precedenceXor
	case lexer.TokenAnd:
		return precedenceAnd
	case lexer.TokenStrictEquality, lexer.TokenNotEqual:
		return precedenceEquality
	case lexer.TokenGreaterThan, lexer.TokenGreaterThanOrEqualTo, lexer.TokenLessThan, lexer.TokenLessThanOrEqualTo:
		return precedenceComparison
	case lexer.TokenPlus, lexer.TokenMinus:
		return precedenceTerm
	case lexer.TokenMultiplication, lexer.TokenDivision, lexer.TokenFloorDivision, lexer.TokenModulus:
		return precedenceFactor
	case lexer.TokenExponent:
		return precedencePower
	case lexer.TokenAs:
		return precedenceCast
	case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace, lexer.TokenDot, lexer.TokenQuestion:
		return precedenceCall
	default:
		return precedenceLowest
	}
}

func joinTypeParts(parts []string) string {
	result := ""
	for _, part := range parts {
		result += part
	}
	return result
}

func (parser *expressionParser) match(tokenType lexer.TokenType) bool {
	if !parser.check(tokenType) {
		return false
	}
	parser.advance()
	return true
}

func (parser *expressionParser) check(tokenType lexer.TokenType) bool {
	return !parser.atEnd() && parser.current().Type == tokenType
}

func (parser *expressionParser) advance() lexer.Token {
	token := parser.current()
	if !parser.atEnd() {
		parser.pos++
	}
	return token
}

func (parser *expressionParser) current() lexer.Token {
	if parser.pos >= len(parser.tokens) {
		return lexer.Token{Type: lexer.TokenEOFDescriptor}
	}
	return parser.tokens[parser.pos]
}

func (parser *expressionParser) atEnd() bool {
	return parser.pos >= len(parser.tokens)
}
