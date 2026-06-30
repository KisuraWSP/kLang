package programcache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"kLang/src/engine/file"
	"kLang/src/lexer"
)

const Version = "klang-program-cache-v10-lazy-pipelines"

type Warning struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Message string `json:"message"`
}

type Entry struct {
	Version         string         `json:"version"`
	Key             string         `json:"key"`
	RawLang         bool           `json:"raw_lang"`
	Name            string         `json:"name"`
	Root            string         `json:"root"`
	EntryPoint      string         `json:"entry_point"`
	LanguageVersion int            `json:"language_version"`
	Files           []CachedSource `json:"files"`
	Warnings        []Warning      `json:"warnings,omitempty"`
}

type CachedSource struct {
	Path                 string   `json:"path"`
	Lines                []string `json:"lines"`
	OriginalLines        []string `json:"original_lines,omitempty"`
	Language             string   `json:"language,omitempty"`
	ModuleFunctionFilter []string `json:"module_function_filter,omitempty"`
	Size                 int64    `json:"size"`
	SHA256               string   `json:"sha256"`
}

func Load(program file.Program, rawLang bool) (file.Program, Entry, bool) {
	if HasNoCache(program) {
		return file.Program{}, Entry{}, false
	}
	entryPath, key, ok := cachePath(program, rawLang)
	if !ok {
		return file.Program{}, Entry{}, false
	}
	content, err := os.ReadFile(entryPath)
	if err != nil {
		return file.Program{}, Entry{}, false
	}

	var entry Entry
	if err := json.Unmarshal(content, &entry); err != nil {
		return file.Program{}, Entry{}, false
	}
	if entry.Version != Version || entry.Key != key || entry.RawLang != rawLang {
		return file.Program{}, Entry{}, false
	}
	if cachedEntryHasNoCache(entry) {
		return file.Program{}, Entry{}, false
	}

	cachedFiles := make(map[string]CachedSource, len(entry.Files))
	for _, cached := range entry.Files {
		cachedFiles[filepath.Clean(cached.Path)] = cached
	}
	for _, source := range program.Files {
		cached, exists := cachedFiles[filepath.Clean(source.Path)]
		if !exists {
			return file.Program{}, Entry{}, false
		}
		hash, size, err := fingerprintFile(source.Path)
		if err != nil || hash != cached.SHA256 || size != cached.Size {
			return file.Program{}, Entry{}, false
		}
	}

	files := make([]file.SourceFile, 0, len(entry.Files))
	for _, cached := range entry.Files {
		hash, size, err := fingerprintFile(cached.Path)
		if err != nil || hash != cached.SHA256 || size != cached.Size {
			return file.Program{}, Entry{}, false
		}
		files = append(files, file.SourceFile{
			Path:                 cached.Path,
			Lines:                append([]string(nil), cached.Lines...),
			OriginalLines:        append([]string(nil), cached.OriginalLines...),
			Language:             cached.Language,
			ModuleFunctionFilter: listToFilter(cached.ModuleFunctionFilter),
		})
	}

	return file.Program{
		Name:            entry.Name,
		Root:            entry.Root,
		EntryPoint:      entry.EntryPoint,
		LanguageVersion: entry.LanguageVersion,
		Files:           files,
	}, entry, true
}

func Store(program file.Program, rawLang bool, warnings []Warning) error {
	if HasNoCache(program) {
		return nil
	}
	entryPath, key, ok := cachePath(program, rawLang)
	if !ok {
		return nil
	}

	files := make([]CachedSource, 0, len(program.Files))
	for _, source := range program.Files {
		hash, size, err := fingerprintFile(source.Path)
		if err != nil {
			return err
		}
		files = append(files, CachedSource{
			Path:                 source.Path,
			Lines:                append([]string(nil), source.Lines...),
			OriginalLines:        append([]string(nil), source.OriginalLines...),
			Language:             source.Language,
			ModuleFunctionFilter: filterToList(source.ModuleFunctionFilter),
			Size:                 size,
			SHA256:               hash,
		})
	}

	entry := Entry{
		Version:         Version,
		Key:             key,
		RawLang:         rawLang,
		Name:            program.Name,
		Root:            program.Root,
		EntryPoint:      program.EntryPoint,
		LanguageVersion: program.LanguageVersion,
		Files:           files,
		Warnings:        append([]Warning(nil), warnings...),
	}
	content, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')

	if err := os.MkdirAll(filepath.Dir(entryPath), 0755); err != nil {
		return err
	}
	tempPath := entryPath + ".tmp"
	if err := os.WriteFile(tempPath, content, 0644); err != nil {
		return err
	}
	return os.Rename(tempPath, entryPath)
}

func Path(program file.Program, rawLang bool) (string, bool) {
	path, _, ok := cachePath(program, rawLang)
	return path, ok
}

func HasNoCache(program file.Program) bool {
	for _, source := range program.Files {
		if sourceHasNoCache(source.Lines) {
			return true
		}
	}
	return false
}

func cachedEntryHasNoCache(entry Entry) bool {
	for _, source := range entry.Files {
		if sourceHasNoCache(source.Lines) {
			return true
		}
	}
	return false
}

func sourceHasNoCache(lines []string) bool {
	tokens := lexer.New(strings.Join(lines, "\n")).Tokenize()
	for index, token := range tokens {
		if token.Type != lexer.TokenIdentifier || token.Literal != "no_cache" {
			continue
		}
		if index+1 >= len(tokens) || tokens[index+1].Type == lexer.TokenSemicolon || tokens[index+1].Type == lexer.TokenEOFDescriptor {
			return true
		}
	}
	return false
}

func cachePath(program file.Program, rawLang bool) (string, string, bool) {
	root := program.Root
	if root == "" {
		root = "."
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", "", false
	}
	entry := program.EntryPoint
	if entry == "" && len(program.Files) != 0 {
		entry = program.Files[0].Path
	}
	absEntry, err := filepath.Abs(entry)
	if err != nil {
		return "", "", false
	}

	key := cacheKey(absRoot, absEntry, program.LanguageVersion, rawLang)
	return filepath.Join(absRoot, ".klang-cache", key+".json"), key, true
}

func cacheKey(root string, entry string, languageVersion int, rawLang bool) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{
		Version,
		filepath.Clean(root),
		filepath.Clean(entry),
		strconv.Itoa(languageVersion),
		strconv.FormatBool(rawLang),
	}, "\x00")))
	return hex.EncodeToString(sum[:])
}

func fingerprintFile(path string) (string, int64, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), int64(len(content)), nil
}

func filterToList(filter map[string]bool) []string {
	if len(filter) == 0 {
		return nil
	}
	items := make([]string, 0, len(filter))
	for name, enabled := range filter {
		if enabled {
			items = append(items, name)
		}
	}
	sort.Strings(items)
	return items
}

func listToFilter(items []string) map[string]bool {
	if len(items) == 0 {
		return nil
	}
	filter := make(map[string]bool, len(items))
	for _, item := range items {
		filter[item] = true
	}
	return filter
}
