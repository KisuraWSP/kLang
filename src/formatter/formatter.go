package formatter

import (
	"fmt"
	"strconv"
	"strings"

	"kLang/src/lexer"
	"kLang/src/parser"
)

const indentUnit = "    "

type Error struct {
	Line    int
	Column  int
	Message string
}

func (err Error) Error() string {
	if err.Line > 0 {
		return fmt.Sprintf("line %d:%d: %s", err.Line, err.Column, err.Message)
	}
	return err.Message
}

func Format(input string) (string, error) {
	_, parseErrors := parser.Parse(input)
	if len(parseErrors) != 0 {
		first := parseErrors[0]
		return "", Error{Line: first.Line, Column: first.Column, Message: first.Message}
	}

	lines := strings.Split(strings.ReplaceAll(input, "\r\n", "\n"), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}

	var output []string
	indent := 0
	inBlockComment := false
	inHereString := false
	blankPending := false

	for _, line := range lines {
		raw := strings.TrimSpace(line)
		if raw == "" {
			blankPending = len(output) != 0
			continue
		}

		if blankPending && len(output) != 0 && output[len(output)-1] != "" {
			output = append(output, "")
		}
		blankPending = false

		if inHereString {
			output = append(output, strings.Repeat(indentUnit, indent)+raw)
			if raw == "//;" || raw == "//" {
				inHereString = false
			}
			continue
		}

		if inBlockComment {
			output = append(output, strings.Repeat(indentUnit, indent)+raw)
			if strings.Contains(raw, "*)") {
				inBlockComment = false
			}
			continue
		}

		if strings.HasPrefix(raw, "(*") {
			output = append(output, strings.Repeat(indentUnit, indent)+raw)
			if !strings.Contains(raw, "*)") {
				inBlockComment = true
			}
			continue
		}

		if strings.HasPrefix(raw, "--") {
			output = append(output, strings.Repeat(indentUnit, indent)+raw)
			continue
		}

		if closesBefore(raw) && indent > 0 {
			indent--
		}

		formatted := formatCodeLine(raw)
		output = append(output, strings.Repeat(indentUnit, indent)+formatted)

		if startsHereString(raw) {
			inHereString = true
			continue
		}

		indent += indentDeltaAfter(formatted)
		if indent < 0 {
			indent = 0
		}
	}

	if len(output) == 0 {
		return "", nil
	}
	return strings.Join(output, "\n") + "\n", nil
}

func startsHereString(line string) bool {
	trimmed := strings.TrimSpace(line)
	return strings.HasSuffix(trimmed, "= //") || strings.HasSuffix(trimmed, " //")
}

func closesBefore(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "}") ||
		line == "end" ||
		strings.HasPrefix(line, "end ") ||
		line == "else" ||
		strings.HasPrefix(line, "else ") ||
		line == "catch" ||
		strings.HasPrefix(line, "catch ") ||
		line == "case" ||
		strings.HasPrefix(line, "case ") ||
		strings.HasPrefix(line, "default:")
}

func indentDeltaAfter(line string) int {
	trimmed := strings.TrimSpace(line)
	delta := countRuneOutsideLiterals(trimmed, '{') - countRuneOutsideLiterals(trimmed, '}')
	if strings.HasPrefix(trimmed, "}") {
		delta++
	}
	if strings.HasSuffix(trimmed, " do") ||
		strings.HasSuffix(trimmed, " then") ||
		trimmed == "else" ||
		strings.HasPrefix(trimmed, "else ") ||
		trimmed == "catch" ||
		strings.HasPrefix(trimmed, "catch ") ||
		trimmed == "case" ||
		strings.HasPrefix(trimmed, "case ") ||
		strings.HasPrefix(trimmed, "default:") {
		delta++
	}
	return delta
}

func formatCodeLine(line string) string {
	code, comment := splitInlineComment(line)
	formatted := formatTokens(code)
	if comment == "" {
		return formatted
	}
	if formatted == "" {
		return comment
	}
	return formatted + " " + comment
}

