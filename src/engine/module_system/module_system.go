package modulesystem

import (
	"fmt"
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
}

func NewResolver(stdlibRoot string) *Resolver {
	if stdlibRoot == "" {
		stdlibRoot = DefaultStdlibRoot
	}

	return &Resolver{
		StdlibRoot: stdlibRoot,
		visited:    map[string]bool{},
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

	for _, stmt := range parsed.Program.Statements {
		importStmt, ok := stmt.(parser.ImportStatement)
		if !ok {
			continue
		}

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

		report.Modules = append(report.Modules, module)
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
		if file.FileExists(candidate) {
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
		if file.FileExists(candidate) {
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
