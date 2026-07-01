package runtime

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"kLang/src/diagnostic"
	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	programcache "kLang/src/engine/program_cache"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/lexer"
	"kLang/src/parser"
)

type ValueKind string

const (
	ValueNull        ValueKind = "Null"
	ValueInt         ValueKind = "Int"
	ValueFloat       ValueKind = "Float"
	ValueString      ValueKind = "String"
	ValueBool        ValueKind = "Bool"
	ValueChar        ValueKind = "Char"
	ValueAtom        ValueKind = "Atom"
	ValueList        ValueKind = "List"
	ValueSet         ValueKind = "Set"
	ValueMap         ValueKind = "Map"
	ValueOption      ValueKind = "Option"
	ValueResult      ValueKind = "Result"
	ValueComplex     ValueKind = "Complex"
	ValueSIMD        ValueKind = "SIMD"
	ValueTable       ValueKind = "Table"
	ValueJSON        ValueKind = "JSON"
	ValueAwaitable   ValueKind = "Awaitable"
	ValueIterator    ValueKind = "Iterator"
	ValueCoroutine   ValueKind = "Coroutine"
	ValueThread      ValueKind = "Thread"
	ValueAtomic      ValueKind = "Atomic"
	ValueEnum        ValueKind = "Enum"
	ValueFunction    ValueKind = "Function"
	ValueObject      ValueKind = "Object"
	ValueBoundMethod ValueKind = "BoundMethod"
	ValueThunk       ValueKind = "Thunk"
)

type Value struct {
	Kind ValueKind
	Data any
}

type OptionData struct {
	Some  bool
	Value Value
}

type ResultData struct {
	Ok    bool
	Value Value
}

type ComplexData struct {
	Real float64
	Imag float64
}

type SIMDData struct {
	Lanes []Value
}

type AwaitableData struct {
	Function string
	Args     []Value
	Done     bool
	Value    Value
}

type IteratorData struct {
	Items     []Value
	Index     int
	Stages    []IteratorStage
	Exhausted bool
}

type IteratorStage struct {
	Kind     string
	Function string
	Count    int
	Seen     int
}

type CoroutineData struct {
	Function string
	Done     bool
	Value    Value
}

type ThreadData struct {
	Mutex sync.Mutex
	Done  chan struct{}
	Value Value
	Err   error
}

type AtomicData struct {
	Mutex   sync.Mutex
	Value   Value
	Version atomicUint64
	ID      uint64
}

type TableKey struct {
	Kind ValueKind
	Repr string
}

type SetData struct {
	Entries map[TableKey]Value
	Order   []TableKey
}

type TableData struct {
	Entries  map[TableKey]Value
	Order    []TableKey
	Fallback *TableData
}

type TableEntryData struct {
	Key   Value
	Value Value
}

type EnumData struct {
	Type    string
	Variant string
	Ordinal int
}

type ThunkData struct {
	Expr      parser.ExpressionNode
	Env       *Environment
	Evaluated bool
	Value     Value
}

type ObjectData struct {
	Type     string
	Struct   bool
	Fields   map[string]Value
	JSONTags map[string]string
}

type BoundMethodData struct {
	Type     string
	Name     string
	Receiver Value
}

type callSite struct {
	File   string
	Line   int
	Column int
}

type callArgument struct {
	Value   Value
	Binding *Binding
	Name    string
}

type RegionData struct {
	Name      string
	TypeName  string
	Size      Value
	Count     Value
	Temporary bool
}

type Result struct {
	Value  Value
	Output []string
	Memory MemoryStats
}

type TestResult struct {
	Name   string
	Value  Value
	Output []string
}

type Error struct {
	Code      string
	Message   string
	Rule      string
	Hint      string
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Labels    []diagnostic.Label
	Notes     []string
	Helps     []string
	Fixes     []diagnostic.TextEdit
	Frames    []diagnostic.StackFrame
}

func (err Error) Error() string {
	message := err.Message
	if err.Line > 0 {
		message = fmt.Sprintf("line %d:%d: %s", err.Line, err.Column, message)
	}
	if len(err.Frames) != 0 {
		var frames []string
		for _, frame := range err.Frames {
			rendered := frame.Function
			if frame.File != "" {
				rendered += " (" + frame.File + ")"
			}
			if frame.Line > 0 {
				rendered += fmt.Sprintf(":%d:%d", frame.Line, frame.Column)
			}
			frames = append(frames, rendered)
		}
		message += "\nStack trace:\n  at " + strings.Join(frames, "\n  at ")
	}
	return message
}

func (err Error) Diagnostic() diagnostic.Diagnostic {
	code := err.Code
	if code == "" {
		code = diagnostic.CodeRuntimeFailure
	}
	rule := err.Rule
	if rule == "" {
		rule = "runtime semantics"
	}
	hint := err.Hint
	if hint == "" {
		hint = "The program reached this code while running and could not continue safely."
	}
	return diagnostic.Normalize(diagnostic.Diagnostic{
		Code:      code,
		Severity:  diagnostic.SeverityError,
		Phase:     diagnostic.PhaseRuntime,
		File:      err.File,
		Line:      err.Line,
		Column:    err.Column,
		EndLine:   err.EndLine,
		EndColumn: err.EndColumn,
		Message:   err.Message,
		Rule:      rule,
		Hint:      hint,
		Labels:    err.Labels,
		Notes:     err.Notes,
		Helps:     err.Helps,
		Fixes:     err.Fixes,
		Frames:    err.Frames,
	})
}

func errorAt(pos parser.Position, message string) error {
	return errorAtCode(pos, diagnostic.CodeRuntimeFailure, "runtime semantics", message, "")
}

func errorAtCode(pos parser.Position, code string, rule string, message string, hint string) error {
	if pos.Line > 0 {
		return Error{
			Code: code, Rule: rule, Hint: hint,
			Line: pos.Line, Column: pos.Column,
			EndLine: pos.Line, EndColumn: pos.Column,
			Message: message,
		}
	}
	return Error{Code: code, Rule: rule, Hint: hint, Message: message}
}

type thrownError struct {
	Value Value
}

func (err thrownError) Error() string {
	return "thrown " + valueString(err.Value)
}

func thrownValue(err error) (Value, bool) {
	thrown, ok := err.(thrownError)
	if !ok {
		return NullValue(), false
	}
	return thrown.Value, true
}

type Runtime struct {
	mu                 sync.Mutex
	input              *RuntimeInput
	memory             *Memory
	global             *Environment
	functions          map[string]parser.FunctionStatement
	functionFiles      map[string]string
	globalFunctions    map[string][]string
	aliasFunctions     map[string]parser.AliasFunctionStatement
	extensions         map[string]map[string]parser.FunctionStatement
	keywordMacros      map[string]parser.AliasStatement
	enums              map[string]parser.EnumStatement
	regions            map[string]RegionData
	groups             map[string][]string
	closures           map[string]*Environment
	aliases            map[string]string
	output             *RuntimeOutput
	callDepth          int
	maxDepth           int
	callStack          []string
	callSites          []callSite
	nextFunc           int
	innerSets          []map[string]Value
	args               []string
	parsableArgs       [][]Value
	states             []StateRecord
	worker             bool
	transaction        *transactionContext
	raylibWindowOpen   bool
	raylibThreadLocked bool
	backend            string
}

type RuntimeOutput struct {
	Mutex sync.Mutex
	Lines []string
}

type RuntimeInput struct {
	Mutex  sync.Mutex
	Reader *bufio.Reader
}

type StateRecord struct {
	Phase    string
	Event    string
	Kind     string
	Name     string
	Type     string
	Runtime  string
	Function string
	Mutable  bool
	Moved    bool
}

const defaultMaxCallDepth = 1024

func New() *Runtime {
	return NewForBackend("Standalone")
}

func NewForBackend(backend string) *Runtime {
	if backend != "JS" && backend != "WASM" && backend != "Standalone" {
		backend = "Standalone"
	}
	return &Runtime{
		input:           &RuntimeInput{Reader: bufio.NewReaderSize(os.Stdin, 1<<20)},
		memory:          NewMemory(),
		global:          NewEnvironment(nil),
		functions:       map[string]parser.FunctionStatement{},
		functionFiles:   map[string]string{},
		globalFunctions: map[string][]string{},
		aliasFunctions:  map[string]parser.AliasFunctionStatement{},
		extensions:      map[string]map[string]parser.FunctionStatement{},
		keywordMacros:   map[string]parser.AliasStatement{},
		enums:           map[string]parser.EnumStatement{},
		regions:         map[string]RegionData{},
		groups:          map[string][]string{},
		closures:        map[string]*Environment{},
		aliases:         map[string]string{},
		output:          &RuntimeOutput{},
		maxDepth:        defaultMaxCallDepth,
		backend:         backend,
	}
}

func NewWithArgs(args []string) *Runtime {
	return NewWithArgsForBackend(args, "Standalone")
}

func NewWithArgsForBackend(args []string, backend string) *Runtime {
	runtime := NewForBackend(backend)
	runtime.args = append([]string(nil), args...)
	return runtime
}

func RunProgram(program file.Program) (Result, error) {
	return RunProgramWithArgs(program, nil)
}

func RunProgramWithArgs(program file.Program, args []string) (Result, error) {
	resolvedProgram, _, cacheHit := programcache.Load(program, false)
	if !cacheHit {
		var moduleReport modulesystem.Report
		resolvedProgram, moduleReport = modulesystem.ResolveProgram(program)
		if !moduleReport.Passed() {
			return Result{}, Error{Message: fmt.Sprintf("module resolution failed: %v", moduleReport.Errors)}
		}

		typeReport := typechecker.CheckProgram(resolvedProgram)
		if !typeReport.Passed() {
			return Result{}, Error{Message: fmt.Sprintf("type check failed: %v", typeReport.Errors)}
		}
	}

	parsed := parser.ParseLoadedProgram(resolvedProgram)
	if !parsed.Passed() {
		return Result{}, Error{Message: fmt.Sprintf("parse failed: %v", parsed.Errors())}
	}
	if !cacheHit {
		_ = programcache.Store(resolvedProgram, false, nil)
	}

	return NewWithArgs(args).Run(parsed)
}

func (runtime *Runtime) Run(program parser.ParsedProgram) (Result, error) {
	defer runtime.closeRaylib()
	entryName, entryDiagnostics := parser.ResolveEntryPoint(program)
	if len(entryDiagnostics) != 0 {
		diagnostic := entryDiagnostics[0]
		return Result{}, Error{
			Line:    diagnostic.Line,
			Column:  diagnostic.Column,
			Message: fmt.Sprintf("%s: %s", diagnostic.File, diagnostic.Message),
		}
	}
	if err := runtime.prepareProgram(program); err != nil {
		return Result{}, err
	}
	mainName, err := runtime.resolveFunctionName(entryName)
	if err != nil {
		return Result{}, err
	}
	if mainName == "" {
		return Result{}, Error{Message: fmt.Sprintf("entry point %q is not defined", entryName)}
	}

	value, err := runtime.callFunction(mainName, nil)
	if err != nil {
		if thrown, ok := thrownValue(err); ok {
			return Result{}, Error{Message: "uncaught exception: " + valueString(thrown)}
		}
		return Result{}, err
	}
	return Result{Value: value, Output: runtime.outputLines(), Memory: runtime.memory.Stats()}, nil
}

func (runtime *Runtime) RunTests(program parser.ParsedProgram, names []string) ([]TestResult, error) {
	defer runtime.closeRaylib()
	if err := runtime.prepareProgram(program); err != nil {
		return nil, err
	}
	results := make([]TestResult, 0, len(names))
	for _, name := range names {
		resolvedName, err := runtime.resolveFunctionName(name)
		if err != nil {
			return nil, err
		}
		if resolvedName == "" {
			return nil, Error{Message: fmt.Sprintf("test function %q is not defined", name)}
		}
		outputStart := len(runtime.outputLines())
		value, err := runtime.callFunction(resolvedName, nil)
		if err != nil {
			if thrown, ok := thrownValue(err); ok {
				return nil, Error{Message: "uncaught exception: " + valueString(thrown)}
			}
			return nil, err
		}
		output := runtime.outputLines()[outputStart:]
		results = append(results, TestResult{Name: resolvedName, Value: value, Output: output})
	}
	return results, nil
}

func (runtime *Runtime) prepareProgram(program parser.ParsedProgram) error {
	if err := runtime.defineArgs(); err != nil {
		return err
	}
	symbols, err := collectRuntimeSymbolsConcurrently(program.Sources)
	if err != nil {
		return err
	}
	for _, symbolSet := range symbols {
		if err := runtime.mergeRuntimeSymbols(symbolSet); err != nil {
			return err
		}
	}

	for _, source := range program.Sources {
		statements := filterRuntimeModuleFunctions(source.Program.Statements, "", source.ModuleFunctionFilter)
		signal, err := runtime.executeBlock(statements, runtime.global, false)
		if err != nil {
			return err
		}
		if signal.kind != signalNone {
			if signal.kind == signalThrow {
				return Error{Message: "uncaught exception: " + valueString(signal.value)}
			}
			return Error{Message: "top-level return, break, or continue is not allowed"}
		}
	}
	return nil
}

type runtimeSymbolSet struct {
	functions       map[string]parser.FunctionStatement
	functionFiles   map[string]string
	globalFunctions map[string][]string
	aliasFunctions  map[string]parser.AliasFunctionStatement
	extensions      map[string]map[string]parser.FunctionStatement
	keywordMacros   map[string]parser.AliasStatement
	enums           map[string]parser.EnumStatement
	regions         map[string]RegionData
	groups          map[string][]string
	aliases         map[string]string
}

func collectRuntimeSymbolsConcurrently(sources []parser.ParsedSource) ([]runtimeSymbolSet, error) {
	results := make([]runtimeSymbolSet, len(sources))
	errs := make([]error, len(sources))
	var wait sync.WaitGroup
	for index, source := range sources {
		wait.Add(1)
		go func(index int, source parser.ParsedSource) {
			defer wait.Done()
			results[index], errs[index] = collectRuntimeSymbolsForSource(source)
		}(index, source)
	}
	wait.Wait()
	for _, err := range errs {
		if err != nil {
			return nil, err
		}
	}
	return results, nil
}

func collectRuntimeSymbolsForSource(source parser.ParsedSource) (runtimeSymbolSet, error) {
	local := New()
	for _, stmt := range source.Program.Statements {
		if err := local.collectFunctions(stmt, "", source.ModuleFunctionFilter, source.Path, false); err != nil {
			return runtimeSymbolSet{}, err
		}
	}
	return runtimeSymbolSet{
		functions:       local.functions,
		functionFiles:   local.functionFiles,
		globalFunctions: local.globalFunctions,
		aliasFunctions:  local.aliasFunctions,
		extensions:      local.extensions,
		keywordMacros:   local.keywordMacros,
		enums:           local.enums,
		regions:         local.regions,
		groups:          local.groups,
		aliases:         local.aliases,
	}, nil
}

func (runtime *Runtime) mergeRuntimeSymbols(symbols runtimeSymbolSet) error {
	for name, region := range symbols.regions {
		runtime.regions[name] = region
	}
	for name, aliasFunction := range symbols.aliasFunctions {
		if _, exists := runtime.aliasFunctions[name]; exists {
			return Error{Message: fmt.Sprintf("alias function %q is already defined", name)}
		}
		runtime.aliasFunctions[name] = aliasFunction
	}
	for target, methods := range symbols.extensions {
		if runtime.extensions[target] == nil {
			runtime.extensions[target] = map[string]parser.FunctionStatement{}
		}
		for name, method := range methods {
			if _, exists := runtime.extensions[target][name]; exists {
				return Error{Message: fmt.Sprintf("extension method %s.%s is already defined", target, name)}
			}
			runtime.extensions[target][name] = method
		}
	}
	for name, macro := range symbols.keywordMacros {
		if _, exists := runtime.keywordMacros[name]; exists {
			return Error{Message: fmt.Sprintf("keyword macro %q is already defined", name)}
		}
		runtime.keywordMacros[name] = macro
	}
	for name, enum := range symbols.enums {
		if _, exists := runtime.enums[name]; exists {
			return Error{Message: fmt.Sprintf("enum %q is already defined", name)}
		}
		runtime.enums[name] = enum
	}
	for name, fn := range symbols.functions {
		if _, exists := runtime.functions[name]; exists {
			return Error{Message: fmt.Sprintf("function %q is already defined", name)}
		}
		runtime.functions[name] = fn
		runtime.functionFiles[name] = symbols.functionFiles[name]
	}
	for shortName, names := range symbols.globalFunctions {
		runtime.globalFunctions[shortName] = append(runtime.globalFunctions[shortName], names...)
	}
	for name, group := range symbols.groups {
		if _, exists := runtime.groups[name]; exists {
			return Error{Message: fmt.Sprintf("function_group %q is already defined", name)}
		}
		runtime.groups[name] = append([]string(nil), group...)
	}
	for name, target := range symbols.aliases {
		if _, exists := runtime.aliases[name]; exists {
			return Error{Message: fmt.Sprintf("alias %q is already defined", name)}
		}
		runtime.aliases[name] = target
	}
	return nil
}

func filterRuntimeModuleFunctions(statements []parser.Statement, namespace string, filter map[string]bool) []parser.Statement {
	if filter == nil {
		return statements
	}
	filtered := make([]parser.Statement, 0, len(statements))
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.FunctionStatement:
			if filter[namespace+current.Name] {
				filtered = append(filtered, current)
			}
		case parser.NamespaceStatement:
			current.Body = filterRuntimeModuleFunctions(current.Body, namespace+current.Name+".", filter)
			filtered = append(filtered, current)
		case parser.RunStatement:
			if current.Stmt != nil {
				filteredStmt := filterRuntimeModuleFunctions([]parser.Statement{current.Stmt}, namespace, filter)
				if len(filteredStmt) == 1 {
					current.Stmt = filteredStmt[0]
				} else {
					current.Stmt = nil
				}
			}
			current.Body = filterRuntimeModuleFunctions(current.Body, namespace, filter)
			filtered = append(filtered, current)
		default:
			filtered = append(filtered, current)
		}
	}
	return filtered
}

func (runtime *Runtime) appendOutput(line string) {
	runtime.output.Mutex.Lock()
	runtime.output.Lines = append(runtime.output.Lines, line)
	runtime.output.Mutex.Unlock()
}

func (runtime *Runtime) outputLines() []string {
	runtime.output.Mutex.Lock()
	defer runtime.output.Mutex.Unlock()
	return append([]string(nil), runtime.output.Lines...)
}

func (runtime *Runtime) reportValue(label string, value Value, pos parser.Position) {
	if strings.TrimSpace(label) == "" {
		label = "<expression>"
	}
	lines := []string{
		fmt.Sprintf("[report] %s = %s :: %s", label, valueString(value), runtimeTypeName(value)),
	}
	frames := runtime.stackTraceLines(pos)
	if len(frames) == 0 {
		lines = append(lines, "  at <top-level>")
	} else {
		for _, frame := range frames {
			lines = append(lines, "  at "+frame)
		}
	}
	runtime.appendOutput(strings.Join(lines, "\n"))
}

func (runtime *Runtime) errorWithStack(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := thrownValue(err); ok {
		return err
	}
	frames := runtime.stackTraceFrames(parser.Position{})
	if len(frames) == 0 {
		return err
	}
	if current, ok := err.(Error); ok {
		if len(current.Frames) != 0 {
			return current
		}
		if current.File == "" && len(runtime.callStack) > 0 {
			current.File = runtime.functionFiles[runtime.callStack[len(runtime.callStack)-1]]
		}
		current.Frames = frames
		return current
	}
	return Error{Code: diagnostic.CodeRuntimeFailure, Message: err.Error(), Frames: frames}
}

func (runtime *Runtime) stackTraceLines(pos parser.Position) []string {
	structured := runtime.stackTraceFrames(pos)
	frames := make([]string, 0, len(structured))
	for _, frame := range structured {
		rendered := frame.Function
		if frame.File != "" {
			rendered += " (" + frame.File + ")"
		}
		if frame.Line > 0 {
			rendered += fmt.Sprintf(":%d:%d", frame.Line, frame.Column)
		}
		frames = append(frames, rendered)
	}
	return frames
}

func (runtime *Runtime) stackTraceFrames(pos parser.Position) []diagnostic.StackFrame {
	frames := make([]diagnostic.StackFrame, 0, len(runtime.callStack)+1)
	if len(runtime.callStack) == 0 {
		if pos.Line > 0 {
			frames = append(frames, diagnostic.StackFrame{Function: "<top-level>", Line: pos.Line, Column: pos.Column})
		}
		return frames
	}
	for index := len(runtime.callStack) - 1; index >= 0; index-- {
		name := runtime.callStack[index]
		file := runtime.functionFiles[name]
		frame := diagnostic.StackFrame{Function: name, File: file}
		siteIndex := len(runtime.callSites) - (len(runtime.callStack) - index)
		if siteIndex >= 0 && siteIndex < len(runtime.callSites) {
			site := runtime.callSites[siteIndex]
			if site.File != "" {
				frame.File = site.File
			}
			frame.Line = site.Line
			frame.Column = site.Column
		}
		frames = append(frames, frame)
	}
	if pos.Line > 0 {
		frames = append(frames, diagnostic.StackFrame{Function: "<report>", Line: pos.Line, Column: pos.Column})
	}
	return frames
}