func splitInlineComment(line string) (string, string) {
	inString := false
	inChar := false
	escaped := false
	for index := 0; index+1 < len(line); index++ {
		ch := line[index]
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && (inString || inChar) {
			escaped = true
			continue
		}
		switch ch {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		case '-':
			if !inString && !inChar && line[index+1] == '-' {
				return strings.TrimSpace(line[:index]), strings.TrimSpace(line[index:])
			}
		}
	}
	return strings.TrimSpace(line), ""
}

func formatTokens(code string) string {
	tokens := lexer.New(code).Tokenize()
	var builder strings.Builder
	var previous lexer.Token
	hasPrevious := false

	for _, token := range tokens {
		if token.Type == lexer.TokenEOFDescriptor {
			break
		}
		if token.Type == lexer.TokenIllegal {
			return strings.TrimSpace(code)
		}
		literal := tokenLiteral(token)
		if literal == "" {
			continue
		}
		if hasPrevious && needsSpace(previous, token) && !strings.HasSuffix(builder.String(), " ") {
			builder.WriteByte(' ')
		}
		builder.WriteString(literal)
		if needsTrailingSpace(token) {
			builder.WriteByte(' ')
		}
		previous = token
		hasPrevious = true
	}
	return strings.TrimSpace(builder.String())
}

func tokenLiteral(token lexer.Token) string {
	switch token.Type {
	case lexer.TokenString:
		return strconv.Quote(token.Literal)
	case lexer.TokenChar:
		return "'" + token.Literal + "'"
	default:
		return token.Literal
	}
}

func needsSpace(previous lexer.Token, current lexer.Token) bool {
	if current.Type == lexer.TokenComma || current.Type == lexer.TokenSemicolon ||
		current.Type == lexer.TokenRightBrace || current.Type == lexer.TokenRightSquareBrace ||
		current.Type == lexer.TokenDot || current.Type == lexer.TokenQuestion || current.Type == lexer.TokenBang {
		return false
	}
	if current.Type == lexer.TokenLeftBrace || current.Type == lexer.TokenLeftSquareBrace {
		return false
	}
	if previous.Type == lexer.TokenLeftBrace || previous.Type == lexer.TokenLeftSquareBrace ||
		previous.Type == lexer.TokenDot || previous.Type == lexer.TokenNamespaceAccess ||
		previous.Type == lexer.TokenHash {
		return false
	}
	if isOperator(previous.Type) || isOperator(current.Type) {
		return true
	}
	if previous.Type == lexer.TokenComma || previous.Type == lexer.TokenSemicolon {
		return false
	}
	return true
}

func needsTrailingSpace(token lexer.Token) bool {
	return token.Type == lexer.TokenComma || token.Type == lexer.TokenSemicolon
}

func isOperator(tokenType lexer.TokenType) bool {
	switch tokenType {
	case lexer.TokenPlus, lexer.TokenMinus, lexer.TokenMultiplication, lexer.TokenDivision, lexer.TokenModulus,
		lexer.TokenExponent, lexer.TokenFloorDivision, lexer.TokenPipe, lexer.TokenTypeUnion,
		lexer.TokenAssign, lexer.TokenPlusEqual, lexer.TokenMinusEqual, lexer.TokenMultiEqual, lexer.TokenDivideEqual,
		lexer.TokenStrictEquality, lexer.TokenNotEqual, lexer.TokenGreaterThan, lexer.TokenLessThan,
		lexer.TokenGreaterThanOrEqualTo, lexer.TokenLessThanOrEqualTo, lexer.TokenArrow, lexer.TokenEvaluationAssign,
		lexer.TokenInferReturn, lexer.TokenAs, lexer.TokenIn, lexer.TokenIs, lexer.TokenAnd, lexer.TokenOr, lexer.TokenXor:
		return true
	default:
		return false
	}
}

func countRuneOutsideLiterals(input string, target rune) int {
	count := 0
	inString := false
	inChar := false
	escaped := false
	for _, ch := range input {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && (inString || inChar) {
			escaped = true
			continue
		}
		switch ch {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		default:
			if !inString && !inChar && ch == target {
				count++
			}
		}
	}
	return count
}
