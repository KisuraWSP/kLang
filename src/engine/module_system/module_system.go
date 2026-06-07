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

type Resolver struct {
	StdlibRoot string
	visited    map[string]bool
	resolving  map[string]bool
	reported   map[string]bool
}

func NewResolver(stdlibRoot string) *Resolver {
	if stdlibRoot == "" {
		stdlibRoot = DefaultStdlibRoot
	}

	return &Resolver{
		StdlibRoot: stdlibRoot,
		visited:    map[string]bool{},
		resolving:  map[string]bool{},
		reported:   map[string]bool{},
	}
}

func ResolveProgram(program file.Program) (file.Program, Report) {
	return NewResolver(DefaultStdlibRoot).ResolveProgram(program)
}

func (resolver *Resolver) ResolveProgram(program file.Program) (file.Program, Report) {
	resolved := program
	report := Report{}

	for _, source := range program.Files {
		resolver.markVisited(source.Path)
	}

	for _, source := range program.Files {
		resolver.resolveSource(&resolved, &report, source)
	}

	return resolved, report
}

func (resolver *Resolver) resolveSource(program *file.Program, report *Report, source file.SourceFile) {
	sourceKey := resolver.pathKey(source.Path)
	resolver.resolving[sourceKey] = true
	defer delete(resolver.resolving, sourceKey)

	parsed := parser.ParseSource(source)
	for _, parseError := range parsed.Errors {
		report.Errors = append(report.Errors, Error{
			File:    source.Path,
			Line:    parseError.Line,
			Column:  parseError.Column,
			Message: parseError.Message,
		})
	}
	if len(parsed.Errors) != 0 {
		return
	}

	for _, importStmt := range collectImports(parsed.Program.Statements) {
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

		if resolver.moduleIsResolving(imported) {
			report.Errors = append(report.Errors, Error{
				File:    source.Path,
				Line:    importStmt.Pos.Line,
				Column:  importStmt.Pos.Column,
				Message: fmt.Sprintf("import cycle detected for %q", importStmt.Path),
			})
			continue
		}

		resolver.addModuleReport(report, module)
		for _, importedSource := range imported.Files {
			if resolver.isVisited(importedSource.Path) {
				continue
			}
			resolver.markVisited(importedSource.Path)
			program.Files = append(program.Files, importedSource)
			resolver.resolveSource(program, report, importedSource)
		}
	}
}

func (resolver *Resolver) resolveImport(importedBy string, importPath string) (file.Program, Module, error) {
	candidates := resolver.localCandidates(importedBy, importPath)
	for _, candidate := range candidates {
		if pathExists(candidate) {
			program, err := file.LoadProgram(candidate)
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

	candidates = resolver.stdlibCandidates(importPath)
	for _, candidate := range candidates {
		if pathExists(candidate) {
			program, err := file.LoadProgram(candidate)
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

func (resolver *Resolver) isVisited(path string) bool {
	key := resolver.pathKey(path)
	return resolver.visited[key]
}

func (resolver *Resolver) markVisited(path string) {
	key := resolver.pathKey(path)
	resolver.visited[key] = true
}

func (resolver *Resolver) pathKey(path string) string {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(absolute)
}

func (resolver *Resolver) moduleIsResolving(program file.Program) bool {
	for _, source := range program.Files {
		if resolver.resolving[resolver.pathKey(source.Path)] {
			return true
		}
	}
	return false
}

func (resolver *Resolver) addModuleReport(report *Report, module Module) {
	key := resolver.pathKey(module.Path)
	if resolver.reported[key] {
		return
	}
	resolver.reported[key] = true
	report.Modules = append(report.Modules, module)
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
		case parser.IfStatement:
			imports = append(imports, collectImports(current.Consequence)...)
			imports = append(imports, collectImports(current.Alternative)...)
			if current.ElseIf != nil {
				imports = append(imports, collectImports([]parser.Statement{*current.ElseIf})...)
			}
		case parser.LoopStatement:
			imports = append(imports, collectImports(current.Body)...)
		}
	}
	return imports
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
