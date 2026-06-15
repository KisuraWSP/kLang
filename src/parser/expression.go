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
	if tokens[0].Type == lexer.TokenLambdaFunc {
		if lambda, ok := parseLambdaExpressionTokens(tokens); ok {
			return lambda
		}
		return RawExpression{Tokens: tokens}
	}
	if conditional, ok := parseConditionalExpression(tokens); ok {
		return conditional
	}

	parser := &expressionParser{tokens: tokens}
	node := parser.parseExpression(precedenceLowest)
	if node == nil || !parser.atEnd() {
		return RawExpression{Tokens: tokens}
	}
	return node
}

func parseConditionalExpression(tokens []lexer.Token) (ExpressionNode, bool) {
	if len(tokens) < 6 || tokens[0].Type != lexer.TokenIf {
		return nil, false
	}
	thenIndex := findTopLevelExpressionToken(tokens, lexer.TokenThen, 1)
	if thenIndex <= 1 {
		return nil, false
	}
	separatorIndex := findTopLevelExpressionToken(tokens, lexer.TokenInferReturn, thenIndex+1)
	if separatorIndex <= thenIndex+1 || separatorIndex+1 >= len(tokens) {
		return nil, false
	}

	consequenceTokens := trimBranchReturn(tokens[thenIndex+1 : separatorIndex])
	alternativeTokens := trimBranchReturn(tokens[separatorIndex+1:])
	if len(consequenceTokens) == 0 || len(alternativeTokens) == 0 {
		return nil, false
	}
	return ConditionalExpression{
		Condition:   parseExpressionNode(tokens[1:thenIndex]),
		Consequence: parseExpressionNode(consequenceTokens),
		Alternative: parseExpressionNode(alternativeTokens),
	}, true
}