func (runtime *Runtime) childRuntime() *Runtime {
	child := &Runtime{
		input:           runtime.input,
		memory:          runtime.memory,
		global:          runtime.global,
		functions:       cloneFunctionMap(runtime.functions),
		functionFiles:   cloneStringMap(runtime.functionFiles),
		globalFunctions: cloneGroupMap(runtime.globalFunctions),
		aliasFunctions:  cloneAliasFunctionMap(runtime.aliasFunctions),
		extensions:      cloneExtensionMap(runtime.extensions),
		keywordMacros:   cloneKeywordMacroMap(runtime.keywordMacros),
		enums:           cloneEnumMap(runtime.enums),
		regions:         cloneRegionMap(runtime.regions),
		groups:          cloneGroupMap(runtime.groups),
		closures:        cloneClosureMap(runtime.closures),
		aliases:         cloneStringMap(runtime.aliases),
		output:          runtime.output,
		maxDepth:        runtime.maxDepth,
		args:            append([]string(nil), runtime.args...),
		worker:          true,
		backend:         runtime.backend,
	}
	return child
}

func cloneKeywordMacroMap(items map[string]parser.AliasStatement) map[string]parser.AliasStatement {
	copied := make(map[string]parser.AliasStatement, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneFunctionMap(items map[string]parser.FunctionStatement) map[string]parser.FunctionStatement {
	copied := make(map[string]parser.FunctionStatement, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneAliasFunctionMap(items map[string]parser.AliasFunctionStatement) map[string]parser.AliasFunctionStatement {
	copied := make(map[string]parser.AliasFunctionStatement, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneExtensionMap(items map[string]map[string]parser.FunctionStatement) map[string]map[string]parser.FunctionStatement {
	copied := make(map[string]map[string]parser.FunctionStatement, len(items))
	for target, methods := range items {
		copied[target] = make(map[string]parser.FunctionStatement, len(methods))
		for name, method := range methods {
			copied[target][name] = method
		}
	}
	return copied
}

func cloneEnumMap(items map[string]parser.EnumStatement) map[string]parser.EnumStatement {
	copied := make(map[string]parser.EnumStatement, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneRegionMap(items map[string]RegionData) map[string]RegionData {
	copied := make(map[string]RegionData, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneGroupMap(items map[string][]string) map[string][]string {
	copied := make(map[string][]string, len(items))
	for key, value := range items {
		copied[key] = append([]string(nil), value...)
	}
	return copied
}

func cloneClosureMap(items map[string]*Environment) map[string]*Environment {
	copied := make(map[string]*Environment, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func cloneStringMap(items map[string]string) map[string]string {
	copied := make(map[string]string, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func (runtime *Runtime) defineArgs() error {
	values := make([]Value, 0, len(runtime.args))
	for _, arg := range runtime.args {
		values = append(values, StringValue(arg))
	}
	return runtime.defineValueInRegion(runtime.global, "Args", false, "List[String]", Value{Kind: ValueList, Data: values}, MemoryHeap)
}

func (runtime *Runtime) collectFunctions(stmt parser.Statement, namespace string, filter map[string]bool, sourcePath string, globalNamespace bool) error {
	switch current := stmt.(type) {
	case parser.RegionStatement:
		runtime.regions[current.Name] = RegionData{Name: current.Name, TypeName: current.TypeName, Temporary: current.Temporary}
	case parser.AliasFunctionStatement:
		if _, exists := runtime.aliasFunctions[current.Name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("alias function %q is already defined", current.Name))
		}
		runtime.aliasFunctions[current.Name] = current
	case parser.ExtensionStatement:
		target := normalizeRuntimeType(current.Target)
		if runtime.extensions[target] == nil {
			runtime.extensions[target] = map[string]parser.FunctionStatement{}
		}
		for _, method := range current.Methods {
			if _, exists := runtime.extensions[target][method.Name]; exists {
				return errorAt(method.Pos, fmt.Sprintf("extension method %s.%s is already defined", target, method.Name))
			}
			runtime.extensions[target][method.Name] = method
		}
	case parser.EnumStatement:
		if _, exists := runtime.enums[current.Name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("enum %q is already defined", current.Name))
		}
		runtime.enums[current.Name] = current
	case parser.FunctionStatement:
		name := namespace + current.Name
		if filter != nil && !filter[name] {
			return nil
		}
		if _, exists := runtime.functions[name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("function %q is already defined", name))
		}
		runtime.functions[name] = current
		runtime.functionFiles[name] = sourcePath
		if globalNamespace {
			shortName := unqualifiedRuntimeFunctionName(name)
			runtime.globalFunctions[shortName] = append(runtime.globalFunctions[shortName], name)
		}
	case parser.FunctionGroupStatement:
		name := namespace + current.Name
		if _, exists := runtime.groups[name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("function_group %q is already defined", name))
		}
		runtime.groups[name] = append([]string(nil), current.Functions...)
	case parser.TraitStatement:
		return nil
	case parser.ImplStatement:
		return nil
	case parser.AliasStatement:
		if current.KeywordMacro {
			if _, exists := runtime.keywordMacros[current.Name]; exists {
				return errorAt(current.Pos, fmt.Sprintf("keyword macro %q is already defined", current.Name))
			}
			runtime.keywordMacros[current.Name] = current
			return nil
		}
		if current.Target == "" {
			return errorAt(current.Pos, fmt.Sprintf("alias %q is missing a namespace target", current.Name))
		}
		if _, ok := typeSizeof(current.Target); ok {
			return nil
		}
		if _, exists := runtime.aliases[current.Name]; exists {
			return errorAt(current.Pos, fmt.Sprintf("alias %q is already defined", current.Name))
		}
		runtime.aliases[current.Name] = strings.ReplaceAll(strings.TrimSpace(current.Target), "::", ".")
	case parser.NamespaceStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace+current.Name+".", filter, sourcePath, globalNamespace || current.Global); err != nil {
				return err
			}
		}
	case parser.MatchStatement:
		for _, matchCase := range current.Cases {
			for _, nested := range matchCase.Body {
				if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
					return err
				}
			}
		}
	case parser.IfStatement:
		for _, nested := range current.Consequence {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
		for _, nested := range current.Alternative {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.LoopStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.TryCatchStatement:
		for _, nested := range current.TryBody {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
		for _, nested := range current.CatchBody {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.TransactionStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.PrivateBlockStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.ScopeStatement:
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.DeferStatement:
		if current.Stmt != nil {
			if err := runtime.collectFunctions(current.Stmt, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	case parser.RunStatement:
		if current.Stmt != nil {
			if err := runtime.collectFunctions(current.Stmt, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
		for _, nested := range current.Body {
			if err := runtime.collectFunctions(nested, namespace, filter, sourcePath, globalNamespace); err != nil {
				return err
			}
		}
	}
	return nil
}

type signalKind int

const (
	signalNone signalKind = iota
	signalReturn
	signalBreak
	signalContinue
	signalTailCall
	signalThrow
)

type signal struct {
	kind     signalKind
	value    Value
	tailName string
	tailArgs []Value
}

func (runtime *Runtime) executeBlock(statements []parser.Statement, env *Environment, inLoop bool) (signal, error) {
	if err := runtime.executeRunStatements(statements, env, inLoop); err != nil {
		return signal{}, err
	}
	var deferred []parser.DeferStatement
	for _, stmt := range statements {
		if deferStmt, ok := stmt.(parser.DeferStatement); ok {
			deferred = append(deferred, deferStmt)
			continue
		}
		if _, ok := stmt.(parser.RunStatement); ok {
			continue
		}
		currentSignal, err := runtime.executeStatement(stmt, env, inLoop)
		if err != nil {
			return signal{}, runtime.executeDeferred(deferred, env, inLoop, err)
		}
		if currentSignal.kind != signalNone {
			if err := runtime.executeDeferred(deferred, env, inLoop, nil); err != nil {
				return signal{}, err
			}
			return currentSignal, nil
		}
	}
	if err := runtime.executeDeferred(deferred, env, inLoop, nil); err != nil {
		return signal{}, err
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeRunStatements(statements []parser.Statement, env *Environment, inLoop bool) error {
	for _, stmt := range statements {
		current, ok := stmt.(parser.RunStatement)
		if !ok {
			continue
		}
		var currentSignal signal
		var err error
		if len(current.Body) != 0 {
			currentSignal, err = runtime.executeBlock(current.Body, NewEnvironment(env), inLoop)
		} else if current.Stmt != nil {
			currentSignal, err = runtime.executeStatement(current.Stmt, env, inLoop)
		}
		if err != nil {
			return err
		}
		if currentSignal.kind == signalThrow {
			return Error{Message: "run threw exception: " + valueString(currentSignal.value)}
		}
		if currentSignal.kind != signalNone {
			return Error{Message: "run cannot return, break, or continue"}
		}
	}
	return nil
}

func (runtime *Runtime) executeDeferred(deferred []parser.DeferStatement, env *Environment, inLoop bool, existing error) error {
	for index := len(deferred) - 1; index >= 0; index-- {
		current := deferred[index]
		var currentSignal signal
		var err error
		if len(current.Body) != 0 {
			currentSignal, err = runtime.executeBlock(current.Body, NewEnvironment(env), inLoop)
		} else if current.Stmt != nil {
			currentSignal, err = runtime.executeStatement(current.Stmt, env, inLoop)
		}
		if err != nil && existing == nil {
			existing = err
		}
		if currentSignal.kind == signalThrow && existing == nil {
			existing = Error{Message: "defer threw exception: " + valueString(currentSignal.value)}
		}
	}
	return existing
}

func (runtime *Runtime) evalReturnValue(stmt parser.ReturnStatement, env *Environment) (Value, error) {
	if len(stmt.Values) == 0 {
		return runtime.evalExpression(stmt.Expression.Node, env)
	}
	items := make([]Value, 0, len(stmt.Values))
	for _, expr := range stmt.Values {
		value, err := runtime.evalExpression(expr.Node, env)
		if err != nil {
			return NullValue(), err
		}
		items = append(items, value)
	}
	return Value{Kind: ValueList, Data: items}, nil
}

func (runtime *Runtime) executeStatement(stmt parser.Statement, env *Environment, inLoop bool) (signal, error) {
	switch current := stmt.(type) {
	case parser.ImportStatement:
		return signal{kind: signalNone}, nil
	case parser.ModuleDirectiveStatement:
		return signal{kind: signalNone}, nil
	case parser.EntryPointStatement:
		return signal{kind: signalNone}, nil
	case parser.AliasStatement:
		return signal{kind: signalNone}, nil
	case parser.TypeAliasStatement:
		return signal{kind: signalNone}, nil
	case parser.NamespaceStatement:
		return signal{kind: signalNone}, nil
	case parser.RegionStatement:
		size, err := runtime.evalExpression(current.Size.Node, env)
		if err != nil {
			size = NullValue()
		}
		count, err := runtime.evalExpression(current.Count.Node, env)
		if err != nil {
			count = NullValue()
		}
		runtime.regions[current.Name] = RegionData{Name: current.Name, TypeName: current.TypeName, Size: size, Count: count, Temporary: current.Temporary}
		runtime.recordState(StateRecord{
			Phase:    "runtime",
			Event:    "define",
			Kind:     regionRuntimeStateKind(current),
			Name:     current.Name,
			Type:     current.TypeName,
			Runtime:  "Region",
			Function: runtime.currentFunctionName(),
		})
		return signal{kind: signalNone}, nil
	case parser.AliasFunctionStatement:
		return signal{kind: signalNone}, nil
	case parser.ExtensionStatement:
		return signal{kind: signalNone}, nil
	case parser.EnumStatement:
		return signal{kind: signalNone}, nil
	case parser.FunctionGroupStatement:
		return signal{kind: signalNone}, nil
	case parser.FunctionStatement:
		name, err := runtime.defineLocalFunction(current, env)
		if err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		value := FunctionValue(name)
		if current.Inner && len(runtime.innerSets) > 0 {
			runtime.innerSets[len(runtime.innerSets)-1][current.Name] = value
		}
		if err := runtime.defineValue(env, current.Name, false, functionTypeName(current), value); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.TraitStatement:
		return signal{kind: signalNone}, nil
	case parser.ImplStatement:
		return signal{kind: signalNone}, nil
	case parser.VariableStatement:
		value := zeroValue(current.Type)
		if current.Expression.Node != nil {
			if current.Lazy {
				value = ThunkValue(current.Expression.Node, env)
			} else {
				var err error
				value, err = runtime.evalExpression(current.Expression.Node, env)
				if err != nil {
					if thrown, ok := thrownValue(err); ok {
						return signal{kind: signalThrow, value: thrown}, nil
					}
					return signal{}, err
				}
			}
		}
		if current.Type == "Table" && value.Kind == ValueMap {
			value = TableValue(value.Data.(map[string]Value))
		}
		if strings.HasPrefix(normalizeRuntimeType(current.Type), "Map[") && value.Kind == ValueTable {
			items, err := tableToStringMap(value.Data.(TableData))
			if err != nil {
				return signal{}, errorAt(current.Pos, err.Error())
			}
			value = Value{Kind: ValueMap, Data: items}
		}
		typeName := current.Type
		if current.Inferred && typeName == "T" && !current.Lazy {
			typeName = runtimeTypeName(value)
		}
		typeName = normalizeRuntimeType(typeName)
		if !valueMatchesType(value, typeName) {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("cannot assign %s to %s variable %q", value.Kind, typeName, current.Name))
		}
		if isDiscardIdentifier(current.Name) {
			return signal{kind: signalNone}, nil
		}
		targetEnv := env
		region := runtime.memoryRegionForType(typeName)
		if current.Scope == "global" || current.Exported {
			targetEnv = runtime.global
			region = MemoryHeap
		} else if preferred := preferredMemoryRegion(value); preferred != "" {
			region = preferred
		}
		kind := "variable"
		if current.Temporary {
			kind = "temporary"
		}
		if err := runtime.defineValueWithState(targetEnv, current.Name, current.Mutable, typeName, value, region, kind, "define"); err != nil {
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.MultiVariableStatement:
		if current.Lazy {
			return signal{}, errorAt(current.Pos, "lazy multi-variable declarations are not supported")
		}
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			if thrown, ok := thrownValue(err); ok {
				return signal{kind: signalThrow, value: thrown}, nil
			}
			return signal{}, err
		}
		if value.Kind != ValueList {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("multi-variable declaration expects multiple return values, got %s", value.Kind))
		}
		items := value.Data.([]Value)
		if len(items) != len(current.Bindings) {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("multi-variable declaration expects %d value(s), got %d", len(current.Bindings), len(items)))
		}
		for index, binding := range current.Bindings {
			item := items[index]
			typeName := normalizeRuntimeType(binding.Type)
			if binding.Type == "Table" && item.Kind == ValueMap {
				item = TableValue(item.Data.(map[string]Value))
			}
			if strings.HasPrefix(typeName, "Map[") && item.Kind == ValueTable {
				converted, err := tableToStringMap(item.Data.(TableData))
				if err != nil {
					return signal{}, errorAt(current.Pos, err.Error())
				}
				item = Value{Kind: ValueMap, Data: converted}
			}
			if !valueMatchesType(item, typeName) {
				return signal{}, errorAt(current.Pos, fmt.Sprintf("cannot assign return value %d (%s) to %s variable %q", index+1, item.Kind, typeName, binding.Name))
			}
			if isDiscardIdentifier(binding.Name) {
				continue
			}
			targetEnv := env
			region := runtime.memoryRegionForType(typeName)
			if current.Scope == "global" || current.Exported {
				targetEnv = runtime.global
				region = MemoryHeap
			} else if preferred := preferredMemoryRegion(item); preferred != "" {
				region = preferred
			}
			kind := "variable"
			if current.Temporary {
				kind = "temporary"
			}
			if err := runtime.defineValueWithState(targetEnv, binding.Name, current.Mutable, typeName, item, region, kind, "define"); err != nil {
				return signal{}, errorAt(current.Pos, err.Error())
			}
		}
		return signal{kind: signalNone}, nil
	case parser.ReturnStatement:
		if len(current.Values) == 0 {
			if tailSignal, ok, err := runtime.tailCallSignal(current.Expression.Node, env); ok || err != nil {
				if err != nil {
					return signal{}, err
				}
				return tailSignal, nil
			}
		}
		value, err := runtime.evalReturnValue(current, env)
		if err != nil {
			if thrown, ok := thrownValue(err); ok {
				return signal{kind: signalThrow, value: thrown}, nil
			}
			return signal{}, err
		}
		return signal{kind: signalReturn, value: value}, nil
	case parser.DeferStatement:
		return signal{kind: signalNone}, nil
	case parser.RunStatement:
		return signal{kind: signalNone}, nil
	case parser.PrivateBlockStatement:
		return runtime.executeBlock(current.Body, NewEnvironment(env), inLoop)
	case parser.ScopeStatement:
		return runtime.executeBlock(current.Body, NewEnvironment(env), inLoop)
	case parser.ThrowStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		if value.Kind != ValueAtom {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("throw expects Atom, got %s", value.Kind))
		}
		return signal{kind: signalThrow, value: value}, nil
	case parser.AssertStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		if value.Kind != ValueBool {
			return signal{}, errorAt(current.Pos, fmt.Sprintf("assert expects Bool, got %s", value.Kind))
		}
		if !value.Data.(bool) {
			return signal{}, errorAtCode(
				current.Pos,
				diagnostic.CodeRuntimeAssertion,
				"runtime assertion",
				"assertion failed",
				"Inspect the asserted condition and the values used to compute it.",
			)
		}
		return signal{kind: signalNone}, nil
	case parser.ReportStatement:
		value, err := runtime.evalExpression(current.Expression.Node, env)
		if err != nil {
			return signal{}, err
		}
		runtime.reportValue(current.Expression.Literal(), value, current.Pos)
		return signal{kind: signalNone}, nil
	case parser.BreakStatement:
		if !inLoop {
			return signal{}, errorAt(current.Pos, "break is only allowed inside a loop")
		}
		return signal{kind: signalBreak}, nil
	case parser.ContinueStatement:
		if !inLoop {
			return signal{}, errorAt(current.Pos, "continue is only allowed inside a loop or pattern match case")
		}
		return signal{kind: signalContinue}, nil
	case parser.ExpressionStatement:
		_, err := runtime.evalExpression(current.Expression.Node, env)
		if thrown, ok := thrownValue(err); ok {
			return signal{kind: signalThrow, value: thrown}, nil
		}
		return signal{kind: signalNone}, err
	case parser.AssignmentStatement:
		if err := runtime.executeAssignment(current, env); err != nil {
			if thrown, ok := thrownValue(err); ok {
				return signal{kind: signalThrow, value: thrown}, nil
			}
			return signal{}, errorAt(current.Pos, err.Error())
		}
		return signal{kind: signalNone}, nil
	case parser.IfStatement:
		return runtime.executeIf(current, env, inLoop)
	case parser.MatchStatement:
		return runtime.executeMatch(current, env, inLoop)
	case parser.LoopStatement:
		return runtime.executeLoop(current, env)
	case parser.TryCatchStatement:
		return runtime.executeTryCatch(current, env, inLoop)
	case parser.TransactionStatement:
		return runtime.executeTransaction(current, env, inLoop)
	default:
		return signal{}, Error{Message: fmt.Sprintf("unsupported statement %T", stmt)}
	}
}

func (runtime *Runtime) executeMatch(stmt parser.MatchStatement, env *Environment, inLoop bool) (signal, error) {
	value, err := runtime.evalExpression(stmt.Value.Node, env)
	if err != nil {
		return signal{}, err
	}
	if !isRuntimePatternMatchValue(value) {
		return signal{}, errorAt(stmt.Pos, fmt.Sprintf("pattern match value must be Bool, String, Int, Float, Atom, Enum, Option, Result, List, or Table, got %s", value.Kind))
	}

	matched := false
	var captures map[string]Value
	for _, matchCase := range stmt.Cases {
		if !matched {
			if matchCase.Default {
				matched = true
				captures = nil
			} else {
				caseCaptures := map[string]Value{}
				matched, err = runtime.matchPattern(value, matchCase.Pattern.Node, env, caseCaptures)
				if err != nil {
					return signal{}, errorAt(matchCase.Pos, err.Error())
				}
				captures = caseCaptures
			}
		}
		if !matched {
			continue
		}

		caseEnv := NewEnvironment(env)
		for name, captured := range captures {
			if err := runtime.defineValueInRegion(caseEnv, name, false, runtimeTypeName(captured), captured, MemoryStack); err != nil {
				return signal{}, errorAt(matchCase.Pos, err.Error())
			}
		}
		currentSignal, err := runtime.executeBlock(matchCase.Body, caseEnv, true)
		if err != nil {
			return signal{}, err
		}
		switch currentSignal.kind {
		case signalNone, signalBreak:
			return signal{kind: signalNone}, nil
		case signalContinue:
			continue
		default:
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) executeTryCatch(stmt parser.TryCatchStatement, env *Environment, inLoop bool) (signal, error) {
	currentSignal, err := runtime.executeBlock(stmt.TryBody, NewEnvironment(env), inLoop)
	if err != nil {
		return signal{}, err
	}
	if currentSignal.kind != signalThrow {
		return currentSignal, nil
	}
	catchEnv := NewEnvironment(env)
	if currentSignal.value.Kind != ValueAtom {
		return signal{}, errorAt(stmt.Pos, fmt.Sprintf("catch received non-Atom exception %s", currentSignal.value.Kind))
	}
	if err := runtime.defineValueInRegion(catchEnv, stmt.ErrorName, false, "Atom", currentSignal.value, MemoryStack); err != nil {
		return signal{}, errorAt(stmt.Pos, err.Error())
	}
	return runtime.executeBlock(stmt.CatchBody, catchEnv, inLoop)
}

func (runtime *Runtime) executeIf(stmt parser.IfStatement, env *Environment, inLoop bool) (signal, error) {
	condition, err := runtime.evalExpression(stmt.Condition.Node, env)
	if err != nil {
		return signal{}, err
	}

	conditionValue := isTruthy(condition)
	shouldRun := conditionValue
	if stmt.Kind == "unless" {
		shouldRun = !conditionValue
	}

	if shouldRun {
		return runtime.executeBlock(stmt.Consequence, NewEnvironment(env), inLoop)
	}
	if stmt.ElseIf != nil {
		return runtime.executeIf(*stmt.ElseIf, env, inLoop)
	}
	if len(stmt.Alternative) != 0 {
		return runtime.executeBlock(stmt.Alternative, NewEnvironment(env), inLoop)
	}
	return signal{kind: signalNone}, nil
}

func (runtime *Runtime) tailCallSignal(expr parser.ExpressionNode, env *Environment) (signal, bool, error) {
	call, ok := expr.(parser.CallExpression)
	if !ok || len(call.Arguments) == 0 || len(runtime.callStack) == 0 {
		return signal{}, false, nil
	}
	callee, err := runtime.evalExpression(call.Callee, env)
	if err != nil {
		return signal{}, false, err
	}
	if callee.Kind != ValueFunction {
		return signal{}, false, nil
	}
	name, err := runtime.resolveFunctionName(callee.Data.(string))
	if err != nil {
		return signal{}, false, err
	}
	if name == "" || name != runtime.callStack[len(runtime.callStack)-1] {
		return signal{}, false, nil
	}
	args := make([]Value, 0, len(call.Arguments))
	if runtime.isLazyFunction(name) {
		for _, arg := range call.Arguments {
			args = append(args, ThunkValue(arg, env))
		}
	} else {
		for _, arg := range call.Arguments {
			value, err := runtime.evalExpression(arg, env)
			if err != nil {
				return signal{}, false, err
			}
			args = append(args, value)
		}
	}
	return signal{kind: signalTailCall, tailName: name, tailArgs: args}, true, nil
}

func (runtime *Runtime) executeLoop(stmt parser.LoopStatement, env *Environment) (signal, error) {
	if stmt.Kind == "for_each" {
		iterator, iterable, ok := parseForEachHeader(stmt.Header)
		if !ok {
			return signal{}, errorAt(stmt.Pos, "for_each expects 'name in iterable'")
		}
		iterableValue, err := runtime.evalExpression(iterable.Node, env)
		if err != nil {
			return signal{}, err
		}
		values, err := runtime.iterableValues(iterableValue)
		if err != nil {
			return signal{}, errorAt(stmt.Pos, err.Error())
		}
		for _, value := range values {
			loopEnv := NewEnvironment(env)
			if err := runtime.defineValue(loopEnv, iterator, false, runtimeTypeName(value), value); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, loopEnv, true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				return signal{kind: signalNone}, nil
			}
			if currentSignal.kind == signalContinue {
				continue
			}
			if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
				return currentSignal, nil
			}
		}
		return signal{kind: signalNone}, nil
	}

	if init, condition, post, ok := parseCStyleForHeader(stmt.Header); ok {
		loopEnv := NewEnvironment(env)
		if len(init.Tokens) != 0 {
			if err := runtime.executeLoopHeaderAssignment(init, loopEnv); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
		}
		for {
			if len(condition.Tokens) != 0 {
				conditionValue, err := runtime.evalExpression(condition.Node, loopEnv)
				if err != nil {
					return signal{}, err
				}
				if !isTruthy(conditionValue) {
					break
				}
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(loopEnv), true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				break
			}
			if currentSignal.kind == signalContinue {
				if len(post.Tokens) != 0 {
					if err := runtime.executeLoopHeaderAssignment(post, loopEnv); err != nil {
						return signal{}, errorAt(stmt.Pos, err.Error())
					}
				}
				continue
			}
			if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
				return currentSignal, nil
			}
			if len(post.Tokens) != 0 {
				if err := runtime.executeLoopHeaderAssignment(post, loopEnv); err != nil {
					return signal{}, errorAt(stmt.Pos, err.Error())
				}
			}
		}
		return signal{kind: signalNone}, nil
	}

	if iterator, iterable, ok := parseRangeHeader(stmt.Header); ok {
		countValue, err := runtime.evalExpression(iterable.Node, env)
		if err != nil {
			return signal{}, err
		}
		count, err := asInt(countValue)
		if err != nil {
			return signal{}, errorAt(stmt.Pos, "range expects an Int count")
		}
		if count < 0 {
			return signal{}, errorAt(stmt.Pos, "range count cannot be negative")
		}
		for index := 0; index < count; index++ {
			loopEnv := NewEnvironment(env)
			value := IntValue(index)
			if err := runtime.defineValue(loopEnv, iterator, false, "Int", value); err != nil {
				return signal{}, errorAt(stmt.Pos, err.Error())
			}
			currentSignal, err := runtime.executeBlock(stmt.Body, loopEnv, true)
			if err != nil {
				return signal{}, err
			}
			if currentSignal.kind == signalBreak {
				return signal{kind: signalNone}, nil
			}
			if currentSignal.kind == signalContinue {
				continue
			}
			if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
				return currentSignal, nil
			}
		}
		return signal{kind: signalNone}, nil
	}

	loopEnv := env
	headerName, headerExpr, hasHeaderBinding := parseEvaluationHeader(stmt.Header)
	if hasHeaderBinding {
		loopEnv = NewEnvironment(env)
	}
	first := true
	for {
		if stmt.Kind != "do_while" && stmt.Kind != "do" || !first {
			conditionExpression := loopCondition(stmt.Header)
			if hasHeaderBinding {
				conditionExpression = headerExpr
			}
			condition, err := runtime.evalExpression(conditionExpression.Node, loopEnv)
			if err != nil {
				return signal{}, err
			}
			if hasHeaderBinding {
				if err := runtime.storeLoopHeaderBinding(headerName, condition, loopEnv); err != nil {
					return signal{}, errorAt(stmt.Pos, err.Error())
				}
			}
			if !isTruthy(condition) {
				break
			}
		}
		first = false
		currentSignal, err := runtime.executeBlock(stmt.Body, NewEnvironment(loopEnv), true)
		if err != nil {
			return signal{}, err
		}
		if currentSignal.kind == signalBreak {
			break
		}
		if currentSignal.kind == signalContinue {
			continue
		}
		if currentSignal.kind == signalReturn || currentSignal.kind == signalTailCall || currentSignal.kind == signalThrow {
			return currentSignal, nil
		}
	}
	return signal{kind: signalNone}, nil
}

func isRuntimePatternMatchValue(value Value) bool {
	switch value.Kind {
	case ValueBool, ValueString, ValueInt, ValueFloat, ValueAtom, ValueEnum, ValueOption, ValueResult, ValueList, ValueTable:
		return true
	default:
		return false
	}
}

func (runtime *Runtime) matchPattern(value Value, pattern parser.ExpressionNode, env *Environment, captures map[string]Value) (bool, error) {
	switch current := pattern.(type) {
	case parser.IdentifierExpression:
		if current.Name == "_" {
			return true, nil
		}
		if binding, ok := env.Get(current.Name); ok {
			patternValue, err := runtime.forceBindingValue(binding)
			if err != nil {
				return false, err
			}
			if value.Kind != patternValue.Kind {
				return false, nil
			}
			return valuesEqual(value, patternValue), nil
		}
		captures[current.Name] = cloneValue(value)
		return true, nil
	case parser.GroupExpression:
		return runtime.matchPattern(value, current.Inner, env, captures)
	case parser.CallExpression:
		return runtime.matchConstructorPattern(value, current, env, captures)
	case parser.ListExpression:
		return runtime.matchListPattern(value, current, env, captures)
	case parser.MapExpression:
		return runtime.matchTablePattern(value, current, env, captures)
	default:
		patternValue, err := runtime.evalExpression(pattern, env)
		if err != nil {
			return false, err
		}
		if !isRuntimePatternMatchValue(patternValue) {
			return false, Error{Message: fmt.Sprintf("case pattern must be Bool, String, Int, Float, Atom, Enum, Option, Result, List, or Table, got %s", patternValue.Kind)}
		}
		if value.Kind != patternValue.Kind {
			return false, nil
		}
		return valuesEqual(value, patternValue), nil
	}
}

func (runtime *Runtime) matchConstructorPattern(value Value, pattern parser.CallExpression, env *Environment, captures map[string]Value) (bool, error) {
	callee, ok := pattern.Callee.(parser.IdentifierExpression)
	if !ok {
		patternValue, err := runtime.evalExpression(pattern, env)
		if err != nil {
			return false, err
		}
		return valuesEqual(value, patternValue), nil
	}
	switch callee.Name {
	case "Some":
		if len(pattern.Arguments) != 1 || value.Kind != ValueOption {
			return false, nil
		}
		option := value.Data.(OptionData)
		if !option.Some {
			return false, nil
		}
		return runtime.matchPattern(option.Value, pattern.Arguments[0], env, captures)
	case "None":
		if len(pattern.Arguments) != 0 || value.Kind != ValueOption {
			return false, nil
		}
		return !value.Data.(OptionData).Some, nil
	case "Ok":
		if len(pattern.Arguments) != 1 || value.Kind != ValueResult {
			return false, nil
		}
		result := value.Data.(ResultData)
		if !result.Ok {
			return false, nil
		}
		return runtime.matchPattern(result.Value, pattern.Arguments[0], env, captures)
	case "Err":
		if len(pattern.Arguments) != 1 || value.Kind != ValueResult {
			return false, nil
		}
		result := value.Data.(ResultData)
		if result.Ok {
			return false, nil
		}
		return runtime.matchPattern(result.Value, pattern.Arguments[0], env, captures)
	default:
		patternValue, err := runtime.evalExpression(pattern, env)
		if err != nil {
			return false, err
		}
		return valuesEqual(value, patternValue), nil
	}
}

func (runtime *Runtime) matchListPattern(value Value, pattern parser.ListExpression, env *Environment, captures map[string]Value) (bool, error) {
	if value.Kind != ValueList {
		return false, nil
	}
	items := value.Data.([]Value)
	if len(items) != len(pattern.Items) {
		return false, nil
	}
	for index, itemPattern := range pattern.Items {
		matched, err := runtime.matchPattern(items[index], itemPattern, env, captures)
		if err != nil || !matched {
			return matched, err
		}
	}
	return true, nil
}

func (runtime *Runtime) matchTablePattern(value Value, pattern parser.MapExpression, env *Environment, captures map[string]Value) (bool, error) {
	if value.Kind != ValueTable {
		return false, nil
	}
	table := value.Data.(TableData)
	for _, entry := range pattern.Entries {
		keyValue, err := runtime.evalExpression(entry.Key, env)
		if err != nil {
			return false, err
		}
		key, err := tableKey(keyValue)
		if err != nil {
			return false, err
		}
		tableValue, ok := tableGet(table, key)
		if !ok {
			return false, nil
		}
		matched, err := runtime.matchPattern(tableValue, entry.Value, env, captures)
		if err != nil || !matched {
			return matched, err
		}
	}
	return true, nil
}

func valuesEqual(left Value, right Value) bool {
	if left.Kind != right.Kind {
		return false
	}
	switch left.Kind {
	case ValueBool:
		return left.Data.(bool) == right.Data.(bool)
	case ValueString:
		return left.Data.(string) == right.Data.(string)
	case ValueAtom:
		return left.Data.(string) == right.Data.(string)
	case ValueInt:
		return left.Data.(int) == right.Data.(int)
	case ValueFloat:
		return left.Data.(float64) == right.Data.(float64)
	case ValueEnum:
		leftData := left.Data.(EnumData)
		rightData := right.Data.(EnumData)
		return leftData.Type == rightData.Type && leftData.Variant == rightData.Variant
	case ValueOption:
		leftData := left.Data.(OptionData)
		rightData := right.Data.(OptionData)
		if leftData.Some != rightData.Some {
			return false
		}
		if !leftData.Some {
			return true
		}
		return valuesEqual(leftData.Value, rightData.Value)
	case ValueResult:
		leftData := left.Data.(ResultData)
		rightData := right.Data.(ResultData)
		return leftData.Ok == rightData.Ok && valuesEqual(leftData.Value, rightData.Value)
	case ValueList:
		leftItems := left.Data.([]Value)
		rightItems := right.Data.([]Value)
		if len(leftItems) != len(rightItems) {
			return false
		}
		for index := range leftItems {
			if !valuesEqual(leftItems[index], rightItems[index]) {
				return false
			}
		}
		return true
	case ValueTable:
		leftTable := left.Data.(TableData)
		rightTable := right.Data.(TableData)
		if len(leftTable.Entries) != len(rightTable.Entries) {
			return false
		}
		for key, leftValue := range leftTable.Entries {
			rightValue, ok := rightTable.Entries[key]
			if !ok || !valuesEqual(leftValue, rightValue) {
				return false
			}
		}
		return true
	case ValueJSON:
		leftJSON, leftErr := stringifyJSONValue(left)
		rightJSON, rightErr := stringifyJSONValue(right)
		return leftErr == nil && rightErr == nil && leftJSON == rightJSON
	default:
		return false
	}
}

func typeSizeof(typeName string) (int, bool) {
	typeName = normalizeRuntimeType(typeName)
	if spec, ok := runtimeChildType(typeName); ok {
		return spec.Bits / 8, true
	}
	switch typeName {
	case "Bool", "Char":
		return 1, true
	case "Int", "UInt", "Float":
		return 8, true
	case "Complex":
		return 16, true
	case "String", "Atom", "List", "Map", "Table", "JSON", "File", "OS", "Parsable", "T", "Type", "Function", "Option", "Result", "SIMD", "Awaitable", "Iterator", "Coroutine", "Thread", "Atomic", "Context", "ErrorContext":
		return 16, true
	default:
		return 0, false
	}
}

func runtimeTypeInfoCallTarget(name string) (string, bool) {
	const suffix = ".get_runtime_type_info"
	if !strings.HasSuffix(name, suffix) {
		return "", false
	}
	typeName := normalizeRuntimeType(strings.TrimSuffix(name, suffix))
	if typeName == "" {
		return "", false
	}
	return typeName, true
}

func typeInfoValue(typeName string) Value {
	typeName = normalizeRuntimeType(typeName)
	size, ok := typeSizeof(typeName)
	if !ok {
		size = 0
	}
	alignment := size
	if alignment > 8 {
		alignment = 8
	}
	if alignment <= 0 {
		alignment = 1
	}
	return objectValue("Type", map[string]Value{
		"name":                   StringValue(typeName),
		"type_name":              StringValue(typeName),
		"category":               StringValue(typeMetadataCategory(typeName)),
		"size":                   IntValue(size),
		"alignment":              IntValue(alignment),
		"footprint":              IntValue(size),
		"fields":                 TableValue(map[string]Value{}),
		"field_count":            IntValue(0),
		"supports_serialization": BoolValue(true),
		"supports_introspection": BoolValue(true),
		"supports_memory_layout": BoolValue(true),
		"serialization":          TableValue(map[string]Value{"pack": StringValue("metadata"), "unpack": StringValue("metadata")}),
		"introspection":          TableValue(map[string]Value{"fields": TableValue(map[string]Value{})}),
		"layout":                 TableValue(map[string]Value{"size": IntValue(size), "alignment": IntValue(alignment), "footprint": IntValue(size)}),
	})
}

func typeMetadataCategory(typeName string) string {
	if _, ok := runtimeChildType(typeName); ok {
		return "child"
	}
	switch {
	case strings.HasPrefix(typeName, "List["), strings.HasPrefix(typeName, "Map["), typeName == "Table", typeName == "JSON":
		return "collection"
	case strings.HasPrefix(typeName, "Function["):
		return "function"
	case strings.HasPrefix(typeName, "Option["), strings.HasPrefix(typeName, "Result["):
		return "sum"
	default:
		return "builtin"
	}
}

func (runtime *Runtime) storeLoopHeaderBinding(name string, value Value, env *Environment) error {
	if binding, ok := env.bindings[name]; ok {
		runtime.storeBindingValue(binding, value)
		return nil
	}
	return runtime.defineValue(env, name, true, runtimeTypeName(value), value)
}

func (runtime *Runtime) defineValue(env *Environment, name string, mutable bool, typeName string, value Value) error {
	return runtime.defineValueWithState(env, name, mutable, typeName, value, MemoryStack, "variable", "define")
}

func (runtime *Runtime) defineValueInRegion(env *Environment, name string, mutable bool, typeName string, value Value, region MemoryRegion) error {
	return runtime.defineValueWithState(env, name, mutable, typeName, value, region, "variable", "define")
}

func (runtime *Runtime) defineValueWithState(env *Environment, name string, mutable bool, typeName string, value Value, region MemoryRegion, kind string, event string) error {
	typeName = normalizeRuntimeType(typeName)
	snapshot := shareValue(value)
	if err := env.Define(name, mutable, typeName, snapshot, runtime.memory.Allocate(snapshot, region)); err != nil {
		return err
	}
	if binding, ok := env.bindings[name]; ok {
		binding.Kind = kind
	}
	runtime.recordState(StateRecord{
		Phase:    "runtime",
		Event:    event,
		Kind:     kind,
		Name:     name,
		Type:     typeName,
		Runtime:  runtimeTypeName(snapshot),
		Function: runtime.currentFunctionName(),
		Mutable:  mutable,
	})
	return nil
}

func (runtime *Runtime) storeBindingValue(binding *Binding, value Value) {
	binding.mu.Lock()
	defer binding.mu.Unlock()
	runtime.storeBindingValueLocked(binding, value)
}

func (runtime *Runtime) storeBindingValueLocked(binding *Binding, value Value) {
	snapshot := shareValue(value)
	binding.Value = snapshot
	runtime.memory.Store(binding.ObjectID, snapshot)
	runtime.recordState(StateRecord{
		Phase:    "runtime",
		Event:    "assign",
		Kind:     binding.Kind,
		Name:     binding.Name,
		Type:     binding.Type,
		Runtime:  runtimeTypeName(snapshot),
		Function: runtime.currentFunctionName(),
		Mutable:  binding.Mutable,
		Moved:    binding.Moved,
	})
}

func (runtime *Runtime) recordState(record StateRecord) {
	if record.Kind == "" {
		record.Kind = "variable"
	}
	if record.Event == "" {
		record.Event = "observe"
	}
	if record.Phase == "" {
		record.Phase = "runtime"
	}
	runtime.states = append(runtime.states, record)
}

func (runtime *Runtime) stateRecordsValue() Value {
	items := make([]Value, 0, len(runtime.states))
	for _, state := range runtime.states {
		items = append(items, TableValue(map[string]Value{
			"phase":    StringValue(state.Phase),
			"event":    StringValue(state.Event),
			"kind":     StringValue(state.Kind),
			"name":     StringValue(state.Name),
			"type":     StringValue(state.Type),
			"runtime":  StringValue(state.Runtime),
			"function": StringValue(state.Function),
			"mutable":  BoolValue(state.Mutable),
			"moved":    BoolValue(state.Moved),
		}))
	}
	return Value{Kind: ValueList, Data: items}
}

func (runtime *Runtime) defineLocalFunction(fn parser.FunctionStatement, env *Environment) (string, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	runtime.nextFunc++
	name := fmt.Sprintf("<local:%s:%d>", fn.Name, runtime.nextFunc)
	if _, exists := runtime.functions[name]; exists {
		return "", Error{Message: fmt.Sprintf("function %q is already defined", name)}
	}
	runtime.functions[name] = fn
	if len(runtime.callStack) > 0 {
		runtime.functionFiles[name] = runtime.functionFiles[runtime.callStack[len(runtime.callStack)-1]]
	}
	runtime.closures[name] = env
	return name, nil
}

func functionTypeName(fn parser.FunctionStatement) string {
	parts := make([]string, 0, len(fn.Params)+1)
	for _, param := range fn.Params {
		parts = append(parts, param.Type)
	}
	parts = append(parts, fn.ReturnType)
	return "Function[" + strings.Join(parts, ",") + "]"
}

func (runtime *Runtime) forceBindingValue(binding *Binding) (Value, error) {
	snapshot := binding.Snapshot()
	value, err := runtime.forceValue(snapshot.Value)
	if err != nil {
		return NullValue(), err
	}
	if snapshot.Value.Kind == ValueThunk {
		if !valueMatchesType(value, snapshot.Type) {
			return NullValue(), Error{Message: fmt.Sprintf("lazy value expects %s, got %s", snapshot.Type, value.Kind)}
		}
	}
	return value, nil
}

func (runtime *Runtime) forceValue(value Value) (Value, error) {
	if value.Kind != ValueThunk {
		return value, nil
	}
	thunk := value.Data.(*ThunkData)
	if thunk.Evaluated {
		return thunk.Value, nil
	}
	result, err := runtime.evalExpression(thunk.Expr, thunk.Env)
	if err != nil {
		return NullValue(), err
	}
	thunk.Value = cloneValue(result)
	thunk.Evaluated = true
	return thunk.Value, nil
}

func (runtime *Runtime) executeLoopHeaderAssignment(expr parser.Expression, env *Environment) error {
	if len(expr.Tokens) == 0 {
		return nil
	}
	if stmt, ok := loopHeaderStatement(expr); ok {
		currentSignal, err := runtime.executeStatement(stmt, env, true)
		if err != nil {
			return err
		}
		if currentSignal.kind != signalNone {
			return Error{Message: "loop header cannot return or break"}
		}
		return nil
	}
	_, err := runtime.evalExpression(expr.Node, env)
	return err
}

func loopHeaderStatement(expr parser.Expression) (parser.Statement, bool) {
	tokens := expr.Tokens
	if len(tokens) < 3 {
		return nil, false
	}

	if tokens[0].Type == lexer.TokenIdentifier && tokens[1].Type == lexer.TokenEvaluationAssign {
		value := parser.Expression{Tokens: tokens[2:], Node: parser.ParseExpressionTokens(tokens[2:])}
		return parser.VariableStatement{
			Pos:        parser.Position{Line: tokens[0].Line, Column: tokens[0].Column},
			Scope:      "local",
			Mutable:    true,
			Type:       "Int",
			Name:       tokens[0].Literal,
			Expression: value,
		}, true
	}

	if index := assignmentOperatorIndex(tokens); index != -1 {
		target := parser.Expression{Tokens: tokens[:index], Node: parser.ParseExpressionTokens(tokens[:index])}
		value := parser.Expression{Tokens: tokens[index+1:], Node: parser.ParseExpressionTokens(tokens[index+1:])}
		return parser.AssignmentStatement{
			Pos:        parser.Position{Line: tokens[0].Line, Column: tokens[0].Column},
			Target:     target,
			Operator:   tokens[index].Literal,
			Expression: value,
		}, true
	}

	return nil, false
}

func assignmentOperatorIndex(tokens []lexer.Token) int {
	depth := 0
	for index, token := range tokens {
		switch token.Type {
		case lexer.TokenLeftBrace, lexer.TokenLeftSquareBrace:
			depth++
		case lexer.TokenRightBrace, lexer.TokenRightSquareBrace:
			if depth > 0 {
				depth--
			}
		case lexer.TokenAssign, lexer.TokenPlusEqual, lexer.TokenMinusEqual, lexer.TokenMultiEqual, lexer.TokenDivideEqual:
			if depth == 0 {
				return index
			}
		}
	}
	return -1
}

func (runtime *Runtime) executeAssignment(stmt parser.AssignmentStatement, env *Environment) error {
	value, err := runtime.evalExpression(stmt.Expression.Node, env)
	if err != nil {
		return err
	}

	if indexExpr, ok := stmt.Target.Node.(parser.IndexExpression); ok {
		return runtime.assignIndex(indexExpr, stmt.Operator, value, env)
	}

	identifier, ok := stmt.Target.Node.(parser.IdentifierExpression)
	if !ok {
		return Error{Message: "assignment target must be an lvalue"}
	}
	if isDiscardIdentifier(identifier.Name) {
		if stmt.Operator != "=" {
			return Error{Message: "discard assignment only supports ="}
		}
		return nil
	}

	binding, ok := env.Get(identifier.Name)
	if !ok {
		return Error{Message: fmt.Sprintf("unknown variable %q", identifier.Name)}
	}
	if err := runtime.ensureThreadAssignmentSafe(binding); err != nil {
		return err
	}
	return binding.WithLock(func() error {
		if binding.Moved {
			return Error{Message: fmt.Sprintf("variable %q was moved", identifier.Name)}
		}
		if err := runtime.memory.EnsureWritable(binding.ObjectID); err != nil {
			return err
		}
		if !binding.Mutable {
			return Error{Message: fmt.Sprintf("cannot mutate immutable variable %q", identifier.Name)}
		}

		var next Value
		var err error
		if stmt.Operator != "=" {
			operator := strings.TrimSuffix(stmt.Operator, "=")
			if overloaded, ok, overloadErr := runtime.evalOverloadedBinaryOperator(binding.Value, operator, value, env); ok {
				next, err = overloaded, overloadErr
			} else {
				next, err = applyAssignmentOperator(binding.Value, stmt.Operator, value)
			}
		} else {
			next, err = applyAssignmentOperator(binding.Value, stmt.Operator, value)
		}
		if err != nil {
			return err
		}
		if !valueMatchesType(next, binding.Type) {
			return Error{Message: fmt.Sprintf("cannot assign %s to %s variable %q", next.Kind, binding.Type, identifier.Name)}
		}
		runtime.storeBindingValueLocked(binding, next)
		return nil
	})
}

func isDiscardIdentifier(name string) bool {
	return name == "_"
}

func (runtime *Runtime) assignIndex(indexExpr parser.IndexExpression, operator string, value Value, env *Environment) error {
	targetIdentifier, ok := indexExpr.Target.(parser.IdentifierExpression)
	if !ok {
		return Error{Message: "indexed assignment target must start from a variable"}
	}
	binding, ok := env.Get(targetIdentifier.Name)
	if !ok {
		return Error{Message: fmt.Sprintf("unknown variable %q", targetIdentifier.Name)}
	}
	if err := runtime.ensureThreadAssignmentSafe(binding); err != nil {
		return err
	}

	index, err := runtime.evalExpression(indexExpr.Index, env)
	if err != nil {
		return err
	}

	return binding.WithLock(func() error {
		if binding.Moved {
			return Error{Message: fmt.Sprintf("variable %q was moved", targetIdentifier.Name)}
		}
		if !binding.Mutable {
			return Error{Message: fmt.Sprintf("cannot mutate immutable variable %q", targetIdentifier.Name)}
		}
		if err := runtime.memory.EnsureWritable(binding.ObjectID); err != nil {
			return err
		}
		switch binding.Value.Kind {
		case ValueList:
			elementType, hasElementType := listElementType(binding.Type)
			if !hasElementType {
				elementType, hasElementType = arrayElementRuntimeType(binding.Type)
			}
			items := append([]Value(nil), binding.Value.Data.([]Value)...)
			position, err := asIndex(index)
			if err != nil {
				return err
			}
			if position < 0 {
				return Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
			}
			if capacity, ok := runtime.regionArrayCapacity(binding.Type); ok && position >= capacity {
				return Error{Message: fmt.Sprintf("array index %d exceeds region %s capacity %d", position, regionNameFromRuntimeArrayType(binding.Type), capacity)}
			}
			if operator != "=" && position >= len(items) {
				return Error{Message: fmt.Sprintf("compound assignment requires existing list index %d", position)}
			}
			for len(items) <= position {
				items = append(items, NullValue())
			}
			current := items[position]
			next, err := applyAssignmentOperator(current, operator, value)
			if err != nil {
				return err
			}
			if hasElementType && !valueMatchesType(next, elementType) {
				return Error{Message: fmt.Sprintf("cannot assign %s to list element type %s", next.Kind, elementType)}
			}
			items[position] = next
			runtime.storeBindingValueLocked(binding, Value{Kind: ValueList, Data: items})
		case ValueMap:
			keyType, valueType, hasMapTypes := mapTypes(binding.Type)
			items := make(map[string]Value, len(binding.Value.Data.(map[string]Value)))
			for existingKey, existingValue := range binding.Value.Data.(map[string]Value) {
				items[existingKey] = existingValue
			}
			if hasMapTypes && !valueMatchesType(index, keyType) {
				return Error{Message: fmt.Sprintf("cannot use %s as map key type %s", index.Kind, keyType)}
			}
			key, err := mapKey(index)
			if err != nil {
				return err
			}
			if operator != "=" {
				if _, ok := items[key]; !ok {
					return Error{Message: fmt.Sprintf("compound assignment requires existing map key %q", key)}
				}
			}
			current := items[key]
			next, err := applyAssignmentOperator(current, operator, value)
			if err != nil {
				return err
			}
			if hasMapTypes && !valueMatchesType(next, valueType) {
				return Error{Message: fmt.Sprintf("cannot assign %s to map value type %s", next.Kind, valueType)}
			}
			items[key] = next
			runtime.storeBindingValueLocked(binding, Value{Kind: binding.Value.Kind, Data: items})
		case ValueTable:
			table := cloneTableData(binding.Value.Data.(TableData))
			key, err := tableKey(index)
			if err != nil {
				return err
			}
			if operator != "=" {
				if !tableHas(table, key) {
					return Error{Message: fmt.Sprintf("compound assignment requires existing table key %s", valueString(tableKeyValue(key)))}
				}
			}
			current, ok := tableGet(table, key)
			if !ok {
				current = NullValue()
			}
			next, err := applyAssignmentOperator(current, operator, value)
			if err != nil {
				return err
			}
			tableSet(&table, key, next)
			runtime.storeBindingValueLocked(binding, Value{Kind: ValueTable, Data: table})
		default:
			return Error{Message: fmt.Sprintf("%s is not index-assignable", binding.Value.Kind)}
		}
		return nil
	})
}

func (runtime *Runtime) evalExpression(expr parser.ExpressionNode, env *Environment) (Value, error) {
	switch current := expr.(type) {
	case nil:
		return NullValue(), nil
	case parser.IdentifierExpression:
		binding, ok := env.Get(current.Name)
		if ok {
			snapshot := binding.Snapshot()
			if snapshot.Moved {
				return NullValue(), Error{Message: fmt.Sprintf("variable %q was moved", current.Name)}
			}
			if err := runtime.memory.BorrowImmutable(snapshot.ObjectID); err != nil {
				return NullValue(), err
			}
			runtime.memory.ReleaseImmutable(snapshot.ObjectID)
			return runtime.threadSafeGlobalValue(binding)
		}
		if isBuiltinFunction(current.Name) {
			return FunctionValue(current.Name), nil
		}
		if target, ok := runtime.aliases[current.Name]; ok {
			return FunctionValue(target), nil
		}
		if _, ok := runtime.aliasFunctions[current.Name]; ok {
			return FunctionValue(current.Name), nil
		}
		if _, ok := runtime.keywordMacros[current.Name]; ok {
			return FunctionValue(current.Name), nil
		}
		if _, ok := runtime.groups[current.Name]; ok {
			return FunctionValue(current.Name), nil
		}
		name, err := runtime.resolveFunctionName(current.Name)
		if err != nil {
			return NullValue(), err
		}
		if name != "" {
			return FunctionValue(name), nil
		}
		return NullValue(), Error{Message: fmt.Sprintf("unknown identifier %q", current.Name)}
	case parser.LiteralExpression:
		return literalValue(current)
	case parser.GroupExpression:
		return runtime.evalExpression(current.Inner, env)
	case parser.UnaryExpression:
		return runtime.evalUnary(current, env)
	case parser.BinaryExpression:
		return runtime.evalBinary(current, env)
	case parser.CallExpression:
		return runtime.evalCall(current, env)
	case parser.SelectorExpression:
		if current.Field == "sizeof" {
			if typeName, ok := builtinTypeExpressionName(current.Target); ok {
				size, _ := typeSizeof(typeName)
				return IntValue(size), nil
			}
		}
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			if value, ok := runtime.enumVariantValue(target.Name, current.Field); ok {
				return value, nil
			}
		}
		value, err := runtime.evalExpression(current.Target, env)
		if err == nil && value.Kind == ValueFunction {
			return FunctionValue(runtime.resolveAliasPath(value.Data.(string)) + "." + current.Field), nil
		}
		if err == nil && value.Kind == ValueTable {
			if field, ok := builtinProtocolField(value, current.Field); ok {
				return field, nil
			}
			if builtinProtocolMethodExists(value, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			key := TableKey{Kind: ValueString, Repr: current.Field}
			field, ok := tableGet(value.Data.(TableData), key)
			if !ok {
				return NullValue(), Error{Message: fmt.Sprintf("table key %q does not exist", current.Field)}
			}
			return field, nil
		}
		if err == nil && value.Kind == ValueJSON {
			if field, ok := builtinProtocolField(value, current.Field); ok {
				return field, nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			field, ok, lookupErr := jsonLookup(value, StringValue(current.Field))
			if lookupErr != nil {
				return NullValue(), lookupErr
			}
			if !ok {
				return NullValue(), Error{Message: fmt.Sprintf("JSON object key %q does not exist", current.Field)}
			}
			return field, nil
		}
		if err == nil && value.Kind == ValueMap {
			if builtinProtocolMethodExists(value, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			fields := value.Data.(map[string]Value)
			field, ok := fields[current.Field]
			if !ok {
				return NullValue(), Error{Message: fmt.Sprintf("unknown field %q", current.Field)}
			}
			return field, nil
		}
		if err == nil && value.Kind == ValueOption {
			option := value.Data.(OptionData)
			switch current.Field {
			case "value":
				if !option.Some {
					return NullValue(), Error{Message: "cannot access None.value; check .some, pattern match Some(...), or use option_unwrap_or"}
				}
				return option.Value, nil
			case "some":
				return BoolValue(option.Some), nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown Option field %q", current.Field)}
		}
		if err == nil && value.Kind == ValueResult {
			result := value.Data.(ResultData)
			switch current.Field {
			case "value":
				if !result.Ok {
					return NullValue(), Error{Message: "cannot access Err.value; check .ok, pattern match Ok(...), use result_unwrap_or, or propagate with !"}
				}
				return result.Value, nil
			case "ok":
				return BoolValue(result.Ok), nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown Result field %q", current.Field)}
		}
		if err == nil && value.Kind == ValueEnum {
			data := value.Data.(EnumData)
			switch current.Field {
			case "ordinal":
				return IntValue(data.Ordinal), nil
			case "name", "variant":
				return StringValue(data.Variant), nil
			}
			if runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown enum field %q", current.Field)}
		}
		if err == nil {
			if value.Kind == ValueIterator && current.Field == "count" {
				iterator := cloneIteratorData(value.Data.(*IteratorData))
				items, collectErr := runtime.collectIterator(iterator)
				if collectErr != nil {
					return NullValue(), collectErr
				}
				return IntValue(len(items)), nil
			}
			if field, ok := builtinProtocolField(value, current.Field); ok {
				return field, nil
			}
			if builtinProtocolMethodExists(value, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
			if value.Kind != ValueObject && runtime.extensionMethodExists(runtimeTypeName(value), current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: runtimeTypeName(value), Name: current.Field, Receiver: value}}, nil
			}
		}
		if err == nil && value.Kind == ValueObject {
			object := value.Data.(ObjectData)
			if object.Struct && current.Field == "cast_as" {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: object.Type, Name: current.Field, Receiver: value}}, nil
			}
			if field, ok := object.Fields[current.Field]; ok {
				return field, nil
			}
			if runtime.aliasMethodExists(object.Type, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: object.Type, Name: current.Field, Receiver: value}}, nil
			}
			if runtime.extensionMethodExists(object.Type, current.Field) {
				return Value{Kind: ValueBoundMethod, Data: BoundMethodData{Type: object.Type, Name: current.Field, Receiver: value}}, nil
			}
			return NullValue(), Error{Message: fmt.Sprintf("unknown field or method %q", current.Field)}
		}
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			return FunctionValue(runtime.resolveAliasPath(target.Name) + "." + current.Field), nil
		}
		return NullValue(), Error{Message: "unsupported selector target"}
	case parser.CastExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		return castValue(value, current.Type)
	case parser.NullCheckExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		if value.Kind == ValueOption {
			return BoolValue(value.Data.(OptionData).Some), nil
		}
		if value.Kind == ValueResult {
			return BoolValue(value.Data.(ResultData).Ok), nil
		}
		return BoolValue(value.Kind != ValueNull), nil
	case parser.PropagateExpression:
		value, err := runtime.evalExpression(current.Value, env)
		if err != nil {
			return NullValue(), err
		}
		if value.Kind != ValueResult {
			return NullValue(), Error{Message: fmt.Sprintf("! expects Result, got %s", value.Kind)}
		}
		result := value.Data.(ResultData)
		if result.Ok {
			return result.Value, nil
		}
		if result.Value.Kind != ValueAtom {
			return NullValue(), Error{Message: fmt.Sprintf("! only propagates Result[T, Atom], got Err(%s)", result.Value.Kind)}
		}
		return NullValue(), thrownError{Value: result.Value}
	case parser.ConditionalExpression:
		condition, err := runtime.evalExpression(current.Condition, env)
		if err != nil {
			return NullValue(), err
		}
		if isTruthy(condition) {
			return runtime.evalExpression(current.Consequence, env)
		}
		return runtime.evalExpression(current.Alternative, env)
	case parser.IndexExpression:
		return runtime.evalIndex(current, env)
	case parser.ListExpression:
		items := make([]Value, 0, len(current.Items))
		for _, item := range current.Items {
			value, err := runtime.evalExpression(item, env)
			if err != nil {
				return NullValue(), err
			}
			items = append(items, value)
		}
		return Value{Kind: ValueList, Data: items}, nil
	case parser.ListComprehensionExpression:
		return runtime.evalListComprehension(current, env)
	case parser.MapExpression:
		entries := []TableEntryData{}
		for _, entry := range current.Entries {
			key, err := runtime.evalExpression(entry.Key, env)
			if err != nil {
				return NullValue(), err
			}
			value, err := runtime.evalExpression(entry.Value, env)
			if err != nil {
				return NullValue(), err
			}
			if _, err := tableKey(key); err != nil {
				return NullValue(), err
			}
			entries = append(entries, TableEntryData{Key: key, Value: value})
		}
		return TableValueFromEntries(entries), nil
	case parser.LambdaExpression:
		name, err := runtime.defineLocalFunction(parser.FunctionStatement{
			Name:       "lambda",
			Params:     current.Params,
			ReturnType: current.ReturnType,
			Body:       current.Body,
		}, env)
		if err != nil {
			return NullValue(), err
		}
		return FunctionValue(name), nil
	case parser.RawExpression:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %q", parser.Expression{Tokens: current.Tokens}.Literal())}
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported expression %T", expr)}
	}
}

func builtinTypeExpressionName(expr parser.ExpressionNode) (string, bool) {
	switch current := expr.(type) {
	case parser.IdentifierExpression:
		typeName := normalizeRuntimeType(current.Name)
		if _, ok := typeSizeof(typeName); ok {
			return typeName, true
		}
	case parser.SelectorExpression:
		if target, ok := current.Target.(parser.IdentifierExpression); ok {
			typeName := normalizeRuntimeType(target.Name + "." + current.Field)
			if _, ok := typeSizeof(typeName); ok {
				return typeName, true
			}
		}
	case parser.CallExpression:
		selector, ok := current.Callee.(parser.SelectorExpression)
		if !ok || selector.Field != "child" || len(current.Arguments) != 1 {
			return "", false
		}
		target, ok := selector.Target.(parser.IdentifierExpression)
		if !ok {
			return "", false
		}
		literal, ok := current.Arguments[0].(parser.LiteralExpression)
		if !ok || literal.Kind != "Int" {
			return "", false
		}
		typeName := normalizeRuntimeType(target.Name + ".child(" + literal.Value + ")")
		if _, ok := typeSizeof(typeName); ok {
			return typeName, true
		}
	}
	return "", false
}

func (runtime *Runtime) evalListComprehension(expr parser.ListComprehensionExpression, env *Environment) (Value, error) {
	iterable, err := runtime.evalExpression(expr.Iterable, env)
	if err != nil {
		return NullValue(), err
	}

	values, err := runtime.iterableValues(iterable)
	if err != nil {
		return NullValue(), err
	}

	items := make([]Value, 0, len(values))
	for _, value := range values {
		itemEnv := NewEnvironment(env)
		if err := runtime.defineValue(itemEnv, expr.Iterator, false, runtimeTypeName(value), value); err != nil {
			return NullValue(), err
		}

		if expr.Condition != nil {
			condition, err := runtime.evalExpression(expr.Condition, itemEnv)
			if err != nil {
				return NullValue(), err
			}
			if !isTruthy(condition) {
				continue
			}
		}

		item, err := runtime.evalExpression(expr.Value, itemEnv)
		if err != nil {
			return NullValue(), err
		}
		items = append(items, item)
	}
	return Value{Kind: ValueList, Data: items}, nil
}

func (runtime *Runtime) iterableValues(value Value) ([]Value, error) {
	switch value.Kind {
	case ValueList:
		return value.Data.([]Value), nil
	case ValueSet:
		return setValues(value.Data.(SetData)), nil
	case ValueIterator:
		return runtime.collectIterator(value.Data.(*IteratorData))
	case ValueString:
		runes := []rune(value.Data.(string))
		values := make([]Value, 0, len(runes))
		for _, current := range runes {
			values = append(values, CharValue(string(current)))
		}
		return values, nil
	case ValueInt:
		count := value.Data.(int)
		if count < 0 {
			return nil, Error{Message: "list comprehension count cannot be negative"}
		}
		values := make([]Value, 0, count)
		for index := 0; index < count; index++ {
			values = append(values, IntValue(index))
		}
		return values, nil
	case ValueTable:
		return tableEntries(value.Data.(TableData)), nil
	case ValueMap:
		iterator, err := runtime.iteratorFromValue(value)
		if err != nil {
			return nil, err
		}
		return runtime.collectIterator(iterator.Data.(*IteratorData))
	default:
		return nil, Error{Message: fmt.Sprintf("list comprehension cannot iterate over %s", value.Kind)}
	}
}

func (runtime *Runtime) evalUnary(expr parser.UnaryExpression, env *Environment) (Value, error) {
	if expr.Operator == "move" {
		return runtime.evalMove(expr.Right, env)
	}
	value, err := runtime.evalExpression(expr.Right, env)
	if err != nil {
		return NullValue(), err
	}
	switch expr.Operator {
	case "copy", "clone":
		return cloneValue(value), nil
	case "-":
		if value.Kind == ValueFloat {
			return FloatValue(-value.Data.(float64)), nil
		}
		intValue, err := asInt(value)
		if err != nil {
			return NullValue(), err
		}
		return IntValue(-intValue), nil
	case "not":
		return BoolValue(!isTruthy(value)), nil
	case "call":
		if call, ok := expr.Right.(parser.CallExpression); ok {
			return runtime.evalCall(call, env)
		}
		return value, nil
	case "await":
		return runtime.awaitValue(value)
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported unary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) awaitValue(value Value) (Value, error) {
	if value.Kind != ValueAwaitable {
		return NullValue(), Error{Message: fmt.Sprintf("await expects Awaitable, got %s", value.Kind)}
	}
	data := value.Data.(*AwaitableData)
	if data.Done {
		return data.Value, nil
	}
	result, err := runtime.callFunctionMode(data.Function, data.Args, nil, false)
	if err != nil {
		return NullValue(), err
	}
	data.Done = true
	data.Value = result
	return result, nil
}

func (runtime *Runtime) evalMove(expr parser.ExpressionNode, env *Environment) (Value, error) {
	identifier, ok := expr.(parser.IdentifierExpression)
	if !ok {
		return NullValue(), Error{Message: "move expects a variable"}
	}
	binding, ok := env.Get(identifier.Name)
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown variable %q", identifier.Name)}
	}
	var moved Value
	err := binding.WithLock(func() error {
		if binding.Moved {
			return Error{Message: fmt.Sprintf("variable %q was moved", identifier.Name)}
		}
		value, err := runtime.forceValue(binding.Value)
		if err != nil {
			return err
		}
		moved = value
		binding.Moved = true
		binding.Value = NullValue()
		runtime.memory.Store(binding.ObjectID, NullValue())
		runtime.recordState(StateRecord{
			Phase:    "runtime",
			Event:    "move",
			Kind:     binding.Kind,
			Name:     binding.Name,
			Type:     binding.Type,
			Runtime:  runtimeTypeName(NullValue()),
			Function: runtime.currentFunctionName(),
			Mutable:  binding.Mutable,
			Moved:    true,
		})
		return nil
	})
	if err != nil {
		return NullValue(), err
	}
	return moved, nil
}

func (runtime *Runtime) evalBinary(expr parser.BinaryExpression, env *Environment) (Value, error) {
	left, err := runtime.evalExpression(expr.Left, env)
	if err != nil {
		return NullValue(), err
	}

	switch expr.Operator {
	case "|>":
		return runtime.evalPipe(left, expr.Right, env)
	case "and":
		if !isTruthy(left) {
			return BoolValue(false), nil
		}
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(right)), nil
	case "or":
		if isTruthy(left) {
			return BoolValue(true), nil
		}
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(right)), nil
	case "xor":
		right, err := runtime.evalExpression(expr.Right, env)
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(isTruthy(left) != isTruthy(right)), nil
	}

	right, err := runtime.evalExpression(expr.Right, env)
	if err != nil {
		return NullValue(), err
	}
	if overloaded, ok, overloadErr := runtime.evalOverloadedBinaryOperator(left, expr.Operator, right, env); ok {
		return overloaded, overloadErr
	}

	switch expr.Operator {
	case "+":
		if left.Kind == ValueString || right.Kind == ValueString {
			return StringValue(valueString(left) + valueString(right)), nil
		}
		return numericBinary(left, right, func(a, b float64) float64 { return a + b })
	case "-":
		return numericBinary(left, right, func(a, b float64) float64 { return a - b })
	case "*":
		if left.Kind == ValueComplex || right.Kind == ValueComplex {
			return complexBinary(left, right, func(a, b float64) float64 { return a * b }, "*")
		}
		return numericBinary(left, right, func(a, b float64) float64 { return a * b })
	case "/":
		return divideValues(left, right)
	case "//":
		return floorDivideValues(left, right)
	case "%":
		return moduloValues(left, right)
	case "**":
		return exponentValues(left, right)
	case "==":
		return BoolValue(valueString(left) == valueString(right)), nil
	case "!=":
		return BoolValue(valueString(left) != valueString(right)), nil
	case ">":
		return compareOrdered(left, right, func(compare int) bool { return compare > 0 })
	case ">=":
		return compareOrdered(left, right, func(compare int) bool { return compare >= 0 })
	case "<":
		return compareOrdered(left, right, func(compare int) bool { return compare < 0 })
	case "<=":
		return compareOrdered(left, right, func(compare int) bool { return compare <= 0 })
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unsupported binary operator %q", expr.Operator)}
	}
}

func (runtime *Runtime) evalOverloadedBinaryOperator(left Value, operator string, right Value, env *Environment) (Value, bool, error) {
	if left.Kind != ValueObject {
		return NullValue(), false, nil
	}
	methodName, ok := parser.OperatorMethodName(operator)
	if !ok {
		return NullValue(), false, nil
	}
	typeName := left.Data.(ObjectData).Type
	if !runtime.aliasMethodExists(typeName, methodName) {
		return NullValue(), false, nil
	}
	value, err := runtime.callAliasMethodValues(BoundMethodData{
		Type: typeName, Name: methodName, Receiver: left,
	}, []Value{right}, env)
	return value, true, err
}

func (runtime *Runtime) evalPipe(value Value, target parser.ExpressionNode, env *Environment) (Value, error) {
	switch current := target.(type) {
	case parser.CallExpression:
		callee, err := runtime.evalExpression(current.Callee, env)
		if err != nil {
			return NullValue(), err
		}
		if callee.Kind != ValueFunction {
			return NullValue(), Error{Message: "pipe target is not a function"}
		}
		args := []Value{value}
		for _, arg := range current.Arguments {
			argValue, err := runtime.evalExpression(arg, env)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, argValue)
		}
		return runtime.callFunction(callee.Data.(string), args)
	case parser.UnaryExpression:
		if current.Operator == "call" {
			return runtime.evalPipe(value, current.Right, env)
		}
	case parser.IdentifierExpression, parser.SelectorExpression:
		callee, err := runtime.evalExpression(current, env)
		if err != nil {
			return NullValue(), err
		}
		if callee.Kind != ValueFunction {
			return NullValue(), Error{Message: "pipe target is not a function"}
		}
		return runtime.callFunction(callee.Data.(string), []Value{value})
	}
	return NullValue(), Error{Message: "pipe target must be a function or function call"}
}

func (runtime *Runtime) evalCall(expr parser.CallExpression, env *Environment) (Value, error) {
	if identifier, ok := expr.Callee.(parser.IdentifierExpression); ok {
		if macro, exists := runtime.keywordMacros[identifier.Name]; exists {
			args := make([]Value, 0, len(expr.Arguments))
			for _, arg := range expr.Arguments {
				value, err := runtime.evalExpression(arg, env)
				if err != nil {
					return NullValue(), err
				}
				args = append(args, value)
			}
			return runtime.executeKeywordMacro(macro, args, env)
		}
	}
	callee, err := runtime.evalExpression(expr.Callee, env)
	if err != nil {
		return NullValue(), err
	}
	if callee.Kind != ValueFunction {
		if callee.Kind == ValueBoundMethod {
			return runtime.callBoundMethod(callee.Data.(BoundMethodData), expr.Arguments, env)
		}
		return NullValue(), Error{Message: "callee is not a function"}
	}

	args := make([]Value, 0, len(expr.Arguments))
	callArgs := make([]callArgument, 0, len(expr.Arguments))
	if runtime.isLazyFunction(callee.Data.(string)) {
		for _, arg := range expr.Arguments {
			value := ThunkValue(arg, env)
			args = append(args, value)
			callArgs = append(callArgs, callArgument{Value: value})
		}
	} else {
		for _, arg := range expr.Arguments {
			value, err := runtime.evalExpression(arg, env)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, value)
			callArgs = append(callArgs, runtime.callArgument(arg, env, value))
		}
	}
	return runtime.callFunctionAt(callee.Data.(string), args, callArgs, runtime.callSiteFor(expr.Pos))
}

func (runtime *Runtime) callArgument(arg parser.ExpressionNode, env *Environment, value Value) callArgument {
	if identifier, ok := arg.(parser.IdentifierExpression); ok {
		if binding, exists := env.Get(identifier.Name); exists {
			return callArgument{Value: value, Binding: binding, Name: identifier.Name}
		}
	}
	return callArgument{Value: value}
}

func (runtime *Runtime) callFunctionAt(name string, args []Value, callArgs []callArgument, site callSite) (Value, error) {
	runtime.callSites = append(runtime.callSites, site)
	defer func() {
		runtime.callSites = runtime.callSites[:len(runtime.callSites)-1]
	}()
	return runtime.callFunctionMode(name, args, callArgs, true)
}

func (runtime *Runtime) callSiteFor(pos parser.Position) callSite {
	site := callSite{Line: pos.Line, Column: pos.Column}
	if len(runtime.callStack) > 0 {
		site.File = runtime.functionFiles[runtime.callStack[len(runtime.callStack)-1]]
	}
	return site
}

func (runtime *Runtime) evalIndex(expr parser.IndexExpression, env *Environment) (Value, error) {
	target, err := runtime.evalExpression(expr.Target, env)
	if err != nil {
		return NullValue(), err
	}
	index, err := runtime.evalExpression(expr.Index, env)
	if err != nil {
		return NullValue(), err
	}
	switch target.Kind {
	case ValueString:
		position, err := asIndex(index)
		if err != nil {
			return NullValue(), err
		}
		runes := []rune(target.Data.(string))
		if position < 0 || position >= len(runes) {
			return NullValue(), Error{Message: fmt.Sprintf("string index %d is out of bounds", position)}
		}
		return CharValue(string(runes[position])), nil
	case ValueList:
		items := target.Data.([]Value)
		position, err := asIndex(index)
		if err != nil {
			return NullValue(), err
		}
		if position < 0 || position >= len(items) {
			return NullValue(), Error{Message: fmt.Sprintf("list index %d is out of bounds", position)}
		}
		return items[position], nil
	case ValueMap:
		items := target.Data.(map[string]Value)
		key, err := mapKey(index)
		if err != nil {
			return NullValue(), err
		}
		value, ok := items[key]
		if !ok {
			return NullValue(), Error{Message: fmt.Sprintf("map key %q does not exist", key)}
		}
		return value, nil
	case ValueTable:
		key, err := tableKey(index)
		if err != nil {
			return NullValue(), err
		}
		value, ok := tableGet(target.Data.(TableData), key)
		if !ok {
			return NullValue(), Error{Message: fmt.Sprintf("table key %s does not exist", valueString(tableKeyValue(key)))}
		}
		return value, nil
	case ValueJSON:
		value, ok, err := jsonLookup(target, index)
		if err != nil {
			return NullValue(), err
		}
		if !ok {
			return NullValue(), Error{Message: fmt.Sprintf("JSON index %s does not exist", valueString(index))}
		}
		return value, nil
	default:
		return NullValue(), Error{Message: fmt.Sprintf("%s is not indexable", target.Kind)}
	}
}

func (runtime *Runtime) callFunction(name string, args []Value) (Value, error) {
	return runtime.callFunctionMode(name, args, nil, true)
}

func (runtime *Runtime) callFunctionMode(name string, args []Value, callArgs []callArgument, wrapAsync bool) (result Value, err error) {
	name = runtime.resolveAliasPath(name)
	if typeName, ok := runtimeTypeInfoCallTarget(name); ok {
		if len(args) != 0 {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects no arguments", name)}
		}
		if alias, exists := runtime.aliasFunctionForType(typeName); exists {
			return aliasTypeInfoValue(typeName, alias), nil
		}
		if _, ok := typeSizeof(typeName); !ok {
			return NullValue(), Error{Message: fmt.Sprintf("unknown type %s", typeName)}
		}
		return typeInfoValue(typeName), nil
	}
	switch name {
	case "get_args_from_parsable":
		if len(args) != 0 {
			return NullValue(), Error{Message: "get_args_from_parsable expects no arguments"}
		}
		if len(runtime.parsableArgs) == 0 {
			return NullValue(), Error{Message: "get_args_from_parsable is only available inside a keyword macro"}
		}
		items := runtime.parsableArgs[len(runtime.parsableArgs)-1]
		return Value{Kind: ValueList, Data: append([]Value(nil), items...)}, nil
	case "macro_context":
		return runtime.macroContext(args)
	case "macro_expand":
		return runtime.macroExpand(args)
	case "Parsable":
		return runtime.newParsable(args)
	case "Atom":
		if len(args) != 1 || args[0].Kind != ValueString {
			return NullValue(), Error{Message: "Atom expects one String name"}
		}
		return AtomValue(args[0].Data.(string))
	case "File", "file_read", "file_read_lines", "file_write", "file_append", "file_exists", "file_size", "file_create", "file_remove":
		return runtime.callFileBuiltin(name, args)
	case "read_int", "read_ints", "print_ints", "interval_walk_max_scores":
		return runtime.callIntervalWalkBuiltin(name, args)
	case "OS", "os_current_dir", "os_change_dir", "os_temp_dir", "os_home_dir", "os_hostname", "os_process_id",
		"os_get_env", "os_set_env", "os_unset_env", "os_environment", "os_execute":
		return runtime.callOSBuiltin(name, args)
	case "raylib_init_window", "raylib_close_window", "raylib_window_should_close", "raylib_is_window_ready",
		"raylib_set_target_fps", "raylib_get_fps", "raylib_get_frame_time", "raylib_begin_drawing", "raylib_end_drawing",
		"raylib_clear_background", "raylib_draw_text", "raylib_draw_rectangle", "raylib_draw_circle",
		"raylib_is_key_pressed", "raylib_is_key_down", "raylib_get_screen_width", "raylib_get_screen_height",
		"raylib_set_window_title", "raylib_set_window_size", "raylib_set_window_position", "raylib_toggle_fullscreen",
		"raylib_maximize_window", "raylib_minimize_window", "raylib_restore_window", "raylib_is_window_fullscreen",
		"raylib_is_window_hidden", "raylib_is_window_minimized", "raylib_is_window_maximized", "raylib_is_window_focused",
		"raylib_get_time", "raylib_set_exit_key", "raylib_is_key_pressed_repeat", "raylib_is_key_released", "raylib_is_key_up",
		"raylib_get_key_pressed", "raylib_get_char_pressed", "raylib_is_mouse_button_pressed", "raylib_is_mouse_button_down",
		"raylib_is_mouse_button_released", "raylib_get_mouse_x", "raylib_get_mouse_y", "raylib_set_mouse_position",
		"raylib_get_mouse_wheel_move", "raylib_draw_pixel", "raylib_draw_line", "raylib_draw_rectangle_lines",
		"raylib_draw_circle_lines", "raylib_draw_ellipse", "raylib_measure_text", "raylib_take_screenshot",
		"raylib_get_random_value", "raylib_check_collision_recs", "raylib_check_collision_circles", "raylib_check_collision_point_rec":
		return runtime.callRaylibBuiltin(name, args)
	case "parsable_source", "parsable_ast", "parsable_args", "parsable_runtime_info", "parsable_workspace":
		return runtime.parsableField(name, args)
	case "parsable_with_source", "parsable_replace", "parsable_append":
		return runtime.transformParsable(name, args)
	case "parsable_begin_polling":
		return runtime.beginParsablePolling(args)
	case "parsable_poll_message", "parsable_intercept_message":
		return runtime.pollParsableMessage(name, args)
	case "print":
		values := make([]string, 0, len(args))
		for _, arg := range args {
			value, err := runtime.forceValue(arg)
			if err != nil {
				return NullValue(), err
			}
			values = append(values, valueString(value))
		}
		runtime.appendOutput(strings.Join(values, " "))
		return NullValue(), nil
	case "format":
		formatted, err := formatValues(args)
		if err != nil {
			return NullValue(), err
		}
		return StringValue(formatted), nil
	case "printf":
		formatted, err := formatValues(args)
		if err != nil {
			return NullValue(), err
		}
		runtime.appendOutput(formatted)
		return IntValue(len([]rune(formatted))), nil
	case "input":
		if len(args) > 1 {
			return NullValue(), Error{Message: "input expects zero or one argument"}
		}
		if len(args) == 1 {
			value, err := runtime.forceValue(args[0])
			if err != nil {
				return NullValue(), err
			}
			runtime.appendOutput(valueString(value))
		}
		runtime.input.Mutex.Lock()
		text, err := runtime.input.Reader.ReadString('\n')
		runtime.input.Mutex.Unlock()
		if err != nil && len(text) == 0 {
			return StringValue(""), nil
		}
		return StringValue(strings.TrimRight(text, "\r\n")), nil
	case "len":
		if len(args) != 1 {
			return NullValue(), Error{Message: "len expects one argument"}
		}
		length, err := valueLen(args[0])
		if err != nil {
			return NullValue(), err
		}
		return IntValue(length), nil
	case "JSON":
		if len(args) != 1 {
			return NullValue(), Error{Message: "JSON expects one String or serializable value"}
		}
		if args[0].Kind == ValueString {
			return parseJSONValue(args[0].Data.(string))
		}
		converted, err := runtimeValueToJSON(args[0])
		if err != nil {
			return NullValue(), err
		}
		return JSONValue(converted), nil
	case "json_parse":
		if len(args) != 1 || args[0].Kind != ValueString {
			return NullValue(), Error{Message: "json_parse expects one String argument"}
		}
		parsed, err := parseJSONValue(args[0].Data.(string))
		if err != nil {
			return ResultErrValue(StringValue(err.Error())), nil
		}
		return ResultOkValue(parsed), nil
	case "json_decode":
		if len(args) != 1 || args[0].Kind != ValueString {
			return NullValue(), Error{Message: "json_decode expects one String argument"}
		}
		parsed, err := parseJSONValue(args[0].Data.(string))
		if err != nil {
			return ResultErrValue(StringValue(err.Error())), nil
		}
		decoded, err := jsonDataToRuntime(parsed.Data.(JSONData).Value)
		if err != nil {
			return ResultErrValue(StringValue(err.Error())), nil
		}
		return ResultOkValue(decoded), nil
	case "json_encode":
		if len(args) != 1 {
			return NullValue(), Error{Message: "json_encode expects one serializable argument"}
		}
		converted, err := runtimeValueToJSON(args[0])
		if err != nil {
			return ResultErrValue(StringValue(err.Error())), nil
		}
		encoded, err := stringifyJSONValue(JSONValue(converted))
		if err != nil {
			return ResultErrValue(StringValue(err.Error())), nil
		}
		return ResultOkValue(StringValue(encoded)), nil
	case "json_stringify":
		if len(args) != 1 {
			return NullValue(), Error{Message: "json_stringify expects one serializable argument"}
		}
		value := args[0]
		if value.Kind != ValueJSON {
			converted, err := runtimeValueToJSON(value)
			if err != nil {
				return NullValue(), err
			}
			value = JSONValue(converted)
		}
		encoded, err := stringifyJSONValue(value)
		if err != nil {
			return NullValue(), err
		}
		return StringValue(encoded), nil
	case "json_get":
		if len(args) != 2 {
			return NullValue(), Error{Message: "json_get expects JSON and a String or Int index"}
		}
		value, ok, err := jsonLookup(args[0], args[1])
		if err != nil {
			return NullValue(), err
		}
		if !ok {
			return OptionNoneValue(), nil
		}
		return OptionSomeValue(value), nil
	case "json_kind":
		if len(args) != 1 {
			return NullValue(), Error{Message: "json_kind expects one JSON argument"}
		}
		kind, err := jsonValueKind(args[0])
		if err != nil {
			return NullValue(), err
		}
		return StringValue(kind), nil
	case "json_string", "json_int", "json_float", "json_bool":
		if len(args) != 1 {
			return NullValue(), Error{Message: name + " expects one JSON argument"}
		}
		if args[0].Kind != ValueJSON {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects JSON, got %s", name, args[0].Kind)}
		}
		var value Value
		var ok bool
		switch name {
		case "json_string":
			value, ok = jsonStringValue(args[0])
		case "json_int":
			value, ok = jsonIntValue(args[0])
		case "json_float":
			value, ok = jsonFloatValue(args[0])
		case "json_bool":
			value, ok = jsonBoolValue(args[0])
		}
		if !ok {
			return OptionNoneValue(), nil
		}
		return OptionSomeValue(value), nil
	case "json_is_null":
		if len(args) != 1 || args[0].Kind != ValueJSON {
			return NullValue(), Error{Message: "json_is_null expects one JSON argument"}
		}
		return BoolValue(jsonIsNull(args[0])), nil
	case "table_has", "has_key":
		if len(args) != 2 {
			return NullValue(), Error{Message: name + " expects two arguments"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: name + " expects a Table as the first argument"}
		}
		key, err := tableKey(args[1])
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(tableHas(args[0].Data.(TableData), key)), nil
	case "set_has":
		if len(args) != 2 {
			return NullValue(), Error{Message: "set_has expects two arguments"}
		}
		if args[0].Kind != ValueSet {
			return NullValue(), Error{Message: "set_has expects a Set as the first argument"}
		}
		ok, err := setHas(args[0].Data.(SetData), args[1])
		if err != nil {
			return NullValue(), err
		}
		return BoolValue(ok), nil
	case "table_delete":
		if len(args) != 2 {
			return NullValue(), Error{Message: "table_delete expects two arguments"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: "table_delete expects a Table as the first argument"}
		}
		key, err := tableKey(args[1])
		if err != nil {
			return NullValue(), err
		}
		table := cloneTableData(args[0].Data.(TableData))
		tableDelete(&table, key)
		return Value{Kind: ValueTable, Data: table}, nil
	case "table_keys":
		if len(args) != 1 {
			return NullValue(), Error{Message: "table_keys expects one argument"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: "table_keys expects a Table"}
		}
		return Value{Kind: ValueList, Data: tableKeys(args[0].Data.(TableData))}, nil
	case "table_values":
		if len(args) != 1 {
			return NullValue(), Error{Message: "table_values expects one argument"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: "table_values expects a Table"}
		}
		return Value{Kind: ValueList, Data: tableValues(args[0].Data.(TableData))}, nil
	case "table_entries":
		if len(args) != 1 {
			return NullValue(), Error{Message: "table_entries expects one argument"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: "table_entries expects a Table"}
		}
		return Value{Kind: ValueList, Data: tableEntries(args[0].Data.(TableData))}, nil
	case "table_sequence_count":
		if len(args) != 1 {
			return NullValue(), Error{Message: "table_sequence_count expects one argument"}
		}
		if args[0].Kind != ValueTable {
			return NullValue(), Error{Message: "table_sequence_count expects a Table"}
		}
		return IntValue(tableSequenceCount(args[0].Data.(TableData))), nil
	case "table_set_fallback":
		if len(args) != 2 {
			return NullValue(), Error{Message: "table_set_fallback expects two arguments"}
		}
		if args[0].Kind != ValueTable || args[1].Kind != ValueTable {
			return NullValue(), Error{Message: "table_set_fallback expects two Tables"}
		}
		table := cloneTableData(args[0].Data.(TableData))
		fallback := cloneTableData(args[1].Data.(TableData))
		table.Fallback = &fallback
		return Value{Kind: ValueTable, Data: table}, nil
	case "range":
		if len(args) != 1 {
			return NullValue(), Error{Message: "range expects one argument"}
		}
		return args[0], nil
	case "option_map":
		if len(args) != 2 {
			return NullValue(), Error{Message: "option_map expects Option[T] and Function[T,U]"}
		}
		if args[0].Kind != ValueOption || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "option_map expects Option[T] and Function[T,U]"}
		}
		option := args[0].Data.(OptionData)
		if !option.Some {
			return OptionNoneValue(), nil
		}
		mapped, err := runtime.callFunction(args[1].Data.(string), []Value{option.Value})
		if err != nil {
			return NullValue(), err
		}
		return OptionSomeValue(mapped), nil
	case "option_unwrap_or":
		if len(args) != 2 {
			return NullValue(), Error{Message: "option_unwrap_or expects Option[T] and fallback T"}
		}
		if args[0].Kind != ValueOption {
			return NullValue(), Error{Message: "option_unwrap_or expects Option[T] as first argument"}
		}
		option := args[0].Data.(OptionData)
		if option.Some {
			return option.Value, nil
		}
		return args[1], nil
	case "option_and_then":
		if len(args) != 2 {
			return NullValue(), Error{Message: "option_and_then expects Option[T] and Function[T,Option[U]]"}
		}
		if args[0].Kind != ValueOption || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "option_and_then expects Option[T] and Function[T,Option[U]]"}
		}
		option := args[0].Data.(OptionData)
		if !option.Some {
			return OptionNoneValue(), nil
		}
		next, err := runtime.callFunction(args[1].Data.(string), []Value{option.Value})
		if err != nil {
			return NullValue(), err
		}
		if next.Kind != ValueOption {
			return NullValue(), Error{Message: fmt.Sprintf("option_and_then callback must return Option, got %s", next.Kind)}
		}
		return next, nil
	case "result_map":
		if len(args) != 2 {
			return NullValue(), Error{Message: "result_map expects Result[T,E] and Function[T,U]"}
		}
		if args[0].Kind != ValueResult || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "result_map expects Result[T,E] and Function[T,U]"}
		}
		result := args[0].Data.(ResultData)
		if !result.Ok {
			return args[0], nil
		}
		mapped, err := runtime.callFunction(args[1].Data.(string), []Value{result.Value})
		if err != nil {
			return NullValue(), err
		}
		return ResultOkValue(mapped), nil
	case "result_map_err":
		if len(args) != 2 {
			return NullValue(), Error{Message: "result_map_err expects Result[T,E] and Function[E,F]"}
		}
		if args[0].Kind != ValueResult || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "result_map_err expects Result[T,E] and Function[E,F]"}
		}
		result := args[0].Data.(ResultData)
		if result.Ok {
			return args[0], nil
		}
		mapped, err := runtime.callFunction(args[1].Data.(string), []Value{result.Value})
		if err != nil {
			return NullValue(), err
		}
		return ResultErrValue(mapped), nil
	case "result_unwrap_or":
		if len(args) != 2 {
			return NullValue(), Error{Message: "result_unwrap_or expects Result[T,E] and fallback T"}
		}
		if args[0].Kind != ValueResult {
			return NullValue(), Error{Message: "result_unwrap_or expects Result[T,E] as first argument"}
		}
		result := args[0].Data.(ResultData)
		if result.Ok {
			return result.Value, nil
		}
		return args[1], nil
	case "result_and_then":
		if len(args) != 2 {
			return NullValue(), Error{Message: "result_and_then expects Result[T,E] and Function[T,Result[U,E]]"}
		}
		if args[0].Kind != ValueResult || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "result_and_then expects Result[T,E] and Function[T,Result[U,E]]"}
		}
		result := args[0].Data.(ResultData)
		if !result.Ok {
			return args[0], nil
		}
		next, err := runtime.callFunction(args[1].Data.(string), []Value{result.Value})
		if err != nil {
			return NullValue(), err
		}
		if next.Kind != ValueResult {
			return NullValue(), Error{Message: fmt.Sprintf("result_and_then callback must return Result, got %s", next.Kind)}
		}
		return next, nil
	case "Some":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Some expects one argument"}
		}
		return OptionSomeValue(args[0]), nil
	case "None":
		if len(args) != 0 {
			return NullValue(), Error{Message: "None expects no arguments"}
		}
		return OptionNoneValue(), nil
	case "Ok":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Ok expects one argument"}
		}
		return ResultOkValue(args[0]), nil
	case "Err":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Err expects one argument"}
		}
		return ResultErrValue(args[0]), nil
	case "Result":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Result expects one argument"}
		}
		return ResultOkValue(args[0]), nil
	case "Complex":
		if len(args) != 2 {
			return NullValue(), Error{Message: "Complex expects two arguments"}
		}
		real, err := asFloat(args[0])
		if err != nil {
			return NullValue(), Error{Message: "Complex real component must be numeric"}
		}
		imag, err := asFloat(args[1])
		if err != nil {
			return NullValue(), Error{Message: "Complex imaginary component must be numeric"}
		}
		return ComplexValue(real, imag), nil
	case "SIMD":
		if len(args) != 1 {
			return NullValue(), Error{Message: "SIMD expects one list argument"}
		}
		if args[0].Kind != ValueList {
			return NullValue(), Error{Message: "SIMD expects a List argument"}
		}
		return SIMDValue(args[0].Data.([]Value)), nil
	case "Set":
		if len(args) > 1 {
			return NullValue(), Error{Message: "Set expects zero or one argument"}
		}
		if len(args) == 0 {
			return Value{Kind: ValueSet, Data: newSetData()}, nil
		}
		if args[0].Kind == ValueSet {
			return args[0], nil
		}
		if args[0].Kind != ValueList {
			return NullValue(), Error{Message: "Set expects a List or Set argument"}
		}
		return SetValue(args[0].Data.([]Value))
	case "Table":
		if len(args) > 1 {
			return NullValue(), Error{Message: "Table expects zero or one argument"}
		}
		if len(args) == 0 {
			return Value{Kind: ValueTable, Data: newTableData()}, nil
		}
		if args[0].Kind == ValueTable {
			return args[0], nil
		}
		if args[0].Kind != ValueMap {
			return NullValue(), Error{Message: "Table expects a map literal or Table value"}
		}
		return TableValue(args[0].Data.(map[string]Value)), nil
	case "iter":
		if len(args) != 1 {
			return NullValue(), Error{Message: "iter expects one iterable value"}
		}
		return runtime.iteratorFromValue(args[0])
	case "next":
		if len(args) != 1 {
			return NullValue(), Error{Message: "next expects one Iterator"}
		}
		if args[0].Kind != ValueIterator {
			return NullValue(), Error{Message: fmt.Sprintf("next expects Iterator, got %s", args[0].Kind)}
		}
		value, ok, err := runtime.nextIterator(args[0].Data.(*IteratorData))
		if err != nil {
			return NullValue(), err
		}
		if !ok {
			return OptionNoneValue(), nil
		}
		return OptionSomeValue(value), nil
	case "coroutine":
		if len(args) != 1 {
			return NullValue(), Error{Message: "coroutine expects one function"}
		}
		if args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: fmt.Sprintf("coroutine expects Function, got %s", args[0].Kind)}
		}
		return CoroutineValue(args[0].Data.(string)), nil
	case "resume":
		if len(args) != 1 {
			return NullValue(), Error{Message: "resume expects one Coroutine"}
		}
		if args[0].Kind != ValueCoroutine {
			return NullValue(), Error{Message: fmt.Sprintf("resume expects Coroutine, got %s", args[0].Kind)}
		}
		coroutine := args[0].Data.(*CoroutineData)
		if coroutine.Done {
			return OptionNoneValue(), nil
		}
		value, err := runtime.callFunctionMode(coroutine.Function, nil, nil, false)
		if err != nil {
			return NullValue(), err
		}
		coroutine.Done = true
		coroutine.Value = value
		return OptionSomeValue(value), nil
	case "spawn":
		if len(args) < 1 || len(args) > 2 || args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: "spawn expects Function and optional List arguments"}
		}
		threadArgs := []Value{}
		if len(args) == 2 {
			if args[1].Kind != ValueList {
				return NullValue(), Error{Message: "spawn arguments must be a List"}
			}
			for index, argument := range args[1].Data.([]Value) {
				cloned, err := cloneThreadTransferValue(argument)
				if err != nil {
					return NullValue(), Error{Message: fmt.Sprintf(
						"spawn argument %d is not thread-transfer-safe: %v", index+1, err,
					)}
				}
				threadArgs = append(threadArgs, cloned)
			}
		}
		thread := &ThreadData{Done: make(chan struct{})}
		functionName := args[0].Data.(string)
		if closure, captured := runtime.closures[functionName]; captured && closure != runtime.global {
			return NullValue(), Error{Message: "spawn target must be a named workspace function without captured local state"}
		}
		if function, ok := runtime.functions[functionName]; ok && function.Async {
			return NullValue(), Error{Message: "spawn target cannot be async; await async work on its owning thread"}
		}
		child := runtime.childRuntime()
		go func() {
			var value Value
			var workerErr error
			defer func() {
				if recovered := recover(); recovered != nil {
					workerErr = Error{Message: fmt.Sprintf("thread worker panicked: %v", recovered)}
				}
				if workerErr == nil {
					value, workerErr = cloneThreadTransferValue(value)
				}
				thread.Mutex.Lock()
				thread.Value = value
				thread.Err = workerErr
				thread.Mutex.Unlock()
				close(thread.Done)
			}()
			value, workerErr = child.callFunctionMode(functionName, threadArgs, nil, false)
		}()
		return Value{Kind: ValueThread, Data: thread}, nil
	case "join":
		if len(args) != 1 || args[0].Kind != ValueThread {
			return NullValue(), Error{Message: "join expects one Thread"}
		}
		thread := args[0].Data.(*ThreadData)
		<-thread.Done
		thread.Mutex.Lock()
		defer thread.Mutex.Unlock()
		if thread.Err != nil {
			return NullValue(), thread.Err
		}
		return cloneValue(thread.Value), nil
	case "thread_status":
		if len(args) != 1 || args[0].Kind != ValueThread {
			return NullValue(), Error{Message: "thread_status expects one Thread"}
		}
		thread := args[0].Data.(*ThreadData)
		select {
		case <-thread.Done:
			return StringValue("done"), nil
		default:
			return StringValue("running"), nil
		}
	case "Atomic":
		if len(args) != 1 {
			return NullValue(), Error{Message: "Atomic expects one value"}
		}
		value, err := cloneThreadTransferValue(args[0])
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("Atomic value is not thread-transfer-safe: %v", err)}
		}
		return newAtomicValue(value), nil
	case "atomic_load":
		if len(args) != 1 || args[0].Kind != ValueAtomic {
			return NullValue(), Error{Message: "atomic_load expects one Atomic value"}
		}
		atomic := args[0].Data.(*AtomicData)
		if runtime.transaction != nil {
			return runtime.transaction.load(atomic)
		}
		atomic.Mutex.Lock()
		defer atomic.Mutex.Unlock()
		return cloneValue(atomic.Value), nil
	case "atomic_store":
		if len(args) != 2 || args[0].Kind != ValueAtomic {
			return NullValue(), Error{Message: "atomic_store expects Atomic and value"}
		}
		value, err := cloneThreadTransferValue(args[1])
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("atomic_store value is not thread-transfer-safe: %v", err)}
		}
		atomic := args[0].Data.(*AtomicData)
		if runtime.transaction != nil {
			runtime.transaction.store(atomic, value)
			return args[0], nil
		}
		atomic.Mutex.Lock()
		atomic.Value = value
		atomic.Version.Store(stmClock.Add(1))
		atomic.Mutex.Unlock()
		return args[0], nil
	case "atomic_add":
		if len(args) != 2 || args[0].Kind != ValueAtomic {
			return NullValue(), Error{Message: "atomic_add expects Atomic and numeric value"}
		}
		atomic := args[0].Data.(*AtomicData)
		if runtime.transaction != nil {
			current, err := runtime.transaction.load(atomic)
			if err != nil {
				return NullValue(), err
			}
			next, err := numericBinary(current, args[1], func(a, b float64) float64 { return a + b })
			if err != nil {
				return NullValue(), err
			}
			runtime.transaction.store(atomic, next)
			return cloneValue(next), nil
		}
		atomic.Mutex.Lock()
		defer atomic.Mutex.Unlock()
		next, err := numericBinary(atomic.Value, args[1], func(a, b float64) float64 { return a + b })
		if err != nil {
			return NullValue(), err
		}
		atomic.Value = next
		atomic.Version.Store(stmClock.Add(1))
		return cloneValue(atomic.Value), nil
	case "Program":
		if len(args) != 1 || args[0].Kind != ValueList {
			return NullValue(), Error{Message: "Program expects module: List[String]"}
		}
		modules, err := stringList(args[0])
		if err != nil {
			return NullValue(), Error{Message: "Program module expects List[String]"}
		}
		return objectValue("Program", map[string]Value{
			"module":  listFromStrings(modules),
			"modules": listFromStrings(modules),
		}), nil
	case "BuildSystem":
		if len(args) != 4 {
			return NullValue(), Error{Message: "BuildSystem expects project_name, number_of_files, files, backend"}
		}
		projectName, ok := stringData(args[0])
		if !ok {
			return NullValue(), Error{Message: "BuildSystem project_name expects String"}
		}
		numberOfFiles, err := asInt(args[1])
		if err != nil {
			return NullValue(), Error{Message: "BuildSystem number_of_files expects Int"}
		}
		files, err := stringList(args[2])
		if err != nil {
			return NullValue(), Error{Message: "BuildSystem files expects List[String]"}
		}
		backend, ok := stringData(args[3])
		if !ok || !isBuildBackendName(backend) {
			return NullValue(), Error{Message: "BuildSystem backend must be WASM, JS, or Standalone"}
		}
		if numberOfFiles != len(files) {
			return NullValue(), Error{Message: fmt.Sprintf("BuildSystem number_of_files is %d but files has %d item(s)", numberOfFiles, len(files))}
		}
		return objectValue("BuildSystem", map[string]Value{
			"project_name":    StringValue(projectName),
			"number_of_files": IntValue(numberOfFiles),
			"files":           listFromStrings(files),
			"backend":         StringValue(backend),
		}), nil
	case "WorkSpace":
		if len(args) != 2 {
			return NullValue(), Error{Message: "WorkSpace expects Program and BuildSystem"}
		}
		if !isObjectType(args[0], "Program") {
			return NullValue(), Error{Message: "WorkSpace first argument expects Program"}
		}
		if !isObjectType(args[1], "BuildSystem") {
			return NullValue(), Error{Message: "WorkSpace second argument expects BuildSystem"}
		}
		return objectValue("WorkSpace", map[string]Value{
			"Program":      args[0],
			"BuildSystem":  args[1],
			"program":      args[0],
			"build_system": args[1],
		}), nil
	case "workspace_backend":
		workspace, err := requireObject(args, "WorkSpace", "workspace_backend")
		if err != nil {
			return NullValue(), err
		}
		buildSystem := workspace.Fields["build_system"].Data.(ObjectData)
		return buildSystem.Fields["backend"], nil
	case "workspace_files":
		workspace, err := requireObject(args, "WorkSpace", "workspace_files")
		if err != nil {
			return NullValue(), err
		}
		buildSystem := workspace.Fields["build_system"].Data.(ObjectData)
		return buildSystem.Fields["files"], nil
	case "workspace_manifest":
		workspace, err := requireObject(args, "WorkSpace", "workspace_manifest")
		if err != nil {
			return NullValue(), err
		}
		buildSystem := workspace.Fields["build_system"].Data.(ObjectData)
		program := workspace.Fields["program"].Data.(ObjectData)
		files := buildSystem.Fields["files"].Data.([]Value)
		modules := program.Fields["modules"].Data.([]Value)
		return StringValue(fmt.Sprintf("workspace %s backend=%s files=%d modules=%d",
			valueString(buildSystem.Fields["project_name"]),
			valueString(buildSystem.Fields["backend"]),
			len(files),
			len(modules),
		)), nil
	case "runtime_debug_loc", "runtime.debug.__LOC__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return StringValue(runtime.debugLocationString(runtime.currentCallSite())), nil
	case "runtime_debug_file", "runtime.debug.__FILE__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return StringValue(runtime.currentCallSite().File), nil
	case "runtime_debug_line", "runtime.debug.__LINE__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return IntValue(runtime.currentCallSite().Line), nil
	case "runtime_debug_module", "runtime.debug.__MODULE__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return StringValue(runtime.debugModuleName(runtime.currentCallSite())), nil
	case "runtime_debug_pos", "runtime.debug.__POS__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return runtime.debugPositionTable(runtime.currentCallSite()), nil
	case "runtime_debug_function", "runtime.debug.__FUNCTION__":
		if len(args) != 0 {
			return NullValue(), Error{Message: name + " expects no arguments"}
		}
		return StringValue(runtime.currentFunctionName()), nil
	case "runtime_debug_loc_of", "runtime.debug.__LOC_OF__":
		if len(args) != 1 {
			return NullValue(), Error{Message: name + " expects one value"}
		}
		return TableValue(map[string]Value{
			"loc":   StringValue(runtime.debugLocationString(runtime.currentCallSite())),
			"value": args[0],
		}), nil
	case "runtime_debug_line_of", "runtime.debug.__LINE_OF__":
		if len(args) != 1 {
			return NullValue(), Error{Message: name + " expects one value"}
		}
		return TableValue(map[string]Value{
			"line":  IntValue(runtime.currentCallSite().Line),
			"value": args[0],
		}), nil
	case "runtime_debug_pos_of", "runtime.debug.__POS_OF__":
		if len(args) != 1 {
			return NullValue(), Error{Message: name + " expects one value"}
		}
		return TableValue(map[string]Value{
			"pos":   runtime.debugPositionTable(runtime.currentCallSite()),
			"value": args[0],
		}), nil
	case "debug":
		if len(args) != 1 {
			return NullValue(), Error{Message: "debug expects one value"}
		}
		runtime.appendOutput(fmt.Sprintf("[debug] %s = %s", runtimeTypeName(args[0]), valueString(args[0])))
		return args[0], nil
	case "debug_type":
		if len(args) != 1 {
			return NullValue(), Error{Message: "debug_type expects one value"}
		}
		return StringValue(runtimeTypeName(args[0])), nil
	case "debug_stack":
		if len(args) != 0 {
			return NullValue(), Error{Message: "debug_stack expects no arguments"}
		}
		return listFromStrings(runtime.callStack), nil
	case "debug_state":
		if len(args) != 0 {
			return NullValue(), Error{Message: "debug_state expects no arguments"}
		}
		return runtime.stateRecordsValue(), nil
	case "breakpoint":
		if len(args) > 1 {
			return NullValue(), Error{Message: "breakpoint expects zero or one label"}
		}
		label := "breakpoint"
		if len(args) == 1 {
			label = valueString(args[0])
		}
		runtime.appendOutput(fmt.Sprintf("[breakpoint] %s stack=%s", label, strings.Join(runtime.callStack, " -> ")))
		return NullValue(), nil
	case "js_import":
		if len(args) != 1 {
			return NullValue(), Error{Message: "js_import expects one .js file path"}
		}
		path, ok := stringData(args[0])
		if !ok || filepath.Ext(path) != ".js" {
			return NullValue(), Error{Message: "js_import expects a String path ending in .js"}
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return NullValue(), Error{Message: fmt.Sprintf("js_import failed: %v", err)}
		}
		source := string(contents)
		exports := jsExports(source)
		return objectValue("JSModule", map[string]Value{
			"path":    StringValue(path),
			"source":  StringValue(source),
			"exports": listFromStrings(exports),
		}), nil
	case "js_source":
		module, err := requireObject(args, "JSModule", "js_source")
		if err != nil {
			return NullValue(), err
		}
		return module.Fields["source"], nil
	case "js_exports":
		module, err := requireObject(args, "JSModule", "js_exports")
		if err != nil {
			return NullValue(), err
		}
		return module.Fields["exports"], nil
	case "js_call":
		if len(args) != 3 || !isObjectType(args[0], "JSModule") {
			return NullValue(), Error{Message: "js_call expects JSModule, api name, and List arguments"}
		}
		apiName, ok := stringData(args[1])
		if !ok || args[2].Kind != ValueList {
			return NullValue(), Error{Message: "js_call expects JSModule, String, List"}
		}
		module := args[0].Data.(ObjectData)
		return objectValue("JSCall", map[string]Value{
			"module": module.Fields["path"],
			"api":    StringValue(apiName),
			"args":   args[2],
			"status": StringValue("filesystem-only"),
		}), nil
	case "Box", "Ref", "RefMut", "RefCell":
		if len(args) != 1 {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects one value", name)}
		}
		return allocatorObject(name, map[string]Value{"value": args[0]}), nil
	case "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		fields := map[string]Value{}
		if len(args) > 0 {
			fields["region"] = args[0]
		}
		return allocatorObject(name, fields), nil
	}

	if _, ok := runtime.aliasFunctions[name]; ok {
		return runtime.callAliasFunction(name, args)
	}

	resolvedName, err := runtime.resolveFunctionName(name)
	if err != nil {
		return NullValue(), err
	}
	if resolvedName == "" {
		if _, ok := runtime.groups[name]; ok {
			return runtime.callFunctionGroup(name, args)
		}
		return NullValue(), Error{Message: fmt.Sprintf("unknown function %q", name)}
	}
	if function := runtime.functions[resolvedName]; function.Backend != "" && function.Backend != runtime.backend {
		return NullValue(), Error{Message: fmt.Sprintf(
			"function %s requires backend %s, but the active runtime backend is %s",
			resolvedName, function.Backend, runtime.backend,
		)}
	}
	if wrapAsync && runtime.functions[resolvedName].Async {
		return AwaitableValue(resolvedName, args), nil
	}
	if runtime.callDepth >= runtime.maxDepth {
		return NullValue(), Error{Message: fmt.Sprintf("maximum call depth %d exceeded while calling %s", runtime.maxDepth, name)}
	}

	runtime.callDepth++
	runtime.callStack = append(runtime.callStack, resolvedName)
	defer func() {
		if err != nil {
			err = runtime.errorWithStack(err)
		}
		runtime.callDepth--
		runtime.callStack = runtime.callStack[:len(runtime.callStack)-1]
	}()

	for {
		function := runtime.functions[resolvedName]
		required := requiredRuntimeParamCount(function.Params)
		if len(args) < required || len(args) > len(function.Params) {
			return NullValue(), Error{Message: fmt.Sprintf("function %s expects %d to %d argument(s), got %d", resolvedName, required, len(function.Params), len(args))}
		}

		parent := runtime.global
		if closureEnv, ok := runtime.closures[resolvedName]; ok {
			parent = closureEnv
		}
		env := NewEnvironment(parent)
		for index, param := range function.Params {
			var value Value
			if index < len(args) {
				value = args[index]
			} else {
				var err error
				if function.Lazy {
					value = ThunkValue(param.Default.Node, env)
				} else {
					value, err = runtime.evalExpression(param.Default.Node, env)
				}
				if err != nil {
					return NullValue(), err
				}
			}
			if !valueMatchesType(value, param.Type) {
				return NullValue(), Error{Message: fmt.Sprintf("function %s argument %q expects %s, got %s", resolvedName, param.Name, param.Type, value.Kind)}
			}
			if param.ByRef {
				if index >= len(callArgs) || callArgs[index].Binding == nil {
					return NullValue(), Error{Message: fmt.Sprintf("function %s reference argument %q expects a variable", resolvedName, param.Name)}
				}
				sourceBinding := callArgs[index].Binding
				sourceSnapshot := sourceBinding.Snapshot()
				if !sourceSnapshot.Mutable {
					return NullValue(), Error{Message: fmt.Sprintf("function %s reference argument %q requires mutable variable %q", resolvedName, param.Name, callArgs[index].Name)}
				}
				if !valueMatchesType(sourceSnapshot.Value, param.Type) {
					return NullValue(), Error{Message: fmt.Sprintf("function %s reference argument %q expects %s, got %s", resolvedName, param.Name, param.Type, sourceSnapshot.Value.Kind)}
				}
				if err := env.DefineAlias(param.Name, sourceBinding); err != nil {
					return NullValue(), err
				}
				sourceBinding.Kind = "parameter"
				runtime.recordState(StateRecord{
					Phase:    "runtime",
					Event:    "bind",
					Kind:     "parameter",
					Name:     param.Name,
					Type:     param.Type,
					Runtime:  runtimeTypeName(sourceSnapshot.Value),
					Function: function.Name,
					Mutable:  sourceSnapshot.Mutable,
				})
				continue
			}
			if err := runtime.defineValueWithState(env, param.Name, param.Mutable, param.Type, value, MemoryStack, "parameter", "bind"); err != nil {
				return NullValue(), err
			}
		}
		for _, returnValue := range function.ReturnValues {
			if returnValue.Name == "" {
				continue
			}
			if err := runtime.defineValueWithState(env, returnValue.Name, returnValue.Mutable, returnValue.Type, zeroValue(returnValue.Type), MemoryStack, "named_return", "define"); err != nil {
				return NullValue(), err
			}
		}
		runtime.innerSets = append(runtime.innerSets, map[string]Value{})
		currentSignal, err := runtime.executeBlock(function.Body, env, false)
		innerFields := runtime.innerSets[len(runtime.innerSets)-1]
		runtime.innerSets = runtime.innerSets[:len(runtime.innerSets)-1]
		if err != nil {
			return NullValue(), err
		}
		if currentSignal.kind == signalTailCall {
			args = currentSignal.tailArgs
			callArgs = nil
			continue
		}
		if currentSignal.kind == signalThrow {
			return NullValue(), thrownError{Value: currentSignal.value}
		}
		if currentSignal.kind == signalReturn {
			if !valueMatchesType(currentSignal.value, function.ReturnType) {
				return NullValue(), Error{Message: fmt.Sprintf("function %s returns %s, got %s", resolvedName, function.ReturnType, currentSignal.value.Kind)}
			}
			runtime.recordState(StateRecord{
				Phase:    "runtime",
				Event:    "return",
				Kind:     "return",
				Name:     function.Name,
				Type:     function.ReturnType,
				Runtime:  runtimeTypeName(currentSignal.value),
				Function: function.Name,
			})
			return currentSignal.value, nil
		}
		if function.ReturnType != "" && function.ReturnType != "T" {
			if value, ok, err := runtime.namedReturnValue(function, env); ok || err != nil {
				if err == nil {
					runtime.recordState(StateRecord{
						Phase:    "runtime",
						Event:    "return",
						Kind:     "return",
						Name:     function.Name,
						Type:     function.ReturnType,
						Runtime:  runtimeTypeName(value),
						Function: function.Name,
					})
				}
				return value, err
			}
			return NullValue(), Error{Message: fmt.Sprintf("function %s returns %s, got Null", resolvedName, function.ReturnType)}
		}
		if len(innerFields) != 0 {
			return Value{Kind: ValueMap, Data: innerFields}, nil
		}
		return NullValue(), nil
	}
}

