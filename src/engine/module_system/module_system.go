package modulesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kLang/src/diagnostic"
	"kLang/src/engine/file"
	"kLang/src/grua"
	"kLang/src/parser"
	embeddedstdlib "kLang/stdlib"
)

const DefaultStdlibRoot = "stdlib"

type ImportKind string

const (
	ImportLocal  ImportKind = "local"
	ImportStdlib ImportKind = "stdlib"
)

type Module struct {
	Name       string
	Path       string
	Kind       ImportKind
	ImportedBy string
}

type Error struct {
	Code      string
	File      string
	Line      int
	Column    int
	EndLine   int
	EndColumn int
	Message   string
	Rule      string
	Hint      string
}

func moduleError(file string, line int, column int, message string) Error {
	return Error{
		Code: diagnostic.CodeImportResolution, File: file, Line: line, Column: column,
		EndLine: line, EndColumn: column, Message: message,
		Rule: "import resolution",
		Hint: "Check that the imported module exists, is spelled correctly, and is reachable from this workspace.",
	}
}

type Report struct {
	Modules []Module
	Errors  []Error
}

func (report Report) Passed() bool {
	return len(report.Errors) == 0
}

type CacheStats struct {
	ExistsEntries  int
	ProgramEntries int
	ImportEntries  int
}

type Resolver struct {
	StdlibRoot    string
	DisableStdlib bool
	exists        map[string]bool
	programs      map[string]file.Program
	imports       map[string][]parser.ImportStatement
	metadata      map[string]sourceMetadata
}

type resolutionState struct {
	visited   map[string]bool
	resolving map[string]bool
	reported  map[string]bool
}

type sourceMetadata struct {
	Imports          []parser.ImportStatement
	ModuleDisabled   bool
	CallEntireModule bool
	GlobalFunctions  map[string]bool
}

func NewResolver(stdlibRoot string) *Resolver {
	if stdlibRoot == "" {
		stdlibRoot = DefaultStdlibRoot
	}

	return &Resolver{
		StdlibRoot: stdlibRoot,
		exists:     map[string]bool{},
		programs:   map[string]file.Program{},
		imports:    map[string][]parser.ImportStatement{},
		metadata:   map[string]sourceMetadata{},
	}
}

func ResolveProgram(program file.Program) (file.Program, Report) {
	return NewResolver(DefaultStdlibRoot).ResolveProgram(program)
}

func (resolver *Resolver) Stats() CacheStats {
	return CacheStats{
		ExistsEntries:  len(resolver.exists),
		ProgramEntries: len(resolver.programs),
		ImportEntries:  len(resolver.imports),
	}
}

func (resolver *Resolver) ResolveProgram(program file.Program) (file.Program, Report) {
	resolved := program
	report := Report{}
	state := &resolutionState{
		visited:   map[string]bool{},
		resolving: map[string]bool{},
		reported:  map[string]bool{},
	}

	for _, source := range program.Files {
		resolver.markVisited(state, source.Path)
	}

	for _, source := range program.Files {
		resolver.resolveSource(state, &resolved, &report, source)
	}
	resolver.resolveStdlibGlobalNamespaces(state, &resolved, &report)

	return resolved, report
}

func (resolver *Resolver) resolveStdlibGlobalNamespaces(state *resolutionState, program *file.Program, report *Report) {
	if resolver.DisableStdlib {
		return
	}
	paths, err := resolver.globalStdlibCandidates()
	if err != nil {
		report.Errors = append(report.Errors, moduleError("", 0, 1, err.Error()))
		return
	}
	for _, path := range paths {
		if state.visited[resolver.pathKey(path)] {
			continue
		}
		imported, err := resolver.loadProgram(path)
		if err != nil {
			report.Errors = append(report.Errors, moduleError(path, 0, 1, err.Error()))
			continue
		}
		if resolver.moduleDisabled(imported) {
			continue
		}
		selected := resolver.globalNamespaceFunctionFilter(imported)
		if len(selected) == 0 {
			continue
		}
		if resolver.mergeVisitedGlobalNamespaceFilter(program, imported, selected) {
			continue
		}
		for _, importedSource := range imported.Files {
			key := resolver.pathKey(importedSource.Path)
			if state.visited[key] {
				continue
			}
			importedSource.ModuleFunctionFilter = cloneBoolMap(selected)
			state.visited[key] = true
			program.Files = append(program.Files, importedSource)
			resolver.resolveSource(state, program, report, importedSource)
		}
	}
}

