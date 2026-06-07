package file

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const KlangExtension = ".klang"
const KlangEntryPoint = "first.klang"

type SourceFile struct {
	Path  string
	Lines []string
}

type TestProgram struct {
	Name       string
	Root       string
	EntryPoint string
	Files      []SourceFile
}

func FileExists(file string) bool {
	info, err := os.Stat(file)
	return err == nil && !info.IsDir()
}

func LinesFrom(file string) []string {
	lines, _ := ReadLines(file)
	return lines
}

func ReadLines(file string) ([]string, error) {
	var lines []string
	content, err := os.Open(file)
	if err != nil {
		return lines, err
	}
	defer content.Close()

	scanner := bufio.NewScanner(content)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return lines, err
	}

	return lines, nil
}

func DiscoverTestPrograms(testsDir string) ([]TestProgram, error) {
	var programs []TestProgram

	entries, err := os.ReadDir(testsDir)
	if err != nil {
		return programs, err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(testsDir, entry.Name())

		if entry.IsDir() {
			program, err := readDirectoryProgram(entryPath)
			if err != nil {
				return nil, err
			}
			if program.EntryPoint != "" {
				programs = append(programs, program)
			}
			continue
		}

		if filepath.Ext(entry.Name()) != KlangExtension {
			continue
		}

		source, err := readSourceFile(entryPath)
		if err != nil {
			return nil, err
		}

		programs = append(programs, TestProgram{
			Name:       strings.TrimSuffix(entry.Name(), KlangExtension),
			Root:       testsDir,
			EntryPoint: entryPath,
			Files:      []SourceFile{source},
		})
	}

	sort.Slice(programs, func(left, right int) bool {
		return naturalPathLess(programs[left].EntryPoint, programs[right].EntryPoint)
	})

	return programs, nil
}

func readDirectoryProgram(programDir string) (TestProgram, error) {
	entryPoint := filepath.Join(programDir, KlangEntryPoint)
	if !FileExists(entryPoint) {
		return TestProgram{}, nil
	}

	var paths []string
	err := filepath.WalkDir(programDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != KlangExtension {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return TestProgram{}, err
	}

	sort.Slice(paths, func(left, right int) bool {
		if paths[left] == entryPoint {
			return true
		}
		if paths[right] == entryPoint {
			return false
		}
		return naturalPathLess(paths[left], paths[right])
	})

	files := make([]SourceFile, 0, len(paths))
	for _, path := range paths {
		source, err := readSourceFile(path)
		if err != nil {
			return TestProgram{}, err
		}
		files = append(files, source)
	}

	return TestProgram{
		Name:       filepath.Base(programDir),
		Root:       programDir,
		EntryPoint: entryPoint,
		Files:      files,
	}, nil
}

func readSourceFile(path string) (SourceFile, error) {
	lines, err := ReadLines(path)
	if err != nil {
		return SourceFile{}, err
	}

	return SourceFile{
		Path:  path,
		Lines: lines,
	}, nil
}

func naturalPathLess(left string, right string) bool {
	leftIndex := 0
	rightIndex := 0

	for leftIndex < len(left) && rightIndex < len(right) {
		leftChar := left[leftIndex]
		rightChar := right[rightIndex]

		if isDigit(leftChar) && isDigit(rightChar) {
			leftNumber, nextLeft := readNumber(left, leftIndex)
			rightNumber, nextRight := readNumber(right, rightIndex)
			if leftNumber != rightNumber {
				return leftNumber < rightNumber
			}
			leftIndex = nextLeft
			rightIndex = nextRight
			continue
		}

		if leftChar != rightChar {
			return leftChar < rightChar
		}

		leftIndex++
		rightIndex++
	}

	return len(left) < len(right)
}

func readNumber(value string, start int) (int, int) {
	number := 0
	index := start
	for index < len(value) && isDigit(value[index]) {
		number = number*10 + int(value[index]-'0')
		index++
	}
	return number, index
}

func isDigit(value byte) bool {
	return value >= '0' && value <= '9'
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