func (runtime *Runtime) aliasFunctionForType(typeName string) (parser.AliasFunctionStatement, bool) {
	base := normalizeRuntimeType(typeName)
	if index := strings.Index(base, "["); index > 0 {
		base = base[:index]
	}
	alias, ok := runtime.aliasFunctions[base]
	return alias, ok
}

func aliasTypeInfoValue(typeName string, alias parser.AliasFunctionStatement) Value {
	info := typeInfoValue(typeName)
	object := info.Data.(ObjectData)
	fieldMetadata := map[string]Value{}
	jsonTags := map[string]Value{}
	tagsByField := map[string]string{}
	for _, tag := range alias.FieldTags {
		if tag.Kind == "json" {
			tagsByField[tag.Field] = tag.Name
			jsonTags[tag.Field] = StringValue(tag.Name)
		}
	}
	for _, field := range alias.Params {
		metadata := map[string]Value{"type": StringValue(field.Type)}
		if name, ok := tagsByField[field.Name]; ok {
			metadata["json"] = StringValue(name)
		}
		fieldMetadata[field.Name] = TableValue(metadata)
	}
	fields := TableValue(fieldMetadata)
	serialization := TableValue(map[string]Value{
		"pack":      StringValue("json"),
		"unpack":    StringValue("json"),
		"json_tags": TableValue(jsonTags),
	})
	object.Fields["category"] = StringValue("struct")
	object.Fields["fields"] = fields
	object.Fields["field_count"] = IntValue(len(alias.Params))
	object.Fields["serialization"] = serialization
	object.Fields["introspection"] = TableValue(map[string]Value{"fields": fields})
	info.Data = object
	return info
}