func programUsesOnlyLanguage(program file.Program, language string) bool {
	if len(program.Files) == 0 {
		return false
	}
	for _, source := range program.Files {
		if source.Language != language {
			return false
		}
	}
	return true
}

func (resolver *Resolver) mergeVisitedGlobalNamespaceFilter(program *file.Program, imported file.Program, selected map[string]bool) bool {
	merged := false
	for _, importedSource := range imported.Files {
		importedKey := resolver.pathKey(importedSource.Path)
		for index := range program.Files {
			if resolver.pathKey(program.Files[index].Path) != importedKey {
				continue
			}
			if program.Files[index].ModuleFunctionFilter != nil {
				for name := range selected {
					program.Files[index].ModuleFunctionFilter[name] = true
				}
			}
			merged = true
		}
	}
	return merged
}

func (resolver *Resolver) globalStdlibCandidates() ([]string, error) {
	if !resolver.pathExists(resolver.StdlibRoot) {
		return nil, nil
	}
	var paths []string
	entries, err := os.ReadDir(resolver.StdlibRoot)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		path := filepath.Join(resolver.StdlibRoot, entry.Name())
		if filepath.Ext(path) != file.KlangExtension {
			continue
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if strings.Contains(string(contents), "global namespace") {
			paths = append(paths, filepath.Clean(path))
		}
	}
	sort.Strings(paths)
	return paths, nil
}

func (resolver *Resolver) globalNamespaceFunctionFilter(program file.Program) map[string]bool {
	selected := map[string]bool{}
	for _, source := range program.Files {
		metadata, errors := resolver.metadataFor(source)
		if len(errors) != 0 {
			continue
		}
		for name := range metadata.GlobalFunctions {
			selected[name] = true
		}
	}
	return selected
}

func (resolver *Resolver) resolveSource(state *resolutionState, program *file.Program, report *Report, source file.SourceFile) {
	sourceKey := resolver.pathKey(source.Path)
	state.resolving[sourceKey] = true
	defer delete(state.resolving, sourceKey)

	imports, errors := resolver.importsFor(source, *program)
	for _, parseError := range errors {
		current := moduleError(source.Path, parseError.Line, parseError.Column, parseError.Message)
		current.EndLine = parseError.EndLine
		current.EndColumn = parseError.EndColumn
		report.Errors = append(report.Errors, current)
	}
	if len(errors) != 0 {
		return
	}

	for _, importStmt := range imports {
		imported, module, err := resolver.resolveImport(source.Path, importStmt.Path)
		if err != nil {
			current := moduleError(source.Path, importStmt.Pos.Line, importStmt.Pos.Column, err.Error())
			current.EndLine = importStmt.Pos.EndLine
			current.EndColumn = importStmt.Pos.EndColumn
			report.Errors = append(report.Errors, current)
			continue
		}
		if source.Language == file.LanguageGrua {
			if module.Kind == ImportStdlib && !grua.AllowsModule(importStmt.Path) {
				report.Errors = append(report.Errors, moduleError(
					source.Path, importStmt.Pos.Line, importStmt.Pos.Column,
					fmt.Sprintf("Grua only exposes the basic, file, io, and repl modules; %q is unavailable", importStmt.Path),
				))
				continue
			}
			if module.Kind == ImportLocal && !programUsesOnlyLanguage(imported, file.LanguageGrua) {
				report.Errors = append(report.Errors, moduleError(
					source.Path, importStmt.Pos.Line, importStmt.Pos.Column,
					fmt.Sprintf("Grua source cannot import non-Grua local module %q", importStmt.Path),
				))
				continue
			}
		}

		if resolver.moduleDisabled(imported) {
			report.Errors = append(report.Errors, moduleError(
				source.Path, importStmt.Pos.Line, importStmt.Pos.Column,
				fmt.Sprintf("module %q is disabled", importStmt.Path),
			))
			continue
		}

		if module.Kind == ImportStdlib && !importStmt.CallEntireModule {
			imported = resolver.applyStdlibFunctionFilter(imported, importStmt.Path, source)
		}

		if resolver.moduleIsResolving(state, imported) {
			report.Errors = append(report.Errors, moduleError(
				source.Path, importStmt.Pos.Line, importStmt.Pos.Column,
				fmt.Sprintf("import cycle detected for %q", importStmt.Path),
			))
			continue
		}

		resolver.addModuleReport(state, report, module)
		for _, importedSource := range imported.Files {
			if resolver.isVisited(state, importedSource.Path) {
				resolver.mergeVisitedSourceFilter(program, importedSource)
				continue
			}
			resolver.markVisited(state, importedSource.Path)
			program.Files = append(program.Files, importedSource)
			resolver.resolveSource(state, program, report, importedSource)
		}
	}
}

