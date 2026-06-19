package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	stdruntime "runtime"
	"strconv"
	"strings"
	"time"

	langcontext "kLang/src/engine/context"
	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	programcache "kLang/src/engine/program_cache"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

const cliName = "kLang"

type commandOptions struct {
	Run         bool
	Verbose     bool
	RawLang     bool
	ProgramArgs []string
}

type entrySpec struct {
	Name string
	Type string
}

type packageOptions struct {
	Backend string
	Out     string
	Serve   bool
	Host    string
	Port    int
	PortSet bool
}

type docOptions struct {
	SourceFiles []string
	Out         string
}

type docFile struct {
	Path       string
	Name       string
	LineCount  int
	Source     []string
	Items      []docItem
	ParseError []parser.Error
}

type docItem struct {
	Kind      string
	Name      string
	Signature string
	Detail    string
	Line      int
}

func main() {
	if err := runCLI(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func runCLI(args []string) error {
	if len(args) == 0 || hasFlag(args, "--help") || hasFlag(args, "-h") {
		printUsage()
		return nil
	}

	if handled, err := runLegacyFlags(args); handled {
		return err
	}

	command := args[0]
	rest := args[1:]
	options := commandOptions{
		Run:     hasFlag(rest, "--run"),
		Verbose: hasFlag(rest, "--verbose") || hasFlag(rest, "-v"),
		RawLang: hasFlag(rest, "--raw-lang"),
	}
	values := positionalArgs(rest)

	switch command {
	case "new", "init":
		if len(values) != 1 {
			return fmt.Errorf("%s %s expects a project path", cliName, command)
		}
		entry, err := parseEntryFlag(rest)
		if err != nil {
			return err
		}
		return createProject(values[0], entry)
	case "run":
		if len(values) < 1 {
			return fmt.Errorf("%s run expects a .klang file or project folder", cliName)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		options.Run = true
		options.ProgramArgs = append([]string(nil), values[1:]...)
		return executePrograms([]file.Program{program}, options)
	case "check":
		if len(values) != 1 {
			return fmt.Errorf("%s check expects a .klang file or project folder", cliName)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		return executePrograms([]file.Program{program}, commandOptions{Run: false, Verbose: options.Verbose, RawLang: options.RawLang})
	case "package", "build":
		if len(values) != 1 {
			return fmt.Errorf("%s %s expects a .klang file or project folder", cliName, command)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		packageOptions, err := parsePackageOptions(rest)
		if err != nil {
			return err
		}
		return packageProgram(program, packageOptions, commandOptions{Verbose: options.Verbose, RawLang: options.RawLang})
	case "serve", "web":
		if len(values) != 1 {
			return fmt.Errorf("%s %s expects a .klang file or project folder", cliName, command)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		packageOptions, err := parsePackageOptions(rest)
		if err != nil {
			return err
		}
		packageOptions.Backend = "WASM"
		packageOptions.Serve = true
		if !packageOptions.PortSet {
			packageOptions.Port = 8080
			packageOptions.PortSet = true
		}
		if packageOptions.Out == "" {
			tempRoot, err := os.MkdirTemp("", "klang-wasm-serve-*")
			if err != nil {
				return err
			}
			packageOptions.Out = tempRoot
		}
		return packageProgram(program, packageOptions, commandOptions{Verbose: options.Verbose, RawLang: options.RawLang})
	case "doc", "docs":
		docOptions, err := parseDocOptions(rest)
		if err != nil {
			return err
		}
		return generateDocumentation(docOptions)
	case "test", "tests":
		if len(values) != 1 {
			return fmt.Errorf("%s test expects a folder containing .klang tests", cliName)
		}
		programs, err := file.DiscoverPrograms(values[0])
		if err != nil {
			return err
		}
		return executePrograms(programs, options)
	case "file", "show":
		if len(values) != 1 {
			return fmt.Errorf("%s %s expects a .klang file path", cliName, command)
		}
		file.PrintFile(values[0])
		return nil
	default:
		return fmt.Errorf("unknown command %q\n\n%s", command, usageText())
	}
}

func runLegacyFlags(args []string) (bool, error) {
	testsPath := file.GetTestsPath(args)
	if testsPath != "" {
		programs, err := file.DiscoverPrograms(testsPath)
		if err != nil {
			return true, fmt.Errorf("failed to read tests: %w", err)
		}
		return true, executePrograms(programs, commandOptions{Run: file.HasRunFlag(args), Verbose: true, RawLang: hasFlag(args, "--raw-lang")})
	}

	programPath := file.GetProgramPath(args)
	if programPath != "" {
		program, err := file.LoadProgram(programPath)
		if err != nil {
			return true, fmt.Errorf("failed to read program: %w", err)
		}
		return true, executePrograms([]file.Program{program}, commandOptions{Run: file.HasRunFlag(args), Verbose: true, RawLang: hasFlag(args, "--raw-lang")})
	}

	filePath := file.GetFilePath(args)
	if filePath != "" {
		file.PrintFile(filePath)
		return true, nil
	}

	return false, nil
}

func createProject(projectPath string, entry entrySpec) error {
	cleanPath := filepath.Clean(projectPath)
	if cleanPath == "." || cleanPath == string(filepath.Separator) {
		return fmt.Errorf("refusing to create project at %q", projectPath)
	}

	if info, err := os.Stat(cleanPath); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("%s already exists and is not a directory", cleanPath)
		}
		entries, err := os.ReadDir(cleanPath)
		if err != nil {
			return err
		}
		if len(entries) != 0 {
			return fmt.Errorf("%s already exists and is not empty", cleanPath)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.MkdirAll(cleanPath, 0755); err != nil {
		return err
	}

	files := map[string]string{
		file.KlangEntryPoint: newProjectEntrySource(),
		"app.klang":          newProjectModuleSource(projectNameFromPath(cleanPath), entry),
	}
	for name, contents := range files {
		path := filepath.Join(cleanPath, name)
		if err := os.WriteFile(path, []byte(contents), 0644); err != nil {
			return err
		}
	}

	fmt.Printf("created Klang project %s\n", cleanPath)
	fmt.Printf("  %s\n", filepath.Join(cleanPath, file.KlangEntryPoint))
	fmt.Printf("  %s\n", filepath.Join(cleanPath, "app.klang"))
	fmt.Printf("\nnext steps:\n")
	fmt.Printf("  go run . run %s\n", cleanPath)
	fmt.Printf("  go run . check %s\n", cleanPath)
	return nil
}

func packageProgram(program file.Program, packageOptions packageOptions, options commandOptions) error {
	backend := packageOptions.Backend
	if backend == "" {
		backend = "Standalone"
	}
	if packageOptions.Serve {
		backend = "WASM"
		if !packageOptions.PortSet {
			packageOptions.Port = 8080
			packageOptions.PortSet = true
		}
	}
	if !isBuildBackend(backend) {
		return fmt.Errorf("backend must be one of WASM, JS, Standalone")
	}
	outRoot := packageOptions.Out
	if outRoot == "" {
		outRoot = filepath.Join(program.Root, "dist")
	}

	resolver := modulesystem.NewResolver("")
	resolver.DisableStdlib = options.RawLang
	resolvedProgram, moduleReport := resolver.ResolveProgram(program)
	if !moduleReport.Passed() {
		printModuleErrors(resolvedProgram, moduleReport)
		return fmt.Errorf("module resolution failed")
	}
	typeReport := typechecker.CheckProgram(resolvedProgram)
	if !typeReport.Passed() {
		printTypeErrors(resolvedProgram, typeReport)
		return fmt.Errorf("type check failed")
	}
	parsedProgram := parser.ParseLoadedProgram(resolvedProgram)
	if !parsedProgram.Passed() {
		printContextErrors(langcontext.ParseErrors(resolvedProgram, parsedProgram))
		return fmt.Errorf("parse failed")
	}

	bundleDir := filepath.Join(outRoot, program.Name+"-"+strings.ToLower(backend))
	sourceDir := filepath.Join(bundleDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return err
	}

	manifestFiles := make([]string, 0, len(resolvedProgram.Files))
	entry := ""
	for _, source := range resolvedProgram.Files {
		relativePath := bundleSourcePath(resolvedProgram.Root, source.Path)
		targetPath := filepath.Join(sourceDir, filepath.Clean(relativePath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			return err
		}
		contents := strings.Join(source.Lines, "\n")
		if len(source.Lines) != 0 {
			contents += "\n"
		}
		if err := os.WriteFile(targetPath, []byte(contents), 0644); err != nil {
			return err
		}
		manifestPath := filepath.ToSlash(filepath.Join("src", filepath.Clean(relativePath)))
		manifestFiles = append(manifestFiles, manifestPath)
		if filepath.Clean(source.Path) == filepath.Clean(resolvedProgram.EntryPoint) {
			entry = manifestPath
		}
	}
	if entry == "" && len(manifestFiles) != 0 {
		entry = manifestFiles[0]
	}

	manifest := map[string]any{
		"project_name":    resolvedProgram.Name,
		"backend":         backend,
		"entry":           entry,
		"number_of_files": len(resolvedProgram.Files),
		"files":           manifestFiles,
		"raw_lang":        options.RawLang,
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "klang-build.json"), append(manifestBytes, '\n'), 0644); err != nil {
		return err
	}
	if backend == "WASM" {
		if err := writeWASMBrowserBundle(bundleDir); err != nil {
			printContextErrors([]langcontext.ErrorContext{langcontext.BackendError(resolvedProgram, backend, err)})
			return err
		}
	}

	fmt.Printf("packaged Klang project %s\n", resolvedProgram.Name)
	fmt.Printf("  backend: %s\n", backend)
	fmt.Printf("  files: %d\n", len(resolvedProgram.Files))
	fmt.Printf("  bundle: %s\n", bundleDir)
	fmt.Printf("  manifest: %s\n", filepath.Join(bundleDir, "klang-build.json"))
	if backend == "WASM" {
		fmt.Printf("  wasm: %s\n", filepath.Join(bundleDir, "klang.wasm"))
		fmt.Printf("  browser: %s\n", filepath.Join(bundleDir, "index.html"))
	}
	if packageOptions.Serve {
		return serveBundle(bundleDir, packageOptions)
	}
	return nil
}

func serveBundle(bundleDir string, options packageOptions) error {
	host := strings.TrimSpace(options.Host)
	if host == "" {
		host = "127.0.0.1"
	}
	port := options.Port
	if port < 0 || port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}

	address := fmt.Sprintf("%s:%d", host, port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return err
	}
	defer listener.Close()

	actualAddress := listener.Addr().String()
	urlHost, urlPort, err := net.SplitHostPort(actualAddress)
	if err != nil {
		urlHost = host
		urlPort = strconv.Itoa(port)
	}
	if urlHost == "::" || urlHost == "0.0.0.0" || urlHost == "" {
		urlHost = "127.0.0.1"
	}

	fmt.Printf("\nserving Klang browser runtime\n")
	fmt.Printf("  bundle: %s\n", bundleDir)
	fmt.Printf("  url: http://%s:%s\n", urlHost, urlPort)
	fmt.Printf("  stop: Ctrl+C\n")

	server := &http.Server{
		Handler: http.FileServer(http.Dir(bundleDir)),
	}
	return server.Serve(listener)
}

func bundleSourcePath(root string, sourcePath string) string {
	relativePath, err := filepath.Rel(root, sourcePath)
	if err != nil || strings.HasPrefix(relativePath, "..") || filepath.IsAbs(relativePath) {
		return filepath.Base(sourcePath)
	}
	return filepath.Clean(relativePath)
}

func generateDocumentation(options docOptions) error {
	if len(options.SourceFiles) == 0 {
		return fmt.Errorf("%s doc expects --sourcefile=[file.klang,...]", cliName)
	}
	outPath := strings.TrimSpace(options.Out)
	if outPath == "" {
		outPath = "klang-docs.html"
	}

	sourceFiles, err := expandDocSourceFiles(options.SourceFiles)
	if err != nil {
		return err
	}

	files := make([]docFile, 0, len(sourceFiles))
	totalItems := 0
	for _, source := range sourceFiles {
		sourcePath := filepath.Clean(source.Path)
		lines := append([]string(nil), source.Lines...)
		parsed, errors := parser.Parse(strings.Join(lines, "\n"))
		doc := docFile{
			Path:      sourcePath,
			Name:      filepath.Base(sourcePath),
			LineCount: len(lines),
			Source:    lines,
		}
		if len(errors) != 0 {
			doc.ParseError = errors
		} else {
			doc.Items = collectDocItems(parsed.Statements, "")
			totalItems += len(doc.Items)
		}
		files = append(files, doc)
	}

	if err := os.MkdirAll(filepath.Dir(filepath.Clean(outPath)), 0755); err != nil && filepath.Dir(filepath.Clean(outPath)) != "." {
		return err
	}
	htmlText := renderDocumentationHTML(files, totalItems)
	if err := os.WriteFile(outPath, []byte(htmlText), 0644); err != nil {
		return err
	}
	fmt.Printf("generated Klang documentation\n")
	fmt.Printf("  sources: %d\n", len(files))
	fmt.Printf("  items: %d\n", totalItems)
	fmt.Printf("  output: %s\n", outPath)
	return nil
}

func expandDocSourceFiles(sourcePaths []string) ([]file.SourceFile, error) {
	var sources []file.SourceFile
	seen := map[string]bool{}
	for _, sourcePath := range sourcePaths {
		sourcePath = filepath.Clean(strings.TrimSpace(sourcePath))
		if sourcePath == "" {
			continue
		}
		program, err := file.LoadProgram(sourcePath)
		if err != nil {
			return nil, fmt.Errorf("load doc source %s: %w", sourcePath, err)
		}
		for _, source := range program.Files {
			cleanPath := filepath.Clean(source.Path)
			if seen[cleanPath] {
				continue
			}
			seen[cleanPath] = true
			source.Path = cleanPath
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return nil, fmt.Errorf("%s doc found no source files", cliName)
	}
	return sources, nil
}

func collectDocItems(statements []parser.Statement, namespace string) []docItem {
	var items []docItem
	for _, stmt := range statements {
		switch current := stmt.(type) {
		case parser.ImportStatement:
			items = append(items, docItem{Kind: "import", Name: current.Path, Signature: fmt.Sprintf(`import "%s"`, current.Path), Line: current.Pos.Line})
		case parser.ModuleDirectiveStatement:
			items = append(items, docItem{Kind: "module", Name: current.Name, Signature: moduleDirectiveSignature(current), Line: current.Pos.Line})
		case parser.AliasStatement:
			items = append(items, docItem{Kind: "alias", Name: current.Name, Signature: fmt.Sprintf("alias %s = %s", current.Name, current.Target), Line: current.Pos.Line})
		case parser.RegionStatement:
			items = append(items, docItem{Kind: "region", Name: current.Name, Signature: fmt.Sprintf("region %s(%s, ...)", current.Name, current.TypeName), Detail: "Region-backed array storage", Line: current.Pos.Line})
		case parser.NamespaceStatement:
			name := namespace + current.Name
			scope := "namespace"
			if current.Global {
				scope = "global namespace"
			}
			items = append(items, docItem{Kind: "namespace", Name: name, Signature: scope + " " + name, Line: current.Pos.Line})
			items = append(items, collectDocItems(current.Body, name+".")...)
		case parser.TraitStatement:
			items = append(items, docItem{Kind: "trait", Name: namespace + current.Name, Signature: fmt.Sprintf("trait %s (%d method(s))", namespace+current.Name, len(current.Methods)), Line: current.Pos.Line})
		case parser.ImplStatement:
			items = append(items, docItem{Kind: "impl", Name: current.Trait + " for " + current.Type, Signature: fmt.Sprintf("impl %s for %s", current.Trait, current.Type), Detail: fmt.Sprintf("%d method(s)", len(current.Methods)), Line: current.Pos.Line})
		case parser.EnumStatement:
			items = append(items, docItem{Kind: "enum", Name: namespace + current.Name, Signature: fmt.Sprintf("enum %s", namespace+current.Name), Detail: enumVariantSummary(current.Variants), Line: current.Pos.Line})
		case parser.FunctionGroupStatement:
			items = append(items, docItem{Kind: "function group", Name: namespace + current.Name, Signature: fmt.Sprintf("function_group %s", namespace+current.Name), Detail: fmt.Sprintf("%d function(s)", len(current.Functions)), Line: current.Pos.Line})
		case parser.FunctionStatement:
			items = append(items, docItem{Kind: functionDocKind(current), Name: namespace + current.Name, Signature: functionDocSignature(namespace, current), Line: current.Pos.Line})
			items = append(items, collectDocItems(current.Body, namespace)...)
		case parser.AliasFunctionStatement:
			items = append(items, docItem{Kind: "alias function", Name: namespace + current.Name, Signature: aliasFunctionDocSignature(namespace, current), Detail: aliasFunctionDetail(current), Line: current.Pos.Line})
			items = append(items, collectDocItems(current.Body, namespace)...)
			for _, method := range current.Methods {
				items = append(items, docItem{Kind: "extension method", Name: namespace + current.Name + "." + method.Name, Signature: functionDocSignature(namespace+current.Name+".", method), Line: method.Pos.Line})
			}
		case parser.VariableStatement:
			if current.Scope == "global" || current.Scope == "const" || current.Exported {
				items = append(items, docItem{Kind: variableDocKind(current), Name: namespace + current.Name, Signature: variableDocSignature(namespace, current), Line: current.Pos.Line})
			}
		}
	}
	return items
}

func renderDocumentationHTML(files []docFile, totalItems int) string {
	var builder strings.Builder
	totalSourceLines := 0
	for _, file := range files {
		totalSourceLines += file.LineCount
	}
	builder.WriteString(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Klang Source Documentation</title>
  <style>
    :root { color-scheme: light; --ink: #17202a; --muted: #5d6d7e; --line: #d7dde5; --panel: #ffffff; --paper: #f5f7fb; --accent: #0f766e; --accent-2: #334155; }
    * { box-sizing: border-box; }
    body { margin: 0; font-family: Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: var(--paper); color: var(--ink); }
    header { background: #102a43; color: #f8fafc; padding: 32px clamp(18px, 4vw, 56px); }
    header h1 { margin: 0 0 10px; font-size: 34px; line-height: 1.1; letter-spacing: 0; }
    header p { margin: 0; color: #cbd5e1; max-width: 820px; }
    main { max-width: 1180px; margin: 0 auto; padding: 24px clamp(14px, 3vw, 32px) 48px; }
    .summary { display: grid; grid-template-columns: repeat(auto-fit, minmax(160px, 1fr)); gap: 12px; margin-top: -44px; margin-bottom: 22px; }
    .metric { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 16px; box-shadow: 0 8px 20px rgba(15, 23, 42, 0.08); }
    .metric strong { display: block; font-size: 26px; }
    .metric span { color: var(--muted); font-size: 13px; }
    .toc { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; padding: 16px; margin-bottom: 20px; }
    .toc h2 { margin: 0 0 10px; font-size: 18px; letter-spacing: 0; }
    .toc ol { margin: 0; padding-left: 22px; columns: 2; column-gap: 28px; }
    .toc li { break-inside: avoid; margin: 0 0 6px; color: var(--muted); }
    .toc a { color: var(--accent); text-decoration: none; font-weight: 650; }
    section.file { background: var(--panel); border: 1px solid var(--line); border-radius: 8px; margin-top: 18px; overflow: hidden; }
    .file-head { display: flex; justify-content: space-between; gap: 16px; padding: 18px 20px; background: #eef3f8; border-bottom: 1px solid var(--line); }
    .file-head h2 { margin: 0; font-size: 19px; letter-spacing: 0; }
    .file-head p { margin: 4px 0 0; color: var(--muted); font-size: 13px; overflow-wrap: anywhere; }
    .badge { align-self: start; white-space: nowrap; color: #0f172a; background: #dbeafe; border: 1px solid #bfdbfe; border-radius: 999px; padding: 5px 9px; font-size: 12px; }
    .items { display: grid; grid-template-columns: repeat(auto-fit, minmax(280px, 1fr)); gap: 12px; padding: 16px; }
    .item { border: 1px solid var(--line); border-radius: 8px; padding: 14px; background: #fff; min-width: 0; }
    .item-kind { color: var(--accent); font-weight: 700; font-size: 12px; text-transform: uppercase; }
    .item h3 { margin: 8px 0; font-size: 17px; letter-spacing: 0; overflow-wrap: anywhere; }
    code { display: block; white-space: pre-wrap; overflow-wrap: anywhere; background: #f8fafc; border: 1px solid #e2e8f0; border-radius: 6px; padding: 10px; color: var(--accent-2); }
    .detail, .line { color: var(--muted); font-size: 13px; }
    .empty { padding: 20px; color: var(--muted); }
    .error { margin: 16px; border: 1px solid #fecaca; background: #fff1f2; border-radius: 8px; padding: 14px; color: #991b1b; }
    .source-chapter { border-top: 1px solid var(--line); padding: 16px; background: #fbfdff; }
    .source-chapter h3 { margin: 0 0 10px; font-size: 16px; letter-spacing: 0; }
    .source-code { margin: 0; border: 1px solid #dbe3ee; border-radius: 8px; overflow: auto; background: #0b1020; color: #e5edf8; font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace; font-size: 13px; line-height: 1.55; }
    .source-line { display: grid; grid-template-columns: 56px minmax(0, 1fr); min-width: max-content; }
    .source-line-number { user-select: none; text-align: right; color: #8aa0bd; padding: 0 12px; border-right: 1px solid #24304a; }
    .source-line-code { white-space: pre; padding: 0 14px; }
    @media (max-width: 720px) { .toc ol { columns: 1; } .file-head { display: block; } .badge { display: inline-block; margin-top: 10px; } }
  </style>
</head>
<body>
  <header>
    <h1>Klang Source Documentation</h1>
    <p>Generated from Klang source files. Use the declaration cards and source chapters below to scan public APIs, module shape, and the exact code behind each file.</p>
  </header>
  <main>
`)
	builder.WriteString(fmt.Sprintf(`    <div class="summary"><div class="metric"><strong>%d</strong><span>source files</span></div><div class="metric"><strong>%d</strong><span>documented items</span></div><div class="metric"><strong>%d</strong><span>source lines</span></div></div>`, len(files), totalItems, totalSourceLines))
	builder.WriteString(`<nav class="toc"><h2>Source Chapters</h2><ol>`)
	for index, file := range files {
		builder.WriteString(fmt.Sprintf(`<li><a href="#source-%d">%s</a> <span>%d line(s)</span></li>`, index+1, html.EscapeString(file.Name), file.LineCount))
	}
	builder.WriteString(`</ol></nav>`)
	for index, file := range files {
		builder.WriteString(fmt.Sprintf(`<section class="file" id="source-%d">`, index+1))
		builder.WriteString(`<div class="file-head"><div>`)
		builder.WriteString(`<h2>` + fmt.Sprintf("Chapter %d: ", index+1) + html.EscapeString(file.Name) + `</h2>`)
		builder.WriteString(`<p>` + html.EscapeString(file.Path) + `</p>`)
		builder.WriteString(`</div><span class="badge">` + fmt.Sprintf("%d line(s)", file.LineCount) + `</span></div>`)
		if len(file.ParseError) != 0 {
			for _, parseErr := range file.ParseError {
				builder.WriteString(`<div class="error">`)
				builder.WriteString(fmt.Sprintf("Parse error at %d:%d: %s", parseErr.Line, parseErr.Column, html.EscapeString(parseErr.Message)))
				builder.WriteString(`</div>`)
			}
		} else if len(file.Items) == 0 {
			builder.WriteString(`<div class="empty">No documentable declarations found.</div>`)
		} else {
			builder.WriteString(`<div class="items">`)
			for _, item := range file.Items {
				builder.WriteString(`<article class="item">`)
				builder.WriteString(`<div class="item-kind">` + html.EscapeString(item.Kind) + `</div>`)
				builder.WriteString(`<h3>` + html.EscapeString(item.Name) + `</h3>`)
				builder.WriteString(`<code>` + html.EscapeString(item.Signature) + `</code>`)
				if item.Detail != "" {
					builder.WriteString(`<p class="detail">` + html.EscapeString(item.Detail) + `</p>`)
				}
				if item.Line > 0 {
					builder.WriteString(fmt.Sprintf(`<p class="line">line %d</p>`, item.Line))
				}
				builder.WriteString(`</article>`)
			}
			builder.WriteString(`</div>`)
		}
		builder.WriteString(renderSourceChapter(file))
		builder.WriteString(`</section>`)
	}
	builder.WriteString(`
  </main>
</body>
</html>
`)
	return builder.String()
}

func renderSourceChapter(file docFile) string {
	var builder strings.Builder
	builder.WriteString(`<div class="source-chapter">`)
	builder.WriteString(`<h3>Source Code</h3>`)
	builder.WriteString(`<pre class="source-code" aria-label="Source code for ` + html.EscapeString(file.Name) + `">`)
	for index, line := range file.Source {
		builder.WriteString(`<span class="source-line">`)
		builder.WriteString(fmt.Sprintf(`<span class="source-line-number">%d</span>`, index+1))
		builder.WriteString(`<span class="source-line-code">` + html.EscapeString(line) + `</span>`)
		builder.WriteString(`</span>`)
	}
	if len(file.Source) == 0 {
		builder.WriteString(`<span class="source-line"><span class="source-line-number">1</span><span class="source-line-code"></span></span>`)
	}
	builder.WriteString(`</pre></div>`)
	return builder.String()
}

func writeWASMBrowserBundle(bundleDir string) error {
	wasmPath := filepath.Join(bundleDir, "klang.wasm")
	if err := buildWASMRuntime(wasmPath); err != nil {
		return err
	}
	if err := copyWASMExec(filepath.Join(bundleDir, "wasm_exec.js")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "klang_browser.js"), []byte(klangBrowserJS()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "index.html"), []byte(klangBrowserHTML()), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundleDir, "README.md"), []byte(klangWASMReadme()), 0644); err != nil {
		return err
	}
	return nil
}

func buildWASMRuntime(outputPath string) error {
	var stderr bytes.Buffer
	cmd := exec.Command("go", "build", "-o", outputPath, "./cmd/klang-wasm")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("build WASM runtime failed: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	return nil
}

func copyWASMExec(targetPath string) error {
	sourcePath, err := wasmExecPath()
	if err != nil {
		return err
	}
	contents, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}
	return os.WriteFile(targetPath, contents, 0644)
}

func wasmExecPath() (string, error) {
	candidates := []string{
		filepath.Join(stdruntime.GOROOT(), "misc", "wasm", "wasm_exec.js"),
		filepath.Join(stdruntime.GOROOT(), "lib", "wasm", "wasm_exec.js"),
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("could not find wasm_exec.js in Go installation")
}

func klangBrowserJS() string {
	return `let klangGoRuntime = null;
let klangWasmStarted = false;
let klangProject = null;

async function startKlangWASM() {
  if (klangWasmStarted) return;
  klangGoRuntime = new Go();
  const response = await fetch("klang.wasm");
  const bytes = await response.arrayBuffer();
  const result = await WebAssembly.instantiate(bytes, klangGoRuntime.importObject);
  klangWasmStarted = true;
  klangGoRuntime.run(result.instance);
  await Promise.resolve();
}

async function loadKlangProject() {
  if (klangProject) return klangProject;
  const manifest = await fetch("klang-build.json").then((response) => response.json());
  const files = {};
  for (const file of manifest.files) {
    files[file] = await fetch(file).then((response) => response.text());
  }
  klangProject = {
    name: manifest.project_name,
    entry: manifest.entry,
    files,
  };
  return klangProject;
}

async function runKlangProject(args = []) {
  await startKlangWASM();
  const project = await loadKlangProject();
  return JSON.parse(globalThis.klangRunProject(project, args));
}

async function checkKlangProject() {
  await startKlangWASM();
  const project = await loadKlangProject();
  return JSON.parse(globalThis.klangCheckProject(project));
}

async function runKlangSource(source, args = []) {
  await startKlangWASM();
  return JSON.parse(globalThis.klangRun(source, args));
}

globalThis.KlangBrowser = {
  start: startKlangWASM,
  loadProject: loadKlangProject,
  runProject: runKlangProject,
  checkProject: checkKlangProject,
  runSource: runKlangSource,
};
`
}

func klangBrowserHTML() string {
	return `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Klang WASM Runtime</title>
  <style>
    body { margin: 0; font-family: ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif; background: #111; color: #f3f3f3; }
    main { max-width: 960px; margin: 0 auto; padding: 32px 20px; }
    button { border: 0; border-radius: 6px; padding: 10px 14px; font: inherit; cursor: pointer; background: #00c2a8; color: #071614; }
    pre { min-height: 260px; overflow: auto; padding: 16px; border-radius: 6px; background: #050505; border: 1px solid #333; }
  </style>
</head>
<body>
  <main>
    <h1>Klang WASM Runtime</h1>
    <p>This page runs the packaged Klang project through the browser-hosted WASM runtime.</p>
    <button id="run">Run Project</button>
    <pre id="output">Ready.</pre>
  </main>
  <script src="wasm_exec.js"></script>
  <script src="klang_browser.js"></script>
  <script>
    const output = document.getElementById("output");
    document.getElementById("run").addEventListener("click", async () => {
      output.textContent = "Running...";
      try {
        const result = await KlangBrowser.runProject([]);
        output.textContent = JSON.stringify(result, null, 2);
      } catch (error) {
        output.textContent = String(error && error.stack ? error.stack : error);
      }
    });
  </script>
</body>
</html>
`
}

func klangWASMReadme() string {
	return `# Klang WASM Bundle

This bundle contains a browser-hosted Klang runtime.

## Files

- ` + "`klang.wasm`" + `: the Go interpreter/runtime compiled with ` + "`GOOS=js GOARCH=wasm`" + `.
- ` + "`wasm_exec.js`" + `: Go's JavaScript support shim for WASM.
- ` + "`klang_browser.js`" + `: browser loader exposing ` + "`KlangBrowser.runProject()`" + ` and ` + "`KlangBrowser.runSource(source, args)`" + `.
- ` + "`klang-build.json`" + `: package manifest.
- ` + "`src/`" + `: resolved Klang source files.

## Run Locally

Serve this folder through any static file server and open ` + "`index.html`" + `:

` + "```sh" + `
python3 -m http.server 8080
` + "```" + `

Then visit http://localhost:8080.
`
}

func executePrograms(programs []file.Program, options commandOptions) error {
	if len(programs) == 0 {
		return fmt.Errorf("no Klang programs found")
	}

	failed := false
	resolver := modulesystem.NewResolver("")
	resolver.DisableStdlib = options.RawLang
	for _, program := range programs {
		if err := executeProgram(resolver, program, options); err != nil {
			failed = true
			fmt.Fprintf(os.Stderr, "%s: %v\n", program.Name, err)
		}
	}

	if failed {
		return fmt.Errorf("one or more Klang programs failed")
	}
	return nil
}

func executeProgram(resolver *modulesystem.Resolver, program file.Program, options commandOptions) error {
	fmt.Printf("%s\n", program.Name)
	fmt.Printf("  entry: %s\n", program.EntryPoint)
	fmt.Printf("  files: %d\n", len(program.Files))

	resolvedProgram, cacheEntry, cacheHit := programcache.Load(program, options.RawLang)
	typeReport := typechecker.Report{}
	if cacheHit {
		typeReport.Warnings = warningsFromCache(cacheEntry.Warnings)
		if options.Verbose {
			if cachePath, ok := programcache.Path(program, options.RawLang); ok {
				fmt.Printf("  program cache: hit (%s)\n", cachePath)
			}
		}
		fmt.Printf("  modules: ok (cached)\n")
		fmt.Printf("  type check: ok (cached)\n")
		printTypeWarnings(typeReport)
	} else {
		if options.Verbose {
			if cachePath, ok := programcache.Path(program, options.RawLang); ok {
				fmt.Printf("  program cache: miss (%s)\n", cachePath)
			}
		}
		var moduleReport modulesystem.Report
		resolvedProgram, moduleReport = resolver.ResolveProgram(program)
		if !moduleReport.Passed() {
			printModuleErrors(resolvedProgram, moduleReport)
			return fmt.Errorf("module resolution failed")
		}
		if options.Verbose {
			for _, module := range moduleReport.Modules {
				fmt.Printf("  import: %s -> %s (%s)\n", module.Name, module.Path, module.Kind)
			}
		}
		fmt.Printf("  modules: ok")
		if len(moduleReport.Modules) != 0 {
			fmt.Printf(" (%d import(s))", len(moduleReport.Modules))
		}
		fmt.Println()
		if options.Verbose {
			stats := resolver.Stats()
			fmt.Printf("  resolver cache: paths=%d program(s)=%d import-set(s)=%d\n", stats.ExistsEntries, stats.ProgramEntries, stats.ImportEntries)
		}

		typeReport = typechecker.CheckProgram(resolvedProgram)
		if !typeReport.Passed() {
			printTypeErrors(resolvedProgram, typeReport)
			return fmt.Errorf("type check failed")
		}
		fmt.Printf("  type check: ok\n")
		printTypeWarnings(typeReport)
	}

	parsedProgram := parser.ParseLoadedProgram(resolvedProgram)
	if !parsedProgram.Passed() {
		printContextErrors(langcontext.ParseErrors(resolvedProgram, parsedProgram))
		return fmt.Errorf("parse failed")
	}
	fmt.Printf("  parse: ok\n")
	if !cacheHit {
		_ = programcache.Store(resolvedProgram, options.RawLang, warningsToCache(typeReport.Warnings))
	}

	if !options.Run {
		return nil
	}

	started := time.Now()
	fmt.Printf("  system: os=%s arch=%s cpus=%d go=%s\n", stdruntime.GOOS, stdruntime.GOARCH, stdruntime.NumCPU(), stdruntime.Version())
	result, err := runtime.NewWithArgs(options.ProgramArgs).Run(parsedProgram)
	elapsed := time.Since(started)
	if err != nil {
		printContextErrors([]langcontext.ErrorContext{langcontext.RuntimeError(resolvedProgram, err)})
		return fmt.Errorf("runtime failed: %w", err)
	}
	for _, line := range result.Output {
		fmt.Println(line)
	}
	fmt.Printf("  runtime: returned %s\n", describeValue(result.Value))
	fmt.Printf("  time: %s\n", elapsed.Round(time.Microsecond))
	if options.Verbose {
		fmt.Printf("  memory: stack=%d object(s)/%d byte(s), heap=%d object(s)/%d byte(s)\n",
			result.Memory.StackObjects, result.Memory.StackBytes,
			result.Memory.HeapObjects, result.Memory.HeapBytes)
	}
	return nil
}

func warningsToCache(warnings []typechecker.Warning) []programcache.Warning {
	if len(warnings) == 0 {
		return nil
	}
	cached := make([]programcache.Warning, 0, len(warnings))
	for _, warning := range warnings {
		cached = append(cached, programcache.Warning{
			File:    warning.File,
			Line:    warning.Line,
			Message: warning.Message,
		})
	}
	return cached
}

func warningsFromCache(warnings []programcache.Warning) []typechecker.Warning {
	if len(warnings) == 0 {
		return nil
	}
	restored := make([]typechecker.Warning, 0, len(warnings))
	for _, warning := range warnings {
		restored = append(restored, typechecker.Warning{
			File:    warning.File,
			Line:    warning.Line,
			Message: warning.Message,
		})
	}
	return restored
}

func printModuleErrors(program file.Program, report modulesystem.Report) {
	printContextErrors(langcontext.ModuleErrors(program, report))
}

func printTypeErrors(program file.Program, report typechecker.Report) {
	printContextErrors(langcontext.TypeErrors(program, report))
}

func printTypeWarnings(report typechecker.Report) {
	for _, warning := range report.Warnings {
		fmt.Printf("  warning: %s:%d: %s\n", warning.File, warning.Line, warning.Message)
	}
}

func printContextErrors(errors []langcontext.ErrorContext) {
	for _, err := range errors {
		printDiagnostic(os.Stderr, err)
	}
}

func printDiagnostic(out *os.File, diag langcontext.ErrorContext) {
	location := diag.File
	if diag.Line > 0 {
		location = fmt.Sprintf("%s:%d:%d", diag.File, diag.Line, maxInt(diag.Column, 1))
	}
	kind := string(diag.Phase) + " ERROR"
	fmt.Fprintf(out, "\n-- %s %s\n\n", kind, strings.Repeat("-", maxInt(1, 72-len(kind))))
	fmt.Fprintf(out, "%s\n\n", location)
	if diag.Rule != "" {
		fmt.Fprintf(out, "Rule: %s\n\n", diag.Rule)
	}
	fmt.Fprintf(out, "%s\n\n", diag.Message)
	if diag.Line > 0 && diag.SourceLine != "" {
		width := len(strconv.Itoa(diag.Line))
		fmt.Fprintf(out, "%*d | %s\n", width, diag.Line, diag.SourceLine)
		caretColumn := maxInt(diag.Column, 1)
		fmt.Fprintf(out, "%*s | %s^\n\n", width, "", strings.Repeat(" ", maxInt(0, caretColumn-1)))
	}
	if diag.Hint != "" {
		fmt.Fprintf(out, "Hint: %s\n\n", diag.Hint)
	}
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

func describeValue(value runtime.Value) string {
	if value.Kind == runtime.ValueNull {
		return "Null"
	}
	if value.Data == nil {
		return string(value.Kind)
	}
	return fmt.Sprintf("%s(%v)", value.Kind, value.Data)
}

func newProjectEntrySource() string {
	return `import "app";

function Main() : Int {
    return App.Start();
}
`
}

func newProjectModuleSource(projectName string, entry entrySpec) string {
	if entry.Name == "" {
		entry.Name = "Start"
		entry.Type = "Int"
	}
	if entry.Type == "" {
		return fmt.Sprintf(`namespace App {
    #set_entry_point_to_here
    function %s() {
        print("Welcome to %s");
    }
}
`, entry.Name, escapeKlangString(projectName))
	}
	return fmt.Sprintf(`namespace App {
    #set_entry_point_to_here
    function %s() : %s {
        print("Welcome to %s");
        return %s;
    }
}
`, entry.Name, entry.Type, escapeKlangString(projectName), entryReturnValue(entry.Type))
}

func entryReturnValue(typeName string) string {
	switch strings.ToLower(strings.TrimSpace(typeName)) {
	case "bool":
		return "False"
	case "float":
		return "0.0"
	case "string":
		return `""`
	case "char":
		return `" "[0]`
	default:
		return "0"
	}
}

func parseEntryFlag(args []string) (entrySpec, error) {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		value := ""
		switch {
		case strings.HasPrefix(arg, "--entry="):
			value = strings.TrimSpace(strings.TrimPrefix(arg, "--entry="))
		case arg == "--entry" && index+1 < len(args):
			value = strings.TrimSpace(args[index+1])
		default:
			continue
		}
		return parseEntrySpec(value)
	}
	return entrySpec{}, nil
}

func parseEntrySpec(value string) (entrySpec, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, `'`, "")
	if value == "" {
		return entrySpec{}, nil
	}
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	if len(parts) == 1 {
		return entrySpec{Name: parts[0]}, nil
	}
	if len(parts) >= 2 {
		return entrySpec{Name: parts[0], Type: parts[1]}, nil
	}
	return entrySpec{}, fmt.Errorf("--entry expects a function name or [name,type]")
}

func parsePackageOptions(args []string) (packageOptions, error) {
	options := packageOptions{Backend: "Standalone"}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case strings.HasPrefix(arg, "--backend="):
			options.Backend = strings.TrimSpace(strings.TrimPrefix(arg, "--backend="))
		case arg == "--backend" && index+1 < len(args):
			index++
			options.Backend = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--out="):
			options.Out = strings.TrimSpace(strings.TrimPrefix(arg, "--out="))
		case arg == "--out" && index+1 < len(args):
			index++
			options.Out = strings.TrimSpace(args[index])
		case arg == "--serve":
			options.Serve = true
		case strings.HasPrefix(arg, "--host="):
			options.Host = strings.TrimSpace(strings.TrimPrefix(arg, "--host="))
		case arg == "--host" && index+1 < len(args):
			index++
			options.Host = strings.TrimSpace(args[index])
		case strings.HasPrefix(arg, "--port="):
			port, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(arg, "--port=")))
			if err != nil {
				return packageOptions{}, fmt.Errorf("--port expects a number")
			}
			options.Port = port
			options.PortSet = true
		case arg == "--port" && index+1 < len(args):
			index++
			port, err := strconv.Atoi(strings.TrimSpace(args[index]))
			if err != nil {
				return packageOptions{}, fmt.Errorf("--port expects a number")
			}
			options.Port = port
			options.PortSet = true
		}
	}
	if !isBuildBackend(options.Backend) {
		return packageOptions{}, fmt.Errorf("backend must be one of WASM, JS, Standalone")
	}
	return options, nil
}

func parseDocOptions(args []string) (docOptions, error) {
	options := docOptions{}
	for index := 0; index < len(args); index++ {
		arg := args[index]
		switch {
		case strings.HasPrefix(arg, "--sourcefile="):
			files, err := parseDocSourceFiles(strings.TrimSpace(strings.TrimPrefix(arg, "--sourcefile=")))
			if err != nil {
				return docOptions{}, err
			}
			options.SourceFiles = append(options.SourceFiles, files...)
		case arg == "--sourcefile" && index+1 < len(args):
			index++
			files, err := parseDocSourceFiles(args[index])
			if err != nil {
				return docOptions{}, err
			}
			options.SourceFiles = append(options.SourceFiles, files...)
		case strings.HasPrefix(arg, "--out="):
			options.Out = strings.TrimSpace(strings.TrimPrefix(arg, "--out="))
		case arg == "--out" && index+1 < len(args):
			index++
			options.Out = strings.TrimSpace(args[index])
		}
	}
	if len(options.SourceFiles) == 0 {
		return docOptions{}, fmt.Errorf("%s doc expects --sourcefile=[file.klang,...]", cliName)
	}
	return options, nil
}

func parseDocSourceFiles(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	value = strings.Trim(value, "[]")
	value = strings.ReplaceAll(value, `"`, "")
	value = strings.ReplaceAll(value, `'`, "")
	if value == "" {
		return nil, fmt.Errorf("--sourcefile expects at least one .klang file")
	}
	parts := strings.Split(value, ",")
	files := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		files = append(files, part)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("--sourcefile expects at least one .klang file")
	}
	return files, nil
}

func moduleDirectiveSignature(stmt parser.ModuleDirectiveStatement) string {
	parts := make([]string, 0, len(stmt.Options))
	for key, value := range stmt.Options {
		parts = append(parts, fmt.Sprintf("%s : %t", key, value))
	}
	return fmt.Sprintf("%s(%s)", stmt.Name, strings.Join(parts, ", "))
}

func functionDocKind(fn parser.FunctionStatement) string {
	if fn.Private {
		return "private function"
	}
	if fn.Async {
		return "async function"
	}
	if fn.Lazy {
		return "lazy function"
	}
	if fn.Inline {
		return "inline function"
	}
	return "function"
}

func functionDocSignature(namespace string, fn parser.FunctionStatement) string {
	prefix := ""
	if fn.Private {
		prefix += "private "
	}
	if fn.Inline {
		prefix += "inline "
	}
	if fn.Lazy {
		prefix += "lazy "
	}
	if fn.Async {
		prefix += "async "
	}
	params := make([]string, 0, len(fn.Params))
	for _, param := range fn.Params {
		params = append(params, fmt.Sprintf("%s%s : %s", parameterModifier(param), param.Name, param.Type))
	}
	return fmt.Sprintf("%sfunction %s(%s) : %s", prefix, namespace+fn.Name, strings.Join(params, ", "), fn.ReturnType)
}

func aliasFunctionDocSignature(namespace string, alias parser.AliasFunctionStatement) string {
	params := make([]string, 0, len(alias.Params))
	for _, param := range alias.Params {
		params = append(params, fmt.Sprintf("%s%s : %s", parameterModifier(param), param.Name, param.Type))
	}
	return fmt.Sprintf("alias function %s(%s) : %s", namespace+alias.Name, strings.Join(params, ", "), alias.ReturnType)
}

func parameterModifier(param parser.Parameter) string {
	if param.ByRef {
		return "ref "
	}
	if param.Mutable {
		return "mut "
	}
	return ""
}

func aliasFunctionDetail(alias parser.AliasFunctionStatement) string {
	parts := []string{}
	if alias.Struct {
		parts = append(parts, "struct")
	}
	if alias.Inline {
		parts = append(parts, "inline")
	}
	if alias.Private {
		parts = append(parts, "private")
	}
	if len(alias.Methods) != 0 {
		parts = append(parts, fmt.Sprintf("%d extension method(s)", len(alias.Methods)))
	}
	return strings.Join(parts, ", ")
}

func variableDocKind(stmt parser.VariableStatement) string {
	if stmt.Scope == "const" {
		return "const"
	}
	if stmt.Exported {
		return "exported variable"
	}
	return "global variable"
}

func variableDocSignature(namespace string, stmt parser.VariableStatement) string {
	mut := ""
	if stmt.Mutable {
		mut = "mut "
	}
	if stmt.Scope == "const" {
		return fmt.Sprintf("const %s", namespace+stmt.Name)
	}
	if stmt.Exported {
		return fmt.Sprintf("export %s %s%s %s", stmt.Scope, mut, stmt.Type, namespace+stmt.Name)
	}
	return fmt.Sprintf("%s %s%s %s", stmt.Scope, mut, stmt.Type, namespace+stmt.Name)
}

func enumVariantSummary(variants []parser.EnumVariant) string {
	parts := make([]string, 0, len(variants))
	for _, variant := range variants {
		parts = append(parts, fmt.Sprintf("%s=%d", variant.Name, variant.Ordinal))
	}
	return strings.Join(parts, ", ")
}

func isBuildBackend(value string) bool {
	switch value {
	case "WASM", "JS", "Standalone":
		return true
	default:
		return false
	}
}

func projectNameFromPath(path string) string {
	name := filepath.Base(path)
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "Klang"
	}
	return name
}

func escapeKlangString(value string) string {
	replacer := strings.NewReplacer(`\`, `\\`, `"`, `\"`)
	return replacer.Replace(value)
}

func positionalArgs(args []string) []string {
	values := make([]string, 0, len(args))
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--entry" {
			index++
			continue
		}
		if arg == "--backend" || arg == "--out" {
			index++
			continue
		}
		if arg == "--host" || arg == "--port" {
			index++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		values = append(values, arg)
	}
	return values
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

func printUsage() {
	fmt.Print(usageText())
}

func usageText() string {
	return `Klang CLI

Usage:
  kLang new <project-path>                    Create a folder-based Klang project
  kLang new <project-path> --entry=[Name,Int] Create a project with a custom entry point
  kLang run <file-or-folder>                  Check, parse, and execute a Klang program
  kLang check <file-or-folder>                Resolve modules, type check, and parse
  kLang package <file-or-folder>              Package checked source into a compact bundle
  kLang serve <file-or-folder>                Package and serve a browser WASM runtime bundle
  kLang doc --sourcefile=[file.klang,...]     Generate static HTML source documentation
  kLang test <tests-folder>                   Check every Klang program in a folder
  kLang test <tests-folder> --run             Check and run every discovered program
  kLang file <file.klang>                     Print a Klang source file with line labels

Options:
  --run                           Run programs after checks, for test mode
  --entry=[Name,Type]              Set generated project entry point for new projects
  --backend=Standalone|JS|WASM      Select package backend metadata
  --out=<folder>                    Select package output folder
  --sourcefile=[file.klang,...]      Select one or more source files for docs
  --serve                         Serve a WASM browser bundle after packaging
  --host=<host>                    Host for the built-in web server, default 127.0.0.1
  --port=<port>                    Port for the built-in web server, default 8080
  --raw-lang                      Disable stdlib imports while resolving modules
  --verbose, -v                   Print import details
  --help, -h                      Show this help

Legacy flags still work:
  kLang --program <file-or-folder> [--run]
  kLang --tests <tests-folder> [--run]
  kLang --file <file.klang>
`
}