func formatValues(args []Value) (string, error) {
	if len(args) != 2 {
		return "", Error{Message: "format expects String and List arguments"}
	}
	if args[0].Kind != ValueString {
		return "", Error{Message: fmt.Sprintf("format pattern expects String, got %s", args[0].Kind)}
	}
	if args[1].Kind != ValueList {
		return "", Error{Message: fmt.Sprintf("format values expect List, got %s", args[1].Kind)}
	}
	pattern := args[0].Data.(string)
	values := args[1].Data.([]Value)
	var builder strings.Builder
	valueIndex := 0
	for index := 0; index < len(pattern); index++ {
		if pattern[index] != '%' {
			builder.WriteByte(pattern[index])
			continue
		}
		if index+1 < len(pattern) && pattern[index+1] == '%' {
			builder.WriteByte('%')
			index++
			continue
		}
		if valueIndex >= len(values) {
			return "", Error{Message: "format missing value for placeholder"}
		}
		builder.WriteString(valueString(values[valueIndex]))
		valueIndex++
	}
	if valueIndex != len(values) {
		return "", Error{Message: "format received more values than placeholders"}
	}
	return builder.String(), nil
}

func (runtime *Runtime) namedReturnValue(function parser.FunctionStatement, env *Environment) (Value, bool, error) {
	if len(function.ReturnValues) == 0 {
		return NullValue(), false, nil
	}
	items := make([]Value, 0, len(function.ReturnValues))
	for _, returnValue := range function.ReturnValues {
		if returnValue.Name == "" {
			return NullValue(), false, nil
		}
		binding, ok := env.Get(returnValue.Name)
		if !ok {
			return NullValue(), false, Error{Message: fmt.Sprintf("named return value %q is not defined", returnValue.Name)}
		}
		items = append(items, binding.Snapshot().Value)
	}
	return Value{Kind: ValueList, Data: items}, true, nil
}

