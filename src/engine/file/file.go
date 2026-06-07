package file

import (
	"bufio"
	"fmt"
	"log"
	"os"
)

func FileExists(file string) bool {
	_, err := os.ReadFile(file)
	if err != nil {
		log.Fatal(err)
		return false
	}
	return true
}

func LinesFrom(file string) []string {
	if !FileExists(file) {
		return nil
	}

	var lines []string
	content, err := os.Open(file)
	if err != nil {
		return lines
	}
	defer content.Close()

	scanner := bufio.NewScanner(content)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())

		if scanner.Err() != nil {
			log.Fatal("Scanner Failed")
		}
	}
	return lines
}

func PrintFile(file string) {
	var lines = LinesFrom(file)

	for key, value := range lines {
		fmt.Printf("line[%d], code[ %s ]\n", key, value)
	}
}

func PrintFileLineCount(file string) {
	var lines = LinesFrom(file)

	for line := range lines {
		fmt.Printf("line_count:= %d", line)
	}
}
