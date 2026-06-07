package main

import (
	"fmt"
	"os"

	"kLang/src/engine/file"
)

func main() {
	args := os.Args[1:]

	testsPath := file.GetTestsPath(args)
	if testsPath != "" {
		programs, err := file.DiscoverTestPrograms(testsPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to read tests: %v\n", err)
			os.Exit(1)
		}

		for _, program := range programs {
			fmt.Printf("%s -> %s (%d file(s))\n", program.Name, program.EntryPoint, len(program.Files))
		}
		return
	}

	filePath := file.GetFilePath(args)
	if filePath != "" {
		file.PrintFile(filePath)
		return
	}

	fmt.Println("usage: kLang --tests tests | --file path/to/file.klang")
}