func allocatorObject(kind string, fields map[string]Value) Value {
	return objectValue(kind, fields)
}

func objectValue(kind string, fields map[string]Value) Value {
	copied := map[string]Value{"kind": StringValue(kind)}
	for key, value := range fields {
		copied[key] = value
	}
	return Value{Kind: ValueObject, Data: ObjectData{Type: kind, Fields: copied}}
}

func isObjectType(value Value, typeName string) bool {
	if value.Kind != ValueObject {
		return false
	}
	return value.Data.(ObjectData).Type == typeName
}

func requireObject(args []Value, typeName string, functionName string) (ObjectData, error) {
	if len(args) != 1 || !isObjectType(args[0], typeName) {
		return ObjectData{}, Error{Message: fmt.Sprintf("%s expects %s", functionName, typeName)}
	}
	return args[0].Data.(ObjectData), nil
}

func (runtime *Runtime) currentCallSite() callSite {
	if len(runtime.callSites) == 0 {
		return callSite{}
	}
	return runtime.callSites[len(runtime.callSites)-1]
}

func (runtime *Runtime) currentFunctionName() string {
	if len(runtime.callStack) == 0 {
		return ""
	}
	return runtime.callStack[len(runtime.callStack)-1]
}

func (runtime *Runtime) debugModuleName(site callSite) string {
	if site.File != "" {
		base := filepath.Base(site.File)
		return strings.TrimSuffix(base, filepath.Ext(base))
	}
	current := runtime.currentFunctionName()
	if index := strings.Index(current, "."); index != -1 {
		return current[:index]
	}
	return current
}

