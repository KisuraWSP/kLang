package file

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"kLang/src/lexer"
)

const KlangExtension = ".klang"
const KlangEntryPoint = "first.klang"
const KlangProjectFile = "klang.project"
const KlangDistDir = "dist"
const CurrentLanguageVersion = 1

type SourceFile struct {
	Path                 string
	Lines                []string
	ModuleFunctionFilter map[string]bool
}

type Program struct {
	Name            string
	Root            string
	EntryPoint      string
	LanguageVersion int
	Files           []SourceFile
}

type ProjectManifest struct {
	Name            string
	Entry           string
	Sources         []string
	LanguageVersion int
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

	if filepath.Base(path) == KlangProjectFile {
		return readManifestProgram(path)
	}

	if filepath.Ext(path) != KlangExtension {
		return Program{}, fmt.Errorf("expected %s, a %s script with load_as_script;, or a project folder: %s", KlangProjectFile, KlangExtension, path)
	}

	source, err := readSourceFile(path)
	if err != nil {
		return Program{}, err
	}
	if !sourceHasDirective(source.Lines, "load_as_script") {
		return Program{}, fmt.Errorf("%s scripts must opt in with load_as_script; or be listed in %s", path, KlangProjectFile)
	}

	return Program{
		Name:       strings.TrimSuffix(filepath.Base(path), KlangExtension),
		Root:       filepath.Dir(path),
		EntryPoint: path,
		Files:      []SourceFile{source},
	}, nil
}