func (resolver *Resolver) mergeVisitedSourceFilter(program *file.Program, importedSource file.SourceFile) {
	if importedSource.ModuleFunctionFilter == nil {
		return
	}
	importedKey := resolver.pathKey(importedSource.Path)
	for index := range program.Files {
		if resolver.pathKey(program.Files[index].Path) != importedKey {
			continue
		}
		if program.Files[index].ModuleFunctionFilter == nil {
			program.Files[index].ModuleFunctionFilter = cloneBoolMap(importedSource.ModuleFunctionFilter)
			return
		}
		for name := range importedSource.ModuleFunctionFilter {
			program.Files[index].ModuleFunctionFilter[name] = true
		}
		return
	}
}

func (resolver *Resolver) resolveImport(importedBy string, importPath string) (file.Program, Module, error) {
	candidates := resolver.localCandidates(importedBy, importPath)
	for _, candidate := range candidates {
		if resolver.pathExists(candidate) {
			program, err := resolver.loadProgram(candidate)
			if err != nil {
				return file.Program{}, Module{}, err
			}
			kind := ImportLocal
			if resolver.pathInsideStdlib(candidate) {
				kind = ImportStdlib
			}
			return program, Module{
				Name:       importPath,
				Path:       candidate,
				Kind:       kind,
				ImportedBy: importedBy,
			}, nil
		}
	}

	if !resolver.DisableStdlib {
		candidates = resolver.stdlibCandidates(importedBy, importPath)
		for _, candidate := range candidates {
			if resolver.pathExists(candidate) {
				program, err := resolver.loadProgram(candidate)
				if err != nil {
					return file.Program{}, Module{}, err
				}
				return program, Module{
					Name:       importPath,
					Path:       candidate,
					Kind:       ImportStdlib,
					ImportedBy: importedBy,
				}, nil
			}
		}
		if lines, ok := embeddedstdlib.Source(importPath); ok {
			name := strings.TrimSuffix(filepath.Base(importPath), file.KlangExtension)
			virtualPath := filepath.Join("<embedded-stdlib>", name+file.KlangExtension)
			return file.Program{
					Name:       name,
					Root:       "<embedded-stdlib>",
					EntryPoint: virtualPath,
					Files: []file.SourceFile{{
						Path:          virtualPath,
						Lines:         lines,
						OriginalLines: append([]string(nil), lines...),
						Language:      file.LanguageKlang,
					}},
				}, Module{
					Name:       importPath,
					Path:       virtualPath,
					Kind:       ImportStdlib,
					ImportedBy: importedBy,
				}, nil
		}
	}

	if resolver.DisableStdlib {
		return file.Program{}, Module{}, fmt.Errorf("could not resolve import %q in raw-lang mode", importPath)
	}
	return file.Program{}, Module{}, fmt.Errorf("could not resolve import %q", importPath)
}

func (resolver *Resolver) localCandidates(importedBy string, importPath string) []string {
	baseDir := filepath.Dir(importedBy)
	candidates := []string{filepath.Clean(filepath.Join(baseDir, importPath))}
	if filepath.Ext(importPath) == "" {
		if filepath.Ext(importedBy) == grua.Extension {
			return append(candidates, filepath.Clean(filepath.Join(baseDir, importPath+grua.Extension)))
		}
		candidates = append(
			candidates,
			filepath.Clean(filepath.Join(baseDir, importPath+file.KlangExtension)),
			filepath.Clean(filepath.Join(baseDir, importPath+grua.Extension)),
		)
	}
	return candidates
}

