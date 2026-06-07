package lexer

type Lexer struct {
	input        string
	position     int
	readPosition int
	ch           byte
	line         int
	column       int
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

	if isLetter(lexer.ch) {
		literal := lexer.readIdentifier()
		return Token{
			Type:    lookupIdentifier(literal),
			Literal: literal,
			Line:    line,
			Column:  column,
		}
	}

	if isDigit(lexer.ch) {
		literal, tokenType := lexer.readNumber()
		return Token{
			Type:    tokenType,
			Literal: literal,
			Line:    line,
			Column:  column,
		}
	}

	switch lexer.ch {
	case '"':
		literal, ok := lexer.readString()
		if !ok {
			return Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column}
		}
		return Token{Type: TokenString, Literal: literal, Line: line, Column: column}
	case '\'':
		literal, ok := lexer.readCharLiteral()
		if !ok {
			return Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column}
		}
		return Token{Type: TokenChar, Literal: literal, Line: line, Column: column}
	}

	if tokenType, literal, ok := lexer.readOperator(); ok {
		return Token{Type: tokenType, Literal: literal, Line: line, Column: column}
	}

	literal := string(lexer.ch)
	if tokenType, ok := Punctuations[literal]; ok {
		lexer.readChar()
		return Token{Type: tokenType, Literal: literal, Line: line, Column: column}
	}

	lexer.readChar()
	return Token{Type: TokenIllegal, Literal: literal, Line: line, Column: column}
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

		return
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

	for isDigit(lexer.ch) {
		lexer.readChar()
	}

	if lexer.ch == '.' && isDigit(lexer.peekChar()) {
		tokenType = TokenFloat
		lexer.readChar()
		for isDigit(lexer.ch) {
			lexer.readChar()
		}
	}

	if lexer.ch == '.' || isLetter(lexer.ch) {
		for lexer.ch == '.' || isLetter(lexer.ch) || isDigit(lexer.ch) {
			lexer.readChar()
		}
		return lexer.input[position:lexer.position], TokenIllegal
	}

	return lexer.input[position:lexer.position], tokenType
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

func (lexer *Lexer) readOperator() (TokenType, string, bool) {
	for _, literal := range []string{
		"**", "//", "+=", "-=", "*=", "/=", "==", "!=", ">=", "<=", "->", ":=",
		"+", "-", "*", "/", "%", "=", ">", "<", ":",
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
	return ch == '_' || ('a' <= ch && ch <= 'z') || ('A' <= ch && ch <= 'Z')
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
