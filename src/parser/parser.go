package parser

import (
	"fmt"
	"strconv"
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
	program.Statements = lowerDestructuringStatements(program.Statements)
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
	case lexer.TokenModule:
		if parser.peek().Type == lexer.TokenLeftBrace {
			return parser.parseModuleDirective()
		}
		return parser.parseExpressionOrAssignment()
	case lexer.TokenHash:
		return parser.parseDirective()
	case lexer.TokenAlias:
		if parser.peek().Type == lexer.TokenFunc {
			return parser.parseAliasFunction(false, false)
		}
		return parser.parseAlias()
	case lexer.TokenInline:
		return parser.parseInline()
	case lexer.TokenPrivate:
		return parser.parsePrivate()
	case lexer.TokenRegion:
		return parser.parseRegion()
	case lexer.TokenNameSpace:
		return parser.parseNamespace()
	case lexer.TokenTrait:
		return parser.parseTrait()
	case lexer.TokenImpl:
		return parser.parseImpl()
	case lexer.TokenEnum:
		return parser.parseEnum()
	case lexer.TokenFuncGroup:
		return parser.parseFunctionGroup()
	case lexer.TokenAt:
		return parser.parseTag()
	case lexer.TokenFunc:
		return parser.parseFunction(false, "", false, false, false, false, false)
	case lexer.TokenLazy:
		return parser.parseLazy()
	case lexer.TokenAsync:
		return parser.parseAsyncFunction()
	case lexer.TokenInner:
		return parser.parseInnerFunction()
	case lexer.TokenExport:
		return parser.parseExport()
	case lexer.TokenGlobal:
		if parser.peek().Type == lexer.TokenNameSpace {
			return parser.parseGlobalNamespace()
		}
		return parser.parseVariable("global", false)
	case lexer.TokenLocal:
		return parser.parseVariable("local", false)
	case lexer.TokenLet:
		return parser.parseInferredVariable("local", false, true)
	case lexer.TokenVal:
		return parser.parseInferredVariable("global", false, false)
	case lexer.TokenVar:
		return parser.parseInferredVariable("global", false, true)
	case lexer.TokenConst:
		return parser.parseInferredVariable("const", true, false)
	case lexer.TokenReturn:
		return parser.parseReturn()
	case lexer.TokenThrow:
		return parser.parseThrow()
	case lexer.TokenDefer:
		return parser.parseDefer()
	case lexer.TokenRun:
		return parser.parseRun()
	case lexer.TokenBreak:
		return parser.parseBreak()
	case lexer.TokenContinue:
		return parser.parseContinue()
	case lexer.TokenTry:
		return parser.parseTryCatch()
	case lexer.TokenPartial:
		return parser.parsePartial()
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
		if token.Type == lexer.TokenIdentifier && token.Literal == "module_caller" {
			return parser.parseModuleDirective()
		}
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

func (parser *Parser) parseModuleDirective() Statement {
	start := parser.advance()
	name := start.Literal
	parser.consume(lexer.TokenLeftBrace, fmt.Sprintf("expected '(' after %s", name))
	options := map[string]bool{}
	if !parser.check(lexer.TokenRightBrace) {
		for {
			key := parser.consume(lexer.TokenIdentifier, "expected module directive option")
			parser.consume(lexer.TokenInferReturn, "expected ':' after module directive option")
			value := parser.consume(lexer.TokenBool, "expected Bool module directive option value")
			options[key.Literal] = value.Literal == "True"
			if !parser.match(lexer.TokenComma) {
				break
			}
		}
	}
	parser.consume(lexer.TokenRightBrace, fmt.Sprintf("expected ')' after %s options", name))
	parser.consumeOptionalSemicolon()
	return ModuleDirectiveStatement{
		Pos:     positionFromToken(start),
		Name:    name,
		Options: options,
	}
}

func (parser *Parser) parseDirective() Statement {
	start := parser.consume(lexer.TokenHash, "expected directive")
	name := parser.consume(lexer.TokenIdentifier, "expected directive name")
	parser.consumeOptionalSemicolon()
	if name.Literal == "set_entry_point_to_here" {
		return EntryPointStatement{Pos: positionFromToken(start)}
	}
	parser.addError(name, fmt.Sprintf("unknown directive #%s", name.Literal))
	return nil
}

func (parser *Parser) parseAlias() Statement {
	start := parser.consume(lexer.TokenAlias, "expected alias")
	name := parser.consume(lexer.TokenIdentifier, "expected alias name")
	parser.consume(lexer.TokenAssign, "expected '=' after alias name")

	var parts []string
	for !parser.check(lexer.TokenSemicolon) && !parser.atEnd() {
		parts = append(parts, parser.current().Literal)
		parser.advance()
	}
	parser.consumeOptionalSemicolon()

	return AliasStatement{
		Pos:    positionFromToken(start),
		Name:   name.Literal,
		Target: strings.Join(parts, ""),
	}
}

func (parser *Parser) parseRegion() Statement {
	start := parser.consume(lexer.TokenRegion, "expected region")
	name := parser.consume(lexer.TokenIdentifier, "expected region name")
	parser.consume(lexer.TokenLeftBrace, "expected '(' after region name")
	typeName := parser.parseType()
	parser.consume(lexer.TokenComma, "expected ',' after region type")
	size := parser.parseExpressionUntil(lexer.TokenComma)
	parser.consume(lexer.TokenComma, "expected ',' after region size")
	count := parser.parseExpressionUntil(lexer.TokenRightBrace)
	parser.consume(lexer.TokenRightBrace, "expected ')' after region count")
	parser.consumeOptionalSemicolon()
	return RegionStatement{
		Pos:      positionFromToken(start),
		Name:     name.Literal,
		TypeName: typeName,
		Size:     size,
		Count:    count,
	}
}

func (parser *Parser) parseAliasFunction(inline bool, private bool) Statement {
	start := parser.consume(lexer.TokenAlias, "expected alias")
	parser.consume(lexer.TokenFunc, "expected function after alias")
	name := parser.consume(lexer.TokenIdentifier, "expected alias function name")
	typeParams := parser.parseAliasTypeParameters()
	parser.consume(lexer.TokenLeftBrace, "expected '(' after alias function name")
	params := parser.parseAliasParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after alias function parameters")
	returnType := "T"
	if parser.match(lexer.TokenArrow) || parser.match(lexer.TokenInferReturn) {
		returnType = parser.parseTypeOnCurrentLine()
	}
	structStyle := false
	if parser.match(lexer.TokenAssign) {
		parser.consume(lexer.TokenStruct, "expected struct after '=' in alias function type body")
		structStyle = true
	} else if parser.match(lexer.TokenStruct) {
		structStyle = true
	}
	blockStyle := parser.match(lexer.TokenScopeBegin)
	if structStyle && !blockStyle {
		parser.consume(lexer.TokenScopeBegin, "expected '{' after alias function struct")
		blockStyle = true
	}

	stmt := AliasFunctionStatement{
		Pos:        positionFromToken(start),
		Name:       name.Literal,
		TypeParams: typeParams,
		Params:     params,
		ReturnType: normalizeAliasReturnType(returnType),
		Struct:     structStyle,
		Inline:     inline,
		Private:    private,
	}

	for !parser.atEnd() && !(blockStyle && parser.check(lexer.TokenScopeEnd)) && !(!blockStyle && parser.check(lexer.TokenEnd)) {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		if parser.match(lexer.TokenLeftSquareBrace) {
			hookName := parser.consume(lexer.TokenIdentifier, "expected alias hook name")
			parser.consume(lexer.TokenRightSquareBrace, "expected ']' after alias hook")
			stmt.Hooks = append(stmt.Hooks, AliasHook{Name: hookName.Literal, Body: parser.collectAliasBodyTokens("alias hook")})
			continue
		}
		if parser.match(lexer.TokenHash) {
			directive := parser.consume(lexer.TokenIdentifier, "expected alias directive")
			if directive.Literal == "extend" {
				stmt.Methods = append(stmt.Methods, parser.parseAliasExtensionMethodsBlock()...)
				continue
			}
			stmt.Hooks = append(stmt.Hooks, AliasHook{Name: directive.Literal, Body: parser.collectAliasBodyTokens("alias directive")})
			continue
		}
		if parser.check(lexer.TokenTrait) {
			if trait, ok := parser.parseTrait().(TraitStatement); ok {
				stmt.Body = append(stmt.Body, trait)
			}
			continue
		}
		if parser.check(lexer.TokenImpl) {
			if impl, ok := parser.parseImpl().(ImplStatement); ok {
				stmt.Body = append(stmt.Body, impl)
			}
			continue
		}
		if parser.check(lexer.TokenAlias) {
			stmt.Body = append(stmt.Body, parser.parseAlias())
			continue
		}
		parser.advance()
	}
	if blockStyle {
		parser.consume(lexer.TokenScopeEnd, "expected '}' after alias function")
	} else {
		parser.consume(lexer.TokenEnd, "expected end after alias function")
	}
	return stmt
}

func (parser *Parser) collectAliasBodyTokens(label string) []lexer.Token {
	if parser.match(lexer.TokenScopeBegin) {
		return parser.collectUntilMatchingScopeEnd()
	}
	parser.consume(lexer.TokenDo, fmt.Sprintf("expected '{' or do after %s", label))
	return parser.collectUntilMatchingEnd()
}

func (parser *Parser) parseAliasParameters() []Parameter {
	var params []Parameter
	if parser.check(lexer.TokenRightBrace) || parser.atEnd() {
		return params
	}
	for {
		mutable := parser.match(lexer.TokenMut)
		name := parser.consumeIdentifierLike("expected alias parameter name")
		typeName := "T"
		var defaultExpr Expression
		if parser.match(lexer.TokenEvaluationAssign) {
			defaultExpr = parser.parseExpressionUntil(lexer.TokenComma, lexer.TokenRightBrace)
		} else if parser.match(lexer.TokenInferReturn) {
			typeName = normalizeAliasReturnType(parser.parseType())
		}
		if parser.match(lexer.TokenAssign) {
			defaultExpr = parser.parseExpressionUntil(lexer.TokenComma, lexer.TokenRightBrace)
		}
		params = append(params, Parameter{Name: name.Literal, Type: typeName, Mutable: mutable, Default: defaultExpr})
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	return params
}

func (parser *Parser) parseAliasTypeParameters() []TypeParameter {
	if !parser.match(lexer.TokenLeftSquareBrace) {
		return nil
	}
	var params []TypeParameter
	for !parser.check(lexer.TokenRightSquareBrace) && !parser.atEnd() {
		name := parser.consume(lexer.TokenIdentifier, "expected alias generic name")
		typeName := "T"
		if parser.match(lexer.TokenInferReturn) {
			typeName = parser.parseType()
		}
		params = append(params, TypeParameter{Name: name.Literal, Type: normalizeAliasReturnType(typeName)})
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	parser.consume(lexer.TokenRightSquareBrace, "expected ']' after alias generic list")
	return params
}

func (parser *Parser) parseAliasExtensionMethods() []FunctionStatement {
	var methods []FunctionStatement
	for !parser.check(lexer.TokenEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		if !parser.check(lexer.TokenFunc) {
			parser.advance()
			continue
		}
		methods = append(methods, parser.parseAliasExtensionMethod())
	}
	parser.consume(lexer.TokenEnd, "expected end after #extend")
	return methods
}

func (parser *Parser) parseAliasExtensionMethodsBlock() []FunctionStatement {
	if parser.match(lexer.TokenScopeBegin) {
		var methods []FunctionStatement
		for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
			if parser.match(lexer.TokenSemicolon) {
				continue
			}
			if !parser.check(lexer.TokenFunc) {
				parser.advance()
				continue
			}
			methods = append(methods, parser.parseAliasExtensionMethodBlock())
		}
		parser.consume(lexer.TokenScopeEnd, "expected '}' after #extend")
		return methods
	}
	parser.consume(lexer.TokenDo, "expected '{' or do after #extend")
	return parser.parseAliasExtensionMethods()
}

func (parser *Parser) parseAliasExtensionMethodBlock() FunctionStatement {
	start := parser.consume(lexer.TokenFunc, "expected function")
	name := parser.consume(lexer.TokenIdentifier, "expected extension method name")
	parser.consume(lexer.TokenLeftBrace, "expected '(' after extension method name")
	params := parser.parseParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after extension method parameters")
	returnType := "T"
	if parser.match(lexer.TokenArrow) || parser.match(lexer.TokenInferReturn) {
		returnType = parser.parseTypeOnCurrentLine()
	}
	body := parser.parseBlock()
	return FunctionStatement{
		Pos:        positionFromToken(start),
		Name:       name.Literal,
		Params:     params,
		ReturnType: normalizeAliasReturnType(returnType),
		Body:       body,
	}
}

func (parser *Parser) parseAliasExtensionMethod() FunctionStatement {
	start := parser.consume(lexer.TokenFunc, "expected function")
	name := parser.consume(lexer.TokenIdentifier, "expected extension method name")
	parser.consume(lexer.TokenLeftBrace, "expected '(' after extension method name")
	params := parser.parseParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after extension method parameters")
	returnType := "T"
	if parser.match(lexer.TokenArrow) || parser.match(lexer.TokenInferReturn) {
		returnType = parser.parseTypeOnCurrentLine()
	}
	bodyTokens := parser.collectUntilMatchingEnd()
	body := aliasMethodBodyFromTokens(bodyTokens)
	return FunctionStatement{
		Pos:        positionFromToken(start),
		Name:       name.Literal,
		Params:     params,
		ReturnType: normalizeAliasReturnType(returnType),
		Body:       body,
	}
}

func (parser *Parser) collectUntilMatchingEnd() []lexer.Token {
	var body []lexer.Token
	depth := 1
	for !parser.atEnd() && depth > 0 {
		token := parser.advance()
		switch token.Type {
		case lexer.TokenDo:
			depth++
		case lexer.TokenEnd:
			depth--
			if depth == 0 {
				return body
			}
		}
		body = append(body, token)
	}
	return body
}

func (parser *Parser) collectUntilMatchingScopeEnd() []lexer.Token {
	var body []lexer.Token
	depth := 1
	for !parser.atEnd() && depth > 0 {
		token := parser.advance()
		switch token.Type {
		case lexer.TokenScopeBegin:
			depth++
		case lexer.TokenScopeEnd:
			depth--
			if depth == 0 {
				return body
			}
		}
		body = append(body, token)
	}
	return body
}

func aliasMethodBodyFromTokens(tokens []lexer.Token) []Statement {
	for index, token := range tokens {
		if token.Type != lexer.TokenReturn {
			continue
		}
		end := index + 1
		for end < len(tokens) && tokens[end].Type != lexer.TokenSemicolon {
			end++
		}
		expr := expressionFromTokens(tokens[index+1 : end])
		return []Statement{ReturnStatement{Pos: positionFromToken(token), Expression: expr}}
	}
	return nil
}

func normalizeAliasReturnType(typeName string) string {
	switch typeName {
	case "type":
		return "T"
	case "Any":
		return "Any"
	case "int":
		return "Int"
	case "bool":
		return "Bool"
	case "string":
		return "String"
	default:
		return typeName
	}
}

func (parser *Parser) parseTypeOnCurrentLine() string {
	if parser.atEnd() {
		parser.addError(parser.previous(), "expected type")
		return ""
	}
	line := parser.current().Line
	var parts []string
	depth := 0
	for !parser.atEnd() && parser.current().Line == line {
		token := parser.current()
		if depth == 0 && len(parts) > 0 && (token.Type == lexer.TokenDo || token.Type == lexer.TokenScopeBegin || token.Type == lexer.TokenSemicolon) {
			break
		}
		switch token.Type {
		case lexer.TokenIdentifier, lexer.TokenRegion:
			parts = append(parts, token.Literal)
		case lexer.TokenInferReturn, lexer.TokenTypeUnion:
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
			if depth == 0 {
				return strings.Join(parts, "")
			}
			parts = append(parts, token.Literal)
		default:
			if len(parts) == 0 {
				parser.addError(token, "expected type")
			}
			return strings.Join(parts, "")
		}
		parser.advance()
		if depth == 0 && len(parts) > 0 && !parser.check(lexer.TokenLeftSquareBrace) &&
			!parser.check(lexer.TokenInferReturn) && !parser.check(lexer.TokenTypeUnion) &&
			parts[len(parts)-1] != ":" && parts[len(parts)-1] != "|" {
			break
		}
	}
	if len(parts) == 0 {
		parser.addError(parser.previous(), "expected type")
		return ""
	}
	if depth != 0 {
		parser.addError(parser.previous(), "expected ']' to close type")
	}
	return strings.Join(parts, "")
}

func (parser *Parser) parseNamespace() Statement {
	start := parser.consume(lexer.TokenNameSpace, "expected namespace")
	name := parser.consume(lexer.TokenIdentifier, "expected namespace name")
	body := parser.parseBlock()
	return NamespaceStatement{
		Pos:     positionFromToken(start),
		Name:    name.Literal,
		Private: false,
		Body:    body,
	}
}

func (parser *Parser) parseGlobalNamespace() Statement {
	start := parser.consume(lexer.TokenGlobal, "expected global")
	parser.consume(lexer.TokenNameSpace, "expected namespace")
	name := parser.consume(lexer.TokenIdentifier, "expected namespace name")
	body := parser.parseBlock()
	return NamespaceStatement{
		Pos:     positionFromToken(start),
		Name:    name.Literal,
		Private: false,
		Global:  true,
		Body:    body,
	}
}

func (parser *Parser) parseInline() Statement {
	parser.consume(lexer.TokenInline, "expected inline")
	switch {
	case parser.check(lexer.TokenFunc):
		return parser.parseFunction(false, "", false, false, false, true, false)
	case parser.check(lexer.TokenAlias) && parser.peek().Type == lexer.TokenFunc:
		return parser.parseAliasFunction(true, false)
	default:
		parser.addError(parser.current(), "inline must be followed by function or alias function")
		return nil
	}
}

func (parser *Parser) parsePrivate() Statement {
	start := parser.consume(lexer.TokenPrivate, "expected private")
	switch {
	case parser.check(lexer.TokenFunc):
		return parser.parseFunction(false, "", false, false, false, false, true)
	case parser.check(lexer.TokenInline):
		parser.advance()
		if parser.check(lexer.TokenFunc) {
			return parser.parseFunction(false, "", false, false, false, true, true)
		}
		if parser.check(lexer.TokenAlias) && parser.peek().Type == lexer.TokenFunc {
			return parser.parseAliasFunction(true, true)
		}
		parser.addError(parser.current(), "private inline must be followed by function or alias function")
		return nil
	case parser.check(lexer.TokenAlias) && parser.peek().Type == lexer.TokenFunc:
		return parser.parseAliasFunction(false, true)
	case parser.check(lexer.TokenNameSpace):
		namespace := parser.parseNamespace()
		if stmt, ok := namespace.(NamespaceStatement); ok {
			stmt.Private = true
			return stmt
		}
		return namespace
	case parser.check(lexer.TokenScopeBegin):
		body := parser.parseBlock()
		return PrivateBlockStatement{Pos: positionFromToken(start), Body: body}
	default:
		parser.addError(parser.current(), "private must be followed by function, alias function, namespace, or block")
		return nil
	}
}

func (parser *Parser) parseTrait() Statement {
	start := parser.consume(lexer.TokenTrait, "expected trait")
	name := parser.consume(lexer.TokenIdentifier, "expected trait name")
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start trait block")
	var methods []TraitMethod
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		method := parser.parseTraitMethod()
		if method.Name != "" {
			methods = append(methods, method)
		} else {
			parser.synchronize()
		}
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close trait block")
	return TraitStatement{Pos: positionFromToken(start), Name: name.Literal, Methods: methods}
}

func (parser *Parser) parseTraitMethod() TraitMethod {
	start := parser.consume(lexer.TokenFunc, "expected function in trait")
	name := parser.consume(lexer.TokenIdentifier, "expected trait method name")
	parser.consume(lexer.TokenLeftBrace, "expected '(' after trait method name")
	params := parser.parseParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after trait method parameters")
	returnType := "T"
	if parser.match(lexer.TokenInferReturn) {
		returnType = parser.parseType()
	}
	parser.consumeOptionalSemicolon()
	return TraitMethod{
		Pos:        positionFromToken(start),
		Name:       name.Literal,
		Params:     params,
		ReturnType: returnType,
	}
}

func (parser *Parser) parseImpl() Statement {
	start := parser.consume(lexer.TokenImpl, "expected impl")
	traitName := parser.consume(lexer.TokenIdentifier, "expected trait name after impl")
	parser.consume(lexer.TokenFor, "expected for after impl trait name")
	typeName := parser.parseType()
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start impl block")
	var methods []FunctionStatement
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		stmt := parser.parseFunction(false, "", false, false, false, false, false)
		if fn, ok := stmt.(FunctionStatement); ok {
			methods = append(methods, fn)
		} else {
			parser.synchronize()
		}
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close impl block")
	return ImplStatement{
		Pos:     positionFromToken(start),
		Trait:   traitName.Literal,
		Type:    typeName,
		Methods: methods,
	}
}

func (parser *Parser) parseEnum() Statement {
	start := parser.consume(lexer.TokenEnum, "expected enum")
	name := parser.consume(lexer.TokenIdentifier, "expected enum name")
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start enum block")

	var variants []EnumVariant
	nextOrdinal := 0
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) || parser.match(lexer.TokenComma) {
			continue
		}
		variant := parser.consume(lexer.TokenIdentifier, "expected enum variant name")
		ordinal := nextOrdinal
		if parser.match(lexer.TokenAssign) {
			value := parser.consume(lexer.TokenInt, "expected integer ordinal after '='")
			parsed, err := strconv.ParseInt(value.Literal, 0, 0)
			if err != nil {
				parser.addError(value, "expected integer ordinal after '='")
			} else {
				ordinal = int(parsed)
			}
		}
		variants = append(variants, EnumVariant{
			Pos:     positionFromToken(variant),
			Name:    variant.Literal,
			Ordinal: ordinal,
		})
		nextOrdinal = ordinal + 1
		parser.match(lexer.TokenSemicolon)
		parser.match(lexer.TokenComma)
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close enum block")
	parser.consumeOptionalSemicolon()
	return EnumStatement{Pos: positionFromToken(start), Name: name.Literal, Variants: variants}
}

func (parser *Parser) parseFunctionGroup() Statement {
	start := parser.consume(lexer.TokenFuncGroup, "expected function_group")
	name := parser.consume(lexer.TokenIdentifier, "expected function group name")
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start function_group block")
	var functions []string
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		entry := parser.consume(lexer.TokenIdentifier, "expected function_group entry")
		if entry.Literal != "set_function_as_part_of" {
			parser.addError(entry, fmt.Sprintf("unknown function_group entry %q", entry.Literal))
			parser.synchronize()
			continue
		}
		parser.consume(lexer.TokenLeftSquareBrace, "expected '[' after set_function_as_part_of")
		functions = append(functions, parser.parseFunctionGroupMembers()...)
		parser.consumeOptionalSemicolon()
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close function_group block")
	return FunctionGroupStatement{
		Pos:       positionFromToken(start),
		Name:      name.Literal,
		Functions: functions,
	}
}

func (parser *Parser) parseFunctionGroupMembers() []string {
	var functions []string
	squareDepth := 1
	scopeDepth := 0
	afterMetadata := false
	for !parser.atEnd() && squareDepth > 0 {
		token := parser.advance()
		switch token.Type {
		case lexer.TokenLeftSquareBrace:
			squareDepth++
		case lexer.TokenRightSquareBrace:
			squareDepth--
		case lexer.TokenScopeBegin:
			scopeDepth++
		case lexer.TokenScopeEnd:
			if scopeDepth > 0 {
				scopeDepth--
			}
		case lexer.TokenComma:
			if squareDepth == 1 && scopeDepth == 0 {
				afterMetadata = true
			}
		case lexer.TokenString, lexer.TokenIdentifier:
			if afterMetadata && squareDepth == 1 && scopeDepth == 0 {
				functions = append(functions, token.Literal)
				afterMetadata = false
			}
		}
	}
	if squareDepth != 0 {
		parser.addError(parser.previous(), "expected ']' after function_group members")
	}
	return functions
}

func (parser *Parser) parseTag() Statement {
	parser.consume(lexer.TokenAt, "expected '@'")
	tag := parser.consume(lexer.TokenIdentifier, "expected marker tag name")
	message := ""
	if parser.match(lexer.TokenLeftBrace) {
		value := parser.consume(lexer.TokenString, "expected marker tag message string")
		message = value.Literal
		parser.consume(lexer.TokenRightBrace, "expected ')' after marker tag message")
	}
	if tag.Literal != "deprecated" {
		parser.addError(tag, fmt.Sprintf("unknown marker tag @%s", tag.Literal))
		return nil
	}
	if !parser.check(lexer.TokenFunc) {
		parser.addError(parser.current(), "@deprecated must be followed by a function declaration")
		return nil
	}
	return parser.parseFunction(true, message, false, false, false, false, false)
}

func (parser *Parser) parseLazy() Statement {
	parser.consume(lexer.TokenLazy, "expected lazy")
	switch parser.current().Type {
	case lexer.TokenFunc:
		return parser.parseFunction(false, "", true, false, false, false, false)
	case lexer.TokenGlobal:
		start := parser.advance()
		return parser.parseVariableFromStart(start, "global", false, true)
	case lexer.TokenLocal:
		start := parser.advance()
		return parser.parseVariableFromStart(start, "local", false, true)
	case lexer.TokenLet:
		start := parser.advance()
		return parser.parseInferredVariableFromStart(start, "local", false, true, true)
	case lexer.TokenVal:
		start := parser.advance()
		return parser.parseInferredVariableFromStart(start, "global", false, false, true)
	case lexer.TokenVar:
		start := parser.advance()
		return parser.parseInferredVariableFromStart(start, "global", false, true, true)
	case lexer.TokenConst:
		parser.addError(parser.current(), "lazy const is not supported")
		return nil
	default:
		parser.addError(parser.current(), "lazy must be followed by function, local, global, let, val, or var")
		return nil
	}
}

func (parser *Parser) parseAsyncFunction() Statement {
	parser.consume(lexer.TokenAsync, "expected async")
	if !parser.check(lexer.TokenFunc) {
		parser.addError(parser.current(), "async must be followed by a function declaration")
		return nil
	}
	return parser.parseFunction(false, "", false, true, false, false, false)
}

func (parser *Parser) parseInnerFunction() Statement {
	parser.consume(lexer.TokenInner, "expected inner")
	if !parser.check(lexer.TokenFunc) {
		parser.addError(parser.current(), "inner must be followed by a function declaration")
		return nil
	}
	return parser.parseFunction(false, "", false, false, true, false, false)
}

func (parser *Parser) parseFunction(deprecated bool, deprecationMessage string, lazy bool, async bool, inner bool, inline bool, private bool) Statement {
	start := parser.consume(lexer.TokenFunc, "expected function")
	name := parser.consumeFunctionName("expected function name")
	typeParams := parser.parseTypeParameters()
	parser.consume(lexer.TokenLeftBrace, "expected '(' after function name")
	params := parser.parseParameters()
	parser.consume(lexer.TokenRightBrace, "expected ')' after function parameters")
	returnType := "T"
	var returnValues []ReturnValue
	if parser.match(lexer.TokenInferReturn) {
		returnType, returnValues = parser.parseReturnTypeSignature()
	}
	params = applyTypeParameterRestrictions(params, typeParams)
	returnType = applyReturnTypeRestriction(returnType, typeParams)
	for index := range returnValues {
		returnValues[index].Type = applyReturnTypeRestriction(returnValues[index].Type, typeParams)
	}
	body := parser.parseBlock()

	return FunctionStatement{
		Pos:                positionFromToken(start),
		Name:               name.Literal,
		TypeParams:         typeParams,
		Params:             params,
		ReturnType:         returnType,
		ReturnValues:       returnValues,
		Inline:             inline,
		Private:            private,
		Lazy:               lazy,
		Async:              async,
		Inner:              inner,
		Deprecated:         deprecated,
		DeprecationMessage: deprecationMessage,
		Body:               body,
	}
}

func applyTypeParameterRestrictions(params []Parameter, typeParams []TypeParameter) []Parameter {
	for index := range params {
		params[index].Type = applyReturnTypeRestriction(params[index].Type, typeParams)
	}
	return params
}

func applyReturnTypeRestriction(typeName string, typeParams []TypeParameter) string {
	for _, typeParam := range typeParams {
		if typeName == typeParam.Name {
			return typeParam.Type
		}
	}
	return typeName
}

func (parser *Parser) parseReturnTypeSignature() (string, []ReturnValue) {
	if !parser.match(lexer.TokenLeftBrace) {
		typeName := parser.parseType()
		return typeName, nil
	}
	tokens := parser.collectReturnTypeTupleTokens()
	parser.consume(lexer.TokenRightBrace, "expected ')' after return value list")

	parts := splitTopLevelExpressionTokens(tokens, lexer.TokenComma)
	values := make([]ReturnValue, 0, len(parts))
	for _, part := range parts {
		if len(part) == 0 {
			continue
		}
		values = append(values, parseReturnValueTokens(part))
	}
	if len(values) == 0 {
		parser.addError(parser.previous(), "function return tuple must contain at least one type")
		return "T", nil
	}

	typeParts := make([]string, 0, len(values))
	for _, value := range values {
		typeParts = append(typeParts, value.Type)
	}
	return "(" + strings.Join(typeParts, ",") + ")", values
}

func (parser *Parser) collectReturnTypeTupleTokens() []lexer.Token {
	var tokens []lexer.Token
	depth := 0
	for !parser.check(lexer.TokenRightBrace) && !parser.atEnd() {
		token := parser.current()
		switch token.Type {
		case lexer.TokenLeftSquareBrace:
			depth++
		case lexer.TokenRightSquareBrace:
			if depth > 0 {
				depth--
			}
		case lexer.TokenLeftBrace:
			depth++
		case lexer.TokenRightBrace:
			if depth == 0 {
				return tokens
			}
			depth--
		}
		tokens = append(tokens, parser.advance())
	}
	return tokens
}

func parseReturnValueTokens(tokens []lexer.Token) ReturnValue {
	mutable := false
	if len(tokens) > 0 && tokens[0].Type == lexer.TokenMut {
		mutable = true
		tokens = tokens[1:]
	}
	colon := -1
	for index, token := range tokens {
		if token.Type == lexer.TokenInferReturn {
			colon = index
			break
		}
	}
	if colon != -1 {
		return ReturnValue{
			Name:    tokensLiteral(tokens[:colon]),
			Type:    tokensLiteral(tokens[colon+1:]),
			Mutable: mutable,
		}
	}
	return ReturnValue{Type: tokensLiteral(tokens), Mutable: mutable}
}

func tokensLiteral(tokens []lexer.Token) string {
	parts := make([]string, 0, len(tokens))
	for _, token := range tokens {
		parts = append(parts, token.Literal)
	}
	return strings.Join(parts, "")
}

func (parser *Parser) parseTypeParameters() []TypeParameter {
	var params []TypeParameter
	if !parser.match(lexer.TokenLeftSquareBrace) {
		return params
	}
	for !parser.check(lexer.TokenRightSquareBrace) && !parser.atEnd() {
		name := parser.consume(lexer.TokenIdentifier, "expected generic type name")
		typeName := parser.parseRestrictedType(name.Literal)
		params = append(params, TypeParameter{Name: name.Literal, Type: typeName})
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	parser.consume(lexer.TokenRightSquareBrace, "expected ']' after generic type parameters")
	return params
}

func (parser *Parser) parseParameters() []Parameter {
	var params []Parameter
	if parser.check(lexer.TokenRightBrace) || parser.atEnd() {
		return params
	}

	for {
		mutable := parser.match(lexer.TokenMut)
		name := parser.consumeIdentifierLike("expected parameter name")
		var defaultExpr Expression
		typeName := "T"
		if parser.match(lexer.TokenEvaluationAssign) {
			defaultExpr = parser.parseExpressionUntil(lexer.TokenComma, lexer.TokenRightBrace)
		} else {
			parser.consume(lexer.TokenInferReturn, "expected ':' or ':=' after parameter name")
			typeName = parser.parseType()
		}
		if parser.match(lexer.TokenAssign) {
			defaultExpr = parser.parseExpressionUntil(lexer.TokenComma, lexer.TokenRightBrace)
		}
		params = append(params, Parameter{Name: name.Literal, Type: typeName, Mutable: mutable, Default: defaultExpr})

		if !parser.match(lexer.TokenComma) {
			break
		}
	}

	return params
}

func (parser *Parser) parseExport() Statement {
	start := parser.consume(lexer.TokenExport, "expected export")
	if parser.check(lexer.TokenGlobal) {
		return parser.parseVariableFromStart(start, "global", true, false)
	}
	if parser.check(lexer.TokenLocal) {
		return parser.parseVariableFromStart(start, "local", true, false)
	}
	parser.addError(parser.current(), "expected local or global after export")
	return nil
}

func (parser *Parser) parseVariable(scope string, exported bool) Statement {
	start := parser.advance()
	return parser.parseVariableFromStart(start, scope, exported, false)
}

func (parser *Parser) parseVariableFromStart(start lexer.Token, scope string, exported bool, lazy bool) Statement {
	if exported {
		parser.consume(scopeToken(scope), fmt.Sprintf("expected %s after export", scope))
	}
	mutable := parser.match(lexer.TokenMut)
	if parser.check(lexer.TokenLeftSquareBrace) || parser.check(lexer.TokenScopeBegin) {
		return parser.parseDestructuringFromStart(start, scope, exported, mutable, lazy)
	}
	if parser.checkIdentifierLike() && parser.peek().Type == lexer.TokenInferReturn {
		name := parser.consumeIdentifierLike("expected variable name")
		parser.consume(lexer.TokenInferReturn, "expected ':' after variable name")
		typeName := parser.parseType()
		var expr Expression
		if parser.match(lexer.TokenAssign) {
			expr = parser.parseExpressionUntil(lexer.TokenSemicolon)
		}
		if lazy && expr.Node == nil {
			parser.addError(parser.current(), "lazy variable declarations require an initializer")
		}
		parser.consumeOptionalSemicolon()

		return VariableStatement{
			Pos:        positionFromToken(start),
			Scope:      scope,
			Exported:   exported,
			Mutable:    mutable,
			Lazy:       lazy,
			Type:       typeName,
			Name:       name.Literal,
			Expression: expr,
		}
	}
	if scope == "local" && parser.checkIdentifierLike() && parser.peek().Type == lexer.TokenAssign {
		name := parser.consumeIdentifierLike("expected variable name")
		parser.consume(lexer.TokenAssign, "expected '=' after inferred local variable name")
		expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
		parser.consumeOptionalSemicolon()
		return VariableStatement{
			Pos:        positionFromToken(start),
			Scope:      scope,
			Inferred:   true,
			Exported:   exported,
			Mutable:    mutable,
			Lazy:       lazy,
			Type:       "T",
			Name:       name.Literal,
			Expression: expr,
		}
	}
	typeName := parser.parseType()
	name := parser.consumeIdentifierLike("expected variable name")
	var expr Expression
	if parser.match(lexer.TokenAssign) {
		expr = parser.parseExpressionUntil(lexer.TokenSemicolon)
	}
	if lazy && expr.Node == nil {
		parser.addError(parser.current(), "lazy variable declarations require an initializer")
	}
	parser.consumeOptionalSemicolon()

	return VariableStatement{
		Pos:        positionFromToken(start),
		Scope:      scope,
		Exported:   exported,
		Mutable:    mutable,
		Lazy:       lazy,
		Type:       typeName,
		Name:       name.Literal,
		Expression: expr,
	}
}

func (parser *Parser) parseInferredVariable(scope string, constant bool, defaultMutable bool) Statement {
	start := parser.advance()
	return parser.parseInferredVariableFromStart(start, scope, constant, defaultMutable, false)
}

func (parser *Parser) parseInferredVariableFromStart(start lexer.Token, scope string, constant bool, defaultMutable bool, lazy bool) Statement {
	mutable := defaultMutable
	if scope == "local" {
		mutable = parser.match(lexer.TokenMut)
	}
	if !constant && (parser.check(lexer.TokenLeftSquareBrace) || parser.check(lexer.TokenScopeBegin)) {
		return parser.parseDestructuringFromStart(start, scope, false, mutable, lazy)
	}
	typeName := "T"
	name := parser.consumeIdentifierLike("expected variable name")
	if parser.checkIdentifierLike() && parser.peek().Type == lexer.TokenAssign {
		typeName = inferredExplicitType(name.Literal)
		name = parser.consumeIdentifierLike("expected variable name")
	}
	if constant {
		mutable = false
	}
	if !parser.match(lexer.TokenAssign) {
		parser.addError(parser.current(), "inferred variable declarations require an initializer")
		parser.consumeOptionalSemicolon()
		return VariableStatement{Pos: positionFromToken(start), Scope: scope, Inferred: true, Mutable: mutable, Lazy: lazy, Type: typeName, Name: name.Literal}
	}
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	return VariableStatement{
		Pos:        positionFromToken(start),
		Scope:      scope,
		Inferred:   true,
		Mutable:    mutable,
		Lazy:       lazy,
		Type:       typeName,
		Name:       name.Literal,
		Expression: expr,
	}
}

func inferredExplicitType(typeName string) string {
	if typeName == "size" {
		return "Int"
	}
	return typeName
}

func (parser *Parser) parseDestructuringFromStart(start lexer.Token, scope string, exported bool, mutable bool, lazy bool) Statement {
	pattern := parser.parseDestructuringPattern()
	if pattern == nil {
		parser.synchronize()
		return nil
	}
	if !parser.match(lexer.TokenAssign) {
		parser.addError(parser.current(), "destructuring declarations require an initializer")
		parser.consumeOptionalSemicolon()
		return DestructuringStatement{Pos: positionFromToken(start), Scope: scope, Exported: exported, Mutable: mutable, Lazy: lazy, Pattern: pattern}
	}
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	return DestructuringStatement{
		Pos:        positionFromToken(start),
		Scope:      scope,
		Exported:   exported,
		Mutable:    mutable,
		Lazy:       lazy,
		Pattern:    pattern,
		Expression: expr,
	}
}

func (parser *Parser) parseDestructuringPattern() DestructuringPattern {
	if parser.check(lexer.TokenLeftSquareBrace) {
		return parser.parseListDestructuringPattern()
	}
	if parser.check(lexer.TokenScopeBegin) {
		return parser.parseObjectDestructuringPattern()
	}
	name := parser.consume(lexer.TokenIdentifier, "expected destructuring binding name")
	if name.Type != lexer.TokenIdentifier {
		return nil
	}
	return DestructuringBinding{Name: name.Literal}
}

func (parser *Parser) parseListDestructuringPattern() DestructuringPattern {
	parser.consume(lexer.TokenLeftSquareBrace, "expected '[' to start destructuring pattern")
	var items []DestructuringPattern
	for !parser.check(lexer.TokenRightSquareBrace) && !parser.atEnd() {
		items = append(items, parser.parseDestructuringPattern())
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	parser.consume(lexer.TokenRightSquareBrace, "expected ']' to close destructuring pattern")
	return DestructuringListPattern{Items: items}
}

func (parser *Parser) parseObjectDestructuringPattern() DestructuringPattern {
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start destructuring pattern")
	var fields []DestructuringObjectField
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		field := parser.consume(lexer.TokenIdentifier, "expected object field name in destructuring pattern")
		if field.Type != lexer.TokenIdentifier {
			return nil
		}
		pattern := DestructuringPattern(DestructuringBinding{Name: field.Literal})
		if parser.match(lexer.TokenInferReturn) {
			pattern = parser.parseDestructuringPattern()
		}
		fields = append(fields, DestructuringObjectField{Field: field.Literal, Pattern: pattern})
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close destructuring pattern")
	return DestructuringObjectPattern{Fields: fields}
}

func (parser *Parser) parseReturn() Statement {
	start := parser.consume(lexer.TokenReturn, "expected return")
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	values := splitReturnValues(expr)
	return ReturnStatement{
		Pos:        positionFromToken(start),
		Expression: expr,
		Values:     values,
	}
}

func splitReturnValues(expr Expression) []Expression {
	if len(expr.Tokens) == 0 {
		return nil
	}
	parts := splitTopLevelExpressionTokens(expr.Tokens, lexer.TokenComma)
	if len(parts) <= 1 {
		return nil
	}
	values := make([]Expression, 0, len(parts))
	for _, part := range parts {
		values = append(values, expressionFromTokens(part))
	}
	return values
}

func (parser *Parser) parseThrow() Statement {
	start := parser.consume(lexer.TokenThrow, "expected throw")
	expr := parser.parseExpressionUntil(lexer.TokenSemicolon)
	parser.consumeOptionalSemicolon()
	return ThrowStatement{
		Pos:        positionFromToken(start),
		Expression: expr,
	}
}

func (parser *Parser) parseDefer() Statement {
	start := parser.consume(lexer.TokenDefer, "expected defer")
	if parser.check(lexer.TokenScopeBegin) {
		return DeferStatement{Pos: positionFromToken(start), Body: parser.parseBlock()}
	}
	stmt := parser.parseStatement()
	return DeferStatement{Pos: positionFromToken(start), Stmt: stmt}
}

func (parser *Parser) parseRun() Statement {
	start := parser.consume(lexer.TokenRun, "expected run")
	if parser.check(lexer.TokenScopeBegin) {
		return RunStatement{Pos: positionFromToken(start), Body: parser.parseBlock()}
	}
	stmt := parser.parseStatement()
	return RunStatement{Pos: positionFromToken(start), Stmt: stmt}
}

func (parser *Parser) parseBreak() Statement {
	start := parser.consume(lexer.TokenBreak, "expected break")
	parser.consumeOptionalSemicolon()
	return BreakStatement{Pos: positionFromToken(start)}
}

func (parser *Parser) parseContinue() Statement {
	start := parser.consume(lexer.TokenContinue, "expected continue")
	parser.consumeOptionalSemicolon()
	return ContinueStatement{Pos: positionFromToken(start)}
}

func (parser *Parser) parsePartial() Statement {
	start := parser.consume(lexer.TokenPartial, "expected partial")
	if !parser.check(lexer.TokenIf) {
		parser.addError(parser.current(), "partial must be followed by a pattern matching if statement")
		return nil
	}
	return parser.parseConditionFromStart(start, "if", true)
}

func (parser *Parser) parseTryCatch() Statement {
	start := parser.consume(lexer.TokenTry, "expected try")
	tryBody := parser.parseBlock()
	parser.consume(lexer.TokenCatch, "expected catch after try block")
	errorName := parser.consume(lexer.TokenIdentifier, "expected catch error name")
	catchBody := parser.parseBlock()
	return TryCatchStatement{
		Pos:       positionFromToken(start),
		TryBody:   tryBody,
		ErrorName: errorName.Literal,
		CatchBody: catchBody,
	}
}

func (parser *Parser) parseCondition(kind string) Statement {
	start := parser.advance()
	return parser.parseConditionFromStart(start, kind, false)
}

func (parser *Parser) parseConditionFromStart(start lexer.Token, kind string, partial bool) Statement {
	if partial {
		parser.consume(lexer.TokenIf, "expected if after partial")
	} else if start.Type != lexer.TokenIf && start.Type != lexer.TokenUnless {
		parser.addError(start, "expected condition")
		return nil
	}
	condition := parser.parseExpressionUntil(lexer.TokenScopeBegin, lexer.TokenSemicolon)
	if kind == "if" && parser.check(lexer.TokenScopeBegin) {
		if value, ok := matchSubjectExpression(condition); ok {
			return parser.parseMatchFromCondition(start, value, partial)
		}
	}
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

func matchSubjectExpression(expr Expression) (Expression, bool) {
	tokens := trimExpressionTokens(expr.Tokens)
	if len(tokens) < 2 || tokens[len(tokens)-1].Type != lexer.TokenStrictEquality {
		return Expression{}, false
	}
	valueTokens := trimExpressionTokens(tokens[:len(tokens)-1])
	if len(valueTokens) == 0 {
		return Expression{}, false
	}
	return expressionFromTokens(valueTokens), true
}

func (parser *Parser) parseMatchFromCondition(start lexer.Token, value Expression, partial bool) Statement {
	parser.consume(lexer.TokenScopeBegin, "expected '{' to start pattern match")
	stmt := MatchStatement{
		Pos:     positionFromToken(start),
		Partial: partial,
		Value:   value,
	}
	for !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		if !parser.check(lexer.TokenCase) {
			parser.addError(parser.current(), "expected case in pattern match")
			parser.synchronize()
			continue
		}
		stmt.Cases = append(stmt.Cases, parser.parseMatchCase())
	}
	parser.consume(lexer.TokenScopeEnd, "expected '}' to close pattern match")
	return stmt
}

func (parser *Parser) parseMatchCase() MatchCase {
	start := parser.consume(lexer.TokenCase, "expected case")
	current := MatchCase{Pos: positionFromToken(start)}
	if parser.match(lexer.TokenInferReturn) {
		current.Default = true
	} else {
		current.Pattern = parser.parseExpressionUntil(lexer.TokenInferReturn)
		parser.consume(lexer.TokenInferReturn, "expected ':' after case pattern")
	}
	for !parser.check(lexer.TokenCase) && !parser.check(lexer.TokenScopeEnd) && !parser.atEnd() {
		if parser.match(lexer.TokenSemicolon) {
			continue
		}
		stmt := parser.parseStatement()
		if stmt == nil {
			parser.synchronize()
			continue
		}
		current.Body = append(current.Body, stmt)
	}
	return current
}

func parseInlineCondition(expr Expression) (Expression, []Statement) {
	index := inlineStatementStart(expr.Tokens)
	if index == -1 {
		return expr, nil
	}
	condition := expressionFromTokens(expr.Tokens[:index])
	return condition, parseInlineStatements(expr.Tokens[index:])
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
		case lexer.TokenBreak, lexer.TokenContinue, lexer.TokenReturn, lexer.TokenLocal, lexer.TokenGlobal, lexer.TokenExport,
			lexer.TokenThrow, lexer.TokenTry, lexer.TokenCall, lexer.TokenAt, lexer.TokenAlias, lexer.TokenLazy, lexer.TokenAsync, lexer.TokenInline,
			lexer.TokenPrivate, lexer.TokenDefer, lexer.TokenRun, lexer.TokenInner, lexer.TokenTrait, lexer.TokenImpl:
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
	return lowerDestructuringStatements(statements)
}

func (parser *Parser) parseType() string {
	if parser.atEnd() {
		parser.addError(parser.previous(), "expected type")
		return ""
	}

	start := parser.current()
	if start.Type == lexer.TokenIdentifier && parser.peek().Type == lexer.TokenIdentifier && parser.peek().Literal == "restrict" {
		parser.advance()
		return parser.parseRestrictedType(start.Literal)
	}
	var parts []string
	squareDepth := 0
	parenDepth := 0
	for !parser.atEnd() {
		token := parser.current()
		if squareDepth == 0 && parenDepth == 0 && (token.Type == lexer.TokenIdentifier || token.Type == lexer.TokenBool || token.Type == lexer.TokenRegion) && len(parts) > 0 &&
			parts[len(parts)-1] != ":" && parts[len(parts)-1] != "|" && parts[len(parts)-1] != "." {
			break
		}
		if squareDepth == 0 && parenDepth == 0 && (token.Type == lexer.TokenComma || token.Type == lexer.TokenRightBrace ||
			token.Type == lexer.TokenAssign || token.Type == lexer.TokenScopeBegin) {
			break
		}

		switch token.Type {
		case lexer.TokenIdentifier, lexer.TokenRegion:
			parts = append(parts, token.Literal)
		case lexer.TokenInt:
			parts = append(parts, token.Literal)
		case lexer.TokenInferReturn:
			parts = append(parts, token.Literal)
		case lexer.TokenTypeUnion:
			parts = append(parts, token.Literal)
		case lexer.TokenDot:
			parts = append(parts, token.Literal)
		case lexer.TokenLeftBrace:
			parenDepth++
			parts = append(parts, token.Literal)
		case lexer.TokenRightBrace:
			if parenDepth == 0 {
				return strings.Join(parts, "")
			}
			parenDepth--
			parts = append(parts, token.Literal)
		case lexer.TokenLeftSquareBrace:
			squareDepth++
			parts = append(parts, token.Literal)
		case lexer.TokenRightSquareBrace:
			if squareDepth == 0 {
				parser.addError(token, "unexpected ']' in type")
				return strings.Join(parts, "")
			}
			squareDepth--
			parts = append(parts, token.Literal)
		case lexer.TokenComma:
			parts = append(parts, token.Literal)
		default:
			if len(parts) == 0 {
				parser.addError(token, "expected type")
				return ""
			}
			if squareDepth != 0 {
				parser.addError(token, "expected ']' to close type")
			}
			if parenDepth != 0 {
				parser.addError(token, "expected ')' to close type")
			}
			return strings.Join(parts, "")
		}
		parser.advance()
		if squareDepth == 0 && parenDepth == 0 && len(parts) > 0 && !parser.check(lexer.TokenLeftSquareBrace) &&
			!parser.check(lexer.TokenInferReturn) && !parser.check(lexer.TokenTypeUnion) &&
			!parser.check(lexer.TokenDot) && !parser.check(lexer.TokenLeftBrace) &&
			parts[len(parts)-1] != ":" && parts[len(parts)-1] != "|" && parts[len(parts)-1] != "." {
			break
		}
	}

	if len(parts) == 0 {
		parser.addError(start, "expected type")
		return ""
	}
	if squareDepth != 0 {
		parser.addError(start, "expected ']' to close type")
	}
	if parenDepth != 0 {
		parser.addError(start, "expected ')' to close type")
	}
	return strings.Join(parts, "")
}

func (parser *Parser) parseRestrictedType(name string) string {
	if !parser.check(lexer.TokenIdentifier) || parser.current().Literal != "restrict" {
		return name
	}
	parser.advance()
	parser.consume(lexer.TokenLeftSquareBrace, "expected '[' after restrict")
	var allowed []string
	for !parser.check(lexer.TokenRightSquareBrace) && !parser.atEnd() {
		allowed = append(allowed, parser.parseType())
		if !parser.match(lexer.TokenComma) {
			break
		}
	}
	parser.consume(lexer.TokenRightSquareBrace, "expected ']' after restrict type list")
	if len(allowed) == 0 {
		return name
	}
	return name + ":" + strings.Join(allowed, "|")
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

func (parser *Parser) consumeIdentifierLike(message string) lexer.Token {
	if parser.checkIdentifierLike() {
		return parser.advance()
	}
	token := parser.current()
	parser.addError(token, message)
	return lexer.Token{Type: lexer.TokenIdentifier, Line: token.Line, Column: token.Column}
}

func (parser *Parser) checkIdentifierLike() bool {
	return parser.check(lexer.TokenIdentifier) || parser.check(lexer.TokenLet) || parser.check(lexer.TokenVar) ||
		parser.check(lexer.TokenVal) || parser.check(lexer.TokenConst) || parser.check(lexer.TokenModule)
}

func (parser *Parser) consumeFunctionName(message string) lexer.Token {
	if parser.check(lexer.TokenIdentifier) || parser.check(lexer.TokenCopy) || parser.check(lexer.TokenClone) {
		return parser.advance()
	}
	token := parser.current()
	parser.addError(token, message)
	return lexer.Token{Type: lexer.TokenIdentifier, Line: token.Line, Column: token.Column}
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

func (parser *Parser) peek() lexer.Token {
	if parser.pos+1 >= len(parser.tokens) {
		return lexer.Token{Type: lexer.TokenEOFDescriptor}
	}
	return parser.tokens[parser.pos+1]
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
		case lexer.TokenFunc, lexer.TokenFuncGroup, lexer.TokenInner, lexer.TokenGlobal, lexer.TokenLocal, lexer.TokenExport, lexer.TokenReturn,
			lexer.TokenThrow, lexer.TokenTry, lexer.TokenCatch, lexer.TokenRun,
			lexer.TokenIf, lexer.TokenUnless, lexer.TokenFor, lexer.TokenWhile,
			lexer.TokenDoWhile, lexer.TokenImport, lexer.TokenAlias, lexer.TokenLazy,
			lexer.TokenTrait, lexer.TokenImpl, lexer.TokenNameSpace:
			return
		}
		parser.advance()
	}
}

func scopeToken(scope string) lexer.TokenType {
	if scope == "global" {
		return lexer.TokenGlobal
	}
	return lexer.TokenLocal
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