func (resolver *Resolver) stdlibCandidates(importedBy string, importPath string) []string {
	if filepath.Ext(importedBy) == grua.Extension {
		candidates := []string{filepath.Clean(filepath.Join(resolver.StdlibRoot, "grua", importPath))}
		if filepath.Ext(importPath) == "" {
			candidates = append(candidates, filepath.Clean(filepath.Join(resolver.StdlibRoot, "grua", importPath+grua.Extension)))
		}
		return candidates
	}
	candidates := []string{filepath.Clean(filepath.Join(resolver.StdlibRoot, importPath))}
	if filepath.Ext(importPath) == "" {
		candidates = append(candidates, filepath.Clean(filepath.Join(resolver.StdlibRoot, importPath+file.KlangExtension)))
	}
	return candidates
}

func (resolver *Resolver) isVisited(state *resolutionState, path string) bool {
	key := resolver.pathKey(path)
	return state.visited[key]
}

func (resolver *Resolver) markVisited(state *resolutionState, path string) {
	key := resolver.pathKey(path)
	state.visited[key] = true
}

func (resolver *Resolver) pathKey(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func (resolver *Resolver) pathInsideStdlib(path string) bool {
	if resolver.StdlibRoot == "" {
		return false
	}
	pathKey := resolver.pathKey(path)
	stdlibKey := resolver.pathKey(resolver.StdlibRoot)
	if pathKey == stdlibKey {
		return true
	}
	relative, err := filepath.Rel(stdlibKey, pathKey)
	if err != nil {
		return false
	}
	return relative != "." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && relative != ".."
}

func (resolver *Resolver) moduleIsResolving(state *resolutionState, program file.Program) bool {
	for _, source := range program.Files {
		if state.resolving[resolver.pathKey(source.Path)] {
			return true
		}
	}
	return false
}

func (resolver *Resolver) addModuleReport(state *resolutionState, report *Report, module Module) {
	key := resolver.pathKey(module.Path)
	if state.reported[key] {
		return
	}
	state.reported[key] = true
	report.Modules = append(report.Modules, module)
}

func (resolver *Resolver) importsFor(source file.SourceFile, program file.Program) ([]parser.ImportStatement, []parser.Error) {
	metadata, errors := resolver.metadataFor(source)
	if len(errors) != 0 {
		return nil, errors
	}
	return resolver.importsWithInferredModuleCalls(source, program, metadata), nil
}

func (resolver *Resolver) importsWithInferredModuleCalls(source file.SourceFile, program file.Program, metadata sourceMetadata) []parser.ImportStatement {
	imports := append([]parser.ImportStatement(nil), metadata.Imports...)
	seen := map[string]bool{}
	for _, importStmt := range imports {
		seen[importStmt.Path] = true
	}
	for _, path := range resolver.inferredModuleImports(source, program) {
		if seen[path] {
			continue
		}
		seen[path] = true
		imports = append(imports, parser.ImportStatement{
			Pos:              parser.Position{Line: 1, Column: 1},
			Path:             path,
			CallEntireModule: metadata.CallEntireModule,
		})
	}
	return imports
}

func (resolver *Resolver) inferredModuleImports(source file.SourceFile, program file.Program) []string {
	parsed := parser.ParseSource(source)
	if len(parsed.Errors) != 0 {
		return nil
	}
	candidates := collectCalledModuleNames(parsed.Program.Statements)
	if len(candidates) == 0 {
		return nil
	}
	var imports []string
	for _, moduleName := range candidates {
		if moduleName == "" {
			continue
		}
		imported, _, err := resolver.resolveImport(source.Path, moduleName)
		if err != nil {
			continue
		}
		if programContainsSource(imported, source.Path, resolver) {
			continue
		}
		if programContainsAnySource(imported, program, resolver) {
			continue
		}
		imports = append(imports, moduleName)
	}
	sort.Strings(imports)
	return imports
}

func programContainsSource(program file.Program, path string, resolver *Resolver) bool {
	key := resolver.pathKey(path)
	for _, source := range program.Files {
		if samePathKey(resolver.pathKey(source.Path), key) {
			return true
		}
	}
	return false
}

func programContainsAnySource(imported file.Program, program file.Program, resolver *Resolver) bool {
	projectSources := map[string]bool{}
	for _, source := range program.Files {
		projectSources[resolver.pathKey(source.Path)] = true
	}
	for _, source := range imported.Files {
		sourceKey := resolver.pathKey(source.Path)
		if projectSources[sourceKey] {
			return true
		}
		for projectKey := range projectSources {
			if samePathKey(projectKey, sourceKey) {
				return true
			}
		}
	}
	return false
}

func samePathKey(left string, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	return left == right || strings.EqualFold(left, right)
}

func (resolver *Resolver) metadataFor(source file.SourceFile) (sourceMetadata, []parser.Error) {
	key := resolver.pathKey(source.Path)
	if metadata, ok := resolver.metadata[key]; ok {
		return metadata, nil
	}
	parsed := parser.ParseSource(source)
	if len(parsed.Errors) != 0 {
		return sourceMetadata{}, parsed.Errors
	}

	metadata := collectSourceMetadata(parsed.Program.Statements)
	resolver.imports[key] = metadata.Imports
	resolver.metadata[key] = metadata
	return metadata, nil
}

func (resolver *Resolver) loadProgram(path string) (file.Program, error) {
	key := resolver.pathKey(path)
	if program, ok := resolver.programs[key]; ok {
		return program, nil
	}

	program, err := file.LoadModuleProgram(path)
	if err != nil {
		return file.Program{}, err
	}
	resolver.programs[key] = program
	return program, nil
}

func collectImports(statements []parser.Statement) []parser.ImportStatement {
	return collectSourceMetadata(statements).Imports
}

func collectSourceMetadata(statements []parser.Statement) sourceMetadata {
	metadata := sourceMetadata{GlobalFunctions: map[string]bool{}}
	var imports []parser.ImportStatement
	collectSourceMetadataInto(statements, "", false, &metadata, &imports)
	for index := range imports {
		imports[index].CallEntireModule = metadata.CallEntireModule
	}
	metadata.Imports = imports
	return metadata
}

func collectSourceMetadataInto(statements []parser.Statement, namespace string, global bool, metadata *sourceMetadata, imports *[]parser.ImportStatement) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ImportStatement:
			current.CallEntireModule = metadata.CallEntireModule
			*imports = append(*imports, current)
		case parser.ModuleDirectiveStatement:
			switch current.Name {
			case "module":
				if current.Options["disabled"] {
					metadata.ModuleDisabled = true
				}
			case "module_caller":
				if current.Options["call_entire_module"] {
					metadata.CallEntireModule = true
				}
			}
		case parser.NamespaceStatement:
			collectSourceMetadataInto(current.Body, namespace+current.Name+".", global || current.Global, metadata, imports)
		case parser.FunctionStatement:
			if global {
				metadata.GlobalFunctions[namespace+current.Name] = true
			}
			collectSourceMetadataInto(current.Body, namespace, false, metadata, imports)
		case parser.AliasFunctionStatement:
			collectSourceMetadataInto(current.Body, namespace, global, metadata, imports)
			for _, method := range current.Methods {
				collectSourceMetadataInto(method.Body, namespace, global, metadata, imports)
			}
		case parser.ExtensionStatement:
			for _, method := range current.Methods {
				collectSourceMetadataInto(method.Body, namespace, global, metadata, imports)
			}
		case parser.ImplStatement:
			for _, method := range current.Methods {
				collectSourceMetadataInto(method.Body, namespace, global, metadata, imports)
			}
		case parser.IfStatement:
			collectSourceMetadataInto(current.Consequence, namespace, global, metadata, imports)
			collectSourceMetadataInto(current.Alternative, namespace, global, metadata, imports)
			if current.ElseIf != nil {
				collectSourceMetadataInto([]parser.Statement{*current.ElseIf}, namespace, global, metadata, imports)
			}
		case parser.LoopStatement:
			collectSourceMetadataInto(current.Body, namespace, global, metadata, imports)
		case parser.MatchStatement:
			for _, matchCase := range current.Cases {
				collectSourceMetadataInto(matchCase.Body, namespace, global, metadata, imports)
			}
		case parser.RunStatement:
			if current.Stmt != nil {
				collectSourceMetadataInto([]parser.Statement{current.Stmt}, namespace, global, metadata, imports)
			}
			collectSourceMetadataInto(current.Body, namespace, global, metadata, imports)
		case parser.PrivateBlockStatement:
			collectSourceMetadataInto(current.Body, namespace, global, metadata, imports)
		case parser.ScopeStatement:
			collectSourceMetadataInto(current.Body, namespace, global, metadata, imports)
		}
	}
}

