package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
)

const cliName = "kLang"

type commandOptions struct {
	Run     bool
	Verbose bool
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
	}
	values := positionalArgs(rest)

	switch command {
	case "new", "init":
		if len(values) != 1 {
			return fmt.Errorf("%s %s expects a project path", cliName, command)
		}
		return createProject(values[0])
	case "run":
		if len(values) != 1 {
			return fmt.Errorf("%s run expects a .klang file or project folder", cliName)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		return executePrograms([]file.Program{program}, commandOptions{Run: true, Verbose: options.Verbose})
	case "check":
		if len(values) != 1 {
			return fmt.Errorf("%s check expects a .klang file or project folder", cliName)
		}
		program, err := file.LoadProgram(values[0])
		if err != nil {
			return err
		}
		return executePrograms([]file.Program{program}, commandOptions{Run: false, Verbose: options.Verbose})
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
		return true, executePrograms(programs, commandOptions{Run: file.HasRunFlag(args), Verbose: true})
	}

	programPath := file.GetProgramPath(args)
	if programPath != "" {
		program, err := file.LoadProgram(programPath)
		if err != nil {
			return true, fmt.Errorf("failed to read program: %w", err)
		}
		return true, executePrograms([]file.Program{program}, commandOptions{Run: file.HasRunFlag(args), Verbose: true})
	}

	filePath := file.GetFilePath(args)
	if filePath != "" {
		file.PrintFile(filePath)
		return true, nil
	}

	return false, nil
}

func createProject(projectPath string) error {
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
		"app.klang":          newProjectModuleSource(projectNameFromPath(cleanPath)),
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

func executePrograms(programs []file.Program, options commandOptions) error {
	if len(programs) == 0 {
		return fmt.Errorf("no Klang programs found")
	}

	failed := false
	resolver := modulesystem.NewResolver("")
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

	result, err := runtime.New().Run(parsedProgram)
	if err != nil {
		printRuntimeError(resolvedProgram, err)
		return fmt.Errorf("runtime failed: %w", err)
	}
	for _, line := range result.Output {
		fmt.Println(line)
	}
	fmt.Printf("  runtime: returned %s\n", describeValue(result.Value))
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

func newProjectModuleSource(projectName string) string {
	return fmt.Sprintf(`namespace App {
    function Start() : Int {
        print("Welcome to %s");
        return 0;
    }
}
`, escapeKlangString(projectName))
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
  kLang new <project-path>        Create a folder-based Klang project
  kLang run <file-or-folder>      Check, parse, and execute a Klang program
  kLang check <file-or-folder>    Resolve modules, type check, and parse
  kLang test <tests-folder>       Check every Klang program in a folder
  kLang test <tests-folder> --run Check and run every discovered program
  kLang file <file.klang>         Print a Klang source file with line labels

Options:
  --run                           Run programs after checks, for test mode
  --verbose, -v                   Print import details
  --help, -h                      Show this help

Legacy flags still work:
  kLang --program <file-or-folder> [--run]
  kLang --tests <tests-folder> [--run]
  kLang --file <file.klang>
`
}