func (runtime *Runtime) debugLocationString(site callSite) string {
	return fmt.Sprintf("File %q, line %d, characters %d-%d", site.File, site.Line, site.Column, site.Column+1)
}

func (runtime *Runtime) debugPositionTable(site callSite) Value {
	return TableValue(map[string]Value{
		"file":         StringValue(site.File),
		"line":         IntValue(site.Line),
		"start_column": IntValue(site.Column),
		"end_column":   IntValue(site.Column + 1),
	})
}

func stringData(value Value) (string, bool) {
	if value.Kind != ValueString && value.Kind != ValueChar {
		return "", false
	}
	return value.Data.(string), true
}

func stringList(value Value) ([]string, error) {
	if value.Kind != ValueList {
		return nil, fmt.Errorf("expected List[String]")
	}
	items := value.Data.([]Value)
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := stringData(item)
		if !ok {
			return nil, fmt.Errorf("expected List[String]")
		}
		result = append(result, text)
	}
	return result, nil
}

func listFromStrings(items []string) Value {
	values := make([]Value, 0, len(items))
	for _, item := range items {
		values = append(values, StringValue(item))
	}
	return Value{Kind: ValueList, Data: values}
}

func isBuildBackendName(value string) bool {
	switch value {
	case "WASM", "JS", "Standalone":
		return true
	default:
		return false
	}
}