func (resolver *Resolver) moduleDisabled(program file.Program) bool {
	for _, source := range program.Files {
		metadata, errors := resolver.metadataFor(source)
		if len(errors) == 0 && metadata.ModuleDisabled {
			return true
		}
	}
	return false
}

func (resolver *Resolver) applyStdlibFunctionFilter(program file.Program, importPath string, importingSource file.SourceFile) file.Program {
	moduleName := moduleNameFromImportPath(importPath)
	parsedImporter := parser.ParseSource(importingSource)
	if len(parsedImporter.Errors) != 0 {
		return program
	}
	selected := collectCalledModuleFunctions(parsedImporter.Program.Statements, moduleName)
	if len(selected) == 0 {
		selected = map[string]bool{}
	}
	selected = resolver.expandSelectedFunctions(program, selected)

	filtered := program
	filtered.Files = make([]file.SourceFile, 0, len(program.Files))
	for _, source := range program.Files {
		source.ModuleFunctionFilter = cloneBoolMap(selected)
		filtered.Files = append(filtered.Files, source)
	}
	return filtered
}

func (resolver *Resolver) expandSelectedFunctions(program file.Program, selected map[string]bool) map[string]bool {
	definitions := map[string]parser.FunctionStatement{}
	var parsedSources []parser.ParsedSource
	for _, source := range program.Files {
		parsed := parser.ParseSource(source)
		if len(parsed.Errors) != 0 {
			continue
		}
		parsedSources = append(parsedSources, parsed)
		collectFunctionDefinitions(parsed.Program.Statements, "", definitions)
	}
	for _, parsed := range parsedSources {
		collectUnfilteredDeclarationCalls(parsed.Program.Statements, "", func(call string) {
			if _, exists := definitions[call]; exists {
				selected[call] = true
			}
		})
	}

	changed := true
	for changed {
		changed = false
		for name := range selected {
			fn, ok := definitions[name]
			if !ok {
				continue
			}
			namespace := namespacePrefix(name)
			calls := collectFunctionBodyCalls(fn.Body, namespace)
			for call := range calls {
				if _, exists := definitions[call]; !exists || selected[call] {
					continue
				}
				selected[call] = true
				changed = true
			}
		}
	}
	return selected
}

