package runtime

import (
	"fmt"
	"strings"

	"kLang/src/parser"
)

func (runtime *Runtime) newParsable(args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return NullValue(), Error{Message: "Parsable expects source and an optional List[String] of source arguments"}
	}
	source, ok := stringData(args[0])
	if !ok {
		return NullValue(), Error{Message: "Parsable source expects String"}
	}
	var sourceArgs []string
	if len(args) == 2 {
		var err error
		sourceArgs, err = stringList(args[1])
		if err != nil {
			return NullValue(), Error{Message: "Parsable source arguments expect List[String]"}
		}
	}
	return runtime.parsableValue(source, source, sourceArgs)
}

func (runtime *Runtime) macroContext(args []Value) (Value, error) {
	if len(args) != 0 {
		return NullValue(), Error{Message: "macro_context expects no arguments"}
	}
	if len(runtime.parsableArgs) == 0 {
		return NullValue(), Error{Message: "macro_context is only available inside a keyword macro"}
	}
	items := runtime.parsableArgs[len(runtime.parsableArgs)-1]
	types := make([]Value, 0, len(items))
	for _, item := range items {
		types = append(types, StringValue(runtimeTypeName(item)))
	}
	return TableValue(map[string]Value{
		"args":       Value{Kind: ValueList, Data: append([]Value(nil), items...)},
		"arg_count":  IntValue(len(items)),
		"types":      Value{Kind: ValueList, Data: types},
		"runtime":    StringValue("runtime"),
		"macro_kind": StringValue("keyword_macro"),
	}), nil
}

func (runtime *Runtime) macroExpand(args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return NullValue(), Error{Message: "macro_expand expects source and optional List[String] source arguments"}
	}
	return runtime.newParsable(args)
}

func (runtime *Runtime) parsableValue(source, originalSource string, sourceArgs []string) (Value, error) {
	program, parseErrors := parser.Parse(source)
	if len(parseErrors) != 0 {
		first := parseErrors[0]
		return NullValue(), Error{Message: fmt.Sprintf("Parsable source failed to parse at %d:%d: %s", first.Line, first.Column, first.Message)}
	}
	ast := make([]Value, 0, len(program.Statements))
	for _, statement := range program.Statements {
		ast = append(ast, parsableStatementValue(statement))
	}
	allArgs := append(append([]string(nil), runtime.args...), sourceArgs...)
	programValue := objectValue("Program", map[string]Value{
		"module":  listFromStrings([]string{source}),
		"modules": listFromStrings([]string{source}),
	})
	buildValue := objectValue("BuildSystem", map[string]Value{
		"project_name":    StringValue("parsable"),
		"number_of_files": IntValue(1),
		"files":           listFromStrings([]string{"<parsable>"}),
		"backend":         StringValue("Standalone"),
	})
	workspaceValue := objectValue("WorkSpace", map[string]Value{
		"Program":      programValue,
		"BuildSystem":  buildValue,
		"program":      programValue,
		"build_system": buildValue,
	})
	runtimeInfo := TableValue(map[string]Value{
		"source_bytes":    IntValue(len(source)),
		"statement_count": IntValue(len(program.Statements)),
		"argument_count":  IntValue(len(allArgs)),
	})
	messagePoll := TableValue(map[string]Value{
		"active":      BoolValue(false),
		"step":        StringValue("idle"),
		"intercepted": BoolValue(false),
		"messages":    Value{Kind: ValueList, Data: []Value{}},
	})
	return objectValue("Parsable", map[string]Value{
		"source":          StringValue(source),
		"original_source": StringValue(originalSource),
		"ast":             {Kind: ValueList, Data: ast},
		"statement_count": IntValue(len(program.Statements)),
		"runtime_type":    typeInfoValue("Parsable"),
		"runtime_info":    runtimeInfo,
		"cli_args":        listFromStrings(runtime.args),
		"source_args":     listFromStrings(sourceArgs),
		"args":            listFromStrings(allArgs),
		"program":         programValue,
		"build_system":    buildValue,
		"workspace":       workspaceValue,
		"keywords":        {Kind: ValueList, Data: []Value{}},
		"message_poll":    messagePoll,
	}), nil
}

