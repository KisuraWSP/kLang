package lexer

type TokenType int

// ENUMERATIONS
const (
	TokenIllegal       TokenType = iota // Unknown Character
	TokenEOFDescriptor                  // End Of File

	// Literals & Identifiers
	TokenIdentifier // user-defined names (variables, functions)
	TokenInt        // 12345
	TokenFloat      // 1.2344
	TokenString     // "hallo"
	TokenBool       // True False
	TokenChar       // 'C'

	TokenWhiteSpace
	TokenComment

	// Operators
	TokenPlus           // +
	TokenMinus          // -
	TokenMultiplication // *
	TokenDivision       // /
	TokenModulus        // %
	TokenExponent       // **
	TokenFloorDivision  // //

	TokenAssign      // =
	TokenPlusEqual   // +=
	TokenMinusEqual  // -=
	TokenMultiEqual  // *=
	TokenDivideEqual // /=

	TokenStrictEquality       // ==
	TokenNotEqual             // !=
	TokenGreaterThan          // >
	TokenLessThan             // <
	TokenGreaterThanOrEqualTo // >=
	TokenLessThanOrEqualTo    // <=

	TokenArrow            // ->
	TokenEvaluationAssign // :=
	TokenLeftSquareBrace  // [
	TokenRightSquareBrace // ]
	TokenLeftBrace        // (
	TokenRightBrace       // )
	TokenInferReturn      // :
	TokenSemicolon        // ;
	TokenComma            // ,
	TokenDot              // .

	// Special Scope Operators
	TokenScopeBegin // {
	TokenScopeEnd   // }

	// Keywords
	TokenAnd        // and
	TokenOr         // or
	TokenNot        // not
	TokenFunc       // function
	TokenIf         // if
	TokenElse       // else
	TokenUnless     // unless
	TokenFor        // for
	TokenWhile      // while
	TokenDo         // do
	TokenIs         // is
	TokenAs         // as
	TokenIn         // in
	TokenImport     // import
	TokenLambdaFunc // fun
	TokenGlobal     // global
	TokenMut        // mut
	TokenLocal      // local
	TokenCall       // call
	TokenNameSpace  // namespace
	TokenReturn     // return
	TokenBreak      // break
	TokenDoWhile    // do_while
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

var Keywords = map[string]TokenType{
	"and":       TokenAnd,
	"or":        TokenOr,
	"not":       TokenNot,
	"function":  TokenFunc,
	"if":        TokenIf,
	"else":      TokenElse,
	"unless":    TokenUnless,
	"for":       TokenFor,
	"while":     TokenWhile,
	"do":        TokenDo,
	"is":        TokenIs,
	"as":        TokenAs,
	"in":        TokenIn,
	"import":    TokenImport,
	"global":    TokenGlobal,
	"fun":       TokenLambdaFunc,
	"mut":       TokenMut,
	"local":     TokenLocal,
	"call":      TokenCall,
	"namespace": TokenNameSpace,
	"return":    TokenReturn,
	"break":     TokenBreak,
	"do_while":  TokenDoWhile,
}

var Operators = map[string]TokenType{
	"+":  TokenPlus,
	"-":  TokenMinus,
	"*":  TokenMultiplication,
	"/":  TokenDivision,
	"%":  TokenModulus,
	"**": TokenExponent,
	"//": TokenFloorDivision,
	"=":  TokenAssign,
	"+=": TokenPlusEqual,
	"-=": TokenMinusEqual,
	"*=": TokenMultiEqual,
	"/=": TokenDivideEqual,
	"==": TokenStrictEquality,
	"!=": TokenNotEqual,
	">":  TokenGreaterThan,
	"<":  TokenLessThan,
	">=": TokenGreaterThanOrEqualTo,
	"<=": TokenLessThanOrEqualTo,
	"->": TokenArrow,
	":=": TokenEvaluationAssign,
	":":  TokenInferReturn,
}

var Punctuations = map[string]TokenType{
	"[": TokenLeftSquareBrace,
	"]": TokenRightSquareBrace,
	"(": TokenLeftBrace,
	")": TokenRightBrace,
	"{": TokenScopeBegin,
	"}": TokenScopeEnd,
	";": TokenSemicolon,
	",": TokenComma,
	".": TokenDot,
}
