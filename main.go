package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	stdruntime "runtime"
	"strconv"
	"strings"
	"time"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
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
		return fmt.Errorf("parse failed: %v", parsedProgram.Errors())
	}

	bundleDir := filepath.Join(outRoot, program.Name+"-"+strings.ToLower(backend))
	sourceDir := filepath.Join(bundleDir, "src")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		return err
	}

	manifestFiles := make([]string, 0, len(resolvedProgram.Files))
	for _, source := range resolvedProgram.Files {
		relativePath, err := filepath.Rel(resolvedProgram.Root, source.Path)
		if err != nil || strings.HasPrefix(relativePath, "..") {
			relativePath = filepath.Base(source.Path)
		}
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
		manifestFiles = append(manifestFiles, filepath.ToSlash(filepath.Join("src", filepath.Clean(relativePath))))
	}

	manifest := map[string]any{
		"project_name":    resolvedProgram.Name,
		"backend":         backend,
		"entry":           filepath.ToSlash(resolvedProgram.EntryPoint),
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

	fmt.Printf("packaged Klang project %s\n", resolvedProgram.Name)
	fmt.Printf("  backend: %s\n", backend)
	fmt.Printf("  files: %d\n", len(resolvedProgram.Files))
	fmt.Printf("  bundle: %s\n", bundleDir)
	fmt.Printf("  manifest: %s\n", filepath.Join(bundleDir, "klang-build.json"))
	return nil
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

	resolvedProgram, moduleReport := resolver.ResolveProgram(program)
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
		fmt.Printf("  cache: paths=%d program(s)=%d import-set(s)=%d\n", stats.ExistsEntries, stats.ProgramEntries, stats.ImportEntries)
	}

	typeReport := typechecker.CheckProgram(resolvedProgram)
	if !typeReport.Passed() {
		printTypeErrors(resolvedProgram, typeReport)
		return fmt.Errorf("type check failed")
	}
	fmt.Printf("  type check: ok\n")
	printTypeWarnings(typeReport)

	parsedProgram := parser.ParseLoadedProgram(resolvedProgram)
	if !parsedProgram.Passed() {
		for _, source := range parsedProgram.Sources {
			for _, err := range source.Errors {
				printDiagnostic(os.Stderr, diagnostic{
					Kind:    "PARSE ERROR",
					File:    source.Path,
					Line:    err.Line,
					Column:  err.Column,
					Message: err.Message,
					Help:    "The parser could not understand this part of the program. Check the syntax around the marked code.",
				}, sourceLines(resolvedProgram, source.Path))
			}
		}
		return fmt.Errorf("parse failed")
	}
	fmt.Printf("  parse: ok\n")

	if !options.Run {
		return nil
	}

	started := time.Now()
	fmt.Printf("  system: os=%s arch=%s cpus=%d go=%s\n", stdruntime.GOOS, stdruntime.GOARCH, stdruntime.NumCPU(), stdruntime.Version())
	result, err := runtime.NewWithArgs(options.ProgramArgs).Run(parsedProgram)
	elapsed := time.Since(started)
	if err != nil {
		printRuntimeError(resolvedProgram, err)
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

func printModuleErrors(program file.Program, report modulesystem.Report) {
	for _, err := range report.Errors {
		printDiagnostic(os.Stderr, diagnostic{
			Kind:    "MODULE ERROR",
			File:    err.File,
			Line:    err.Line,
			Column:  err.Column,
			Message: err.Message,
			Help:    "The module resolver could not load an import used by this file.",
		}, sourceLines(program, err.File))
	}
}

func printTypeErrors(program file.Program, report typechecker.Report) {
	for _, err := range report.Errors {
		printDiagnostic(os.Stderr, diagnostic{
			Kind:    "TYPE ERROR",
			File:    err.File,
			Line:    err.Line,
			Column:  1,
			Message: humanTypeMessage(err.Message),
			Help:    "I found a conflict between what this code produces and what the surrounding program expects.",
		}, sourceLines(program, err.File))
	}
}

func printTypeWarnings(report typechecker.Report) {
	for _, warning := range report.Warnings {
		fmt.Printf("  warning: %s:%d: %s\n", warning.File, warning.Line, warning.Message)
	}
}

type diagnostic struct {
	Kind    string
	File    string
	Line    int
	Column  int
	Message string
	Help    string
}

func printDiagnostic(out *os.File, diag diagnostic, lines []string) {
	location := diag.File
	if diag.Line > 0 {
		location = fmt.Sprintf("%s:%d:%d", diag.File, diag.Line, maxInt(diag.Column, 1))
	}
	fmt.Fprintf(out, "\n-- %s %s\n\n", diag.Kind, strings.Repeat("-", maxInt(1, 72-len(diag.Kind))))
	fmt.Fprintf(out, "%s\n\n", location)
	fmt.Fprintf(out, "%s\n\n", diag.Message)
	if diag.Line > 0 && diag.Line <= len(lines) {
		code := lines[diag.Line-1]
		width := len(strconv.Itoa(diag.Line))
		fmt.Fprintf(out, "%*d | %s\n", width, diag.Line, code)
		caretColumn := maxInt(diag.Column, 1)
		fmt.Fprintf(out, "%*s | %s^\n\n", width, "", strings.Repeat(" ", maxInt(0, caretColumn-1)))
	}
	if diag.Help != "" {
		fmt.Fprintf(out, "Hint: %s\n\n", diag.Help)
	}
}

func printRuntimeError(program file.Program, err error) {
	line, column, message := runtimeErrorParts(err)
	printDiagnostic(os.Stderr, diagnostic{
		Kind:    "RUNTIME ERROR",
		File:    program.EntryPoint,
		Line:    line,
		Column:  column,
		Message: message,
		Help:    "The program reached this code while running and could not continue safely.",
	}, sourceLines(program, program.EntryPoint))
}

func runtimeErrorParts(err error) (int, int, string) {
	message := err.Error()
	pattern := regexp.MustCompile(`line ([0-9]+):([0-9]+): (.*)`)
	matches := pattern.FindStringSubmatch(message)
	if len(matches) != 4 {
		return 0, 0, message
	}
	line, _ := strconv.Atoi(matches[1])
	column, _ := strconv.Atoi(matches[2])
	return line, column, matches[3]
}

func humanTypeMessage(message string) string {
	switch {
	case strings.Contains(message, "cannot assign"):
		return message + "\n\nThis value does not have the type declared for the variable."
	case strings.Contains(message, "argument") && strings.Contains(message, "expects"):
		return message + "\n\nThis function call is passing a value with an unexpected type."
	case strings.Contains(message, "unknown identifier"):
		return message + "\n\nThis name has not been declared in the current scope."
	default:
		return message
	}
}

func sourceLines(program file.Program, path string) []string {
	clean := filepath.Clean(path)
	for _, source := range program.Files {
		if filepath.Clean(source.Path) == clean {
			return source.Lines
		}
	}
	if lines, err := file.ReadLines(path); err == nil {
		return lines
	}
	return nil
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
		}
	}
	if !isBuildBackend(options.Backend) {
		return packageOptions{}, fmt.Errorf("backend must be one of WASM, JS, Standalone")
	}
	return options, nil
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
  kLang test <tests-folder>                   Check every Klang program in a folder
  kLang test <tests-folder> --run             Check and run every discovered program
  kLang file <file.klang>                     Print a Klang source file with line labels

Options:
  --run                           Run programs after checks, for test mode
  --entry=[Name,Type]              Set generated project entry point for new projects
  --backend=Standalone|JS|WASM      Select package backend metadata
  --out=<folder>                    Select package output folder
  --raw-lang                      Disable stdlib imports while resolving modules
  --verbose, -v                   Print import details
  --help, -h                      Show this help

Legacy flags still work:
  kLang --program <file-or-folder> [--run]
  kLang --tests <tests-folder> [--run]
  kLang --file <file.klang>
`
}