func collectUnfilteredDeclarationCalls(statements []parser.Statement, namespace string, visit func(string)) {
	for _, statement := range statements {
		switch current := statement.(type) {
		case parser.AliasFunctionStatement:
			for _, method := range current.Methods {
				for call := range collectFunctionBodyCalls(method.Body, namespace) {
					visit(call)
				}
			}
			collectUnfilteredDeclarationCalls(current.Body, namespace, visit)
		case parser.ExtensionStatement:
			for _, method := range current.Methods {
				for call := range collectFunctionBodyCalls(method.Body, namespace) {
					visit(call)
				}
			}
		case parser.NamespaceStatement:
			collectUnfilteredDeclarationCalls(current.Body, namespace+current.Name+".", visit)
		case parser.PrivateBlockStatement:
			collectUnfilteredDeclarationCalls(current.Body, namespace, visit)
		case parser.ScopeStatement:
			collectUnfilteredDeclarationCalls(current.Body, namespace, visit)
		}
	}
}

func moduleNameFromImportPath(importPath string) string {
	clean := filepath.Clean(importPath)
	name := strings.TrimSuffix(filepath.Base(clean), filepath.Ext(clean))
	return name
}

func collectCalledModuleFunctions(statements []parser.Statement, moduleName string) map[string]bool {
	selected := map[string]bool{}
	aliases := collectNamespaceAliases(statements)
	collectStatementCalls(statements, func(name string) {
		name = resolveNamespaceAliasPath(name, aliases)
		if strings.HasPrefix(name, moduleName+".") {
			selected[name] = true
		}
	})
	return selected
}