func LoadModuleProgram(path string) (Program, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Program{}, err
	}
	if info.IsDir() {
		return readLegacyDirectoryProgram(path, false)
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
			if !FileExists(filepath.Join(entryPath, KlangProjectFile)) &&
				!sourcePathHasDirective(filepath.Join(entryPath, KlangEntryPoint), "load_as_script") {
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
	manifestPath := filepath.Join(programDir, KlangProjectFile)
	if FileExists(manifestPath) {
		return readManifestProgram(manifestPath)
	}

	return readLegacyDirectoryProgram(programDir, true)
}

func readLegacyDirectoryProgram(programDir string, requireScriptOptIn bool) (Program, error) {
	entryPoint := filepath.Join(programDir, KlangEntryPoint)
	if !FileExists(entryPoint) {
		return Program{}, fmt.Errorf("program folder must contain %s: %s", KlangProjectFile, programDir)
	}
	if requireScriptOptIn {
		entrySource, err := readSourceFile(entryPoint)
		if err != nil {
			return Program{}, err
		}
		if !sourceHasDirective(entrySource.Lines, "load_as_script") {
			return Program{}, fmt.Errorf("legacy folder projects must add load_as_script; to %s or define %s", entryPoint, KlangProjectFile)
		}
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

func readManifestProgram(manifestPath string) (Program, error) {
	manifest, err := readProjectManifest(manifestPath)
	if err != nil {
		return Program{}, err
	}
	if manifest.LanguageVersion > CurrentLanguageVersion {
		return Program{}, fmt.Errorf(
			"%s requires language_version %d, but this kLang supports up to %d",
			manifestPath,
			manifest.LanguageVersion,
			CurrentLanguageVersion,
		)
	}
	programDir := filepath.Dir(manifestPath)
	if manifest.Entry == "" {
		manifest.Entry = KlangEntryPoint
	}
	entryPoint := filepath.Clean(filepath.Join(programDir, manifest.Entry))
	if !FileExists(entryPoint) {
		return Program{}, fmt.Errorf("%s entry %q does not exist", KlangProjectFile, manifest.Entry)
	}

	paths := make([]string, 0, len(manifest.Sources)+1)
	if len(manifest.Sources) == 0 {
		paths, err = discoverProjectSourcePaths(programDir)
		if err != nil {
			return Program{}, err
		}
	} else {
		seen := map[string]bool{}
		for _, source := range manifest.Sources {
			path := filepath.Clean(filepath.Join(programDir, source))
			if filepath.Ext(path) != KlangExtension {
				return Program{}, fmt.Errorf("%s source %q must be a %s file", KlangProjectFile, source, KlangExtension)
			}
			if !FileExists(path) {
				return Program{}, fmt.Errorf("%s source %q does not exist", KlangProjectFile, source)
			}
			if !seen[path] {
				paths = append(paths, path)
				seen[path] = true
			}
		}
		if !seen[entryPoint] {
			paths = append([]string{entryPoint}, paths...)
		}
	}

	sortProjectPaths(paths, entryPoint)
	files := make([]SourceFile, 0, len(paths))
	for _, path := range paths {
		source, err := readSourceFile(path)
		if err != nil {
			return Program{}, err
		}
		files = append(files, source)
	}
	name := manifest.Name
	if name == "" {
		name = filepath.Base(programDir)
	}
	return Program{
		Name:            name,
		Root:            programDir,
		EntryPoint:      entryPoint,
		LanguageVersion: manifest.LanguageVersion,
		Files:           files,
	}, nil
}

func discoverProjectSourcePaths(programDir string) ([]string, error) {
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
		if filepath.Ext(entry.Name()) == KlangExtension {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func sortProjectPaths(paths []string, entryPoint string) {
	sort.Slice(paths, func(left, right int) bool {
		if paths[left] == entryPoint {
			return true
		}
		if paths[right] == entryPoint {
			return false
		}
		return naturalPathLess(paths[left], paths[right])
	})
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

func readProjectManifest(path string) (ProjectManifest, error) {
	lines, err := ReadLines(path)
	if err != nil {
		return ProjectManifest{}, err
	}
	var manifest ProjectManifest
	for lineNumber, raw := range lines {
		line := stripTomlComment(strings.TrimSpace(raw))
		if line == "" || strings.HasPrefix(line, "[") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return ProjectManifest{}, fmt.Errorf("%s:%d: expected key = value", path, lineNumber+1)
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		switch key {
		case "name":
			manifest.Name, err = parseTomlString(value)
		case "entry":
			manifest.Entry, err = parseTomlString(value)
		case "sources":
			manifest.Sources, err = parseTomlStringArray(value)
		case "language_version":
			manifest.LanguageVersion, err = parseTomlInt(value)
		default:
			err = fmt.Errorf("unknown key %q", key)
		}
		if err != nil {
			return ProjectManifest{}, fmt.Errorf("%s:%d: %w", path, lineNumber+1, err)
		}
	}
	return manifest, nil
}

func ReadProjectManifest(path string) (ProjectManifest, error) {
	return readProjectManifest(path)
}

func stripTomlComment(line string) string {
	inString := false
	escaped := false
	for index, ch := range line {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if ch == '#' && !inString {
			return strings.TrimSpace(line[:index])
		}
	}
	return line
}

func parseTomlString(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return "", fmt.Errorf("expected TOML string")
	}
	return strings.ReplaceAll(value[1:len(value)-1], `\"`, `"`), nil
}

func parseTomlInt(value string) (int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("expected TOML integer")
	}
	number := 0
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, fmt.Errorf("expected non-negative TOML integer")
		}
		number = number*10 + int(character-'0')
	}
	return number, nil
}

func parseTomlStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if len(value) < 2 || value[0] != '[' || value[len(value)-1] != ']' {
		return nil, fmt.Errorf("expected TOML string array")
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	if inner == "" {
		return nil, nil
	}
	var values []string
	for _, part := range splitTomlArray(inner) {
		item, err := parseTomlString(strings.TrimSpace(part))
		if err != nil {
			return nil, err
		}
		values = append(values, item)
	}
	return values, nil
}

func splitTomlArray(input string) []string {
	var parts []string
	start := 0
	inString := false
	escaped := false
	for index, ch := range input {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if ch == ',' && !inString {
			parts = append(parts, input[start:index])
			start = index + 1
		}
	}
	parts = append(parts, input[start:])
	return parts
}

func sourcePathHasDirective(path string, name string) bool {
	source, err := readSourceFile(path)
	if err != nil {
		return false
	}
	return sourceHasDirective(source.Lines, name)
}

func sourceHasDirective(lines []string, name string) bool {
	tokens := lexer.New(strings.Join(lines, "\n")).Tokenize()
	for index, token := range tokens {
		if token.Type != lexer.TokenIdentifier || token.Literal != name {
			continue
		}
		if index+1 < len(tokens) && (tokens[index+1].Type == lexer.TokenSemicolon || tokens[index+1].Type == lexer.TokenEOFDescriptor) {
			return true
		}
	}
	return false
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
