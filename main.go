package main

import (
	"fmt"
	"os"

	"kLang/src/engine/file"
	modulesystem "kLang/src/engine/module_system"
	"kLang/src/engine/runtime"
	typechecker "kLang/src/engine/type_checker"
	"kLang/src/parser"
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
		checkPrograms(programs, file.HasRunFlag(args))
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
		checkPrograms(programs, file.HasRunFlag(args))
		return
	}

	filePath := file.GetFilePath(args)
	if filePath != "" {
		file.PrintFile(filePath)
		return
	}

	fmt.Println("usage: kLang --program path/to/file-or-folder [--run] | --tests tests [--run] | --file path/to/file.klang")
}

func printPrograms(programs []file.Program) {
	for _, program := range programs {
		fmt.Printf("%s -> %s (%d file(s))\n", program.Name, program.EntryPoint, len(program.Files))
	}
}

func checkPrograms(programs []file.Program, run bool) {
	hasErrors := false
	resolver := modulesystem.NewResolver("")
	for _, program := range programs {
		resolvedProgram, moduleReport := resolver.ResolveProgram(program)
		if !moduleReport.Passed() {
			hasErrors = true
			fmt.Printf("%s module resolution: failed\n", program.Name)
			for _, err := range moduleReport.Errors {
				fmt.Printf("%s:%d:%d: %s\n", err.File, err.Line, err.Column, err.Message)
			}
			continue
		}
		for _, module := range moduleReport.Modules {
			fmt.Printf("%s import: %s -> %s (%s)\n", program.Name, module.Name, module.Path, module.Kind)
		}

		report := typechecker.CheckProgram(resolvedProgram)
		if report.Passed() {
			fmt.Printf("%s type check: ok\n", program.Name)
			if run {
				parsedProgram := parser.ParseLoadedProgram(resolvedProgram)
				if !parsedProgram.Passed() {
					hasErrors = true
					fmt.Printf("%s parse: failed: %v\n", program.Name, parsedProgram.Errors())
					continue
				}
				result, err := runtime.New().Run(parsedProgram)
				if err != nil {
					hasErrors = true
					fmt.Printf("%s runtime: failed: %v\n", program.Name, err)
					continue
				}
				for _, line := range result.Output {
					fmt.Println(line)
				}
				fmt.Printf("%s runtime: returned %v\n", program.Name, result.Value.Data)
			}
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