func collectCalledModuleNames(statements []parser.Statement) []string {
	names := map[string]bool{}
	aliases := collectNamespaceAliases(statements)
	collectStatementCalls(statements, func(name string) {
		name = resolveNamespaceAliasPath(name, aliases)
		if !strings.Contains(name, ".") {
			return
		}
		moduleName := strings.TrimSpace(strings.SplitN(name, ".", 2)[0])
		if moduleName == "" {
			return
		}
		names[moduleName] = true
	})
	result := make([]string, 0, len(names))
	for name := range names {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

func collectNamespaceAliases(statements []parser.Statement) map[string]string {
	aliases := map[string]string{}
	var collect func([]parser.Statement)
	collect = func(currentStatements []parser.Statement) {
		for _, stmt := range currentStatements {
			switch current := stmt.(type) {
			case parser.AliasStatement:
				if !current.KeywordMacro && current.Name != "" && current.Target != "" {
					aliases[current.Name] = normalizeNamespacePath(current.Target)
				}
			case parser.NamespaceStatement:
				collect(current.Body)
			case parser.FunctionStatement:
				collect(current.Body)
			case parser.AliasFunctionStatement:
				collect(current.Body)
				for _, method := range current.Methods {
					collect(method.Body)
				}
			case parser.ExtensionStatement:
				for _, method := range current.Methods {
					collect(method.Body)
				}
			case parser.IfStatement:
				collect(current.Consequence)
				collect(current.Alternative)
				if current.ElseIf != nil {
					collect([]parser.Statement{*current.ElseIf})
				}
			case parser.MatchStatement:
				for _, matchCase := range current.Cases {
					collect(matchCase.Body)
				}
			case parser.LoopStatement:
				collect(current.Body)
			case parser.TryCatchStatement:
				collect(current.TryBody)
				collect(current.CatchBody)
			case parser.TransactionStatement:
				collect(current.Body)
			case parser.DeferStatement:
				if current.Stmt != nil {
					collect([]parser.Statement{current.Stmt})
				}
				collect(current.Body)
			case parser.RunStatement:
				if current.Stmt != nil {
					collect([]parser.Statement{current.Stmt})
				}
				collect(current.Body)
			case parser.PrivateBlockStatement:
				collect(current.Body)
			case parser.ScopeStatement:
				collect(current.Body)
			}
		}
	}
	collect(statements)
	return aliases
}

func resolveNamespaceAliasPath(name string, aliases map[string]string) string {
	name = normalizeNamespacePath(name)
	seen := map[string]bool{}
	for len(aliases) > 0 && !seen[name] {
		seen[name] = true
		alias, target, ok := longestNamespaceAlias(name, aliases)
		if !ok {
			break
		}
		name = target + strings.TrimPrefix(name, alias)
	}
	return name
}

func longestNamespaceAlias(name string, aliases map[string]string) (string, string, bool) {
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

func normalizeNamespacePath(name string) string {
	return strings.ReplaceAll(strings.TrimSpace(name), "::", ".")
}

func collectFunctionDefinitions(statements []parser.Statement, namespace string, definitions map[string]parser.FunctionStatement) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.NamespaceStatement:
			collectFunctionDefinitions(current.Body, namespace+current.Name+".", definitions)
		case parser.PrivateBlockStatement:
			collectFunctionDefinitions(current.Body, namespace, definitions)
		case parser.ScopeStatement:
			collectFunctionDefinitions(current.Body, namespace, definitions)
		case parser.FunctionStatement:
			definitions[namespace+current.Name] = current
		}
	}
}

func collectFunctionBodyCalls(statements []parser.Statement, namespace string) map[string]bool {
	calls := map[string]bool{}
	collectStatementCalls(statements, func(name string) {
		if strings.Contains(name, ".") {
			calls[name] = true
			return
		}
		calls[namespace+name] = true
	})
	return calls
}

