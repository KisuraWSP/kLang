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
	TokenPipe           // |>
	TokenTypeUnion      // |

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
	TokenNamespaceAccess  // ::
	TokenSemicolon        // ;
	TokenComma            // ,
	TokenDot              // .
	TokenAt               // @
	TokenQuestion         // ?

	// Special Scope Operators
	TokenScopeBegin // {
	TokenScopeEnd   // }

	// Keywords
	TokenAnd        // and
	TokenOr         // or
	TokenXor        // xor
	TokenNot        // not
	TokenFunc       // function
	TokenFuncGroup  // function_group
	TokenIf         // if
	TokenThen       // then
	TokenElse       // else
	TokenUnless     // unless
	TokenFor        // for
	TokenWhile      // while
	TokenDo         // do
	TokenIs         // is
	TokenAs         // as
	TokenIn         // in
	TokenImport     // import
	TokenAlias      // alias
	TokenLazy       // lazy
	TokenInner      // inner
	TokenTrait      // trait
	TokenImpl       // impl
	TokenMove       // move
	TokenLambdaFunc // fun
	TokenGlobal     // global
	TokenMut        // mut
	TokenLocal      // local
	TokenCall       // call
	TokenNameSpace  // namespace
	TokenReturn     // return
	TokenBreak      // break
	TokenDoWhile    // do_while
	TokenExport     // export
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

var Keywords = map[string]TokenType{
	"and":            TokenAnd,
	"or":             TokenOr,
	"xor":            TokenXor,
	"not":            TokenNot,
	"function":       TokenFunc,
	"function_group": TokenFuncGroup,
	"if":             TokenIf,
	"then":           TokenThen,
	"else":           TokenElse,
	"unless":         TokenUnless,
	"for":            TokenFor,
	"while":          TokenWhile,
	"do":             TokenDo,
	"is":             TokenIs,
	"as":             TokenAs,
	"in":             TokenIn,
	"import":         TokenImport,
	"alias":          TokenAlias,
	"lazy":           TokenLazy,
	"inner":          TokenInner,
	"trait":          TokenTrait,
	"impl":           TokenImpl,
	"move":           TokenMove,
	"global":         TokenGlobal,
	"fun":            TokenLambdaFunc,
	"mut":            TokenMut,
	"local":          TokenLocal,
	"call":           TokenCall,
	"namespace":      TokenNameSpace,
	"return":         TokenReturn,
	"break":          TokenBreak,
	"do_while":       TokenDoWhile,
	"export":         TokenExport,
}

var Operators = map[string]TokenType{
	"+":  TokenPlus,
	"-":  TokenMinus,
	"*":  TokenMultiplication,
	"/":  TokenDivision,
	"%":  TokenModulus,
	"**": TokenExponent,
	"//": TokenFloorDivision,
	"|>": TokenPipe,
	"|":  TokenTypeUnion,
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
	"::": TokenNamespaceAccess,
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
	"@": TokenAt,
	"?": TokenQuestion,
}
