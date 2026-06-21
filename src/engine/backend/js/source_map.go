package jsbackend

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"

	"kLang/src/engine/ir"
)

const base64VLQ = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"

type generatedMapping struct {
	line         int
	column       int
	source       int
	originalLine int
	originalCol  int
}

type sourceMapBuilder struct {
	sources         []ir.Source
	index           map[string]int
	entries         []generatedMapping
	scannedBytes    int
	generatedLine   int
	generatedColumn int
}

func newSourceMapBuilder(sources []ir.Source) *sourceMapBuilder {
	builder := &sourceMapBuilder{sources: sources, index: map[string]int{}}
	for index, source := range sources {
		builder.index[filepath.Clean(source.Path)] = index
	}
	return builder
}

func (builder *sourceMapBuilder) mark(output *strings.Builder, position ir.Position) {
	if position.File == "" || position.Line <= 0 {
		return
	}
	source, ok := builder.index[filepath.Clean(position.File)]
	if !ok {
		return
	}
	builder.scan(output.String())
	originalColumn := position.Column - 1
	if originalColumn < 0 {
		originalColumn = 0
	}
	builder.entries = append(builder.entries, generatedMapping{
		line: builder.generatedLine, column: builder.generatedColumn, source: source,
		originalLine: position.Line - 1, originalCol: originalColumn,
	})
}

func (builder *sourceMapBuilder) scan(output string) {
	if builder.scannedBytes > len(output) {
		builder.scannedBytes = 0
		builder.generatedLine = 0
		builder.generatedColumn = 0
	}
	for _, current := range output[builder.scannedBytes:] {
		if current == '\n' {
			builder.generatedLine++
			builder.generatedColumn = 0
		} else {
			builder.generatedColumn++
		}
	}
	builder.scannedBytes = len(output)
}

func (builder *sourceMapBuilder) sourcePath(path string) string {
	if index, ok := builder.index[filepath.Clean(path)]; ok {
		return builder.sources[index].MapPath
	}
	return filepath.ToSlash(path)
}

func (builder *sourceMapBuilder) sourceLine(position ir.Position) string {
	index, ok := builder.index[filepath.Clean(position.File)]
	if !ok || position.Line <= 0 {
		return ""
	}
	lines := strings.Split(builder.sources[index].Content, "\n")
	if position.Line > len(lines) {
		return ""
	}
	return lines[position.Line-1]
}

func (builder *sourceMapBuilder) encode() []byte {
	sort.SliceStable(builder.entries, func(left, right int) bool {
		if builder.entries[left].line != builder.entries[right].line {
			return builder.entries[left].line < builder.entries[right].line
		}
		return builder.entries[left].column < builder.entries[right].column
	})
	paths := make([]string, len(builder.sources))
	contents := make([]string, len(builder.sources))
	for index, source := range builder.sources {
		paths[index] = source.MapPath
		contents[index] = source.Content
	}
	payload := struct {
		Version        int      `json:"version"`
		File           string   `json:"file"`
		SourceRoot     string   `json:"sourceRoot,omitempty"`
		Sources        []string `json:"sources"`
		SourcesContent []string `json:"sourcesContent"`
		Names          []string `json:"names"`
		Mappings       string   `json:"mappings"`
	}{
		Version: 3, File: "program.js", Sources: paths, SourcesContent: contents,
		Names: []string{}, Mappings: encodeMappings(builder.entries),
	}
	encoded, _ := json.Marshal(payload)
	return append(encoded, '\n')
}

func encodeMappings(entries []generatedMapping) string {
	if len(entries) == 0 {
		return ""
	}
	var output strings.Builder
	line := 0
	previousSource := 0
	previousOriginalLine := 0
	previousOriginalColumn := 0
	index := 0
	for index < len(entries) {
		currentLine := entries[index].line
		for line < currentLine {
			output.WriteByte(';')
			line++
		}
		previousGeneratedColumn := 0
		first := true
		for index < len(entries) && entries[index].line == currentLine {
			entry := entries[index]
			if !first {
				output.WriteByte(',')
			}
			output.WriteString(encodeVLQ(entry.column - previousGeneratedColumn))
			output.WriteString(encodeVLQ(entry.source - previousSource))
			output.WriteString(encodeVLQ(entry.originalLine - previousOriginalLine))
			output.WriteString(encodeVLQ(entry.originalCol - previousOriginalColumn))
			previousGeneratedColumn = entry.column
			previousSource = entry.source
			previousOriginalLine = entry.originalLine
			previousOriginalColumn = entry.originalCol
			first = false
			index++
		}
	}
	return output.String()
}

func encodeVLQ(value int) string {
	encoded := value << 1
	if value < 0 {
		encoded = ((-value) << 1) | 1
	}
	var output strings.Builder
	for {
		digit := encoded & 31
		encoded >>= 5
		if encoded != 0 {
			digit |= 32
		}
		output.WriteByte(base64VLQ[digit])
		if encoded == 0 {
			return output.String()
		}
	}
}
