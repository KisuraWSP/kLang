package modulesystem

import (
	"fmt"
	"os"
	"path/filepath"

	"kLang/src/engine/file"
	"kLang/src/parser"
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
	File    string
	Line    int
	Column  int
	Message string
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
}

type resolutionState struct {
	visited   map[string]bool
	resolving map[string]bool
	reported  map[string]bool
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

	return resolved, report
}

func (resolver *Resolver) resolveSource(state *resolutionState, program *file.Program, report *Report, source file.SourceFile) {
	sourceKey := resolver.pathKey(source.Path)
	state.resolving[sourceKey] = true
	defer delete(state.resolving, sourceKey)

	imports, errors := resolver.importsFor(source)
	for _, parseError := range errors {
		report.Errors = append(report.Errors, Error{
			File:    source.Path,
			Line:    parseError.Line,
			Column:  parseError.Column,
			Message: parseError.Message,
		})
	}
	if len(errors) != 0 {
		return
	}

	for _, importStmt := range imports {
		imported, module, err := resolver.resolveImport(source.Path, importStmt.Path)
		if err != nil {
			report.Errors = append(report.Errors, Error{
				File:    source.Path,
				Line:    importStmt.Pos.Line,
				Column:  importStmt.Pos.Column,
				Message: err.Error(),
			})
			continue
		}

		if resolver.moduleIsResolving(state, imported) {
			report.Errors = append(report.Errors, Error{
				File:    source.Path,
				Line:    importStmt.Pos.Line,
				Column:  importStmt.Pos.Column,
				Message: fmt.Sprintf("import cycle detected for %q", importStmt.Path),
			})
			continue
		}

		resolver.addModuleReport(state, report, module)
		for _, importedSource := range imported.Files {
			if resolver.isVisited(state, importedSource.Path) {
				continue
			}
			resolver.markVisited(state, importedSource.Path)
			program.Files = append(program.Files, importedSource)
			resolver.resolveSource(state, program, report, importedSource)
		}
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
			return program, Module{
				Name:       importPath,
				Path:       candidate,
				Kind:       ImportLocal,
				ImportedBy: importedBy,
			}, nil
		}
	}

	if !resolver.DisableStdlib {
		candidates = resolver.stdlibCandidates(importPath)
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
		candidates = append(candidates, filepath.Clean(filepath.Join(baseDir, importPath+file.KlangExtension)))
	}
	return candidates
}

func (resolver *Resolver) stdlibCandidates(importPath string) []string {
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

func (resolver *Resolver) importsFor(source file.SourceFile) ([]parser.ImportStatement, []parser.Error) {
	key := resolver.pathKey(source.Path)
	if imports, ok := resolver.imports[key]; ok {
		return imports, nil
	}

	parsed := parser.ParseSource(source)
	if len(parsed.Errors) != 0 {
		return nil, parsed.Errors
	}

	imports := collectImports(parsed.Program.Statements)
	resolver.imports[key] = imports
	return imports, nil
}

func (resolver *Resolver) loadProgram(path string) (file.Program, error) {
	key := resolver.pathKey(path)
	if program, ok := resolver.programs[key]; ok {
		return program, nil
	}

	program, err := file.LoadProgram(path)
	if err != nil {
		return file.Program{}, err
	}
	resolver.programs[key] = program
	return program, nil
}

func collectImports(statements []parser.Statement) []parser.ImportStatement {
	var imports []parser.ImportStatement
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ImportStatement:
			imports = append(imports, current)
		case parser.NamespaceStatement:
			imports = append(imports, collectImports(current.Body)...)
		case parser.FunctionStatement:
			imports = append(imports, collectImports(current.Body)...)
		case parser.AliasFunctionStatement:
			imports = append(imports, collectImports(current.Body)...)
			for _, method := range current.Methods {
				imports = append(imports, collectImports(method.Body)...)
			}
		case parser.ImplStatement:
			for _, method := range current.Methods {
				imports = append(imports, collectImports(method.Body)...)
			}
		case parser.IfStatement:
			imports = append(imports, collectImports(current.Consequence)...)
			imports = append(imports, collectImports(current.Alternative)...)
			if current.ElseIf != nil {
				imports = append(imports, collectImports([]parser.Statement{*current.ElseIf})...)
			}
		case parser.LoopStatement:
			imports = append(imports, collectImports(current.Body)...)
		case parser.MatchStatement:
			for _, matchCase := range current.Cases {
				imports = append(imports, collectImports(matchCase.Body)...)
			}
		}
	}
	return imports
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
