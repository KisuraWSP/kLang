package main

import (
	"fmt"
	"os"

	"kLang/src/engine/file"
	typechecker "kLang/src/engine/type_checker"
)

func main() {
	args := os.Args[1:]

	testsPath := file.GetTestsPath(args)
	if testsPath != "" {
		programs, err := file.DiscoverPrograms(testsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read tests: %v\n", err)
			os.Exit(1)
		}

		printPrograms(programs)
		checkPrograms(programs)
		return
	}

	programPath := file.GetProgramPath(args)
	if programPath != "" {
		program, err := file.LoadProgram(programPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read program: %v\n", err)
			os.Exit(1)
		}

		programs := []file.Program{program}
		printPrograms(programs)
		checkPrograms(programs)
		return
	}

	filePath := file.GetFilePath(args)
	if filePath != "" {
		file.PrintFile(filePath)
		return
	}

	fmt.Println("usage: kLang --program path/to/file-or-folder | --tests tests | --file path/to/file.klang")
}

func printPrograms(programs []file.Program) {
	for _, program := range programs {
		fmt.Printf("%s -> %s (%d file(s))\n", program.Name, program.EntryPoint, len(program.Files))
	}
}

func checkPrograms(programs []file.Program) {
	hasErrors := false
	for _, program := range programs {
		report := typechecker.CheckProgram(program)
		if report.Passed() {
			fmt.Printf("%s type check: ok\n", program.Name)
			continue
		}

		hasErrors = true
		fmt.Printf("%s type check: failed\n", program.Name)
		for _, err := range report.Errors {
			fmt.Printf("%s:%d: %s\n", err.File, err.Line, err.Message)
		}
	}

	if hasErrors {
		os.Exit(1)
	}
}