func collectStatementCalls(statements []parser.Statement, add func(string)) {
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.FunctionStatement:
			collectStatementCalls(current.Body, add)
		case parser.AliasFunctionStatement:
			collectStatementCalls(current.Body, add)
			for _, method := range current.Methods {
				collectStatementCalls(method.Body, add)
			}
		case parser.ExtensionStatement:
			for _, method := range current.Methods {
				collectStatementCalls(method.Body, add)
			}
		case parser.VariableStatement:
			collectExpressionCalls(current.Expression.Node, add)
		case parser.DestructuringStatement:
			collectExpressionCalls(current.Expression.Node, add)
		case parser.ReturnStatement:
			collectExpressionCalls(current.Expression.Node, add)
			for _, value := range current.Values {
				collectExpressionCalls(value.Node, add)
			}
		case parser.ThrowStatement:
			collectExpressionCalls(current.Expression.Node, add)
		case parser.AssertStatement:
			collectExpressionCalls(current.Expression.Node, add)
		case parser.AssignmentStatement:
			collectExpressionCalls(current.Target.Node, add)
			collectExpressionCalls(current.Expression.Node, add)
		case parser.ExpressionStatement:
			collectExpressionCalls(current.Expression.Node, add)
		case parser.IfStatement:
			collectExpressionCalls(current.Condition.Node, add)
			collectStatementCalls(current.Consequence, add)
			collectStatementCalls(current.Alternative, add)
			if current.ElseIf != nil {
				collectStatementCalls([]parser.Statement{*current.ElseIf}, add)
			}
		case parser.MatchStatement:
			collectExpressionCalls(current.Value.Node, add)
			for _, matchCase := range current.Cases {
				collectExpressionCalls(matchCase.Pattern.Node, add)
				collectStatementCalls(matchCase.Body, add)
			}
		case parser.LoopStatement:
			collectExpressionCalls(current.Header.Node, add)
			collectStatementCalls(current.Body, add)
		case parser.TryCatchStatement:
			collectStatementCalls(current.TryBody, add)
			collectStatementCalls(current.CatchBody, add)
		case parser.TransactionStatement:
			collectStatementCalls(current.Body, add)
		case parser.DeferStatement:
			if current.Stmt != nil {
				collectStatementCalls([]parser.Statement{current.Stmt}, add)
			}
			collectStatementCalls(current.Body, add)
		case parser.RunStatement:
			if current.Stmt != nil {
				collectStatementCalls([]parser.Statement{current.Stmt}, add)
			}
			collectStatementCalls(current.Body, add)
		case parser.PrivateBlockStatement:
			collectStatementCalls(current.Body, add)
		case parser.ScopeStatement:
			collectStatementCalls(current.Body, add)
		case parser.NamespaceStatement:
			collectStatementCalls(current.Body, add)
		}
	}
}

func collectExpressionCalls(expr parser.ExpressionNode, add func(string)) {
	switch current := expr.(type) {
	case parser.CallExpression:
		if name, ok := expressionPath(current.Callee); ok {
			add(name)
		}
		collectExpressionCalls(current.Callee, add)
		for _, arg := range current.Arguments {
			collectExpressionCalls(arg, add)
		}
	case parser.UnaryExpression:
		collectExpressionCalls(current.Right, add)
	case parser.BinaryExpression:
		collectExpressionCalls(current.Left, add)
		collectExpressionCalls(current.Right, add)
	case parser.IndexExpression:
		collectExpressionCalls(current.Target, add)
		collectExpressionCalls(current.Index, add)
	case parser.SelectorExpression:
		collectExpressionCalls(current.Target, add)
	case parser.CastExpression:
		collectExpressionCalls(current.Value, add)
	case parser.NullCheckExpression:
		collectExpressionCalls(current.Value, add)
	case parser.PropagateExpression:
		collectExpressionCalls(current.Value, add)
	case parser.ConditionalExpression:
		collectExpressionCalls(current.Condition, add)
		collectExpressionCalls(current.Consequence, add)
		collectExpressionCalls(current.Alternative, add)
	case parser.ListExpression:
		for _, item := range current.Items {
			collectExpressionCalls(item, add)
		}
	case parser.ListComprehensionExpression:
		collectExpressionCalls(current.Value, add)
		collectExpressionCalls(current.Iterable, add)
		collectExpressionCalls(current.Condition, add)
	case parser.MapExpression:
		for _, entry := range current.Entries {
			collectExpressionCalls(entry.Key, add)
			collectExpressionCalls(entry.Value, add)
		}
	case parser.GroupExpression:
		collectExpressionCalls(current.Inner, add)
	case parser.LambdaExpression:
		collectStatementCalls(current.Body, add)
	}
}

func expressionPath(expr parser.ExpressionNode) (string, bool) {
	switch current := expr.(type) {
	case parser.IdentifierExpression:
		return current.Name, true
	case parser.SelectorExpression:
		target, ok := expressionPath(current.Target)
		if !ok {
			return "", false
		}
		return target + "." + current.Field, true
	default:
		return "", false
	}
}

func namespacePrefix(name string) string {
	index := strings.LastIndex(name, ".")
	if index == -1 {
		return ""
	}
	return name[:index+1]
}

func cloneBoolMap(items map[string]bool) map[string]bool {
	copied := make(map[string]bool, len(items))
	for key, value := range items {
		copied[key] = value
	}
	return copied
}

func (resolver *Resolver) pathExists(path string) bool {
	key := resolver.pathKey(path)
	if exists, ok := resolver.exists[key]; ok {
		return exists
	}

	_, err := os.Stat(path)
	exists := err == nil
	resolver.exists[key] = exists
	return exists
}
