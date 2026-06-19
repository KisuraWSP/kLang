package lexer

import "strings"

type Lexer struct {
	input           string
	position        int
	readPosition    int
	ch              byte
	line            int
	column          int
	lastSignificant TokenType
}

func New(input string) *Lexer {
	lexer := &Lexer{
		input:  input,
		line:   1,
		column: 0,
	}
	lexer.readChar()
	return lexer
}

func (lexer *Lexer) Tokenize() []Token {
	var tokens []Token
	for {
		token := lexer.NextToken()
		tokens = append(tokens, token)
		if token.Type == TokenEOFDescriptor {
			return tokens
		}
	}
}

func (lexer *Lexer) NextToken() Token {
	lexer.skipWhitespaceAndComments()

	line := lexer.line
	column := lexer.column

	if lexer.ch == 0 {
		return Token{Type: TokenEOFDescriptor, Literal: "", Line: line, Column: column}
	}

	if lexer.ch == '/' && lexer.peekChar() == '/' && lexer.canStartHereString() {
		literal, ok := lexer.readHereString()
		return lexer.emit(Token{Type: tokenTypeForHereString(ok), Literal: literal, Line: line, Column: column})
	}

	if isLetter(lexer.ch) {
		literal := lexer.readIdentifier()
		return lexer.emit(Token{
			Type:    lookupIdentifier(literal),
			Literal: literal,
			Line:    line,
			Column:  column,
		})
	}

	if isDigit(lexer.ch) || (lexer.ch == '-' && isDigit(lexer.peekChar()) && lexer.canStartSignedNumber() && !lexer.signedNumberWouldBindPower()) {
		literal, tokenType := lexer.readNumber()
		return lexer.emit(Token{
			Type:    tokenType,
			Literal: literal,
			Line:    line,
			Column:  column,
		})
	}

	switch lexer.ch {
	case '"':
		literal, ok := lexer.readString()
		if !ok {
			return lexer.emit(Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column})
		}
		return lexer.emit(Token{Type: TokenString, Literal: literal, Line: line, Column: column})
	case '\'':
		literal, ok := lexer.readCharLiteral()
		if !ok {
			return lexer.emit(Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column})
		}
		return lexer.emit(Token{Type: TokenChar, Literal: literal, Line: line, Column: column})
	}

	if tokenType, literal, ok := lexer.readOperator(); ok {
		return lexer.emit(Token{Type: tokenType, Literal: literal, Line: line, Column: column})
	}

	literal := string(lexer.ch)
	if tokenType, ok := Punctuations[literal]; ok {
		lexer.readChar()
		return lexer.emit(Token{Type: tokenType, Literal: literal, Line: line, Column: column})
	}

	lexer.readChar()
	return lexer.emit(Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column})
}

func (lexer *Lexer) emit(token Token) Token {
	if token.Type != TokenWhiteSpace && token.Type != TokenComment && token.Type != TokenEOFDescriptor {
		lexer.lastSignificant = token.Type
	}
	return token
}

func tokenTypeForHereString(ok bool) TokenType {
	if ok {
		return TokenString
	}
	return TokenIllegal
}

func (lexer *Lexer) readChar() {
	if lexer.readPosition >= len(lexer.input) {
		lexer.ch = 0
		lexer.position = lexer.readPosition
		lexer.readPosition++
		return
	}

	lexer.ch = lexer.input[lexer.readPosition]
	lexer.position = lexer.readPosition
	lexer.readPosition++

	if lexer.ch == '\n' {
		lexer.line++
		lexer.column = 0
		return
	}
	lexer.column++
}

func (lexer *Lexer) peekChar() byte {
	if lexer.readPosition >= len(lexer.input) {
		return 0
	}
	return lexer.input[lexer.readPosition]
}

func (lexer *Lexer) skipWhitespaceAndComments() {
	for {
		for lexer.ch == ' ' || lexer.ch == '\t' || lexer.ch == '\n' || lexer.ch == '\r' {
			lexer.readChar()
		}

		if lexer.ch == '-' && lexer.peekChar() == '-' {
			for lexer.ch != '\n' && lexer.ch != 0 {
				lexer.readChar()
			}
			continue
		}

		if lexer.ch == '(' && lexer.peekChar() == '*' {
			lexer.skipMultilineComment()
			continue
		}

		return
	}
}

func (lexer *Lexer) skipMultilineComment() {
	depth := 0
	for lexer.ch != 0 {
		if lexer.ch == '(' && lexer.peekChar() == '*' {
			depth++
			lexer.readChar()
			lexer.readChar()
			continue
		}
		if lexer.ch == '*' && lexer.peekChar() == ')' {
			depth--
			lexer.readChar()
			lexer.readChar()
			if depth == 0 {
				return
			}
			continue
		}
		lexer.readChar()
	}
}