func trimBranchReturn(tokens []lexer.Token) []lexer.Token {
	tokens = trimExpressionTokens(tokens)
	if len(tokens) > 0 && tokens[0].Type == lexer.TokenReturn {
		return trimExpressionTokens(tokens[1:])
	}
	return tokens
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
		case lexer.TokenDot, lexer.TokenNamespaceAccess:
			left = parser.parseSelector(left)
		case lexer.TokenAs:
			left = parser.parseCast(left)
		case lexer.TokenQuestion:
			left = parser.parseNullCheck(left)
		case lexer.TokenBang:
			left = parser.parsePropagate(left)
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
	case lexer.TokenIdentifier, lexer.TokenLet, lexer.TokenVar, lexer.TokenVal, lexer.TokenConst:
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
	case lexer.TokenMinus, lexer.TokenNot, lexer.TokenCall, lexer.TokenMove, lexer.TokenCopy, lexer.TokenClone, lexer.TokenAwait:
		return UnaryExpression{
			Operator: token.Literal,
			Right:    parser.parseExpression(precedencePrefix),
		}
	case lexer.TokenLambdaFunc:
		return parser.parseLambda(token)
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

func (parser *expressionParser) parseLambda(start lexer.Token) ExpressionNode {
	startIndex := parser.pos - 1
	bodyStart := findLambdaBodyStart(parser.tokens, parser.pos)
	if bodyStart == -1 {
		return nil
	}
	bodyEnd := findMatchingExpressionToken(parser.tokens, bodyStart, lexer.TokenScopeBegin, lexer.TokenScopeEnd)
	if bodyEnd == -1 {
		return nil
	}
	node, ok := parseLambdaExpressionTokens(parser.tokens[startIndex : bodyEnd+1])
	if !ok {
		return nil
	}
	parser.pos = bodyEnd + 1
	_ = start
	return node
}

func parseLambdaExpressionTokens(tokens []lexer.Token) (ExpressionNode, bool) {
	lambdaTokens := append([]lexer.Token(nil), tokens...)
	lambdaTokens = append(lambdaTokens, lexer.Token{Type: lexer.TokenEOFDescriptor})
	lambdaParser := New(lambdaTokens)
	lambdaParser.consume(lexer.TokenLambdaFunc, "expected fun")
	typeParams := lambdaParser.parseTypeParameters()
	lambdaParser.consume(lexer.TokenLeftBrace, "expected '(' after fun")
	params := lambdaParser.parseParameters()
	lambdaParser.consume(lexer.TokenRightBrace, "expected ')' after lambda parameters")
	returnType := "T"
	if lambdaParser.match(lexer.TokenInferReturn) {
		returnType = lambdaParser.parseType()
	}
	body := lambdaParser.parseBlock()
	if len(lambdaParser.Errors()) != 0 || !lambdaParser.atEnd() {
		return nil, false
	}
	params = applyTypeParameterRestrictions(params, typeParams)
	returnType = applyReturnTypeRestriction(returnType, typeParams)
	return LambdaExpression{TypeParams: typeParams, Params: params, ReturnType: returnType, Body: body}, true
}

func findLambdaBodyStart(tokens []lexer.Token, start int) int {
	depthParen := 0
	depthBracket := 0
	for index := start; index < len(tokens); index++ {
		token := tokens[index]
		switch token.Type {
		case lexer.TokenLeftBrace:
			depthParen++
		case lexer.TokenRightBrace:
			if depthParen > 0 {
				depthParen--
			}
		case lexer.TokenLeftSquareBrace:
			depthBracket++
		case lexer.TokenRightSquareBrace:
			if depthBracket > 0 {
				depthBracket--
			}
		case lexer.TokenScopeBegin:
			if depthParen == 0 && depthBracket == 0 {
				return index
			}
		}
	}
	return -1
}

func findMatchingExpressionToken(tokens []lexer.Token, open int, left lexer.TokenType, right lexer.TokenType) int {
	depth := 0
	for index := open; index < len(tokens); index++ {
		switch tokens[index].Type {
		case left:
			depth++
		case right:
			depth--
			if depth == 0 {
				return index
			}
		}
	}
	return -1
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
	if field.Type != lexer.TokenIdentifier && field.Type != lexer.TokenCopy && field.Type != lexer.TokenClone {
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

func (parser *expressionParser) parsePropagate(value ExpressionNode) ExpressionNode {
	parser.advance()
	return PropagateExpression{Value: value}
}

func (parser *expressionParser) parseList() ExpressionNode {
	body, ok := parser.consumeListBody()
	if !ok {
		return nil
	}
	if len(body) == 0 {
		return ListExpression{}
	}
	if comprehension, ok := parseListComprehension(body); ok {
		return comprehension
	}

	parts := splitTopLevelExpressionTokens(body, lexer.TokenComma)
	items := make([]ExpressionNode, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			return nil
		}
		items = append(items, parseExpressionNode(part))
	}
	return ListExpression{Items: items}
}

func (parser *expressionParser) consumeListBody() ([]lexer.Token, bool) {
	start := parser.pos
	depthSquare := 0
	depthParen := 0
	depthScope := 0
	for !parser.atEnd() {
		token := parser.current()
		if token.Type == lexer.TokenRightSquareBrace && depthSquare == 0 && depthParen == 0 && depthScope == 0 {
			body := append([]lexer.Token(nil), parser.tokens[start:parser.pos]...)
			parser.advance()
			return body, true
		}
		switch token.Type {
		case lexer.TokenLeftSquareBrace:
			depthSquare++
		case lexer.TokenRightSquareBrace:
			depthSquare--
		case lexer.TokenLeftBrace:
			depthParen++
		case lexer.TokenRightBrace:
			depthParen--
		case lexer.TokenScopeBegin:
			depthScope++
		case lexer.TokenScopeEnd:
			depthScope--
		}
		if depthSquare < 0 || depthParen < 0 || depthScope < 0 {
			return nil, false
		}
		parser.advance()
	}
	return nil, false
}

func parseListComprehension(tokens []lexer.Token) (ExpressionNode, bool) {
	forIndex := findTopLevelExpressionToken(tokens, lexer.TokenFor, 0)
	if forIndex <= 0 || forIndex+3 >= len(tokens) {
		return nil, false
	}
	if tokens[forIndex+1].Type != lexer.TokenIdentifier || tokens[forIndex+2].Type != lexer.TokenIn {
		return nil, false
	}

	if findTopLevelExpressionToken(tokens[:forIndex], lexer.TokenComma, 0) != -1 {
		return nil, false
	}

	iterator := tokens[forIndex+1].Literal
	iterableStart := forIndex + 3
	ifIndex := findTopLevelExpressionToken(tokens, lexer.TokenIf, iterableStart)
	iterableEnd := len(tokens)
	var conditionTokens []lexer.Token
	if ifIndex != -1 {
		iterableEnd = ifIndex
		conditionTokens = tokens[ifIndex+1:]
		if len(conditionTokens) == 0 {
			return nil, false
		}
	}
	if iterableStart >= iterableEnd {
		return nil, false
	}

	return ListComprehensionExpression{
		Value:     parseExpressionNode(tokens[:forIndex]),
		Iterator:  iterator,
		Iterable:  parseExpressionNode(tokens[iterableStart:iterableEnd]),
		Condition: parseExpressionNode(conditionTokens),
	}, true
}

func splitTopLevelExpressionTokens(tokens []lexer.Token, separator lexer.TokenType) [][]lexer.Token {
	var parts [][]lexer.Token
	start := 0
	depthSquare := 0
	depthParen := 0
	depthScope := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftSquareBrace:
			depthSquare++
		case lexer.TokenRightSquareBrace:
			depthSquare--
		case lexer.TokenLeftBrace:
			depthParen++
		case lexer.TokenRightBrace:
			depthParen--
		case lexer.TokenScopeBegin:
			depthScope++
		case lexer.TokenScopeEnd:
			depthScope--
		default:
			if token.Type == separator && depthSquare == 0 && depthParen == 0 && depthScope == 0 {
				parts = append(parts, tokens[start:index])
				start = index + 1
			}
		}
	}
	parts = append(parts, tokens[start:])
	return parts
}

func findTopLevelExpressionToken(tokens []lexer.Token, tokenType lexer.TokenType, start int) int {
	depthSquare := 0
	depthParen := 0
	depthScope := 0
	for index := start; index < len(tokens); index++ {
		token := tokens[index]
		switch token.Type {
		case lexer.TokenLeftSquareBrace:
			depthSquare++
		case lexer.TokenRightSquareBrace:
			depthSquare--
		case lexer.TokenLeftBrace:
			depthParen++
		case lexer.TokenRightBrace:
			depthParen--
		case lexer.TokenScopeBegin:
			depthScope++
		case lexer.TokenScopeEnd:
			depthScope--
		default:
			if token.Type == tokenType && depthSquare == 0 && depthParen == 0 && depthScope == 0 {
				return index
			}
		}
	}
	return -1
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
	case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace, lexer.TokenDot, lexer.TokenNamespaceAccess, lexer.TokenQuestion, lexer.TokenBang:
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
