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
	TokenAtom       // :not_found
	TokenStructTag  // `json:"name"`

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
	TokenBang                 // !
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
	TokenHash             // #

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
	TokenTry        // try
	TokenCatch      // catch
	TokenThrow      // throw
	TokenEnd        // end
	TokenRegion     // region
	TokenUnless     // unless
	TokenFor        // for
	TokenForEach    // for_each
	TokenWhile      // while
	TokenDo         // do
	TokenIs         // is
	TokenAs         // as
	TokenIn         // in
	TokenImport     // import
	TokenAlias      // alias
	TokenLazy       // lazy
	TokenTemp       // temp
	TokenAsync      // async
	TokenAwait      // await
	TokenInline     // inline
	TokenPrivate    // private
	TokenDefer      // defer
	TokenRun        // run
	TokenScope      // scope
	TokenAssert     // assert
	TokenReport     // report
	TokenInner      // inner
	TokenTrait      // trait
	TokenImpl       // impl
	TokenEnum       // enum
	TokenStruct     // struct
	TokenMove       // move
	TokenCopy       // copy
	TokenClone      // clone
	TokenLambdaFunc // fun
	TokenGlobal     // global
	TokenMut        // mut
	TokenLocal      // local
	TokenLet        // let
	TokenVar        // var
	TokenVal        // val
	TokenConst      // const
	TokenCall       // call
	TokenModule     // module
	TokenNameSpace  // namespace
	TokenReturn     // return
	TokenBreak      // break
	TokenContinue   // continue
	TokenCase       // case
	TokenPartial    // partial
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
	"try":            TokenTry,
	"catch":          TokenCatch,
	"throw":          TokenThrow,
	"end":            TokenEnd,
	"region":         TokenRegion,
	"unless":         TokenUnless,
	"for":            TokenFor,
	"for_each":       TokenForEach,
	"while":          TokenWhile,
	"do":             TokenDo,
	"is":             TokenIs,
	"as":             TokenAs,
	"in":             TokenIn,
	"import":         TokenImport,
	"alias":          TokenAlias,
	"lazy":           TokenLazy,
	"temp":           TokenTemp,
	"async":          TokenAsync,
	"await":          TokenAwait,
	"inline":         TokenInline,
	"private":        TokenPrivate,
	"defer":          TokenDefer,
	"run":            TokenRun,
	"scope":          TokenScope,
	"assert":         TokenAssert,
	"report":         TokenReport,
	"inner":          TokenInner,
	"trait":          TokenTrait,
	"impl":           TokenImpl,
	"enum":           TokenEnum,
	"struct":         TokenStruct,
	"move":           TokenMove,
	"copy":           TokenCopy,
	"clone":          TokenClone,
	"global":         TokenGlobal,
	"fun":            TokenLambdaFunc,
	"mut":            TokenMut,
	"local":          TokenLocal,
	"let":            TokenLet,
	"var":            TokenVar,
	"val":            TokenVal,
	"const":          TokenConst,
	"call":           TokenCall,
	"module":         TokenModule,
	"namespace":      TokenNameSpace,
	"return":         TokenReturn,
	"break":          TokenBreak,
	"continue":       TokenContinue,
	"case":           TokenCase,
	"partial":        TokenPartial,
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
	"!":  TokenBang,
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
	"#": TokenHash,
}
