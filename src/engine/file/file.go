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
const KlangDistDir = "dist"

type SourceFile struct {
	Path  string
	Lines []string
}

type Program struct {
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

func LoadProgram(path string) (Program, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Program{}, err
	}

	if info.IsDir() {
		return readDirectoryProgram(path)
	}

	if filepath.Ext(path) != KlangExtension {
		return Program{}, fmt.Errorf("expected a %s file or folder with %s: %s", KlangExtension, KlangEntryPoint, path)
	}

	source, err := readSourceFile(path)
	if err != nil {
		return Program{}, err
	}

	return Program{
		Name:       strings.TrimSuffix(filepath.Base(path), KlangExtension),
		Root:       filepath.Dir(path),
		EntryPoint: path,
		Files:      []SourceFile{source},
	}, nil
}

func DiscoverPrograms(root string) ([]Program, error) {
	var programs []Program

	entries, err := os.ReadDir(root)
	if err != nil {
		return programs, err
	}

	for _, entry := range entries {
		entryPath := filepath.Join(root, entry.Name())

		if entry.IsDir() {
			if !FileExists(filepath.Join(entryPath, KlangEntryPoint)) {
				continue
			}
			program, err := LoadProgram(entryPath)
			if err != nil {
				return nil, err
			}
			programs = append(programs, program)
			continue
		}

		if filepath.Ext(entry.Name()) != KlangExtension {
			continue
		}

		program, err := LoadProgram(entryPath)
		if err != nil {
			return nil, err
		}

		programs = append(programs, program)
	}

	sort.Slice(programs, func(left, right int) bool {
		return naturalPathLess(programs[left].EntryPoint, programs[right].EntryPoint)
	})

	return programs, nil
}

func DiscoverTestPrograms(testsDir string) ([]Program, error) {
	return DiscoverPrograms(testsDir)
}

func readDirectoryProgram(programDir string) (Program, error) {
	entryPoint := filepath.Join(programDir, KlangEntryPoint)
	if !FileExists(entryPoint) {
		return Program{}, fmt.Errorf("program folder must contain %s: %s", KlangEntryPoint, programDir)
	}

	var paths []string
	err := filepath.WalkDir(programDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != programDir && entry.Name() == KlangDistDir {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(entry.Name()) != KlangExtension {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return Program{}, err
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
			return Program{}, err
		}
		files = append(files, source)
	}

	return Program{
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