func (lexer *Lexer) readIdentifier() string {
	position := lexer.position
	for isLetter(lexer.ch) || isDigit(lexer.ch) {
		lexer.readChar()
	}
	return lexer.input[position:lexer.position]
}

func (lexer *Lexer) readNumber() (string, TokenType) {
	position := lexer.position
	tokenType := TokenInt
	valid := true

	if lexer.ch == '-' {
		lexer.readChar()
	}

	if lexer.ch == '0' && isBasePrefix(lexer.peekChar()) {
		prefix := lexer.peekChar()
		lexer.readChar()
		lexer.readChar()
		sawDigit, validSeparators := lexer.readDigitsForBase(prefix)
		valid = valid && validSeparators
		if !sawDigit {
			for isLetter(lexer.ch) || isDigit(lexer.ch) {
				lexer.readChar()
			}
			return lexer.input[position:lexer.position], TokenIllegal
		}
		if !valid || isLetter(lexer.ch) || isDigit(lexer.ch) || lexer.ch == '.' {
			for isLetter(lexer.ch) || isDigit(lexer.ch) || lexer.ch == '.' {
				lexer.readChar()
			}
			return lexer.input[position:lexer.position], TokenIllegal
		}
		return lexer.input[position:lexer.position], TokenInt
	}

	_, validSeparators := lexer.readDecimalDigits()
	valid = valid && validSeparators

	if lexer.ch == '.' && isDigit(lexer.peekChar()) {
		tokenType = TokenFloat
		lexer.readChar()
		_, validSeparators := lexer.readDecimalDigits()
		valid = valid && validSeparators
	}

	if lexer.ch == '.' && isLetter(lexer.peekChar()) {
		return lexer.input[position:lexer.position], tokenType
	}

	if !valid || lexer.ch == '.' || isLetter(lexer.ch) {
		for lexer.ch == '.' || isLetter(lexer.ch) || isDigit(lexer.ch) {
			lexer.readChar()
		}
		return lexer.input[position:lexer.position], TokenIllegal
	}

	return lexer.input[position:lexer.position], tokenType
}

func (lexer *Lexer) readDecimalDigits() (bool, bool) {
	return lexer.readSeparatedDigits(func(ch byte) bool {
		return isDigit(ch)
	})
}

func (lexer *Lexer) readDigitsForBase(prefix byte) (bool, bool) {
	return lexer.readSeparatedDigits(func(ch byte) bool {
		return isDigitForBase(ch, prefix)
	})
}

func (lexer *Lexer) readSeparatedDigits(isValidDigit func(byte) bool) (bool, bool) {
	sawDigit := false
	valid := true
	previousWasSeparator := false

	for isValidDigit(lexer.ch) || lexer.ch == '_' {
		if lexer.ch == '_' {
			if !sawDigit || previousWasSeparator || !isValidDigit(lexer.peekChar()) {
				valid = false
			}
			previousWasSeparator = true
			lexer.readChar()
			continue
		}
		sawDigit = true
		previousWasSeparator = false
		lexer.readChar()
	}

	return sawDigit, valid && !previousWasSeparator
}

func (lexer *Lexer) canStartSignedNumber() bool {
	switch lexer.lastSignificant {
	case 0, TokenAssign, TokenEvaluationAssign, TokenReturn, TokenComma, TokenLeftBrace, TokenLeftSquareBrace,
		TokenScopeBegin, TokenInferReturn, TokenPlus, TokenMinus, TokenMultiplication, TokenDivision, TokenModulus,
		TokenExponent, TokenFloorDivision, TokenPipe, TokenTypeUnion, TokenStrictEquality, TokenNotEqual,
		TokenGreaterThan, TokenLessThan, TokenGreaterThanOrEqualTo, TokenLessThanOrEqualTo, TokenArrow:
		return true
	default:
		return false
	}
}

func (lexer *Lexer) signedNumberWouldBindPower() bool {
	position := lexer.readPosition
	if position+1 < len(lexer.input) && lexer.input[position] == '0' && isBasePrefix(lexer.input[position+1]) {
		prefix := lexer.input[position+1]
		position += 2
		for position < len(lexer.input) && (isDigitForBase(lexer.input[position], prefix) || lexer.input[position] == '_') {
			position++
		}
	} else {
		for position < len(lexer.input) && (isDigit(lexer.input[position]) || lexer.input[position] == '_') {
			position++
		}
		if position < len(lexer.input) && lexer.input[position] == '.' {
			position++
			for position < len(lexer.input) && (isDigit(lexer.input[position]) || lexer.input[position] == '_') {
				position++
			}
		}
	}
	for position < len(lexer.input) && (lexer.input[position] == ' ' || lexer.input[position] == '\t' || lexer.input[position] == '\n' || lexer.input[position] == '\r') {
		position++
	}
	return position+1 < len(lexer.input) && lexer.input[position] == '*' && lexer.input[position+1] == '*'
}