func jsExports(source string) []string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`export\s+function\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`export\s+const\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`export\s+let\s+([A-Za-z_][A-Za-z0-9_]*)`),
		regexp.MustCompile(`export\s+var\s+([A-Za-z_][A-Za-z0-9_]*)`),
	}
	seen := map[string]bool{}
	var exports []string
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(source, -1) {
			if len(match) < 2 || seen[match[1]] {
				continue
			}
			seen[match[1]] = true
			exports = append(exports, match[1])
		}
	}
	return exports
}

func preferredMemoryRegion(value Value) MemoryRegion {
	if value.Kind != ValueObject {
		return ""
	}
	object := value.Data.(ObjectData)
	switch object.Type {
	case "Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return MemoryHeap
	default:
		return MemoryHeap
	}
}

func regionRuntimeStateKind(stmt parser.RegionStatement) string {
	if stmt.Temporary {
		return "temporary_region"
	}
	return "region"
}

func (runtime *Runtime) memoryRegionForType(typeName string) MemoryRegion {
	regionName := regionNameFromRuntimeArrayType(typeName)
	if regionName == "" {
		return MemoryStack
	}
	if region, ok := runtime.regions[regionName]; ok && region.Temporary {
		return MemoryTemporary
	}
	return MemoryStack
}

func (runtime *Runtime) callAliasFunction(name string, args []Value) (Value, error) {
	alias, ok := runtime.aliasFunctions[name]
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown alias function %q", name)}
	}
	fields := map[string]Value{}
	required := requiredRuntimeParamCount(alias.Params)
	if len(args) < required || len(args) > len(alias.Params) {
		return NullValue(), Error{Message: fmt.Sprintf("alias function %s expects %d to %d argument(s), got %d", name, required, len(alias.Params), len(args))}
	}
	for index, param := range alias.Params {
		var value Value
		if index < len(args) {
			value = args[index]
		} else if isDefaultAllocator(param.Default) {
			value = allocatorObject("HeapAllocator", nil)
		} else if param.Default.Node != nil {
			evaluated, err := runtime.evalExpression(param.Default.Node, runtime.global)
			if err != nil {
				return NullValue(), err
			}
			value = evaluated
		} else {
			value = zeroValue(param.Type)
		}
		fields[param.Name] = value
	}
	fields["__type"] = StringValue(name)
	fields["__hooks"] = IntValue(len(alias.Hooks))
	fields["__methods"] = IntValue(len(alias.Methods))
	traits, impls := aliasBodyMetadataCounts(alias.Body)
	fields["__traits"] = IntValue(traits)
	fields["__impls"] = IntValue(impls)
	fields["__struct"] = BoolValue(alias.Struct)
	jsonTags := map[string]string{}
	for _, tag := range alias.FieldTags {
		if tag.Kind == "json" {
			jsonTags[tag.Field] = tag.Name
		}
	}
	return Value{Kind: ValueObject, Data: ObjectData{Type: name, Struct: alias.Struct, Fields: fields, JSONTags: jsonTags}}, nil
}

func aliasBodyMetadataCounts(statements []parser.Statement) (int, int) {
	traits := 0
	impls := 0
	for _, stmt := range statements {
		switch stmt.(type) {
		case parser.TraitStatement:
			traits++
		case parser.ImplStatement:
			impls++
		}
	}
	return traits, impls
}

func isDefaultAllocator(expr parser.Expression) bool {
	if len(expr.Tokens) != 2 {
		return false
	}
	return expr.Tokens[0].Type == lexer.TokenDot && expr.Tokens[1].Literal == "DEFAULT"
}

func (runtime *Runtime) enumVariantValue(enumName string, variantName string) (Value, bool) {
	enum, ok := runtime.enums[enumName]
	if !ok {
		return NullValue(), false
	}
	for _, variant := range enum.Variants {
		if variant.Name == variantName {
			return Value{Kind: ValueEnum, Data: EnumData{Type: enumName, Variant: variantName, Ordinal: variant.Ordinal}}, true
		}
	}
	return NullValue(), false
}

func (runtime *Runtime) aliasMethodExists(typeName string, methodName string) bool {
	alias, ok := runtime.aliasFunctions[typeName]
	if !ok {
		return false
	}
	for _, method := range alias.Methods {
		if method.Name == methodName {
			return true
		}
	}
	return false
}

func (runtime *Runtime) extensionMethod(typeName string, methodName string) (parser.FunctionStatement, bool) {
	typeName = normalizeRuntimeType(typeName)
	if method, ok := runtime.extensions[typeName][methodName]; ok {
		return method, true
	}
	if base, _, ok := splitRuntimeGenericType(typeName); ok {
		if method, exists := runtime.extensions[base][methodName]; exists {
			return method, true
		}
	}
	return parser.FunctionStatement{}, false
}

func (runtime *Runtime) extensionMethodExists(typeName string, methodName string) bool {
	_, ok := runtime.extensionMethod(typeName, methodName)
	return ok
}

func (runtime *Runtime) callBoundMethod(method BoundMethodData, argNodes []parser.ExpressionNode, env *Environment) (Value, error) {
	if method.Name == "cast_as" {
		return runtime.callStructCast(method.Receiver, argNodes)
	}
	args := make([]Value, 0, len(argNodes))
	for _, arg := range argNodes {
		value, err := runtime.evalExpression(arg, env)
		if err != nil {
			return NullValue(), err
		}
		args = append(args, value)
	}
	if builtinProtocolMethodExists(method.Receiver, method.Name) {
		return runtime.callBuiltinProtocolMethod(method, args)
	}
	return runtime.callAliasMethodValues(method, args, env)
}