func parsableStatementValue(statement parser.Statement) Value {
	kind := strings.TrimPrefix(fmt.Sprintf("%T", statement), "parser.")
	kind = strings.TrimSuffix(kind, "Statement")
	fields := map[string]Value{
		"kind":   StringValue(kind),
		"line":   IntValue(statement.Position().Line),
		"column": IntValue(statement.Position().Column),
	}
	switch current := statement.(type) {
	case parser.FunctionStatement:
		fields["name"] = StringValue(current.Name)
	case parser.AliasFunctionStatement:
		fields["name"] = StringValue(current.Name)
	case parser.AliasStatement:
		fields["name"] = StringValue(current.Name)
		fields["target"] = StringValue(current.Target)
	case parser.TypeAliasStatement:
		fields["name"] = StringValue(current.Name)
		fields["target"] = StringValue(current.Resolved)
	case parser.VariableStatement:
		fields["name"] = StringValue(current.Name)
		fields["type"] = StringValue(current.Type)
	case parser.NamespaceStatement:
		fields["name"] = StringValue(current.Name)
	case parser.ScopeStatement:
		fields["name"] = StringValue(current.Name)
	case parser.EnumStatement:
		fields["name"] = StringValue(current.Name)
	}
	return TableValue(fields)
}

func (runtime *Runtime) parsableField(name string, args []Value) (Value, error) {
	object, err := requireObject(args, "Parsable", name)
	if err != nil {
		return NullValue(), err
	}
	field := strings.TrimPrefix(name, "parsable_")
	value, ok := object.Fields[field]
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("Parsable has no %s metadata", field)}
	}
	return cloneValue(value), nil
}

func (runtime *Runtime) transformParsable(name string, args []Value) (Value, error) {
	minimum := 2
	if name == "parsable_replace" {
		minimum = 3
	}
	if len(args) != minimum || !isObjectType(args[0], "Parsable") {
		return NullValue(), Error{Message: fmt.Sprintf("%s expects Parsable and %d String argument(s)", name, minimum-1)}
	}
	object := args[0].Data.(ObjectData)
	source := object.Fields["source"].Data.(string)
	original := object.Fields["original_source"].Data.(string)
	var next string
	switch name {
	case "parsable_with_source":
		next, _ = stringData(args[1])
	case "parsable_append":
		suffix, ok := stringData(args[1])
		if !ok {
			return NullValue(), Error{Message: "parsable_append expects a String suffix"}
		}
		next = source + suffix
	case "parsable_replace":
		old, oldOK := stringData(args[1])
		replacement, replacementOK := stringData(args[2])
		if !oldOK || !replacementOK {
			return NullValue(), Error{Message: "parsable_replace expects String search and replacement values"}
		}
		next = strings.ReplaceAll(source, old, replacement)
	}
	if name == "parsable_with_source" {
		if _, ok := stringData(args[1]); !ok {
			return NullValue(), Error{Message: "parsable_with_source expects a String source"}
		}
	}
	sourceArgs, _ := stringList(object.Fields["source_args"])
	value, err := runtime.parsableValue(next, original, sourceArgs)
	if err != nil {
		return Value{Kind: ValueResult, Data: ResultData{Ok: false, Value: StringValue(err.Error())}}, nil
	}
	return Value{Kind: ValueResult, Data: ResultData{Ok: true, Value: value}}, nil
}

func (runtime *Runtime) beginParsablePolling(args []Value) (Value, error) {
	object, err := requireObject(args, "Parsable", "parsable_begin_polling")
	if err != nil {
		return NullValue(), err
	}
	return cloneParsableWithPoll(object, TableValue(map[string]Value{
		"active":      BoolValue(true),
		"step":        StringValue("polling"),
		"intercepted": BoolValue(false),
		"messages":    Value{Kind: ValueList, Data: []Value{}},
	})), nil
}

func (runtime *Runtime) pollParsableMessage(name string, args []Value) (Value, error) {
	if len(args) != 2 || !isObjectType(args[0], "Parsable") || args[1].Kind != ValueTable {
		return NullValue(), Error{Message: fmt.Sprintf("%s expects Parsable and Table", name)}
	}
	object := args[0].Data.(ObjectData)
	message := args[1]
	forceIntercept := name == "parsable_intercept_message"
	requested := forceIntercept || parsableMessageRequestsAST(message)
	if !parsablePollingActive(object) && !forceIntercept {
		return TableValue(map[string]Value{
			"polling":     BoolValue(false),
			"requested":   BoolValue(requested),
			"intercepted": BoolValue(false),
			"step":        StringValue("idle"),
			"message":     cloneValue(message),
			"ast":         Value{Kind: ValueList, Data: []Value{}},
		}), nil
	}
	if requested {
		return parsableInterceptionResult(object, message), nil
	}
	return TableValue(map[string]Value{
		"polling":         BoolValue(true),
		"requested":       BoolValue(false),
		"intercepted":     BoolValue(false),
		"step":            StringValue("polled"),
		"message":         cloneValue(message),
		"ast":             Value{Kind: ValueList, Data: []Value{}},
		"statement_count": IntValue(0),
	}), nil
}