func isBasePrefix(ch byte) bool {
	return ch == 'x' || ch == 'X' || ch == 'o' || ch == 'O' || ch == 'b' || ch == 'B'
}

func isDigitForBase(ch byte, prefix byte) bool {
	switch prefix {
	case 'b', 'B':
		return ch == '0' || ch == '1'
	case 'o', 'O':
		return '0' <= ch && ch <= '7'
	case 'x', 'X':
		return ('0' <= ch && ch <= '9') || ('a' <= ch && ch <= 'f') || ('A' <= ch && ch <= 'F')
	default:
		return false
	}
}

func (lexer *Lexer) readString() (string, bool) {
	lexer.readChar()
	position := lexer.position
	valid := true
	for lexer.ch != '"' && lexer.ch != 0 && lexer.ch != '\n' {
		if lexer.ch == '\\' && lexer.peekChar() != 0 {
			if !isValidEscape(lexer.peekChar()) {
				valid = false
			}
			lexer.readChar()
		}
		lexer.readChar()
	}

	literal := lexer.input[position:lexer.position]
	if lexer.ch != '"' {
		return literal, false
	}

	lexer.readChar()
	return literal, valid
}

func (lexer *Lexer) readCharLiteral() (string, bool) {
	lexer.readChar()
	position := lexer.position
	valid := true
	for lexer.ch != '\'' && lexer.ch != 0 && lexer.ch != '\n' {
		if lexer.ch == '\\' && lexer.peekChar() != 0 {
			if !isValidEscape(lexer.peekChar()) {
				valid = false
			}
			lexer.readChar()
		}
		lexer.readChar()
	}

	literal := lexer.input[position:lexer.position]
	if lexer.ch != '\'' {
		return literal, false
	}

	lexer.readChar()
	return literal, valid && isValidCharLiteral(literal)
}

func (lexer *Lexer) canStartHereString() bool {
	switch lexer.lastSignificant {
	case 0, TokenAssign, TokenEvaluationAssign, TokenReturn, TokenComma, TokenLeftBrace, TokenScopeBegin:
		return true
	default:
		return false
	}
}

func (lexer *Lexer) readHereString() (string, bool) {
	lexer.readChar()
	lexer.readChar()
	if lexer.ch == '\r' && lexer.peekChar() == '\n' {
		lexer.readChar()
		lexer.readChar()
	} else if lexer.ch == '\n' {
		lexer.readChar()
	}

	position := lexer.position
	for lexer.ch != 0 {
		if lexer.ch == '/' && lexer.peekChar() == '/' && (lexer.position == 0 || lexer.input[lexer.position-1] == '\n' || lexer.input[lexer.position-1] == '\r') {
			literal := lexer.input[position:lexer.position]
			lexer.readChar()
			lexer.readChar()
			return strings.TrimSuffix(strings.TrimSuffix(literal, "\n"), "\r"), true
		}
		lexer.readChar()
	}
	return lexer.input[position:lexer.position], false
}

func (lexer *Lexer) readOperator() (TokenType, string, bool) {
	for _, literal := range []string{
		"**", "//", "|>", "+=", "-=", "*=", "/=", "==", "!=", ">=", "<=", "->", ":=", "::",
		"+", "-", "*", "/", "%", "=", ">", "<", ":", "|", "!",
	} {
		if lexer.startsWith(literal) {
			for range literal {
				lexer.readChar()
			}
			return Operators[literal], literal, true
		}
	}

	return TokenIllegal, "", false
}

func (lexer *Lexer) startsWith(value string) bool {
	if lexer.position+len(value) > len(lexer.input) {
		return false
	}
	return lexer.input[lexer.position:lexer.position+len(value)] == value
}

func lookupIdentifier(literal string) TokenType {
	if literal == "True" || literal == "False" {
		return TokenBool
	}
	if tokenType, ok := Keywords[literal]; ok {
		return tokenType
	}
	return TokenIdentifier
}

func isLetter(ch byte) bool {
	return ch == '_' || ch >= 0x80 || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

func isValidCharLiteral(literal string) bool {
	if len(literal) == 1 {
		return true
	}
	return len(literal) == 2 && literal[0] == '\\'
}

func isValidEscape(ch byte) bool {
	switch ch {
	case 'n', 'r', 't', '\\', '"', '\'':
		return true
	default:
		return false
	}
}