func (runtime *Runtime) callStructCast(receiver Value, argNodes []parser.ExpressionNode) (Value, error) {
	if receiver.Kind != ValueObject || !receiver.Data.(ObjectData).Struct {
		return NullValue(), Error{Message: fmt.Sprintf("cast_as requires a struct alias receiver, got %s", runtimeTypeName(receiver))}
	}
	if len(argNodes) != 1 {
		return NullValue(), Error{Message: fmt.Sprintf("cast_as expects exactly 1 target type, got %d", len(argNodes))}
	}
	target, ok := argNodes[0].(parser.IdentifierExpression)
	if !ok {
		return NullValue(), Error{Message: "cast_as target must be a type name"}
	}
	targetType := normalizeRuntimeType(target.Name)
	object := receiver.Data.(ObjectData)
	switch targetType {
	case object.Type:
		return cloneValue(receiver), nil
	case "Table":
		alias := runtime.aliasFunctions[object.Type]
		entries := make([]TableEntryData, 0, len(alias.Params))
		for _, param := range alias.Params {
			if value, exists := object.Fields[param.Name]; exists {
				entries = append(entries, TableEntryData{Key: StringValue(param.Name), Value: value})
			}
		}
		return TableValueFromEntries(entries), nil
	case "JSON":
		converted, err := runtimeValueToJSON(receiver)
		if err != nil {
			return NullValue(), err
		}
		return JSONValue(converted), nil
	case "String":
		encoded, err := runtimeValueJSONString(receiver)
		if err != nil {
			return NullValue(), err
		}
		return StringValue(encoded), nil
	}
	targetAlias, exists := runtime.aliasFunctions[targetType]
	if !exists || !targetAlias.Struct {
		return NullValue(), Error{Message: fmt.Sprintf("cast_as target %s must be Table, JSON, String, or a struct alias", targetType)}
	}
	args := make([]Value, 0, len(targetAlias.Params))
	for _, param := range targetAlias.Params {
		if value, found := object.Fields[param.Name]; found {
			if !valueMatchesType(value, param.Type) {
				return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s: field %q expects %s, got %s", object.Type, targetType, param.Name, param.Type, runtimeTypeName(value))}
			}
			args = append(args, cloneValue(value))
			continue
		}
		if isDefaultAllocator(param.Default) {
			args = append(args, allocatorObject("HeapAllocator", nil))
			continue
		}
		if param.Default.Node != nil {
			value, err := runtime.evalExpression(param.Default.Node, runtime.global)
			if err != nil {
				return NullValue(), err
			}
			args = append(args, value)
			continue
		}
		return NullValue(), Error{Message: fmt.Sprintf("cannot cast %s to %s: required field %q is missing", object.Type, targetType, param.Name)}
	}
	return runtime.callAliasFunction(targetType, args)
}

func (runtime *Runtime) callAliasMethodValues(method BoundMethodData, args []Value, env *Environment) (Value, error) {
	alias := runtime.aliasFunctions[method.Type]
	methods := alias.Methods
	if extension, ok := runtime.extensionMethod(method.Type, method.Name); ok {
		methods = append(methods, extension)
	}
	for _, fn := range methods {
		if fn.Name != method.Name {
			continue
		}
		if fn.Backend != "" && fn.Backend != runtime.backend {
			return NullValue(), Error{Message: fmt.Sprintf(
				"method %s.%s requires backend %s, but the active runtime backend is %s",
				method.Type, method.Name, fn.Backend, runtime.backend,
			)}
		}
		required := requiredRuntimeParamCount(fn.Params)
		if len(args) < required || len(args) > len(fn.Params) {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects %d to %d argument(s), got %d", method.Type, method.Name, required, len(fn.Params), len(args))}
		}
		methodEnv := NewEnvironment(env)
		if err := runtime.defineValue(methodEnv, "this", false, method.Type, method.Receiver); err != nil {
			return NullValue(), err
		}
		for index, param := range fn.Params {
			value := zeroValue(param.Type)
			if index < len(args) {
				value = args[index]
			} else if param.Default.Node != nil {
				evaluated, err := runtime.evalExpression(param.Default.Node, methodEnv)
				if err != nil {
					return NullValue(), err
				}
				value = evaluated
			}
			if !valueMatchesType(value, param.Type) {
				return NullValue(), Error{Message: fmt.Sprintf("method %s.%s argument %d expects %s, got %s", method.Type, method.Name, index+1, param.Type, runtimeTypeName(value))}
			}
			if err := runtime.defineValue(methodEnv, param.Name, param.Mutable, param.Type, value); err != nil {
				return NullValue(), err
			}
		}
		signal, err := runtime.executeBlock(fn.Body, methodEnv, false)
		if err != nil {
			return NullValue(), err
		}
		if signal.kind == signalReturn {
			return signal.value, nil
		}
		return NullValue(), nil
	}
	return NullValue(), Error{Message: fmt.Sprintf("unknown method %s.%s", method.Type, method.Name)}
}

func builtinProtocolField(value Value, field string) (Value, bool) {
	if value.Kind == ValueAtom && field == "name" {
		return StringValue(value.Data.(string)), true
	}
	switch field {
	case "count":
		length, err := valueLen(value)
		if err != nil {
			return NullValue(), false
		}
		return IntValue(length), true
	case "kind":
		if value.Kind != ValueJSON {
			return NullValue(), false
		}
		kind, err := jsonValueKind(value)
		if err != nil {
			return NullValue(), false
		}
		return StringValue(kind), true
	default:
		return NullValue(), false
	}
}

func builtinProtocolMethodExists(value Value, method string) bool {
	if pipelineMethodName(method) {
		switch value.Kind {
		case ValueList, ValueSet, ValueString, ValueInt, ValueMap, ValueTable, ValueIterator:
			return true
		}
	}
	switch value.Kind {
	case ValueString, ValueChar:
		return method == "uppercase" || method == "lowercase"
	case ValueInt:
		return method == "times"
	case ValueObject:
		switch value.Data.(ObjectData).Type {
		case "File":
			switch method {
			case "read", "read_lines", "write", "append", "exists", "size", "create", "remove":
				return true
			}
		case "OS":
			switch method {
			case "current_dir", "change_dir", "temp_dir", "home_dir", "hostname", "process_id",
				"get_env", "set_env", "unset_env", "environment", "execute":
				return true
			}
		}
	default:
		return false
	}
	return false
}

func (runtime *Runtime) callBuiltinProtocolMethod(method BoundMethodData, args []Value) (Value, error) {
	if isObjectType(method.Receiver, "File") {
		return runtime.callFileBuiltin("file_"+method.Name, append([]Value{method.Receiver}, args...))
	}
	if isObjectType(method.Receiver, "OS") {
		return runtime.callOSBuiltin("os_"+method.Name, append([]Value{method.Receiver}, args...))
	}
	if pipelineMethodName(method.Name) {
		return runtime.callPipelineMethod(method, args)
	}
	switch method.Name {
	case "uppercase":
		if len(args) != 0 {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects 0 argument(s), got %d", method.Type, method.Name, len(args))}
		}
		switch method.Receiver.Kind {
		case ValueString:
			return StringValue(strings.ToUpper(method.Receiver.Data.(string))), nil
		case ValueChar:
			return CharValue(strings.ToUpper(method.Receiver.Data.(string))), nil
		}
	case "lowercase":
		if len(args) != 0 {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects 0 argument(s), got %d", method.Type, method.Name, len(args))}
		}
		switch method.Receiver.Kind {
		case ValueString:
			return StringValue(strings.ToLower(method.Receiver.Data.(string))), nil
		case ValueChar:
			return CharValue(strings.ToLower(method.Receiver.Data.(string))), nil
		}
	case "times":
		if len(args) != 1 {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects 1 argument(s), got %d", method.Type, method.Name, len(args))}
		}
		if method.Receiver.Kind != ValueInt || args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: fmt.Sprintf("method %s.%s expects Function[Int,T]", method.Type, method.Name)}
		}
		count := method.Receiver.Data.(int)
		if count < 0 {
			return NullValue(), Error{Message: "Int.times expects a non-negative receiver"}
		}
		result := NullValue()
		for index := 0; index < count; index++ {
			value, err := runtime.callFunction(args[0].Data.(string), []Value{IntValue(index)})
			if err != nil {
				return NullValue(), err
			}
			result = value
		}
		return result, nil
	}
	return NullValue(), Error{Message: fmt.Sprintf("unknown method %s.%s", method.Type, method.Name)}
}

func pipelineMethodName(name string) bool {
	switch name {
	case "iter", "filter", "map", "skip", "limit", "collect", "sort", "fold", "first", "any", "all", "for_each":
		return true
	default:
		return false
	}
}

func (runtime *Runtime) callPipelineMethod(method BoundMethodData, args []Value) (Value, error) {
	switch method.Name {
	case "iter":
		if len(args) != 0 {
			return NullValue(), Error{Message: "iter() expects no arguments"}
		}
		return runtime.iteratorFromValue(method.Receiver)
	case "filter", "map":
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects one Function callback", method.Name)}
		}
		return runtime.appendIteratorStage(method.Receiver, IteratorStage{Kind: method.Name, Function: args[0].Data.(string)})
	case "skip", "limit":
		if len(args) != 1 || args[0].Kind != ValueInt {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects one non-negative Int", method.Name)}
		}
		count := args[0].Data.(int)
		if count < 0 {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects a non-negative Int", method.Name)}
		}
		stageKind := method.Name
		if stageKind == "limit" {
			stageKind = iteratorStageTake
		}
		return runtime.appendIteratorStage(method.Receiver, IteratorStage{Kind: stageKind, Count: count})
	}
	iterator, err := runtime.iteratorFromValue(method.Receiver)
	if err != nil {
		return NullValue(), err
	}
	data := iterator.Data.(*IteratorData)
	switch method.Name {
	case "collect":
		if len(args) != 0 {
			return NullValue(), Error{Message: "collect expects no arguments"}
		}
		items, err := runtime.collectIterator(data)
		if err != nil {
			return NullValue(), err
		}
		return Value{Kind: ValueList, Data: items}, nil
	case "sort":
		if len(args) != 0 {
			return NullValue(), Error{Message: "sort expects no arguments"}
		}
		items, err := runtime.sortIterator(data)
		if err != nil {
			return NullValue(), err
		}
		return Value{Kind: ValueList, Data: items}, nil
	case "first":
		if len(args) != 0 {
			return NullValue(), Error{Message: "first expects no arguments"}
		}
		value, ok, err := runtime.nextIterator(data)
		if err != nil {
			return NullValue(), err
		}
		if !ok {
			return OptionNoneValue(), nil
		}
		return OptionSomeValue(value), nil
	case "fold":
		if len(args) != 2 || args[1].Kind != ValueFunction {
			return NullValue(), Error{Message: "fold expects an initial value and Function[U,T,U]"}
		}
		result := cloneValue(args[0])
		for {
			value, ok, err := runtime.nextIterator(data)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return result, nil
			}
			result, err = runtime.callFunction(args[1].Data.(string), []Value{result, value})
			if err != nil {
				return NullValue(), err
			}
		}
	case "any", "all":
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: fmt.Sprintf("%s expects Function[T,Bool]", method.Name)}
		}
		for {
			value, ok, err := runtime.nextIterator(data)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return BoolValue(method.Name == "all"), nil
			}
			result, err := runtime.callFunction(args[0].Data.(string), []Value{value})
			if err != nil {
				return NullValue(), err
			}
			if result.Kind != ValueBool {
				return NullValue(), Error{Message: fmt.Sprintf("%s callback must return Bool", method.Name)}
			}
			matched := result.Data.(bool)
			if method.Name == "any" && matched {
				return BoolValue(true), nil
			}
			if method.Name == "all" && !matched {
				return BoolValue(false), nil
			}
		}
	case "for_each":
		if len(args) != 1 || args[0].Kind != ValueFunction {
			return NullValue(), Error{Message: "for_each expects Function[T,U]"}
		}
		for {
			value, ok, err := runtime.nextIterator(data)
			if err != nil {
				return NullValue(), err
			}
			if !ok {
				return NullValue(), nil
			}
			if _, err := runtime.callFunction(args[0].Data.(string), []Value{value}); err != nil {
				return NullValue(), err
			}
		}
	default:
		return NullValue(), Error{Message: fmt.Sprintf("unknown pipeline method %s", method.Name)}
	}
}

func (runtime *Runtime) regionArrayCapacity(typeName string) (int, bool) {
	regionName := regionNameFromRuntimeArrayType(typeName)
	if regionName == "" {
		return 0, false
	}
	region, ok := runtime.regions[regionName]
	if !ok || region.Count.Kind != ValueInt {
		return 0, false
	}
	return region.Count.Data.(int), true
}

func (runtime *Runtime) callFunctionGroup(name string, args []Value) (Value, error) {
	name = runtime.resolveAliasPath(name)
	members, ok := runtime.groups[name]
	if !ok {
		return NullValue(), Error{Message: fmt.Sprintf("unknown function_group %q", name)}
	}

	var matches []string
	for _, member := range members {
		resolvedMember, err := runtime.resolveFunctionName(member)
		if err != nil {
			return NullValue(), err
		}
		if resolvedMember == "" {
			return NullValue(), Error{Message: fmt.Sprintf("function_group %s references unknown function %q", name, member)}
		}
		fn := runtime.functions[resolvedMember]
		required := requiredRuntimeParamCount(fn.Params)
		if len(args) < required || len(args) > len(fn.Params) {
			continue
		}
		if runtime.argumentsMatchParameters(args, fn.Params) {
			matches = append(matches, resolvedMember)
		}
	}

	if len(matches) == 0 {
		return NullValue(), Error{Message: fmt.Sprintf("no function_group %s overload matches %d argument(s)", name, len(args))}
	}
	if len(matches) > 1 {
		return NullValue(), Error{Message: fmt.Sprintf("ambiguous function_group %s call matches %s", name, strings.Join(matches, ", "))}
	}
	return runtime.callFunction(matches[0], args)
}

func (runtime *Runtime) argumentsMatchParameters(args []Value, params []parser.Parameter) bool {
	for index, arg := range args {
		if index >= len(params) || !valueMatchesType(arg, params[index].Type) {
			return false
		}
	}
	return true
}

func requiredRuntimeParamCount(params []parser.Parameter) int {
	count := len(params)
	for count > 0 && params[count-1].Default.Node != nil {
		count--
	}
	return count
}

func (runtime *Runtime) resolveFunctionName(name string) (string, error) {
	name = runtime.resolveAliasPath(name)
	if _, ok := runtime.functions[name]; ok {
		return name, nil
	}
	if resolved, ok, err := runtime.resolveGlobalFunctionName(name); ok || err != nil {
		if err != nil {
			return "", err
		}
		return resolved, nil
	}
	var matches []string
	for functionName := range runtime.functions {
		if strings.HasSuffix(functionName, "."+name) {
			matches = append(matches, functionName)
		}
	}
	if len(matches) > 1 {
		return "", Error{Message: fmt.Sprintf("ambiguous function %q matches %s", name, strings.Join(matches, ", "))}
	}
	if len(matches) == 1 {
		return matches[0], nil
	}
	return "", nil
}

func (runtime *Runtime) resolveGlobalFunctionName(name string) (string, bool, error) {
	if strings.Contains(name, ".") {
		return "", false, nil
	}
	matches := runtime.globalFunctions[name]
	if len(matches) == 0 {
		return "", false, nil
	}
	if len(matches) > 1 {
		return "", false, Error{Message: fmt.Sprintf("ambiguous global namespace function %q matches %s", name, strings.Join(matches, ", "))}
	}
	return matches[0], true, nil
}

func unqualifiedRuntimeFunctionName(name string) string {
	if index := strings.LastIndex(name, "."); index != -1 {
		return name[index+1:]
	}
	return name
}

func (runtime *Runtime) isLazyFunction(name string) bool {
	resolvedName, err := runtime.resolveFunctionName(name)
	if err != nil || resolvedName == "" {
		return false
	}
	return runtime.functions[resolvedName].Lazy
}

func (runtime *Runtime) resolveAliasPath(name string) string {
	name = strings.ReplaceAll(strings.TrimSpace(name), "::", ".")
	seen := map[string]bool{}
	for !seen[name] {
		seen[name] = true
		alias, target, ok := longestRuntimeAliasPath(name, runtime.aliases)
		if !ok {
			break
		}
		name = target + strings.TrimPrefix(name, alias)
	}
	return name
}

func longestRuntimeAliasPath(name string, aliases map[string]string) (string, string, bool) {
	best := ""
	for alias := range aliases {
		if name != alias && !strings.HasPrefix(name, alias+".") {
			continue
		}
		if len(alias) > len(best) {
			best = alias
		}
	}
	if best == "" {
		return "", "", false
	}
	return best, aliases[best], true
}

func isBuiltinFunction(name string) bool {
	switch name {
	case "print", "format", "printf", "input", "read_int", "read_ints", "print_ints", "interval_walk_max_scores", "len", "range",
		"option_map", "option_unwrap_or", "option_and_then",
		"result_map", "result_map_err", "result_unwrap_or", "result_and_then",
		"Some", "None", "Ok", "Err", "Result", "Complex", "SIMD", "Set", "JSON", "Parsable", "File", "OS", "Atom",
		"Table", "iter", "next", "coroutine", "resume", "spawn", "join", "thread_status",
		"file_read", "file_read_lines", "file_write", "file_append", "file_exists", "file_size", "file_create", "file_remove",
		"os_current_dir", "os_change_dir", "os_temp_dir", "os_home_dir", "os_hostname", "os_process_id",
		"os_get_env", "os_set_env", "os_unset_env", "os_environment", "os_execute",
		"raylib_init_window", "raylib_close_window", "raylib_window_should_close", "raylib_is_window_ready",
		"raylib_set_target_fps", "raylib_get_fps", "raylib_get_frame_time", "raylib_begin_drawing", "raylib_end_drawing",
		"raylib_clear_background", "raylib_draw_text", "raylib_draw_rectangle", "raylib_draw_circle",
		"raylib_is_key_pressed", "raylib_is_key_down", "raylib_get_screen_width", "raylib_get_screen_height",
		"raylib_set_window_title", "raylib_set_window_size", "raylib_set_window_position", "raylib_toggle_fullscreen",
		"raylib_maximize_window", "raylib_minimize_window", "raylib_restore_window", "raylib_is_window_fullscreen",
		"raylib_is_window_hidden", "raylib_is_window_minimized", "raylib_is_window_maximized", "raylib_is_window_focused",
		"raylib_get_time", "raylib_set_exit_key", "raylib_is_key_pressed_repeat", "raylib_is_key_released", "raylib_is_key_up",
		"raylib_get_key_pressed", "raylib_get_char_pressed", "raylib_is_mouse_button_pressed", "raylib_is_mouse_button_down",
		"raylib_is_mouse_button_released", "raylib_get_mouse_x", "raylib_get_mouse_y", "raylib_set_mouse_position",
		"raylib_get_mouse_wheel_move", "raylib_draw_pixel", "raylib_draw_line", "raylib_draw_rectangle_lines",
		"raylib_draw_circle_lines", "raylib_draw_ellipse", "raylib_measure_text", "raylib_take_screenshot",
		"raylib_get_random_value", "raylib_check_collision_recs", "raylib_check_collision_circles", "raylib_check_collision_point_rec",
		"json_parse", "json_decode", "json_encode", "json_stringify", "json_get", "json_kind", "json_string", "json_int", "json_float", "json_bool", "json_is_null",
		"table_has", "has_key", "set_has", "table_delete", "table_keys", "table_values", "table_entries", "table_sequence_count", "table_set_fallback",
		"Atomic", "atomic_load", "atomic_store", "atomic_add",
		"Program", "BuildSystem", "WorkSpace", "workspace_backend", "workspace_files", "workspace_manifest",
		"parsable_source", "parsable_ast", "parsable_args", "parsable_runtime_info", "parsable_workspace", "parsable_with_source", "parsable_replace", "parsable_append", "parsable_begin_polling", "parsable_poll_message", "parsable_intercept_message", "get_args_from_parsable", "macro_context", "macro_expand",
		"runtime_debug_loc", "runtime_debug_file", "runtime_debug_line", "runtime_debug_module", "runtime_debug_pos", "runtime_debug_function",
		"runtime_debug_loc_of", "runtime_debug_line_of", "runtime_debug_pos_of",
		"runtime.debug.__LOC__", "runtime.debug.__FILE__", "runtime.debug.__LINE__", "runtime.debug.__MODULE__", "runtime.debug.__POS__", "runtime.debug.__FUNCTION__",
		"runtime.debug.__LOC_OF__", "runtime.debug.__LINE_OF__", "runtime.debug.__POS_OF__",
		"debug", "debug_type", "debug_stack", "debug_state", "breakpoint", "js_import", "js_source", "js_exports", "js_call",
		"Box", "Ref", "RefMut", "RefCell", "HeapAllocator", "RegionAllocator", "BumpAllocator", "ArenaAllocator":
		return true
	default:
		return false
	}
}