func cloneParsableWithPoll(object ObjectData, poll Value) Value {
	fields := make(map[string]Value, len(object.Fields))
	for key, value := range object.Fields {
		fields[key] = cloneValue(value)
	}
	fields["message_poll"] = poll
	return objectValue("Parsable", fields)
}

func parsablePollingActive(object ObjectData) bool {
	poll, ok := object.Fields["message_poll"]
	if !ok || poll.Kind != ValueTable {
		return false
	}
	active, ok := tableStringField(poll.Data.(TableData), "active")
	return ok && active.Kind == ValueBool && active.Data.(bool)
}

func parsableMessageRequestsAST(message Value) bool {
	if message.Kind != ValueTable {
		return false
	}
	kind, ok := tableStringField(message.Data.(TableData), "kind")
	if !ok || kind.Kind != ValueString {
		return false
	}
	switch strings.ToUpper(kind.Data.(string)) {
	case "REQUEST_AST", "AST_REQUEST", "POLL_AST":
		return true
	default:
		return false
	}
}

func parsableInterceptionResult(object ObjectData, message Value) Value {
	ast := cloneValue(object.Fields["ast"])
	statementCount := cloneValue(object.Fields["statement_count"])
	return TableValue(map[string]Value{
		"polling":         BoolValue(true),
		"requested":       BoolValue(true),
		"intercepted":     BoolValue(true),
		"step":            StringValue("intercepted"),
		"message":         cloneValue(message),
		"ast":             ast,
		"statement_count": statementCount,
	})
}

func tableStringField(table TableData, name string) (Value, bool) {
	return tableGet(table, TableKey{Kind: ValueString, Repr: name})
}

func (runtime *Runtime) executeKeywordMacro(macro parser.AliasStatement, args []Value, env *Environment) (Value, error) {
	macroEnv := NewEnvironment(env)
	typeName := "T"
	if len(args) != 0 {
		typeName = runtimeTypeName(args[0])
	}
	if err := macroEnv.Define("T", false, "Type", typeInfoValue(typeName), 0); err != nil {
		return NullValue(), err
	}
	runtime.parsableArgs = append(runtime.parsableArgs, append([]Value(nil), args...))
	defer func() { runtime.parsableArgs = runtime.parsableArgs[:len(runtime.parsableArgs)-1] }()
	signal, err := runtime.executeBlock(macro.Body, macroEnv, false)
	if err != nil {
		return NullValue(), err
	}
	switch signal.kind {
	case signalNone:
		return NullValue(), nil
	case signalReturn:
		if isObjectType(signal.value, "Parsable") {
			return runtime.executeParsableMacroExpansion(signal.value, env)
		}
		return signal.value, nil
	case signalThrow:
		return NullValue(), thrownError{Value: signal.value}
	default:
		return NullValue(), Error{Message: "keyword macros cannot break or continue"}
	}
}

func (runtime *Runtime) executeParsableMacroExpansion(value Value, env *Environment) (Value, error) {
	object := value.Data.(ObjectData)
	sourceValue, ok := object.Fields["source"]
	if !ok || sourceValue.Kind != ValueString {
		return NullValue(), Error{Message: "keyword macro expansion Parsable is missing source"}
	}
	program, errors := parser.Parse(sourceValue.Data.(string))
	if len(errors) != 0 {
		first := errors[0]
		return NullValue(), Error{Message: fmt.Sprintf("keyword macro expansion failed to parse at %d:%d: %s", first.Line, first.Column, first.Message)}
	}
	signal, err := runtime.executeBlock(program.Statements, env, false)
	if err != nil {
		return NullValue(), err
	}
	switch signal.kind {
	case signalNone:
		return NullValue(), nil
	case signalReturn:
		return signal.value, nil
	case signalThrow:
		return NullValue(), thrownError{Value: signal.value}
	default:
		return NullValue(), Error{Message: "keyword macro expansion cannot break or continue"}
	}
}
