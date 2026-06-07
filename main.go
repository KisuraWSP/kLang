package main

import (
	"fmt"
	"os"
	"path/filepath"
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
		printModuleErrors(moduleReport)
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
		printTypeErrors(typeReport)
		return fmt.Errorf("type check failed")
	}
	fmt.Printf("  type check: ok\n")

	parsedProgram := parser.ParseLoadedProgram(resolvedProgram)
	if !parsedProgram.Passed() {
		for _, err := range parsedProgram.Errors() {
			fmt.Fprintf(os.Stderr, "  parse error: %v\n", err)
		}
		return fmt.Errorf("parse failed")
	}
	fmt.Printf("  parse: ok\n")

	if !options.Run {
		return nil
	}

	result, err := runtime.New().Run(parsedProgram)
	if err != nil {
		return fmt.Errorf("runtime failed: %w", err)
	}
	for _, line := range result.Output {
		fmt.Println(line)
	}
	fmt.Printf("  runtime: returned %s\n", describeValue(result.Value))
	return nil
}

func printModuleErrors(report modulesystem.Report) {
	for _, err := range report.Errors {
		fmt.Fprintf(os.Stderr, "  %s:%d:%d: %s\n", err.File, err.Line, err.Column, err.Message)
	}
}

func printTypeErrors(report typechecker.Report) {
	for _, err := range report.Errors {
		fmt.Fprintf(os.Stderr, "  %s:%d: %s\n", err.File, err.Line, err.Message)
	}
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
